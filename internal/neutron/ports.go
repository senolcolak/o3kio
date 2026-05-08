package neutron

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/common"
	"github.com/cobaltcore-dev/o3k/pkg/networking"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

// CreatePortRequest represents a port creation request
type CreatePortRequest struct {
	Port struct {
		Name           string   `json:"name"`
		NetworkID      string   `json:"network_id" binding:"required"`
		AdminStateUp   *bool    `json:"admin_state_up"`
		DeviceID       string   `json:"device_id"`
		DeviceOwner    string   `json:"device_owner"`
		SecurityGroups []string `json:"security_groups"` // Security group IDs
	} `json:"port"`
}

// CreatePort creates a new port
func (svc *Service) CreatePort(c *gin.Context) {
	var req CreatePortRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	projectID := c.GetString("project_id")
	portID := uuid.New().String()
	tapName := "tap-" + portID[:8]
	macAddress := generateMAC()

	adminStateUp := true
	if req.Port.AdminStateUp != nil {
		adminStateUp = *req.Port.AdminStateUp
	}

	// Get network and subnet info to allocate IP
	var networkID string
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT id FROM networks WHERE id = $1 AND (project_id = $2 OR shared = true)",
		req.Port.NetworkID, projectID,
	).Scan(&networkID)

	if errors.Is(err, pgx.ErrNoRows) {
		common.SendError(c, common.NewNotFoundError("network"))
		return
	}

	// Get subnet to allocate IP
	var subnetID, cidr string
	err = svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT id, cidr FROM subnets WHERE network_id = $1 LIMIT 1",
		req.Port.NetworkID,
	).Scan(&subnetID, &cidr)

	var fixedIPs []map[string]interface{}
	if err == nil {
		// Allocate IP from subnet using DB-aware allocation
		allocatedIP, allocErr := svc.allocateIPFromSubnet(c.Request.Context(), subnetID, cidr)
		if allocErr != nil {
			log.Error().Err(allocErr).Str("subnet_id", subnetID).Msg("IP allocation failed")
			common.SendError(c, common.NewInternalServerError("failed to allocate IP address"))
			return
		}
		fixedIPs = []map[string]interface{}{
			{
				"subnet_id":  subnetID,
				"ip_address": allocatedIP,
			},
		}
	}

	fixedIPsJSON, _ := json.Marshal(fixedIPs)

	// Create TAP device in namespace
	nsName := svc.nsManager.GetNamespaceName(projectID)
	if err := svc.tapManager.CreateTAPDevice(tapName, true, nsName); err != nil {
		log.Error().Err(err).Str("operation", "create_tap_device").Str("tap", tapName).Msg("failed to create TAP device")
		common.SendError(c, common.NewInternalServerError("failed to create TAP device"))
		return
	}

	// Attach TAP to bridge
	bridgeName := "br-" + req.Port.NetworkID[:8]
	if err := svc.brManager.AttachToBridge(tapName, bridgeName, true, nsName); err != nil {
		log.Error().Err(err).Str("operation", "attach_tap_to_bridge").Str("tap", tapName).Msg("failed to attach TAP to bridge")
		common.SendError(c, common.NewInternalServerError("failed to attach TAP to bridge"))
		return
	}

	// Insert into database
	now := time.Now()
	_, err = svc.activeDB().Exec(c.Request.Context(), `
		INSERT INTO ports (id, name, network_id, project_id, device_id, device_owner, mac_address, admin_state_up, status, fixed_ips, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, portID, req.Port.Name, req.Port.NetworkID, projectID, sql.NullString{String: req.Port.DeviceID, Valid: req.Port.DeviceID != ""},
		sql.NullString{String: req.Port.DeviceOwner, Valid: req.Port.DeviceOwner != ""}, macAddress, adminStateUp, "ACTIVE", fixedIPsJSON, now, now)

	if err != nil {
		log.Error().Err(err).Str("operation", "create_port").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to create port"))
		return
	}

	// Apply security groups (default to "default" security group if none specified)
	securityGroups := req.Port.SecurityGroups
	if len(securityGroups) == 0 {
		// Get default security group for project
		var defaultSGID string
		err := svc.activeDB().QueryRow(c.Request.Context(),
			"SELECT id FROM security_groups WHERE project_id = $1 AND name = 'default'",
			projectID,
		).Scan(&defaultSGID)

		if err == nil {
			securityGroups = []string{defaultSGID}
		}
	}

	// Insert port-security group associations
	for _, sgID := range securityGroups {
		// Verify security group exists and belongs to project
		var exists bool
		err := svc.activeDB().QueryRow(c.Request.Context(),
			"SELECT EXISTS(SELECT 1 FROM security_groups WHERE id = $1 AND project_id = $2)",
			sgID, projectID,
		).Scan(&exists)

		if err != nil || !exists {
			// Clean up port on failure
			svc.activeDB().Exec(c.Request.Context(), "DELETE FROM ports WHERE id = $1", portID)
			common.SendError(c, common.NewNotFoundError(fmt.Sprintf("security group %s", sgID)))
			return
		}

		_, err = svc.activeDB().Exec(c.Request.Context(),
			"INSERT INTO port_security_groups (port_id, security_group_id) VALUES ($1, $2)",
			portID, sgID,
		)
		if err != nil {
			// Clean up port on failure
			svc.activeDB().Exec(c.Request.Context(), "DELETE FROM ports WHERE id = $1", portID)
			log.Error().Err(err).Str("operation", "associate_security_group").Str("sg_id", sgID).Msg("failed to associate security group")
			common.SendError(c, common.NewInternalServerError("failed to associate security group"))
			return
		}
	}

	// Apply security group rules (iptables or eBPF based on mode)
	if svc.sgManager != nil && svc.mode == "ebpf" && len(fixedIPs) > 0 {
		// eBPF mode: Apply rules directly to port
		rules, err := svc.fetchSecurityGroupRulesForPort(c.Request.Context(), securityGroups)
		if err != nil {
			fmt.Printf("Warning: failed to fetch security group rules: %v\n", err)
		} else {
			// Parse MAC address
			mac, err := net.ParseMAC(macAddress)
			if err != nil {
				fmt.Printf("Warning: invalid MAC address %s: %v\n", macAddress, err)
			} else {
				if err := svc.sgManager.ApplySecurityGroupToPort(portID, mac, rules); err != nil {
					fmt.Printf("Warning: failed to apply eBPF security group rules: %v\n", err)
				} else {
					fmt.Printf("Applied %d eBPF security group rules to port %s\n", len(rules), portID)

					// Attach XDP program to TAP interface
					if err := svc.sgManager.AttachToInterface(tapName); err != nil {
						fmt.Printf("Warning: failed to attach XDP program to %s: %v\n", tapName, err)
					} else {
						fmt.Printf("Attached XDP security group filter to interface %s\n", tapName)
					}
				}
			}
		}
	}
	// Note: For iptables mode, rules are applied when security groups are created/updated
	// via CreateSecurityGroupChain() and AddRule() methods

	// Distribute FDB entry if VXLAN is enabled
	if svc.vxlanCoordinator != nil {
		if err := svc.vxlanCoordinator.DistributeFDBEntry(c.Request.Context(), req.Port.NetworkID, portID, macAddress); err != nil {
			// Log but don't fail - FDB will be synced on next poll
			fmt.Printf("Warning: Failed to distribute FDB entry: %v\n", err)
		}
	}

	c.JSON(http.StatusCreated, gin.H{
		"port": gin.H{
			"id":              portID,
			"name":            req.Port.Name,
			"network_id":      req.Port.NetworkID,
			"tenant_id":       projectID,
			"device_id":       req.Port.DeviceID,
			"device_owner":    req.Port.DeviceOwner,
			"mac_address":     macAddress,
			"admin_state_up":  adminStateUp,
			"status":          "ACTIVE",
			"fixed_ips":       fixedIPs,
			"security_groups": securityGroups,
			"created_at":      now.Format(time.RFC3339),
			"updated_at":      now.Format(time.RFC3339),
		},
	})
}

// ListPorts lists all ports
func (svc *Service) ListPorts(c *gin.Context) {
	projectID := c.GetString("project_id")

	// Parse pagination parameters
	limit := common.DefaultPaginationLimit
	if limitParam := c.Query("limit"); limitParam != "" {
		if parsedLimit, err := strconv.Atoi(limitParam); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}
	limit = common.CapLimit(limit)

	// Build dynamic WHERE clause.
	conditions := []string{"(p.project_id = $1 OR n.shared = true)"}
	queryArgs := []interface{}{projectID}
	argIdx := 2

	if v := c.Query("network_id"); v != "" {
		conditions = append(conditions, fmt.Sprintf("p.network_id = $%d", argIdx))
		queryArgs = append(queryArgs, v)
		argIdx++
	}
	if v := c.Query("device_id"); v != "" {
		conditions = append(conditions, fmt.Sprintf("p.device_id = $%d", argIdx))
		queryArgs = append(queryArgs, v)
		argIdx++
	}
	if v := c.Query("device_owner"); v != "" {
		conditions = append(conditions, fmt.Sprintf("p.device_owner = $%d", argIdx))
		queryArgs = append(queryArgs, v)
		argIdx++
	}
	if v := c.Query("status"); v != "" {
		conditions = append(conditions, fmt.Sprintf("p.status = $%d", argIdx))
		queryArgs = append(queryArgs, v)
		argIdx++
	}
	if v := c.Query("mac_address"); v != "" {
		conditions = append(conditions, fmt.Sprintf("p.mac_address = $%d", argIdx))
		queryArgs = append(queryArgs, v)
		argIdx++
	}

	// Marker-based pagination (by ID).
	if marker := c.Query("marker"); marker != "" {
		conditions = append(conditions, fmt.Sprintf("p.id > $%d", argIdx))
		queryArgs = append(queryArgs, marker)
		argIdx++
	}

	queryArgs = append(queryArgs, limit+1)

	rows, err := svc.activeDB().Query(c.Request.Context(), fmt.Sprintf(`
		SELECT p.id, p.name, p.network_id, p.device_id, p.device_owner, p.mac_address, p.admin_state_up, p.status, p.fixed_ips, p.created_at, p.updated_at
		FROM ports p
		JOIN networks n ON p.network_id = n.id
		WHERE %s
		ORDER BY p.id ASC
		LIMIT $%d
	`, strings.Join(conditions, " AND "), argIdx), queryArgs...)

	if err != nil {
		log.Error().Err(err).Str("operation", "list_ports").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to list ports"))
		return
	}
	defer rows.Close()

	type portRow struct {
		id           string
		name         string
		networkID    string
		deviceID     sql.NullString
		deviceOwner  sql.NullString
		macAddress   string
		adminStateUp bool
		status       string
		fixedIPsJSON []byte
		createdAt    time.Time
		updatedAt    time.Time
	}

	var portRows []portRow
	var portIDs []string
	for rows.Next() {
		var pr portRow
		if err := rows.Scan(&pr.id, &pr.name, &pr.networkID, &pr.deviceID, &pr.deviceOwner, &pr.macAddress, &pr.adminStateUp, &pr.status, &pr.fixedIPsJSON, &pr.createdAt, &pr.updatedAt); err != nil {
			log.Warn().Err(err).Msg("failed to scan port row")
			continue
		}
		portRows = append(portRows, pr)
		portIDs = append(portIDs, pr.id)
	}
	if err := rows.Err(); err != nil {
		log.Error().Err(err).Str("operation", "list_ports").Msg("rows iteration error")
		common.SendError(c, common.NewInternalServerError("failed to list ports"))
		return
	}

	// Batch fetch security groups for all ports in one query.
	sgByPort := make(map[string][]string)
	if len(portIDs) > 0 {
		sgRows, err := svc.activeDB().Query(c.Request.Context(),
			"SELECT port_id, security_group_id FROM port_security_groups WHERE port_id = ANY($1)",
			portIDs,
		)
		if err != nil {
			log.Error().Err(err).Str("operation", "list_ports_sg_batch").Msg("database error fetching security groups")
			common.SendError(c, common.NewInternalServerError("failed to list ports"))
			return
		}
		defer sgRows.Close()
		for sgRows.Next() {
			var pid, sgID string
			if err := sgRows.Scan(&pid, &sgID); err == nil {
				sgByPort[pid] = append(sgByPort[pid], sgID)
			}
		}
		if err := sgRows.Err(); err != nil {
			log.Error().Err(err).Str("operation", "list_ports_sg_batch").Msg("rows iteration error fetching security groups")
			common.SendError(c, common.NewInternalServerError("failed to list ports"))
			return
		}
	}

	var ports []gin.H
	for _, pr := range portRows {
		var fixedIPs []map[string]interface{}
		json.Unmarshal(pr.fixedIPsJSON, &fixedIPs)

		sgs := sgByPort[pr.id]
		if sgs == nil {
			sgs = []string{}
		}

		ports = append(ports, gin.H{
			"id":                    pr.id,
			"name":                  pr.name,
			"network_id":            pr.networkID,
			"tenant_id":             projectID,
			"project_id":            projectID,
			"device_id":             pr.deviceID.String,
			"device_owner":          pr.deviceOwner.String,
			"mac_address":           pr.macAddress,
			"admin_state_up":        pr.adminStateUp,
			"status":                pr.status,
			"fixed_ips":             fixedIPs,
			"security_groups":       sgs,
			"binding:vif_type":      "ovs",
			"binding:vnic_type":     "normal",
			"binding:host_id":       "",
			"binding:vif_details":   map[string]interface{}{"port_filter": true, "connectivity": "l2"},
			"binding:profile":       map[string]interface{}{},
			"port_security_enabled": true,
			"allowed_address_pairs": []interface{}{},
			"dns_name":              "",
			"dns_assignment":        []interface{}{},
			"created_at":            pr.createdAt.Format(time.RFC3339),
			"updated_at":            pr.updatedAt.Format(time.RFC3339),
		})
	}

	if ports == nil {
		ports = []gin.H{}
	}

	// Check if there are more results
	resp := gin.H{"ports": ports}
	if len(ports) > limit {
		ports = ports[:limit]
		lastID, _ := ports[limit-1]["id"].(string)
		resp = gin.H{
			"ports":       ports,
			"ports_links": []gin.H{{"rel": "next", "href": fmt.Sprintf("?marker=%s&limit=%d", lastID, limit)}},
		}
	}

	c.JSON(http.StatusOK, resp)
}

// GetPort returns a single port
func (svc *Service) GetPort(c *gin.Context) {
	portID := c.Param("id")
	projectID := c.GetString("project_id")

	var id, name, networkID string
	var deviceID, deviceOwner sql.NullString
	var macAddress, status string
	var adminStateUp bool
	var fixedIPsJSON []byte
	var createdAt, updatedAt time.Time

	err := svc.activeDB().QueryRow(c.Request.Context(), `
		SELECT p.id, p.name, p.network_id, p.device_id, p.device_owner, p.mac_address, p.admin_state_up, p.status, p.fixed_ips, p.created_at, p.updated_at
		FROM ports p
		JOIN networks n ON p.network_id = n.id
		WHERE p.id = $1 AND (p.project_id = $2 OR n.shared = true)
	`, portID, projectID).Scan(&id, &name, &networkID, &deviceID, &deviceOwner, &macAddress, &adminStateUp, &status, &fixedIPsJSON, &createdAt, &updatedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		common.SendError(c, common.NewNotFoundError("port"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "get_port").Str("port_id", portID).Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to get port"))
		return
	}

	var fixedIPs []map[string]interface{}
	json.Unmarshal(fixedIPsJSON, &fixedIPs)

	// Fetch associated security groups
	securityGroups := []string{}
	sgRows, err := svc.activeDB().Query(c.Request.Context(),
		"SELECT security_group_id FROM port_security_groups WHERE port_id = $1",
		portID,
	)
	if err == nil {
		defer sgRows.Close()
		for sgRows.Next() {
			var sgID string
			if err := sgRows.Scan(&sgID); err == nil {
				securityGroups = append(securityGroups, sgID)
			}
		}
		if err := sgRows.Err(); err != nil {
			log.Error().Err(err).Str("port_id", portID).Msg("error reading security groups for port")
			common.SendError(c, common.NewInternalServerError("failed to get port"))
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"port": gin.H{
			"id":                    id,
			"name":                  name,
			"network_id":            networkID,
			"tenant_id":             projectID,
			"project_id":            projectID,
			"device_id":             deviceID.String,
			"device_owner":          deviceOwner.String,
			"mac_address":           macAddress,
			"admin_state_up":        adminStateUp,
			"status":                status,
			"fixed_ips":             fixedIPs,
			"security_groups":       securityGroups,
			"binding:vif_type":      "ovs",
			"binding:vnic_type":     "normal",
			"binding:host_id":       "",
			"binding:vif_details":   map[string]interface{}{"port_filter": true, "connectivity": "l2"},
			"binding:profile":       map[string]interface{}{},
			"port_security_enabled": true,
			"allowed_address_pairs": []interface{}{},
			"dns_name":              "",
			"dns_assignment":        []interface{}{},
			"created_at":            createdAt.Format(time.RFC3339),
			"updated_at":            updatedAt.Format(time.RFC3339),
		},
	})
}

// DeletePort deletes a port
func (svc *Service) DeletePort(c *gin.Context) {
	portID := c.Param("id")
	projectID := c.GetString("project_id")

	tapName := "tap-" + portID[:8]
	nsName := svc.nsManager.GetNamespaceName(projectID)

	// Detach XDP program if eBPF mode
	if svc.sgManager != nil && svc.mode == "ebpf" {
		if err := svc.sgManager.DetachFromInterface(tapName); err != nil {
			fmt.Printf("Warning: failed to detach XDP program from %s: %v\n", tapName, err)
		}

		// Remove port from eBPF maps (need MAC address)
		var macAddress string
		svc.activeDB().QueryRow(c.Request.Context(),
			"SELECT mac_address FROM ports WHERE id = $1",
			portID,
		).Scan(&macAddress)

		if macAddress != "" {
			if mac, err := net.ParseMAC(macAddress); err == nil {
				svc.sgManager.RemoveSecurityGroupFromPort(portID, mac)
			}
		}
	}

	// Delete TAP device
	svc.tapManager.DeleteTAPDevice(tapName, true, nsName)

	// Remove FDB entry if VXLAN is enabled
	if svc.vxlanCoordinator != nil {
		if err := svc.vxlanCoordinator.RemoveFDBEntry(c.Request.Context(), portID); err != nil {
			// Log but don't fail
			fmt.Printf("Warning: Failed to remove FDB entry: %v\n", err)
		}
	}

	// Delete from database
	_, err := svc.activeDB().Exec(c.Request.Context(),
		"DELETE FROM ports WHERE id = $1 AND project_id = $2",
		portID, projectID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "delete_port").Str("port_id", portID).Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to delete port"))
		return
	}

	c.Status(http.StatusNoContent)
}

// UpdatePort updates a port
func (svc *Service) UpdatePort(c *gin.Context) {
	portID := c.Param("id")
	projectID := c.GetString("project_id")

	var req struct {
		Port struct {
			Name         *string `json:"name"`
			AdminStateUp *bool   `json:"admin_state_up"`
			DeviceID     *string `json:"device_id"`
			DeviceOwner  *string `json:"device_owner"`
		} `json:"port"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	updates := []string{}
	args := []interface{}{}
	argID := 1

	if req.Port.Name != nil {
		updates = append(updates, fmt.Sprintf("name = $%d", argID))
		args = append(args, *req.Port.Name)
		argID++
	}

	if req.Port.AdminStateUp != nil {
		updates = append(updates, fmt.Sprintf("admin_state_up = $%d", argID))
		args = append(args, *req.Port.AdminStateUp)
		argID++
	}

	if req.Port.DeviceID != nil {
		updates = append(updates, fmt.Sprintf("device_id = $%d", argID))
		args = append(args, *req.Port.DeviceID)
		argID++
	}

	if req.Port.DeviceOwner != nil {
		updates = append(updates, fmt.Sprintf("device_owner = $%d", argID))
		args = append(args, *req.Port.DeviceOwner)
		argID++
	}

	if len(updates) == 0 {
		common.SendError(c, common.NewBadRequestError("no updates provided"))
		return
	}

	updates = append(updates, fmt.Sprintf("updated_at = $%d", argID))
	args = append(args, time.Now())
	argID++

	args = append(args, portID, projectID)

	query := fmt.Sprintf("UPDATE ports SET %s WHERE id = $%d AND project_id = $%d",
		updateString(updates), argID, argID+1)

	_, err := svc.activeDB().Exec(c.Request.Context(), query, args...)
	if err != nil {
		log.Error().Err(err).Str("operation", "update_port").Str("port_id", portID).Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to update port"))
		return
	}

	// Return updated port
	svc.GetPort(c)
}

