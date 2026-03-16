package neutron

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/cobaltcore-dev/o3k/pkg/networking"
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
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
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT id FROM networks WHERE id = $1 AND (project_id = $2 OR shared = true)",
		req.Port.NetworkID, projectID,
	).Scan(&networkID)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "network not found"})
		return
	}

	// Get subnet to allocate IP
	var subnetID, cidr string
	err = database.DB.QueryRow(c.Request.Context(),
		"SELECT id, cidr FROM subnets WHERE network_id = $1 LIMIT 1",
		req.Port.NetworkID,
	).Scan(&subnetID, &cidr)

	var fixedIPs []map[string]interface{}
	if err == nil {
		// Allocate IP from subnet
		allocatedIP := allocateIP(cidr)
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to create TAP device: %v", err)})
		return
	}

	// Attach TAP to bridge
	bridgeName := "br-" + req.Port.NetworkID[:8]
	if err := svc.brManager.AttachToBridge(tapName, bridgeName, true, nsName); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to attach TAP to bridge: %v", err)})
		return
	}

	// Insert into database
	now := time.Now()
	_, err = database.DB.Exec(c.Request.Context(), `
		INSERT INTO ports (id, name, network_id, project_id, device_id, device_owner, mac_address, admin_state_up, status, fixed_ips, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, portID, req.Port.Name, req.Port.NetworkID, projectID, sql.NullString{String: req.Port.DeviceID, Valid: req.Port.DeviceID != ""},
		sql.NullString{String: req.Port.DeviceOwner, Valid: req.Port.DeviceOwner != ""}, macAddress, adminStateUp, "ACTIVE", fixedIPsJSON, now, now)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Apply security groups (default to "default" security group if none specified)
	securityGroups := req.Port.SecurityGroups
	if len(securityGroups) == 0 {
		// Get default security group for project
		var defaultSGID string
		err := database.DB.QueryRow(c.Request.Context(),
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
		err := database.DB.QueryRow(c.Request.Context(),
			"SELECT EXISTS(SELECT 1 FROM security_groups WHERE id = $1 AND project_id = $2)",
			sgID, projectID,
		).Scan(&exists)

		if err != nil || !exists {
			// Clean up port on failure
			database.DB.Exec(c.Request.Context(), "DELETE FROM ports WHERE id = $1", portID)
			c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("security group %s not found", sgID)})
			return
		}

		_, err = database.DB.Exec(c.Request.Context(),
			"INSERT INTO port_security_groups (port_id, security_group_id) VALUES ($1, $2)",
			portID, sgID,
		)
		if err != nil {
			// Clean up port on failure
			database.DB.Exec(c.Request.Context(), "DELETE FROM ports WHERE id = $1", portID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to associate security group: %v", err)})
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
	limit := 1000
	offset := 0
	if limitParam := c.Query("limit"); limitParam != "" {
		if parsedLimit, err := strconv.Atoi(limitParam); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}
	if offsetParam := c.Query("offset"); offsetParam != "" {
		if parsedOffset, err := strconv.Atoi(offsetParam); err == nil && parsedOffset >= 0 {
			offset = parsedOffset
		}
	}

	// Marker-based pagination
	var markerCondition string
	var queryArgs []interface{}
	queryArgs = append(queryArgs, projectID)
	argIdx := 2

	if marker := c.Query("marker"); marker != "" {
		var markerCreatedAt time.Time
		err := database.DB.QueryRow(c.Request.Context(),
			"SELECT created_at FROM ports WHERE id = $1",
			marker,
		).Scan(&markerCreatedAt)
		if err == nil {
			markerCondition = fmt.Sprintf(" AND p.created_at < $%d", argIdx)
			queryArgs = append(queryArgs, markerCreatedAt)
			argIdx++
		}
	}

	queryArgs = append(queryArgs, limit, offset)

	rows, err := database.DB.Query(c.Request.Context(), fmt.Sprintf(`
		SELECT p.id, p.name, p.network_id, p.device_id, p.device_owner, p.mac_address, p.admin_state_up, p.status, p.fixed_ips, p.created_at, p.updated_at
		FROM ports p
		JOIN networks n ON p.network_id = n.id
		WHERE (p.project_id = $1 OR n.shared = true)%s
		ORDER BY p.created_at DESC
		LIMIT $%d OFFSET $%d
	`, markerCondition, argIdx, argIdx+1), queryArgs...)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var ports []gin.H
	for rows.Next() {
		var id, name, networkID string
		var deviceID, deviceOwner sql.NullString
		var macAddress, status string
		var adminStateUp bool
		var fixedIPsJSON []byte
		var createdAt, updatedAt time.Time

		if err := rows.Scan(&id, &name, &networkID, &deviceID, &deviceOwner, &macAddress, &adminStateUp, &status, &fixedIPsJSON, &createdAt, &updatedAt); err != nil {
			continue
		}

		var fixedIPs []map[string]interface{}
		json.Unmarshal(fixedIPsJSON, &fixedIPs)

		// Fetch associated security groups for this port
		securityGroups := []string{}
		sgRows, err := database.DB.Query(c.Request.Context(),
			"SELECT security_group_id FROM port_security_groups WHERE port_id = $1",
			id,
		)
		if err == nil {
			for sgRows.Next() {
				var sgID string
				if err := sgRows.Scan(&sgID); err == nil {
					securityGroups = append(securityGroups, sgID)
				}
			}
			sgRows.Close()
		}

		ports = append(ports, gin.H{
			"id":              id,
			"name":            name,
			"network_id":      networkID,
			"tenant_id":       projectID,
			"device_id":       deviceID.String,
			"device_owner":    deviceOwner.String,
			"mac_address":     macAddress,
			"admin_state_up":  adminStateUp,
			"status":          status,
			"fixed_ips":       fixedIPs,
			"security_groups": securityGroups,
			"created_at":      createdAt.Format(time.RFC3339),
			"updated_at":      updatedAt.Format(time.RFC3339),
		})
	}

	if ports == nil {
		ports = []gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{"ports": ports})
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

	err := database.DB.QueryRow(c.Request.Context(), `
		SELECT p.id, p.name, p.network_id, p.device_id, p.device_owner, p.mac_address, p.admin_state_up, p.status, p.fixed_ips, p.created_at, p.updated_at
		FROM ports p
		JOIN networks n ON p.network_id = n.id
		WHERE p.id = $1 AND (p.project_id = $2 OR n.shared = true)
	`, portID, projectID).Scan(&id, &name, &networkID, &deviceID, &deviceOwner, &macAddress, &adminStateUp, &status, &fixedIPsJSON, &createdAt, &updatedAt)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "port not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var fixedIPs []map[string]interface{}
	json.Unmarshal(fixedIPsJSON, &fixedIPs)

	// Fetch associated security groups
	securityGroups := []string{}
	sgRows, err := database.DB.Query(c.Request.Context(),
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
	}

	c.JSON(http.StatusOK, gin.H{
		"port": gin.H{
			"id":              id,
			"name":            name,
			"network_id":      networkID,
			"tenant_id":       projectID,
			"device_id":       deviceID.String,
			"device_owner":    deviceOwner.String,
			"mac_address":     macAddress,
			"admin_state_up":  adminStateUp,
			"status":          status,
			"fixed_ips":       fixedIPs,
			"security_groups": securityGroups,
			"created_at":      createdAt.Format(time.RFC3339),
			"updated_at":      updatedAt.Format(time.RFC3339),
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
		database.DB.QueryRow(c.Request.Context(),
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
	_, err := database.DB.Exec(c.Request.Context(),
		"DELETE FROM ports WHERE id = $1 AND project_id = $2",
		portID, projectID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "no updates provided"})
		return
	}

	updates = append(updates, fmt.Sprintf("updated_at = $%d", argID))
	args = append(args, time.Now())
	argID++

	args = append(args, portID, projectID)

	query := fmt.Sprintf("UPDATE ports SET %s WHERE id = $%d AND project_id = $%d",
		updateString(updates), argID, argID+1)

	_, err := database.DB.Exec(c.Request.Context(), query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Return updated port
	svc.GetPort(c)
}

// allocateIP allocates an IP from a CIDR range
func allocateIP(cidr string) string {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return ""
	}

	// Allocate a random IP in the range (simplified)
	ip := incrementIP(ipNet.IP, uint(10+time.Now().Unix()%240))
	return ip.String()
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	projectID := c.GetString("project_id")
	sgID := uuid.New().String()

	// Create iptables chain for security group
	if svc.sgManager != nil {
		if err := svc.sgManager.CreateSecurityGroupChain(sgID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to create security group chain: %v", err)})
			return
		}
	}

	// Insert into database
	now := time.Now()
	_, err := database.DB.Exec(c.Request.Context(), `
		INSERT INTO security_groups (id, name, project_id, description, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, sgID, req.SecurityGroup.Name, projectID, req.SecurityGroup.Description, now, now)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"security_group": gin.H{
			"id":          sgID,
			"name":        req.SecurityGroup.Name,
			"tenant_id":   projectID,
			"description": req.SecurityGroup.Description,
			"created_at":  now.Format(time.RFC3339),
			"updated_at":  now.Format(time.RFC3339),
		},
	})
}

