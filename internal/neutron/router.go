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
	"github.com/rs/zerolog/log"
	"github.com/cobaltcore-dev/o3k/internal/common"
)

// Router represents a Neutron L3 router
type Router struct {
	ID                  string                 `json:"id"`
	Name                string                 `json:"name"`
	ProjectID           string                 `json:"tenant_id"` // Keep tenant_id for compatibility
	AdminStateUp        bool                   `json:"admin_state_up"`
	Status              string                 `json:"status"`
	ExternalGatewayInfo map[string]interface{} `json:"external_gateway_info"`
	Distributed         bool                   `json:"distributed"`
	HA                  bool                   `json:"ha"`
	CreatedAt           time.Time              `json:"created_at"`
	UpdatedAt           time.Time              `json:"updated_at"`
}

// FloatingIP represents a floating IP address
type FloatingIP struct {
	ID                 string    `json:"id"`
	ProjectID          string    `json:"tenant_id"`
	FloatingNetworkID  string    `json:"floating_network_id"`
	FloatingIPAddress  string    `json:"floating_ip_address"`
	FixedIPAddress     string    `json:"fixed_ip_address"`
	PortID             string    `json:"port_id"`
	RouterID           string    `json:"router_id"`
	Status             string    `json:"status"`
	Description        string    `json:"description"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// CreateRouterRequest represents a router creation request
type CreateRouterRequest struct {
	Router struct {
		Name                string                 `json:"name" binding:"required"`
		AdminStateUp        *bool                  `json:"admin_state_up"`
		ExternalGatewayInfo map[string]interface{} `json:"external_gateway_info"`
		Distributed         *bool                  `json:"distributed"`
		HA                  *bool                  `json:"ha"`
	} `json:"router"`
}

// UpdateRouterRequest represents a router update request
type UpdateRouterRequest struct {
	Router struct {
		Name                *string                 `json:"name"`
		AdminStateUp        *bool                   `json:"admin_state_up"`
		ExternalGatewayInfo *map[string]interface{} `json:"external_gateway_info"`
	} `json:"router"`
}

// ListRouters lists all routers for the project
func (svc *Service) ListRouters(c *gin.Context) {
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
		markerCondition = fmt.Sprintf(` AND (created_at, id) > (SELECT created_at, id FROM routers WHERE id = $%d)`, argIdx)
		queryArgs = append(queryArgs, marker)
		argIdx++
	}

	queryArgs = append(queryArgs, limit+1)

	rows, err := svc.activeDB().Query(c.Request.Context(), fmt.Sprintf(`
		SELECT id, name, project_id, admin_state_up, status, external_gateway_info,
		       distributed, ha, created_at, updated_at
		FROM routers
		WHERE project_id = $1%s
		ORDER BY created_at ASC, id ASC
		LIMIT $%d
	`, markerCondition, argIdx), queryArgs...)

	if err != nil {
		log.Error().Err(err).Str("operation", "list_routers").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to list routers"))
		return
	}
	defer rows.Close()

	var routers []gin.H
	for rows.Next() {
		var r Router
		var gatewayInfo sql.NullString

		if err := rows.Scan(&r.ID, &r.Name, &r.ProjectID, &r.AdminStateUp, &r.Status,
			&gatewayInfo, &r.Distributed, &r.HA, &r.CreatedAt, &r.UpdatedAt); err != nil {
			continue
		}

		// Parse external gateway info
		if gatewayInfo.Valid {
			json.Unmarshal([]byte(gatewayInfo.String), &r.ExternalGatewayInfo)
		}

		routers = append(routers, gin.H{
			"id":                    r.ID,
			"name":                  r.Name,
			"tenant_id":             r.ProjectID,
			"project_id":            r.ProjectID,
			"admin_state_up":        r.AdminStateUp,
			"status":                r.Status,
			"external_gateway_info": r.ExternalGatewayInfo,
			"distributed":           r.Distributed,
			"ha":                    r.HA,
			"created_at":            r.CreatedAt.Format(time.RFC3339),
			"updated_at":            r.UpdatedAt.Format(time.RFC3339),
		})
	}
	if err := rows.Err(); err != nil {
		log.Error().Err(err).Str("operation", "list_routers").Msg("rows iteration error")
		common.SendError(c, common.NewInternalServerError("failed to list routers"))
		return
	}

	if routers == nil {
		routers = []gin.H{}
	}

	// Check if there are more results
	resp := gin.H{"routers": routers}
	if len(routers) > limit {
		routers = routers[:limit]
		lastID := routers[limit-1]["id"].(string)
		resp = gin.H{
			"routers":       routers,
			"routers_links": []gin.H{{"rel": "next", "href": fmt.Sprintf("?marker=%s&limit=%d", lastID, limit)}},
		}
	}

	c.JSON(http.StatusOK, resp)
}

// CreateRouter creates a new L3 router
func (svc *Service) CreateRouter(c *gin.Context) {
	var req CreateRouterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	projectID := c.GetString("project_id")
	routerID := uuid.New().String()

	adminStateUp := true
	if req.Router.AdminStateUp != nil {
		adminStateUp = *req.Router.AdminStateUp
	}

	distributed := false
	if req.Router.Distributed != nil {
		distributed = *req.Router.Distributed
	}

	ha := false
	if req.Router.HA != nil {
		ha = *req.Router.HA
	}

	// Serialize external gateway info to JSON
	var gatewayInfoJSON []byte
	if req.Router.ExternalGatewayInfo != nil {
		gatewayInfoJSON, _ = json.Marshal(req.Router.ExternalGatewayInfo)
	}

	// Insert into database first
	now := time.Now()
	_, err := svc.activeDB().Exec(c.Request.Context(), `
		INSERT INTO routers (id, name, project_id, admin_state_up, status, external_gateway_info, distributed, ha, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, routerID, req.Router.Name, projectID, adminStateUp, "ACTIVE", gatewayInfoJSON, distributed, ha, now, now)

	if err != nil {
		log.Error().Err(err).Str("operation", "create_router").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to create router"))
		return
	}

	// Create router namespace after DB insert (best effort)
	if err := svc.routerManager.CreateRouterNamespace(routerID); err != nil {
		log.Warn().Err(err).Str("router_id", routerID).Msg("failed to create router namespace (router record exists in DB)")
	}

	// If external gateway is configured, set it up
	if req.Router.ExternalGatewayInfo != nil {
		if err := svc.configureExternalGateway(c.Request.Context(), routerID, req.Router.ExternalGatewayInfo); err != nil {
			// Log error but don't fail the creation
			fmt.Printf("Warning: failed to configure external gateway: %v\n", err)
		}
	}

	c.JSON(http.StatusCreated, gin.H{
		"router": gin.H{
			"id":                    routerID,
			"name":                  req.Router.Name,
			"tenant_id":             projectID,
			"project_id":            projectID,
			"admin_state_up":        adminStateUp,
			"status":                "ACTIVE",
			"external_gateway_info": req.Router.ExternalGatewayInfo,
			"distributed":           distributed,
			"ha":                    ha,
			"created_at":            now.Format(time.RFC3339),
			"updated_at":            now.Format(time.RFC3339),
		},
	})
}