// allocateIP allocates an IP from a CIDR range (legacy fallback for stub mode)
func allocateIP(cidr string) string {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return ""
	}

	// Start at .10 and increment to find a free one
	ip := incrementIP(ipNet.IP, 10)
	return ip.String()
}

// allocateIPFromSubnet allocates a unique IP from a subnet using a serializable
// transaction with SELECT FOR UPDATE to prevent TOCTOU races under concurrency.
func (svc *Service) allocateIPFromSubnet(ctx context.Context, subnetID, cidr string) (string, error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", fmt.Errorf("invalid CIDR: %w", err)
	}

	tx, err := svc.activeDB().BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.Serializable,
	})
	if err != nil {
		return "", fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Lock and fetch allocated IPs only for ports that have a fixed IP in this subnet.
	// Using a JSONB containment text check narrows the FOR UPDATE lock scope so we
	// don't serialize all port allocations globally.
	rows, err := tx.Query(ctx,
		`SELECT fixed_ips FROM ports WHERE fixed_ips IS NOT NULL AND fixed_ips::text LIKE '%' || $1 || '%' FOR UPDATE`,
		subnetID,
	)
	if err != nil {
		return "", fmt.Errorf("failed to query existing IPs: %w", err)
	}
	defer rows.Close()

	usedIPs := make(map[string]bool)
	for rows.Next() {
		var fixedIPsJSON []byte
		if err := rows.Scan(&fixedIPsJSON); err != nil {
			continue
		}
		var fixedIPs []map[string]interface{}
		if json.Unmarshal(fixedIPsJSON, &fixedIPs) == nil {
			for _, fip := range fixedIPs {
				if sid, ok := fip["subnet_id"].(string); ok && sid == subnetID {
					if addr, ok := fip["ip_address"].(string); ok {
						usedIPs[addr] = true
					}
				}
			}
		}
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("failed to iterate IPs: %w", err)
	}

	// Reserve .1 for gateway, .2-.9 for infrastructure; start at .10
	var candidate net.IP
	for offset := uint(10); offset < 250; offset++ {
		candidate = incrementIP(ipNet.IP, offset)
		if !ipNet.Contains(candidate) {
			break
		}
		if !usedIPs[candidate.String()] {
			if err := tx.Commit(ctx); err != nil {
				return "", fmt.Errorf("failed to commit IP allocation: %w", err)
			}
			return candidate.String(), nil
		}
	}

	return "", fmt.Errorf("no available IPs in subnet %s", subnetID)
}