// ListSecurityGroups lists all security groups
func (svc *Service) ListSecurityGroups(c *gin.Context) {
	projectID := c.GetString("project_id")

	rows, err := database.DB.Query(c.Request.Context(), `
		SELECT id, name, description, created_at, updated_at
		FROM security_groups
		WHERE project_id = $1
		ORDER BY created_at DESC
	`, projectID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var securityGroups []gin.H
	for rows.Next() {
		var id, name, description string
		var createdAt, updatedAt time.Time

		if err := rows.Scan(&id, &name, &description, &createdAt, &updatedAt); err != nil {
			continue
		}

		securityGroups = append(securityGroups, gin.H{
			"id":          id,
			"name":        name,
			"tenant_id":   projectID,
			"description": description,
			"created_at":  createdAt.Format(time.RFC3339),
			"updated_at":  updatedAt.Format(time.RFC3339),
		})
	}

	if securityGroups == nil {
		securityGroups = []gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{"security_groups": securityGroups})
}

// GetSecurityGroup returns a single security group
func (svc *Service) GetSecurityGroup(c *gin.Context) {
	sgID := c.Param("id")
	projectID := c.GetString("project_id")

	var id, name, description string
	var createdAt, updatedAt time.Time

	err := database.DB.QueryRow(c.Request.Context(), `
		SELECT id, name, description, created_at, updated_at
		FROM security_groups
		WHERE id = $1 AND project_id = $2
	`, sgID, projectID).Scan(&id, &name, &description, &createdAt, &updatedAt)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "security group not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Fetch associated rules
	rows, err := database.DB.Query(c.Request.Context(), `
		SELECT id, direction, ethertype, protocol, port_range_min, port_range_max,
		       remote_ip_prefix, remote_group_id, created_at
		FROM security_group_rules
		WHERE security_group_id = $1
	`, sgID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	rules := []gin.H{}
	for rows.Next() {
		var ruleID, direction, ethertype string
		var protocol, remoteIPPrefix, remoteGroupID *string
		var portRangeMin, portRangeMax *int
		var ruleCreatedAt time.Time

		err := rows.Scan(&ruleID, &direction, &ethertype, &protocol, &portRangeMin, &portRangeMax,
			&remoteIPPrefix, &remoteGroupID, &ruleCreatedAt)
		if err != nil {
			continue
		}

		rule := gin.H{
			"id":                ruleID,
			"security_group_id": sgID,
			"direction":         direction,
			"ethertype":         ethertype,
			"created_at":        ruleCreatedAt.Format(time.RFC3339),
		}

		if protocol != nil {
			rule["protocol"] = *protocol
		}
		if portRangeMin != nil {
			rule["port_range_min"] = *portRangeMin
		}
		if portRangeMax != nil {
			rule["port_range_max"] = *portRangeMax
		}
		if remoteIPPrefix != nil {
			rule["remote_ip_prefix"] = *remoteIPPrefix
		} else {
			rule["remote_ip_prefix"] = ""
		}
		if remoteGroupID != nil {
			rule["remote_group_id"] = *remoteGroupID
		} else {
			rule["remote_group_id"] = ""
		}

		rules = append(rules, rule)
	}

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
	_, err := database.DB.Exec(c.Request.Context(),
		"DELETE FROM security_groups WHERE id = $1 AND project_id = $2",
		sgID, projectID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Check security group exists
	var currentName, currentDesc string
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT name, description FROM security_groups WHERE id = $1 AND project_id = $2",
		sgID, projectID,
	).Scan(&currentName, &currentDesc)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "security group not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Apply updates
	if req.SecurityGroup.Name != nil {
		currentName = *req.SecurityGroup.Name
	}
	if req.SecurityGroup.Description != nil {
		currentDesc = *req.SecurityGroup.Description
	}

	_, err = database.DB.Exec(c.Request.Context(),
		"UPDATE security_groups SET name = $1, description = $2 WHERE id = $3",
		currentName, currentDesc, sgID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"security_group": gin.H{
			"id":          sgID,
			"name":        currentName,
			"description": currentDesc,
			"tenant_id":   projectID,
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	ruleID := uuid.New().String()

	protocol := ""
	if req.SecurityGroupRule.Protocol != nil {
		protocol = *req.SecurityGroupRule.Protocol
	}

	portMin := 0
	if req.SecurityGroupRule.PortRangeMin != nil {
		portMin = *req.SecurityGroupRule.PortRangeMin
	}

	portMax := 0
	if req.SecurityGroupRule.PortRangeMax != nil {
		portMax = *req.SecurityGroupRule.PortRangeMax
	}

	remoteIP := ""
	if req.SecurityGroupRule.RemoteIPPrefix != nil {
		remoteIP = *req.SecurityGroupRule.RemoteIPPrefix
	}

	remoteGroup := ""
	if req.SecurityGroupRule.RemoteGroupID != nil {
		remoteGroup = *req.SecurityGroupRule.RemoteGroupID
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
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to add iptables rule: %v", err)})
			return
		}
	}

	// Insert into database
	now := time.Now()
	_, err := database.DB.Exec(c.Request.Context(), `
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"security_group_rule": gin.H{
			"id":                 ruleID,
			"security_group_id":  req.SecurityGroupRule.SecurityGroupID,
			"direction":          req.SecurityGroupRule.Direction,
			"ethertype":          etherType,
			"protocol":           protocol,
			"port_range_min":     portMin,
			"port_range_max":     portMax,
			"remote_ip_prefix":   remoteIP,
			"remote_group_id":    remoteGroup,
			"created_at":         now.Format(time.RFC3339),
		},
	})
}

// ListSecurityGroupRules lists all security group rules
func (svc *Service) ListSecurityGroupRules(c *gin.Context) {
	rows, err := database.DB.Query(c.Request.Context(), `
		SELECT id, security_group_id, direction, ethertype, protocol, port_range_min, port_range_max, remote_ip_prefix, remote_group_id, created_at
		FROM security_group_rules
		ORDER BY created_at DESC
	`)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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

	err := database.DB.QueryRow(c.Request.Context(), `
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

	// Delete from database
	_, err = database.DB.Exec(c.Request.Context(),
		"DELETE FROM security_group_rules WHERE id = $1",
		ruleID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
	tapName := "tap-" + portID[:8]
	macAddress := generateMAC()

	// Get network and subnet info to allocate IP
	var netID string
	err := database.DB.QueryRow(ctx,
		"SELECT id FROM networks WHERE id = $1 AND (project_id = $2 OR shared = true)",
		networkID, projectID,
	).Scan(&netID)

	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("network %s not found", networkID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query network: %w", err)
	}

	// Get subnet to allocate IP
	var subnetID, cidr string
	err = database.DB.QueryRow(ctx,
		"SELECT id, cidr FROM subnets WHERE network_id = $1 LIMIT 1",
		networkID,
	).Scan(&subnetID, &cidr)

	if err != nil {
		return nil, fmt.Errorf("no subnet found for network %s: %w", networkID, err)
	}

	// Allocate IP from subnet
	allocatedIP := allocateIP(cidr)
	if allocatedIP == "" {
		return nil, fmt.Errorf("failed to allocate IP from subnet %s", subnetID)
	}

	fixedIPs := []map[string]interface{}{
		{
			"subnet_id":  subnetID,
			"ip_address": allocatedIP,
		},
	}
	fixedIPsJSON, _ := json.Marshal(fixedIPs)

	// Create TAP device in namespace (skip in stub mode)
	if svc.mode != "stub" {
		nsName := svc.nsManager.GetNamespaceName(projectID)
		if err := svc.tapManager.CreateTAPDevice(tapName, true, nsName); err != nil {
			return nil, fmt.Errorf("failed to create TAP device: %w", err)
		}

		// Attach TAP to bridge
		bridgeName := "br-" + networkID[:8]
		if err := svc.brManager.AttachToBridge(tapName, bridgeName, true, nsName); err != nil {
			return nil, fmt.Errorf("failed to attach TAP to bridge: %w", err)
		}
	}

	// Insert into database
	now := time.Now()
	_, err = database.DB.Exec(ctx, `
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

	rows, err := database.DB.Query(ctx, query, securityGroupIDs)
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

	return rules, nil
}