// GetRouter retrieves a single router
func (svc *Service) GetRouter(c *gin.Context) {
	routerID := c.Param("id")
	projectID := c.GetString("project_id")

	var r Router
	var gatewayInfo sql.NullString

	err := svc.activeDB().QueryRow(c.Request.Context(), `
		SELECT id, name, project_id, admin_state_up, status, external_gateway_info,
		       distributed, ha, created_at, updated_at
		FROM routers
		WHERE id = $1 AND project_id = $2
	`, routerID, projectID).Scan(&r.ID, &r.Name, &r.ProjectID, &r.AdminStateUp, &r.Status,
		&gatewayInfo, &r.Distributed, &r.HA, &r.CreatedAt, &r.UpdatedAt)

	if err == pgx.ErrNoRows {
		common.SendError(c, common.NewNotFoundError("router"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "get_router").Str("router_id", routerID).Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to get router"))
		return
	}

	// Parse external gateway info
	if gatewayInfo.Valid {
		json.Unmarshal([]byte(gatewayInfo.String), &r.ExternalGatewayInfo)
	}

	c.JSON(http.StatusOK, gin.H{
		"router": gin.H{
			"id":                    r.ID,
			"name":                  r.Name,
			"tenant_id":             r.ProjectID,
			"project_id":            r.ProjectID,
			"admin_state_up":        r.AdminStateUp,
			"status":                r.Status,
			"external_gateway_info": r.ExternalGatewayInfo,
			"distributed":           r.Distributed,
			"ha":                    r.HA,
			"created_at":            r.CreatedAt.Format(time.RFC3339),
			"updated_at":            r.UpdatedAt.Format(time.RFC3339),
		},
	})
}

// UpdateRouter updates an existing router
func (svc *Service) UpdateRouter(c *gin.Context) {
	routerID := c.Param("id")
	projectID := c.GetString("project_id")

	var req UpdateRouterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	updates := []string{}
	args := []interface{}{}
	argID := 1

	if req.Router.Name != nil {
		updates = append(updates, fmt.Sprintf("name = $%d", argID))
		args = append(args, *req.Router.Name)
		argID++
	}

	if req.Router.AdminStateUp != nil {
		updates = append(updates, fmt.Sprintf("admin_state_up = $%d", argID))
		args = append(args, *req.Router.AdminStateUp)
		argID++
	}

	if req.Router.ExternalGatewayInfo != nil {
		gatewayInfoJSON, _ := json.Marshal(*req.Router.ExternalGatewayInfo)
		updates = append(updates, fmt.Sprintf("external_gateway_info = $%d", argID))
		args = append(args, gatewayInfoJSON)
		argID++

		// Configure external gateway
		if err := svc.configureExternalGateway(c.Request.Context(), routerID, *req.Router.ExternalGatewayInfo); err != nil {
			fmt.Printf("Warning: failed to configure external gateway: %v\n", err)
		}
	}

	if len(updates) == 0 {
		common.SendError(c, common.NewBadRequestError("no updates provided"))
		return
	}

	updates = append(updates, fmt.Sprintf("updated_at = $%d", argID))
	args = append(args, time.Now())
	argID++

	args = append(args, routerID, projectID)

	query := fmt.Sprintf("UPDATE routers SET %s WHERE id = $%d AND project_id = $%d",
		updateString(updates), argID, argID+1)

	_, err := svc.activeDB().Exec(c.Request.Context(), query, args...)
	if err != nil {
		log.Error().Err(err).Str("operation", "update_router").Str("router_id", routerID).Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to update router"))
		return
	}

	// Return updated router
	svc.GetRouter(c)
}

// DeleteRouter deletes a router
func (svc *Service) DeleteRouter(c *gin.Context) {
	routerID := c.Param("id")
	projectID := c.GetString("project_id")

	// Check if router has any interfaces attached
	var interfaceCount int
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT COUNT(*) FROM router_interfaces WHERE router_id = $1",
		routerID,
	).Scan(&interfaceCount)

	if err != nil && err != pgx.ErrNoRows {
		log.Error().Err(err).Str("operation", "delete_router_check").Str("router_id", routerID).Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to check router interfaces"))
		return
	}

	if interfaceCount > 0 {
		common.SendError(c, common.NewConflictError(
			fmt.Sprintf("router %s has %d interfaces attached", routerID, interfaceCount),
		))
		return
	}

	// Delete router namespace
	if err := svc.routerManager.DeleteRouterNamespace(routerID); err != nil {
		fmt.Printf("Warning: failed to delete router namespace: %v\n", err)
	}

	// Delete from database
	_, err = svc.activeDB().Exec(c.Request.Context(),
		"DELETE FROM routers WHERE id = $1 AND project_id = $2",
		routerID, projectID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "delete_router").Str("router_id", routerID).Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to delete router"))
		return
	}

	c.Status(http.StatusNoContent)
}