// Security Groups implementation

// CreateSecurityGroupRequest represents a security group creation request
type CreateSecurityGroupRequest struct {
	SecurityGroup struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
	} `json:"security_group"`
}

// CreateSecurityGroup creates a new security group
func (svc *Service) CreateSecurityGroup(c *gin.Context) {
	var req CreateSecurityGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	projectID := c.GetString("project_id")
	sgID := uuid.New().String()

	// Create iptables chain for security group
	if svc.sgManager != nil {
		if err := svc.sgManager.CreateSecurityGroupChain(sgID); err != nil {
			log.Error().Err(err).Str("operation", "create_sg_chain").Str("sg_id", sgID).Msg("failed to create security group chain")
			common.SendError(c, common.NewInternalServerError("failed to create security group chain"))
			return
		}
	}

	// Insert into database
	now := time.Now()
	_, err := svc.activeDB().Exec(c.Request.Context(), `
		INSERT INTO security_groups (id, name, project_id, description, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, sgID, req.SecurityGroup.Name, projectID, req.SecurityGroup.Description, now, now)

	if err != nil {
		log.Error().Err(err).Str("operation", "create_security_group").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to create security group"))
		return
	}

	// Insert two default allow-all egress rules (IPv4 and IPv6) that OpenStack
	// creates automatically for every new security group.
	egressRuleIPv4ID := uuid.New().String()
	egressRuleIPv6ID := uuid.New().String()

	svc.activeDB().Exec(c.Request.Context(), `
		INSERT INTO security_group_rules (id, security_group_id, direction, ethertype, protocol, port_range_min, port_range_max, remote_ip_prefix, remote_group_id, created_at)
		VALUES ($1, $2, 'egress', 'IPv4', NULL, NULL, NULL, NULL, NULL, $3)
	`, egressRuleIPv4ID, sgID, now)

	svc.activeDB().Exec(c.Request.Context(), `
		INSERT INTO security_group_rules (id, security_group_id, direction, ethertype, protocol, port_range_min, port_range_max, remote_ip_prefix, remote_group_id, created_at)
		VALUES ($1, $2, 'egress', 'IPv6', NULL, NULL, NULL, NULL, NULL, $3)
	`, egressRuleIPv6ID, sgID, now)

	defaultRules := []gin.H{
		{
			"id":                egressRuleIPv4ID,
			"security_group_id": sgID,
			"direction":         "egress",
			"ethertype":         "IPv4",
			"protocol":          nil,
			"port_range_min":    nil,
			"port_range_max":    nil,
			"remote_ip_prefix":  nil,
			"remote_group_id":   nil,
			"tenant_id":         projectID,
			"created_at":        now.Format(time.RFC3339),
		},
		{
			"id":                egressRuleIPv6ID,
			"security_group_id": sgID,
			"direction":         "egress",
			"ethertype":         "IPv6",
			"protocol":          nil,
			"port_range_min":    nil,
			"port_range_max":    nil,
			"remote_ip_prefix":  nil,
			"remote_group_id":   nil,
			"tenant_id":         projectID,
			"created_at":        now.Format(time.RFC3339),
		},
	}

	c.JSON(http.StatusCreated, gin.H{
		"security_group": gin.H{
			"id":                   sgID,
			"name":                 req.SecurityGroup.Name,
			"tenant_id":            projectID,
			"description":          req.SecurityGroup.Description,
			"security_group_rules": defaultRules,
			"created_at":           now.Format(time.RFC3339),
			"updated_at":           now.Format(time.RFC3339),
		},
	})
}

// ListSecurityGroups lists all security groups
func (svc *Service) ListSecurityGroups(c *gin.Context) {
	projectID := c.GetString("project_id")

	// Parse pagination parameters
	limit := common.DefaultPaginationLimit
	if limitParam := c.Query("limit"); limitParam != "" {
		if parsedLimit, err := strconv.Atoi(limitParam); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}
	limit = common.CapLimit(limit)

	// Marker-based pagination
	var markerCondition string
	var queryArgs []interface{}
	queryArgs = append(queryArgs, projectID)
	argIdx := 2

	if marker := c.Query("marker"); marker != "" {
		markerCondition = fmt.Sprintf(" AND id > $%d", argIdx)
		queryArgs = append(queryArgs, marker)
		argIdx++
	}

	queryArgs = append(queryArgs, limit+1)

	rows, err := svc.activeDB().Query(c.Request.Context(), fmt.Sprintf(`
		SELECT id, name, description, created_at, updated_at
		FROM security_groups
		WHERE project_id = $1%s
		ORDER BY id ASC
		LIMIT $%d
	`, markerCondition, argIdx), queryArgs...)

	if err != nil {
		log.Error().Err(err).Str("operation", "list_security_groups").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to list security groups"))
		return
	}
	defer rows.Close()

	var securityGroups []gin.H
	for rows.Next() {
		var id, name, description string
		var createdAt, updatedAt time.Time

		if err := rows.Scan(&id, &name, &description, &createdAt, &updatedAt); err != nil {
			log.Warn().Err(err).Msg("failed to scan security group row")
			continue
		}

		// Fetch rules for this security group
		sgRules := svc.getSecurityGroupRules(c.Request.Context(), id)

		securityGroups = append(securityGroups, gin.H{
			"id":                   id,
			"name":                 name,
			"tenant_id":            projectID,
			"description":          description,
			"security_group_rules": sgRules,
			"created_at":           createdAt.Format(time.RFC3339),
			"updated_at":           updatedAt.Format(time.RFC3339),
		})
	}
	if err := rows.Err(); err != nil {
		log.Error().Err(err).Str("operation", "list_security_groups").Msg("rows iteration error")
		common.SendError(c, common.NewInternalServerError("failed to list security groups"))
		return
	}

	if securityGroups == nil {
		securityGroups = []gin.H{}
	}

	// Check if there are more results
	resp := gin.H{"security_groups": securityGroups}
	if len(securityGroups) > limit {
		securityGroups = securityGroups[:limit]
		lastID, _ := securityGroups[limit-1]["id"].(string)
		resp = gin.H{
			"security_groups":       securityGroups,
			"security_groups_links": []gin.H{{"rel": "next", "href": fmt.Sprintf("?marker=%s&limit=%d", lastID, limit)}},
		}
	}

	c.JSON(http.StatusOK, resp)
}

// getSecurityGroupRules fetches all rules for a given security group ID.
func (svc *Service) getSecurityGroupRules(ctx context.Context, sgID string) []gin.H {
	rows, err := svc.activeDB().Query(ctx, `
		SELECT sgr.id, sgr.direction, sgr.ethertype, sgr.protocol, sgr.port_range_min, sgr.port_range_max,
		       sgr.remote_ip_prefix, sgr.remote_group_id, sgr.created_at, sg.project_id
		FROM security_group_rules sgr
		JOIN security_groups sg ON sg.id = sgr.security_group_id
		WHERE sgr.security_group_id = $1
	`, sgID)
	if err != nil {
		return []gin.H{}
	}
	defer rows.Close()

	rules := []gin.H{}
	for rows.Next() {
		var ruleID, direction, ethertype, tenantID string
		var protocol, remoteIPPrefix, remoteGroupID *string
		var portRangeMin, portRangeMax *int
		var ruleCreatedAt time.Time

		if err := rows.Scan(&ruleID, &direction, &ethertype, &protocol, &portRangeMin, &portRangeMax,
			&remoteIPPrefix, &remoteGroupID, &ruleCreatedAt, &tenantID); err != nil {
			continue
		}

		// Always include all fields; use nil for absent optional values so JSON
		// serialises them as null (required by OpenStack API compatibility).
		rule := gin.H{
			"id":                ruleID,
			"security_group_id": sgID,
			"direction":         direction,
			"ethertype":         ethertype,
			"protocol":          nil,
			"port_range_min":    nil,
			"port_range_max":    nil,
			"remote_ip_prefix":  nil,
			"remote_group_id":   nil,
			"tenant_id":         tenantID,
			"created_at":        ruleCreatedAt.Format(time.RFC3339),
		}

		if protocol != nil && *protocol != "" {
			rule["protocol"] = *protocol
		}
		if portRangeMin != nil {
			rule["port_range_min"] = *portRangeMin
		}
		if portRangeMax != nil {
			rule["port_range_max"] = *portRangeMax
		}
		if remoteIPPrefix != nil && *remoteIPPrefix != "" {
			rule["remote_ip_prefix"] = *remoteIPPrefix
		}
		if remoteGroupID != nil && *remoteGroupID != "" {
			rule["remote_group_id"] = *remoteGroupID
		}

		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		log.Error().Err(err).Str("sg_id", sgID).Msg("rows iteration error in getSecurityGroupRules")
	}
	return rules
}

// GetSecurityGroup returns a single security group
func (svc *Service) GetSecurityGroup(c *gin.Context) {
	sgID := c.Param("id")
	projectID := c.GetString("project_id")

	var id, name, description string
	var createdAt, updatedAt time.Time

	err := svc.activeDB().QueryRow(c.Request.Context(), `
		SELECT id, name, description, created_at, updated_at
		FROM security_groups
		WHERE id = $1 AND project_id = $2
	`, sgID, projectID).Scan(&id, &name, &description, &createdAt, &updatedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		common.SendError(c, common.NewNotFoundError("security group"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "get_security_group").Str("sg_id", sgID).Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to get security group"))
		return
	}

	// Fetch associated rules
	rules := svc.getSecurityGroupRules(c.Request.Context(), sgID)

	c.JSON(http.StatusOK, gin.H{
		"security_group": gin.H{
			"id":                   id,
			"name":                 name,
			"tenant_id":            projectID,
			"description":          description,
			"created_at":           createdAt.Format(time.RFC3339),
			"updated_at":           updatedAt.Format(time.RFC3339),
			"security_group_rules": rules,
		},
	})
}

// DeleteSecurityGroup deletes a security group
func (svc *Service) DeleteSecurityGroup(c *gin.Context) {
	sgID := c.Param("id")
	projectID := c.GetString("project_id")

	// Delete iptables chain
	if svc.sgManager != nil {
		svc.sgManager.DeleteSecurityGroupChain(sgID)
	}

	// Delete from database
	_, err := svc.activeDB().Exec(c.Request.Context(),
		"DELETE FROM security_groups WHERE id = $1 AND project_id = $2",
		sgID, projectID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "delete_security_group").Str("sg_id", sgID).Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to delete security group"))
		return
	}

	c.Status(http.StatusNoContent)
}

// UpdateSecurityGroup updates a security group
func (svc *Service) UpdateSecurityGroup(c *gin.Context) {
	sgID := c.Param("id")
	projectID := c.GetString("project_id")

	var req struct {
		SecurityGroup struct {
			Name        *string `json:"name"`
			Description *string `json:"description"`
		} `json:"security_group"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Check security group exists
	var currentName, currentDesc string
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT name, description FROM security_groups WHERE id = $1 AND project_id = $2",
		sgID, projectID,
	).Scan(&currentName, &currentDesc)

	if errors.Is(err, pgx.ErrNoRows) {
		common.SendError(c, common.NewNotFoundError("security group"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "update_security_group").Str("sg_id", sgID).Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to update security group"))
		return
	}

	// Apply updates
	if req.SecurityGroup.Name != nil {
		currentName = *req.SecurityGroup.Name
	}
	if req.SecurityGroup.Description != nil {
		currentDesc = *req.SecurityGroup.Description
	}

	_, err = svc.activeDB().Exec(c.Request.Context(),
		"UPDATE security_groups SET name = $1, description = $2 WHERE id = $3",
		currentName, currentDesc, sgID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "update_security_group").Str("sg_id", sgID).Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to update security group"))
		return
	}

	rules := svc.getSecurityGroupRules(c.Request.Context(), sgID)

	c.JSON(http.StatusOK, gin.H{
		"security_group": gin.H{
			"id":                   sgID,
			"name":                 currentName,
			"description":          currentDesc,
			"tenant_id":            projectID,
			"security_group_rules": rules,
		},
	})
}

