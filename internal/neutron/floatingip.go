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
	"time"

	"github.com/cobaltcore-dev/o3k/internal/common"
	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// CreateFloatingIPRequest represents a floating IP creation request
type CreateFloatingIPRequest struct {
	FloatingIP struct {
		FloatingNetworkID string `json:"floating_network_id" binding:"required"`
		PortID            string `json:"port_id"`
		FixedIPAddress    string `json:"fixed_ip_address"`
		SubnetID          string `json:"subnet_id"`
		Description       string `json:"description"`
	} `json:"floatingip"`
}

// UpdateFloatingIPRequest represents a floating IP update request
type UpdateFloatingIPRequest struct {
	FloatingIP struct {
		PortID         *string `json:"port_id"`
		FixedIPAddress *string `json:"fixed_ip_address"`
		Description    *string `json:"description"`
	} `json:"floatingip"`
}

// ListFloatingIPs lists all floating IPs for the project
func (svc *Service) ListFloatingIPs(c *gin.Context) {
	projectID := c.GetString("project_id")

	// Parse pagination parameters
	limit := common.DefaultPaginationLimit
	if limitParam := c.Query("limit"); limitParam != "" {
		if parsedLimit, err := strconv.Atoi(limitParam); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}
	limit = common.CapLimit(limit)

	// Marker-based pagination using (created_at, id) for deterministic ordering.
	var markerCondition string
	var queryArgs []interface{}
	queryArgs = append(queryArgs, projectID)
	argIdx := 2

	if marker := c.Query("marker"); marker != "" {
		markerCondition = fmt.Sprintf(` AND (created_at, id) > (SELECT created_at, id FROM floating_ips WHERE id = $%d)`, argIdx)
		queryArgs = append(queryArgs, marker)
		argIdx++
	}

	queryArgs = append(queryArgs, limit+1)

	rows, err := svc.activeDB().Query(c.Request.Context(), fmt.Sprintf(`
		SELECT id, project_id, floating_network_id, floating_ip_address,
		       fixed_ip_address, port_id, router_id, status, description,
		       created_at, updated_at
		FROM floating_ips
		WHERE project_id = $1%s
		ORDER BY created_at ASC, id ASC
		LIMIT $%d
	`, markerCondition, argIdx), queryArgs...)

	if err != nil {
		log.Error().Err(err).Str("operation", "list_floatingips").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to list floating IPs"))
		return
	}
	defer rows.Close()

	var floatingIPs []gin.H
	for rows.Next() {
		var fip FloatingIP
		var fixedIP, portID, routerID, description sql.NullString

		if err := rows.Scan(&fip.ID, &fip.ProjectID, &fip.FloatingNetworkID, &fip.FloatingIPAddress,
			&fixedIP, &portID, &routerID, &fip.Status, &description, &fip.CreatedAt, &fip.UpdatedAt); err != nil {
			continue
		}

		result := gin.H{
			"id":                  fip.ID,
			"tenant_id":           fip.ProjectID,
			"project_id":          fip.ProjectID,
			"floating_network_id": fip.FloatingNetworkID,
			"floating_ip_address": fip.FloatingIPAddress,
			"status":              fip.Status,
			"created_at":          fip.CreatedAt.Format(time.RFC3339),
			"updated_at":          fip.UpdatedAt.Format(time.RFC3339),
		}

		if fixedIP.Valid {
			result["fixed_ip_address"] = fixedIP.String
		} else {
			result["fixed_ip_address"] = nil
		}

		if portID.Valid {
			result["port_id"] = portID.String
		} else {
			result["port_id"] = nil
		}

		if routerID.Valid {
			result["router_id"] = routerID.String
		} else {
			result["router_id"] = nil
		}

		if description.Valid {
			result["description"] = description.String
		} else {
			result["description"] = ""
		}

		floatingIPs = append(floatingIPs, result)
	}
	if err := rows.Err(); err != nil {
		log.Error().Err(err).Str("operation", "list_floatingips").Msg("rows iteration error")
		common.SendError(c, common.NewInternalServerError("failed to list floating IPs"))
		return
	}

	if floatingIPs == nil {
		floatingIPs = []gin.H{}
	}

	// Check if there are more results
	resp := gin.H{"floatingips": floatingIPs}
	if len(floatingIPs) > limit {
		floatingIPs = floatingIPs[:limit]
		lastID, _ := floatingIPs[limit-1]["id"].(string)
		resp = gin.H{
			"floatingips":       floatingIPs,
			"floatingips_links": []gin.H{{"rel": "next", "href": fmt.Sprintf("?marker=%s&limit=%d", lastID, limit)}},
		}
	}

	c.JSON(http.StatusOK, resp)
}