// AddRouterInterface adds an interface (subnet) to a router
func (svc *Service) AddRouterInterface(c *gin.Context) {
	routerID := c.Param("id")
	projectID := c.GetString("project_id")

	var req struct {
		SubnetID string `json:"subnet_id"`
		PortID   string `json:"port_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Verify router exists
	var routerExists bool
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM routers WHERE id = $1 AND project_id = $2)",
		routerID, projectID,
	).Scan(&routerExists)

	if err != nil || !routerExists {
		common.SendError(c, common.NewNotFoundError("router"))
		return
	}

	// Get subnet information (must belong to same project via network ownership)
	var subnetID, networkID, cidr, gatewayIP string
	err = svc.activeDB().QueryRow(c.Request.Context(),
		`SELECT s.id, s.network_id, s.cidr, s.gateway_ip FROM subnets s
		 JOIN networks n ON n.id = s.network_id
		 WHERE s.id = $1 AND n.project_id = $2`,
		req.SubnetID, projectID,
	).Scan(&subnetID, &networkID, &cidr, &gatewayIP)

	if err == pgx.ErrNoRows {
		common.SendError(c, common.NewNotFoundError("subnet"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "add_router_interface_subnet_lookup").Str("subnet_id", req.SubnetID).Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to get subnet"))
		return
	}

	// Create a port for the router interface
	portID := uuid.New().String()
	macAddress := generateMAC()

	fixedIPs := []map[string]string{
		{
			"subnet_id":  subnetID,
			"ip_address": gatewayIP,
		},
	}
	fixedIPsJSON, _ := json.Marshal(fixedIPs)

	now := time.Now()
	_, err = svc.activeDB().Exec(c.Request.Context(), `
		INSERT INTO ports (id, name, network_id, project_id, device_id, device_owner, mac_address, admin_state_up, status, fixed_ips, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, portID, "router-interface-"+routerID[:8], networkID, projectID, routerID, "network:router_interface",
		macAddress, true, "ACTIVE", fixedIPsJSON, now, now)

	if err != nil {
		log.Error().Err(err).Str("operation", "add_router_interface_port").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to create router interface port"))
		return
	}

	// Create router interface record
	_, err = svc.activeDB().Exec(c.Request.Context(), `
		INSERT INTO router_interfaces (id, router_id, port_id, subnet_id, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`, uuid.New().String(), routerID, portID, subnetID, now)

	if err != nil {
		log.Error().Err(err).Str("operation", "add_router_interface_record").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to create router interface record"))
		return
	}

	// Attach interface to router namespace
	interfaceName := "qg-" + portID[:10]
	_, ipNet, _ := net.ParseCIDR(cidr)
	maskBits, _ := ipNet.Mask.Size()
	cidrSuffix := fmt.Sprintf("%d", maskBits)

	if err := svc.routerManager.AttachInterfaceToRouter(routerID, interfaceName, gatewayIP, cidrSuffix); err != nil {
		fmt.Printf("Warning: failed to attach interface to router: %v\n", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"id":         routerID,
		"subnet_id":  subnetID,
		"network_id": networkID,
		"port_id":    portID,
		"tenant_id":  projectID,
	})
}

// RemoveRouterInterface removes an interface from a router
func (svc *Service) RemoveRouterInterface(c *gin.Context) {
	routerID := c.Param("id")
	projectID := c.GetString("project_id")

	var req struct {
		SubnetID string `json:"subnet_id"`
		PortID   string `json:"port_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Get port ID from subnet if not provided
	portID := req.PortID
	if portID == "" && req.SubnetID != "" {
		err := svc.activeDB().QueryRow(c.Request.Context(),
			"SELECT port_id FROM router_interfaces WHERE router_id = $1 AND subnet_id = $2",
			routerID, req.SubnetID,
		).Scan(&portID)

		if err == pgx.ErrNoRows {
			common.SendError(c, common.NewNotFoundError("router interface"))
			return
		}
	}

	// Detach interface from router namespace
	interfaceName := "qg-" + portID[:10]
	if err := svc.routerManager.DetachInterfaceFromRouter(routerID, interfaceName); err != nil {
		fmt.Printf("Warning: failed to detach interface from router: %v\n", err)
	}

	// Delete router interface record
	_, err := svc.activeDB().Exec(c.Request.Context(),
		"DELETE FROM router_interfaces WHERE router_id = $1 AND port_id = $2",
		routerID, portID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "remove_router_interface_record").Str("router_id", routerID).Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to remove router interface record"))
		return
	}

	// Query network_id from the port before deleting it
	var networkID string
	_ = svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT network_id FROM ports WHERE id = $1",
		portID,
	).Scan(&networkID)

	// Delete the port
	_, err = svc.activeDB().Exec(c.Request.Context(),
		"DELETE FROM ports WHERE id = $1 AND project_id = $2",
		portID, projectID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "remove_router_interface_port").Str("port_id", portID).Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to delete router interface port"))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":         routerID,
		"subnet_id":  req.SubnetID,
		"port_id":    portID,
		"network_id": networkID,
		"tenant_id":  projectID,
	})
}

// Helper function to configure external gateway
func (svc *Service) configureExternalGateway(ctx context.Context, routerID string, gatewayInfo map[string]interface{}) error {
	if gatewayInfo == nil {
		return nil
	}

	networkID, ok := gatewayInfo["network_id"].(string)
	if !ok {
		return fmt.Errorf("missing network_id in external_gateway_info")
	}

	enableSNAT := true
	if val, ok := gatewayInfo["enable_snat"].(bool); ok {
		enableSNAT = val
	}

	// Get external network details
	var externalCIDR string
	err := svc.activeDB().QueryRow(ctx,
		"SELECT cidr FROM subnets WHERE network_id = $1 LIMIT 1",
		networkID,
	).Scan(&externalCIDR)

	if err != nil {
		return fmt.Errorf("failed to get external network subnet: %w", err)
	}

	// Get all internal subnets connected to this router
	rows, err := svc.activeDB().Query(ctx, `
		SELECT s.cidr
		FROM subnets s
		JOIN router_interfaces ri ON ri.subnet_id = s.id
		WHERE ri.router_id = $1
	`, routerID)

	if err != nil {
		return err
	}
	defer rows.Close()

	externalInterface := "qg-ext-" + routerID[:7]

	// Enable SNAT for each internal subnet
	if enableSNAT {
		for rows.Next() {
			var internalCIDR string
			if err := rows.Scan(&internalCIDR); err != nil {
				continue
			}

			if err := svc.routerManager.EnableSNAT(routerID, externalInterface, internalCIDR); err != nil {
				fmt.Printf("Warning: failed to enable SNAT for %s: %v\n", internalCIDR, err)
			}
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterating internal subnets: %w", err)
		}
	}

	return nil
}
