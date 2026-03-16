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
			"SELECT created_at FROM routers WHERE id = $1 AND project_id = $2",
			marker, projectID,
		).Scan(&markerCreatedAt)
		if err == nil {
			markerCondition = fmt.Sprintf(" AND created_at < $%d", argIdx)
			queryArgs = append(queryArgs, markerCreatedAt)
			argIdx++
		}
	}

	queryArgs = append(queryArgs, limit, offset)

	rows, err := database.DB.Query(c.Request.Context(), fmt.Sprintf(`
		SELECT id, name, project_id, admin_state_up, status, external_gateway_info,
		       distributed, ha, created_at, updated_at
		FROM routers
		WHERE project_id = $1%s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, markerCondition, argIdx, argIdx+1), queryArgs...)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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

	if routers == nil {
		routers = []gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{"routers": routers})
}

// CreateRouter creates a new L3 router
func (svc *Service) CreateRouter(c *gin.Context) {
	var req CreateRouterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
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

	// Create router namespace
	if err := svc.routerManager.CreateRouterNamespace(routerID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to create router namespace: %v", err)})
		return
	}

	// Insert into database
	now := time.Now()
	_, err := database.DB.Exec(c.Request.Context(), `
		INSERT INTO routers (id, name, project_id, admin_state_up, status, external_gateway_info, distributed, ha, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, routerID, req.Router.Name, projectID, adminStateUp, "ACTIVE", gatewayInfoJSON, distributed, ha, now, now)

	if err != nil {
		svc.routerManager.DeleteRouterNamespace(routerID) // Cleanup on error
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
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

	err := database.DB.QueryRow(c.Request.Context(), `
		SELECT id, name, project_id, admin_state_up, status, external_gateway_info,
		       distributed, ha, created_at, updated_at
		FROM routers
		WHERE id = $1 AND project_id = $2
	`, routerID, projectID).Scan(&r.ID, &r.Name, &r.ProjectID, &r.AdminStateUp, &r.Status,
		&gatewayInfo, &r.Distributed, &r.HA, &r.CreatedAt, &r.UpdatedAt)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "router not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "no updates provided"})
		return
	}

	updates = append(updates, fmt.Sprintf("updated_at = $%d", argID))
	args = append(args, time.Now())
	argID++

	args = append(args, routerID, projectID)

	query := fmt.Sprintf("UPDATE routers SET %s WHERE id = $%d AND project_id = $%d",
		updateString(updates), argID, argID+1)

	_, err := database.DB.Exec(c.Request.Context(), query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT COUNT(*) FROM router_interfaces WHERE router_id = $1",
		routerID,
	).Scan(&interfaceCount)

	if err != nil && err != pgx.ErrNoRows {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if interfaceCount > 0 {
		c.JSON(http.StatusConflict, gin.H{
			"error": fmt.Sprintf("router %s has %d interfaces attached", routerID, interfaceCount),
		})
		return
	}

	// Delete router namespace
	if err := svc.routerManager.DeleteRouterNamespace(routerID); err != nil {
		fmt.Printf("Warning: failed to delete router namespace: %v\n", err)
	}

	// Delete from database
	_, err = database.DB.Exec(c.Request.Context(),
		"DELETE FROM routers WHERE id = $1 AND project_id = $2",
		routerID, projectID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Verify router exists
	var routerExists bool
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM routers WHERE id = $1 AND project_id = $2)",
		routerID, projectID,
	).Scan(&routerExists)

	if err != nil || !routerExists {
		c.JSON(http.StatusNotFound, gin.H{"error": "router not found"})
		return
	}

	// Get subnet information
	var subnetID, networkID, cidr, gatewayIP string
	err = database.DB.QueryRow(c.Request.Context(),
		"SELECT id, network_id, cidr, gateway_ip FROM subnets WHERE id = $1",
		req.SubnetID,
	).Scan(&subnetID, &networkID, &cidr, &gatewayIP)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "subnet not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
	_, err = database.DB.Exec(c.Request.Context(), `
		INSERT INTO ports (id, name, network_id, project_id, device_id, device_owner, mac_address, admin_state_up, status, fixed_ips, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, portID, "router-interface-"+routerID[:8], networkID, projectID, routerID, "network:router_interface",
		macAddress, true, "ACTIVE", fixedIPsJSON, now, now)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Create router interface record
	_, err = database.DB.Exec(c.Request.Context(), `
		INSERT INTO router_interfaces (id, router_id, port_id, subnet_id, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`, uuid.New().String(), routerID, portID, subnetID, now)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
		"subnet_id": subnetID,
		"port_id":   portID,
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Get port ID from subnet if not provided
	portID := req.PortID
	if portID == "" && req.SubnetID != "" {
		err := database.DB.QueryRow(c.Request.Context(),
			"SELECT port_id FROM router_interfaces WHERE router_id = $1 AND subnet_id = $2",
			routerID, req.SubnetID,
		).Scan(&portID)

		if err == pgx.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "router interface not found"})
			return
		}
	}

	// Detach interface from router namespace
	interfaceName := "qg-" + portID[:10]
	if err := svc.routerManager.DetachInterfaceFromRouter(routerID, interfaceName); err != nil {
		fmt.Printf("Warning: failed to detach interface from router: %v\n", err)
	}

	// Delete router interface record
	_, err := database.DB.Exec(c.Request.Context(),
		"DELETE FROM router_interfaces WHERE router_id = $1 AND port_id = $2",
		routerID, portID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Delete the port
	_, err = database.DB.Exec(c.Request.Context(),
		"DELETE FROM ports WHERE id = $1 AND project_id = $2",
		portID, projectID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"subnet_id": req.SubnetID,
		"port_id":   portID,
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
	err := database.DB.QueryRow(ctx,
		"SELECT cidr FROM subnets WHERE network_id = $1 LIMIT 1",
		networkID,
	).Scan(&externalCIDR)

	if err != nil {
		return fmt.Errorf("failed to get external network subnet: %w", err)
	}

	// Get all internal subnets connected to this router
	rows, err := database.DB.Query(ctx, `
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
	}

	return nil
}