// CreateSecurityGroupRuleRequest represents a security group rule creation request
type CreateSecurityGroupRuleRequest struct {
	SecurityGroupRule struct {
		SecurityGroupID string  `json:"security_group_id" binding:"required"`
		Direction       string  `json:"direction" binding:"required"`
		EtherType       string  `json:"ethertype"`
		Protocol        *string `json:"protocol"`
		PortRangeMin    *int    `json:"port_range_min"`
		PortRangeMax    *int    `json:"port_range_max"`
		RemoteIPPrefix  *string `json:"remote_ip_prefix"`
		RemoteGroupID   *string `json:"remote_group_id"`
	} `json:"security_group_rule"`
}

// CreateSecurityGroupRule creates a new security group rule
func (svc *Service) CreateSecurityGroupRule(c *gin.Context) {
	var req CreateSecurityGroupRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	ruleID := uuid.New().String()

	var protocolVal interface{}
	protocol := ""
	if req.SecurityGroupRule.Protocol != nil && *req.SecurityGroupRule.Protocol != "" {
		protocol = *req.SecurityGroupRule.Protocol
		protocolVal = protocol
	}

	var portMinVal interface{}
	portMin := 0
	if req.SecurityGroupRule.PortRangeMin != nil {
		portMin = *req.SecurityGroupRule.PortRangeMin
		portMinVal = portMin
	}

	var portMaxVal interface{}
	portMax := 0
	if req.SecurityGroupRule.PortRangeMax != nil {
		portMax = *req.SecurityGroupRule.PortRangeMax
		portMaxVal = portMax
	}

	var remoteIPVal interface{}
	remoteIP := ""
	if req.SecurityGroupRule.RemoteIPPrefix != nil && *req.SecurityGroupRule.RemoteIPPrefix != "" {
		remoteIP = *req.SecurityGroupRule.RemoteIPPrefix
		remoteIPVal = remoteIP
	}

	var remoteGroupVal interface{}
	remoteGroup := ""
	if req.SecurityGroupRule.RemoteGroupID != nil && *req.SecurityGroupRule.RemoteGroupID != "" {
		remoteGroup = *req.SecurityGroupRule.RemoteGroupID
		remoteGroupVal = remoteGroup
	}

	etherType := "IPv4"
	if req.SecurityGroupRule.EtherType != "" {
		etherType = req.SecurityGroupRule.EtherType
	}

	// Add iptables rule
	if svc.sgManager != nil {
		rule := networking.SecurityGroupRule{
			ID:             ruleID,
			Direction:      req.SecurityGroupRule.Direction,
			EtherType:      etherType,
			Protocol:       protocol,
			PortRangeMin:   portMin,
			PortRangeMax:   portMax,
			RemoteIPPrefix: remoteIP,
			RemoteGroupID:  remoteGroup,
		}

		if err := svc.sgManager.AddRule(req.SecurityGroupRule.SecurityGroupID, rule); err != nil {
			log.Error().Err(err).Str("operation", "add_sg_rule").Msg("failed to add iptables rule")
			common.SendError(c, common.NewInternalServerError("failed to add security group rule"))
			return
		}
	}

	// Insert into database
	now := time.Now()
	_, err := svc.activeDB().Exec(c.Request.Context(), `
		INSERT INTO security_group_rules (id, security_group_id, direction, ethertype, protocol, port_range_min, port_range_max, remote_ip_prefix, remote_group_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, ruleID, req.SecurityGroupRule.SecurityGroupID, req.SecurityGroupRule.Direction, etherType,
		sql.NullString{String: protocol, Valid: protocol != ""},
		sql.NullInt32{Int32: int32(portMin), Valid: portMin > 0},
		sql.NullInt32{Int32: int32(portMax), Valid: portMax > 0},
		sql.NullString{String: remoteIP, Valid: remoteIP != ""},
		sql.NullString{String: remoteGroup, Valid: remoteGroup != ""},
		now)

	if err != nil {
		log.Error().Err(err).Str("operation", "create_sg_rule").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to create security group rule"))
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"security_group_rule": gin.H{
			"id":                ruleID,
			"security_group_id": req.SecurityGroupRule.SecurityGroupID,
			"direction":         req.SecurityGroupRule.Direction,
			"ethertype":         etherType,
			"protocol":          protocolVal,
			"port_range_min":    portMinVal,
			"port_range_max":    portMaxVal,
			"remote_ip_prefix":  remoteIPVal,
			"remote_group_id":   remoteGroupVal,
			"created_at":        now.Format(time.RFC3339),
		},
	})
}

// ListSecurityGroupRules lists all security group rules
func (svc *Service) ListSecurityGroupRules(c *gin.Context) {
	projectID := c.GetString("project_id")

	rows, err := svc.activeDB().Query(c.Request.Context(), `
		SELECT sgr.id, sgr.security_group_id, sgr.direction, sgr.ethertype, sgr.protocol, sgr.port_range_min, sgr.port_range_max, sgr.remote_ip_prefix, sgr.remote_group_id, sgr.created_at
		FROM security_group_rules sgr
		JOIN security_groups sg ON sgr.security_group_id = sg.id
		WHERE sg.project_id = $1
		ORDER BY sgr.created_at DESC
	`, projectID)

	if err != nil {
		log.Error().Err(err).Str("operation", "list_sg_rules").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to list security group rules"))
		return
	}
	defer rows.Close()

	var rules []gin.H
	for rows.Next() {
		var id, sgID, direction, etherType string
		var protocol, remoteIP, remoteGroup sql.NullString
		var portMin, portMax sql.NullInt32
		var createdAt time.Time

		if err := rows.Scan(&id, &sgID, &direction, &etherType, &protocol, &portMin, &portMax, &remoteIP, &remoteGroup, &createdAt); err != nil {
			log.Warn().Err(err).Msg("failed to scan security group rule row")
			continue
		}

		rule := gin.H{
			"id":                id,
			"security_group_id": sgID,
			"direction":         direction,
			"ethertype":         etherType,
			"created_at":        createdAt.Format(time.RFC3339),
		}

		if protocol.Valid {
			rule["protocol"] = protocol.String
		}
		if portMin.Valid {
			rule["port_range_min"] = portMin.Int32
		}
		if portMax.Valid {
			rule["port_range_max"] = portMax.Int32
		}
		if remoteIP.Valid {
			rule["remote_ip_prefix"] = remoteIP.String
		}
		if remoteGroup.Valid {
			rule["remote_group_id"] = remoteGroup.String
		}

		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		log.Error().Err(err).Str("operation", "list_sg_rules").Msg("rows iteration error")
		common.SendError(c, common.NewInternalServerError("failed to list security group rules"))
		return
	}

	if rules == nil {
		rules = []gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{"security_group_rules": rules})
}

// DeleteSecurityGroupRule deletes a security group rule
func (svc *Service) DeleteSecurityGroupRule(c *gin.Context) {
	ruleID := c.Param("id")

	// Get rule details to remove from iptables
	var sgID, direction, etherType string
	var protocol, remoteIP, remoteGroup sql.NullString
	var portMin, portMax sql.NullInt32

	err := svc.activeDB().QueryRow(c.Request.Context(), `
		SELECT security_group_id, direction, ethertype, protocol, port_range_min, port_range_max, remote_ip_prefix, remote_group_id
		FROM security_group_rules
		WHERE id = $1
	`, ruleID).Scan(&sgID, &direction, &etherType, &protocol, &portMin, &portMax, &remoteIP, &remoteGroup)

	if err == nil && svc.sgManager != nil {
		rule := networking.SecurityGroupRule{
			ID:             ruleID,
			Direction:      direction,
			EtherType:      etherType,
			Protocol:       protocol.String,
			PortRangeMin:   int(portMin.Int32),
			PortRangeMax:   int(portMax.Int32),
			RemoteIPPrefix: remoteIP.String,
			RemoteGroupID:  remoteGroup.String,
		}

		svc.sgManager.RemoveRule(sgID, rule)
	}

	// Delete from database (with ownership check)
	projectID := c.GetString("project_id")
	_, err = svc.activeDB().Exec(c.Request.Context(),
		"DELETE FROM security_group_rules WHERE id = $1 AND security_group_id IN (SELECT id FROM security_groups WHERE project_id = $2)",
		ruleID, projectID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "delete_sg_rule").Str("rule_id", ruleID).Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to delete security group rule"))
		return
	}

	c.Status(http.StatusNoContent)
}

// PortInfo represents port allocation information for cross-service use
type PortInfo struct {
	ID        string
	NetworkID string
	MAC       string
	IPAddress string
	SubnetID  string
}

// AllocatePortForInstance creates a port for a VM instance (called from Nova)
func (svc *Service) AllocatePortForInstance(ctx context.Context, networkID, projectID, instanceID string) (interface{}, error) {
	portID := uuid.New().String()
	macAddress := generateMAC()

	// Get network and subnet info to allocate IP
	var netID string
	err := svc.activeDB().QueryRow(ctx,
		"SELECT id FROM networks WHERE id = $1 AND (project_id = $2 OR shared = true)",
		networkID, projectID,
	).Scan(&netID)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("network %s not found", networkID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query network: %w", err)
	}

	// Get subnet to allocate IP
	var subnetID, cidr string
	err = svc.activeDB().QueryRow(ctx,
		"SELECT id, cidr FROM subnets WHERE network_id = $1 LIMIT 1",
		networkID,
	).Scan(&subnetID, &cidr)

	if err != nil {
		return nil, fmt.Errorf("no subnet found for network %s: %w", networkID, err)
	}

	// Allocate IP from subnet using DB-aware allocation
	allocatedIP, allocErr := svc.allocateIPFromSubnet(ctx, subnetID, cidr)
	if allocErr != nil {
		return nil, fmt.Errorf("failed to allocate IP from subnet %s: %w", subnetID, allocErr)
	}

	fixedIPs := []map[string]interface{}{
		{
			"subnet_id":  subnetID,
			"ip_address": allocatedIP,
		},
	}
	fixedIPsJSON, _ := json.Marshal(fixedIPs)

	// Create TAP device in default namespace (not project namespace) for libvirt access
	if svc.mode != "stub" {
		tapName := "tap-" + portID[:8]

		// Create TAP device in default namespace
		if err := svc.tapManager.CreateTAPDevice(tapName, false, ""); err != nil {
			return nil, fmt.Errorf("failed to create TAP device: %w", err)
		}

		// Ensure bridge exists in default namespace
		bridgeName := "br-" + networkID[:8]
		if err := svc.brManager.CreateBridge(bridgeName, false, ""); err != nil {
			// Bridge might already exist, log but don't fail
			log.Debug().Err(err).Str("bridge", bridgeName).Msg("bridge creation returned error (may already exist)")
		}

		// Attach TAP to bridge in default namespace
		if err := svc.brManager.AttachToBridge(tapName, bridgeName, false, ""); err != nil {
			return nil, fmt.Errorf("failed to attach TAP to bridge: %w", err)
		}
	}

	// Insert into database
	now := time.Now()
	_, err = svc.activeDB().Exec(ctx, `
		INSERT INTO ports (id, name, network_id, project_id, device_id, device_owner, mac_address, admin_state_up, status, fixed_ips, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, portID, fmt.Sprintf("port-for-%s", instanceID[:8]), networkID, projectID, instanceID,
		"compute:nova", macAddress, true, "ACTIVE", fixedIPsJSON, now, now)

	if err != nil {
		return nil, fmt.Errorf("failed to insert port into database: %w", err)
	}

	// Distribute FDB entry if VXLAN is enabled
	if svc.vxlanCoordinator != nil {
		if err := svc.vxlanCoordinator.DistributeFDBEntry(ctx, networkID, portID, macAddress); err != nil {
			// Log but don't fail - FDB will be synced on next poll
			fmt.Printf("Warning: Failed to distribute FDB entry: %v\n", err)
		}
	}

	return &PortInfo{
		ID:        portID,
		NetworkID: networkID,
		MAC:       macAddress,
		IPAddress: allocatedIP,
		SubnetID:  subnetID,
	}, nil
}

// fetchSecurityGroupRulesForPort retrieves all security group rules for given security group IDs
func (svc *Service) fetchSecurityGroupRulesForPort(ctx context.Context, securityGroupIDs []string) ([]networking.SecurityGroupRule, error) {
	if len(securityGroupIDs) == 0 {
		return []networking.SecurityGroupRule{}, nil
	}

	// Build query with IN clause for multiple security groups
	query := `
		SELECT id, security_group_id, direction, ethertype, protocol,
		       port_range_min, port_range_max, remote_ip_prefix, remote_group_id
		FROM security_group_rules
		WHERE security_group_id = ANY($1)
	`

	rows, err := svc.activeDB().Query(ctx, query, securityGroupIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to query security group rules: %w", err)
	}
	defer rows.Close()

	var rules []networking.SecurityGroupRule
	for rows.Next() {
		var rule networking.SecurityGroupRule
		var sgID string // Not part of SecurityGroupRule struct, just for WHERE clause
		var protocol, remoteIP, remoteGroup sql.NullString
		var portMin, portMax sql.NullInt32

		err := rows.Scan(
			&rule.ID,
			&sgID, // security_group_id (not stored in rule struct)
			&rule.Direction,
			&rule.EtherType,
			&protocol,
			&portMin,
			&portMax,
			&remoteIP,
			&remoteGroup,
		)
		if err != nil {
			log.Warn().Err(err).Msg("failed to scan security group rule row")
			continue
		}

		if protocol.Valid {
			rule.Protocol = protocol.String
		}
		if portMin.Valid {
			rule.PortRangeMin = int(portMin.Int32)
		}
		if portMax.Valid {
			rule.PortRangeMax = int(portMax.Int32)
		}
		if remoteIP.Valid {
			rule.RemoteIPPrefix = remoteIP.String
		}
		if remoteGroup.Valid {
			rule.RemoteGroupID = remoteGroup.String
		}

		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating security group rules: %w", err)
	}

	return rules, nil
}