// CreateFloatingIP allocates a new floating IP
func (svc *Service) CreateFloatingIP(c *gin.Context) {
	var req CreateFloatingIPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	projectID := c.GetString("project_id")
	floatingIPID := uuid.New().String()

	// Get external network subnet to allocate IP from
	var subnetCIDR string
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT cidr FROM subnets WHERE network_id = $1 LIMIT 1",
		req.FloatingIP.FloatingNetworkID,
	).Scan(&subnetCIDR)

	if errors.Is(err, database.ErrNoRows) {
		// No subnet exists - use default external IP pool (RFC 5737 TEST-NET-1)
		subnetCIDR = "192.0.2.0/24"
	} else if err != nil {
		log.Error().Err(err).Str("operation", "create_floatingip_subnet_lookup").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to allocate floating IP"))
		return
	}

	// Allocate an IP from the external network subnet
	floatingIP, err := svc.allocateFloatingIP(c.Request.Context(), req.FloatingIP.FloatingNetworkID, subnetCIDR)
	if err != nil {
		log.Error().Err(err).Str("operation", "allocate_floatingip").Msg("failed to allocate floating IP")
		common.SendError(c, common.NewInternalServerError("failed to allocate floating IP"))
		return
	}

	// Determine status and associated details
	status := "DOWN"
	var fixedIP, portID, routerID *string

	if req.FloatingIP.PortID != "" {
		// Associate with port
		portID = &req.FloatingIP.PortID
		status = "ACTIVE"

		// Get fixed IP from port
		var fixedIPsJSON string
		err := svc.activeDB().QueryRow(c.Request.Context(),
			"SELECT fixed_ips FROM ports WHERE id = $1 AND project_id = $2",
			req.FloatingIP.PortID, projectID,
		).Scan(&fixedIPsJSON)

		if err != nil {
			common.SendError(c, common.NewNotFoundError("port"))
			return
		}

		// Parse fixed IPs and get the first one
		var fixedIPAddr string
		if req.FloatingIP.FixedIPAddress != "" {
			fixedIPAddr = req.FloatingIP.FixedIPAddress
		} else {
			// Extract from fixed_ips JSON
			var fixedIPs []map[string]interface{}
			if err := json.Unmarshal([]byte(fixedIPsJSON), &fixedIPs); err != nil {
				log.Error().Err(err).Str("operation", "parse_port_fixed_ips").Msg("failed to parse port fixed_ips")
				common.SendError(c, common.NewInternalServerError("failed to parse port fixed IPs"))
				return
			}

			if len(fixedIPs) == 0 {
				common.SendError(c, common.NewBadRequestError("port has no fixed IP addresses"))
				return
			}

			// Use the first fixed IP
			if ipAddr, ok := fixedIPs[0]["ip_address"].(string); ok {
				fixedIPAddr = ipAddr
			} else {
				log.Error().Str("operation", "parse_port_fixed_ips").Msg("invalid fixed_ips format")
				common.SendError(c, common.NewInternalServerError("invalid fixed_ips format"))
				return
			}
		}
		fixedIP = &fixedIPAddr

		// Get router ID from port's network
		var networkID string
		_ = svc.activeDB().QueryRow(c.Request.Context(),
			"SELECT network_id FROM ports WHERE id = $1",
			req.FloatingIP.PortID,
		).Scan(&networkID)

		// Find router with external gateway and interface to this network
		var rID string
		err = svc.activeDB().QueryRow(c.Request.Context(), `
			SELECT DISTINCT r.id
			FROM routers r
			JOIN router_interfaces ri ON ri.router_id = r.id
			JOIN ports p ON p.id = ri.port_id
			WHERE p.network_id = $1 AND r.external_gateway_info IS NOT NULL
			LIMIT 1
		`, networkID).Scan(&rID)

		if err == nil {
			routerID = &rID

			// Configure DNAT/SNAT rules for floating IP
			rid := rID
			if len(rid) > 7 {
				rid = rid[:7]
			}
			externalInterface := "qg-ext-" + rid
			if err := svc.routerManager.AddFloatingIP(rID, floatingIP, *fixedIP, externalInterface); err != nil {
				fmt.Printf("Warning: failed to configure floating IP NAT: %v\n", err)
			}
		}
	}

	// Insert into database
	now := time.Now()
	_, err = svc.activeDB().Exec(c.Request.Context(), `
		INSERT INTO floating_ips (id, project_id, floating_network_id, floating_ip_address, fixed_ip_address, port_id, router_id, status, description, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, floatingIPID, projectID, req.FloatingIP.FloatingNetworkID, floatingIP, fixedIP, portID, routerID, status, req.FloatingIP.Description, now, now)

	if err != nil {
		log.Error().Err(err).Str("operation", "create_floatingip").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to create floating IP"))
		return
	}

	result := gin.H{
		"id":                  floatingIPID,
		"tenant_id":           projectID,
		"project_id":          projectID,
		"floating_network_id": req.FloatingIP.FloatingNetworkID,
		"floating_ip_address": floatingIP,
		"status":              status,
		"description":         req.FloatingIP.Description,
		"created_at":          now.Format(time.RFC3339),
		"updated_at":          now.Format(time.RFC3339),
	}

	if fixedIP != nil {
		result["fixed_ip_address"] = *fixedIP
	} else {
		result["fixed_ip_address"] = nil
	}

	if portID != nil {
		result["port_id"] = *portID
	} else {
		result["port_id"] = nil
	}

	if routerID != nil {
		result["router_id"] = *routerID
	} else {
		result["router_id"] = nil
	}

	c.JSON(http.StatusCreated, gin.H{"floatingip": result})
}

// GetFloatingIP retrieves a single floating IP
func (svc *Service) GetFloatingIP(c *gin.Context) {
	floatingIPID := c.Param("id")
	projectID := c.GetString("project_id")

	var fip FloatingIP
	var fixedIP, portID, routerID, description sql.NullString

	err := svc.activeDB().QueryRow(c.Request.Context(), `
		SELECT id, project_id, floating_network_id, floating_ip_address,
		       fixed_ip_address, port_id, router_id, status, description,
		       created_at, updated_at
		FROM floating_ips
		WHERE id = $1 AND project_id = $2
	`, floatingIPID, projectID).Scan(&fip.ID, &fip.ProjectID, &fip.FloatingNetworkID, &fip.FloatingIPAddress,
		&fixedIP, &portID, &routerID, &fip.Status, &description, &fip.CreatedAt, &fip.UpdatedAt)

	if errors.Is(err, database.ErrNoRows) {
		common.SendError(c, common.NewNotFoundError("floating IP"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "get_floatingip").Str("floatingip_id", floatingIPID).Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to get floating IP"))
		return
	}

	result := gin.H{
		"id":                  fip.ID,
		"tenant_id":           fip.ProjectID,
		"project_id":          fip.ProjectID,
		"floating_network_id": fip.FloatingNetworkID,
		"floating_ip_address": fip.FloatingIPAddress,
		"status":              fip.Status,
		"created_at":          fip.CreatedAt.Format(time.RFC3339),
		"updated_at":          fip.UpdatedAt.Format(time.RFC3339),
	}

	if fixedIP.Valid {
		result["fixed_ip_address"] = fixedIP.String
	} else {
		result["fixed_ip_address"] = nil
	}

	if portID.Valid {
		result["port_id"] = portID.String
	} else {
		result["port_id"] = nil
	}

	if routerID.Valid {
		result["router_id"] = routerID.String
	} else {
		result["router_id"] = nil
	}

	if description.Valid {
		result["description"] = description.String
	} else {
		result["description"] = ""
	}

	c.JSON(http.StatusOK, gin.H{"floatingip": result})
}

// UpdateFloatingIP updates a floating IP (associate/disassociate with port)
func (svc *Service) UpdateFloatingIP(c *gin.Context) {
	floatingIPID := c.Param("id")
	projectID := c.GetString("project_id")

	// Parse raw JSON to detect if port_id key is present
	var rawReq map[string]map[string]interface{}
	if err := c.ShouldBindJSON(&rawReq); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Get current floating IP details
	var currentFloatingIP, currentFixedIP, currentRouterID sql.NullString
	var currentPortID sql.NullString
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT floating_ip_address, fixed_ip_address, port_id, router_id FROM floating_ips WHERE id = $1 AND project_id = $2",
		floatingIPID, projectID,
	).Scan(&currentFloatingIP, &currentFixedIP, &currentPortID, &currentRouterID)

	if errors.Is(err, database.ErrNoRows) {
		common.SendError(c, common.NewNotFoundError("floating IP"))
		return
	}

	updates := []string{}
	args := []interface{}{}
	argID := 1

	// Handle port association/disassociation
	if floatingIPData, ok := rawReq["floatingip"]; ok {
		if portIDValue, hasPortID := floatingIPData["port_id"]; hasPortID {
			// port_id key is present (could be null or a string)
			shouldDisassociate := portIDValue == nil

			// Remove old NAT rules if there was a previous association
			if currentPortID.Valid && currentFixedIP.Valid && currentRouterID.Valid {
				rid := currentRouterID.String
				if len(rid) > 7 {
					rid = rid[:7]
				}
				externalInterface := "qg-ext-" + rid
				_ = svc.routerManager.RemoveFloatingIP(currentRouterID.String, currentFloatingIP.String, currentFixedIP.String, externalInterface)
			}

			if shouldDisassociate {
				// Disassociate (null value)
				updates = append(updates, fmt.Sprintf("port_id = $%d", argID))
				args = append(args, nil)
				argID++

				updates = append(updates, fmt.Sprintf("fixed_ip_address = $%d", argID))
				args = append(args, nil)
				argID++

				updates = append(updates, fmt.Sprintf("router_id = $%d", argID))
				args = append(args, nil)
				argID++

				updates = append(updates, fmt.Sprintf("status = $%d", argID))
				args = append(args, "DOWN")
				argID++
			} else {
				// Associate with new port
				newPortID, ok := portIDValue.(string)
				if !ok {
					common.SendError(c, common.NewBadRequestError("port_id must be a string or null"))
					return
				}
				// Associate with new port
				var networkID string
				_ = svc.activeDB().QueryRow(c.Request.Context(),
					"SELECT network_id FROM ports WHERE id = $1",
					newPortID,
				).Scan(&networkID)

				// Find router
				var routerID string
				err = svc.activeDB().QueryRow(c.Request.Context(), `
				SELECT DISTINCT r.id
				FROM routers r
				JOIN router_interfaces ri ON ri.router_id = r.id
				JOIN ports p ON p.id = ri.port_id
				WHERE p.network_id = $1 AND r.external_gateway_info IS NOT NULL
				LIMIT 1
			`, networkID).Scan(&routerID)

				if err != nil {
					common.SendError(c, common.NewBadRequestError("no router with external gateway found for this network"))
					return
				}

				// Get fixed IP
				var fixedIP string
				if fixedIPAddr, ok := floatingIPData["fixed_ip_address"]; ok && fixedIPAddr != nil {
					fixedIP, _ = fixedIPAddr.(string)
				}
				if fixedIP == "" {
					// Look up the port's actual fixed IP from the fixed_ips JSONB column
					var fixedIPsJSON string
					if err := svc.activeDB().QueryRow(c.Request.Context(),
						"SELECT fixed_ips FROM ports WHERE id = $1",
						newPortID,
					).Scan(&fixedIPsJSON); err == nil {
						var fixedIPs []map[string]interface{}
						if json.Unmarshal([]byte(fixedIPsJSON), &fixedIPs) == nil && len(fixedIPs) > 0 {
							if ipAddr, ok := fixedIPs[0]["ip_address"].(string); ok {
								fixedIP = ipAddr
							}
						}
					}
				}
				if fixedIP == "" {
					common.SendError(c, common.NewBadRequestError("could not determine fixed IP for port"))
					return
				}

				// Configure NAT rules
				rid := routerID
				if len(rid) > 7 {
					rid = rid[:7]
				}
				externalInterface := "qg-ext-" + rid
				if err := svc.routerManager.AddFloatingIP(routerID, currentFloatingIP.String, fixedIP, externalInterface); err != nil {
					log.Error().Err(err).Str("operation", "configure_nat").Msg("failed to configure NAT rules")
					common.SendError(c, common.NewInternalServerError("failed to configure NAT"))
					return
				}

				updates = append(updates, fmt.Sprintf("port_id = $%d", argID))
				args = append(args, newPortID)
				argID++

				updates = append(updates, fmt.Sprintf("fixed_ip_address = $%d", argID))
				args = append(args, fixedIP)
				argID++

				updates = append(updates, fmt.Sprintf("router_id = $%d", argID))
				args = append(args, routerID)
				argID++

				updates = append(updates, fmt.Sprintf("status = $%d", argID))
				args = append(args, "ACTIVE")
				argID++
			}
		}

		// Handle description update
		if descValue, hasDesc := floatingIPData["description"]; hasDesc {
			if desc, ok := descValue.(string); ok {
				updates = append(updates, fmt.Sprintf("description = $%d", argID))
				args = append(args, desc)
				argID++
			}
		}
	}

	if len(updates) == 0 {
		// No updates, just return current state
		svc.GetFloatingIP(c)
		return
	}

	updates = append(updates, fmt.Sprintf("updated_at = $%d", argID))
	args = append(args, time.Now())
	argID++

	args = append(args, floatingIPID, projectID)

	query := fmt.Sprintf("UPDATE floating_ips SET %s WHERE id = $%d AND project_id = $%d",
		updateString(updates), argID, argID+1)

	_, err = svc.activeDB().Exec(c.Request.Context(), query, args...)
	if err != nil {
		log.Error().Err(err).Str("operation", "update_floatingip").Str("floatingip_id", floatingIPID).Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to update floating IP"))
		return
	}

	// Return updated floating IP
	svc.GetFloatingIP(c)
}

// DeleteFloatingIP releases a floating IP
func (svc *Service) DeleteFloatingIP(c *gin.Context) {
	floatingIPID := c.Param("id")
	projectID := c.GetString("project_id")

	// Get floating IP details before deletion
	var floatingIP, fixedIP, routerID sql.NullString
	var portID sql.NullString
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT floating_ip_address, fixed_ip_address, port_id, router_id FROM floating_ips WHERE id = $1 AND project_id = $2",
		floatingIPID, projectID,
	).Scan(&floatingIP, &fixedIP, &portID, &routerID)

	if errors.Is(err, database.ErrNoRows) {
		common.SendError(c, common.NewNotFoundError("floating IP"))
		return
	}

	// Remove NAT rules if associated
	if portID.Valid && fixedIP.Valid && routerID.Valid {
		rid := routerID.String
		if len(rid) > 7 {
			rid = rid[:7]
		}
		externalInterface := "qg-ext-" + rid
		if err := svc.routerManager.RemoveFloatingIP(routerID.String, floatingIP.String, fixedIP.String, externalInterface); err != nil {
			fmt.Printf("Warning: failed to remove floating IP NAT rules: %v\n", err)
		}
	}

	// Delete from database
	_, err = svc.activeDB().Exec(c.Request.Context(),
		"DELETE FROM floating_ips WHERE id = $1 AND project_id = $2",
		floatingIPID, projectID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "delete_floatingip").Str("floatingip_id", floatingIPID).Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to delete floating IP"))
		return
	}

	c.Status(http.StatusNoContent)
}

// Helper function to allocate a floating IP from subnet
func (svc *Service) allocateFloatingIP(ctx context.Context, floatingNetworkID, subnetCIDR string) (string, error) {
	_, ipNet, err := net.ParseCIDR(subnetCIDR)
	if err != nil {
		return "", err
	}

	tx, err := svc.activeDB().BeginTx(ctx, database.TxOptions{
		IsoLevel: "serializable",
	})
	if err != nil {
		return "", fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Get all allocated floating IPs in this external network
	rows, err := tx.Query(ctx,
		"SELECT floating_ip_address FROM floating_ips WHERE floating_network_id = $1 FOR UPDATE",
		floatingNetworkID,
	)
	if err != nil {
		return "", fmt.Errorf("failed to query allocated IPs: %w", err)
	}
	defer rows.Close()

	allocatedIPs := make(map[string]bool)
	for rows.Next() {
		var ip string
		if err := rows.Scan(&ip); err != nil {
			continue
		}
		allocatedIPs[ip] = true
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("iterating allocated IPs: %w", err)
	}

	// Find first available IP in range
	// Start from .100 to avoid gateway and DHCP range
	ip := incrementIP(ipNet.IP, 100)
	for ipNet.Contains(ip) {
		ipStr := ip.String()
		if !allocatedIPs[ipStr] {
			if err := tx.Commit(ctx); err != nil {
				return "", fmt.Errorf("failed to commit IP allocation: %w", err)
			}
			return ipStr, nil
		}
		ip = incrementIP(ip, 1)
	}

	return "", fmt.Errorf("no available IPs in subnet %s", subnetCIDR)
}
