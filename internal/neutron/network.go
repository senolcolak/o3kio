package neutron

import (
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/cobaltcore-dev/o3k/pkg/networking"
)

// Service handles Neutron API endpoints
type Service struct {
	mode             string
	nsManager        *networking.NetworkNamespaceManager
	brManager        *networking.BridgeManager
	tapManager       *networking.TAPDeviceManager
	dhcpManager      *networking.DHCPManager
	sgManager        *networking.SecurityGroupManager
	routerManager    *networking.RouterManager
	vxlanCoordinator *VXLANCoordinator
}

// NewService creates a new Neutron service
func NewService(mode string) *Service {
	sgManager, _ := networking.NewSecurityGroupManager(mode) // Ignore error for now
	return &Service{
		mode:          mode,
		nsManager:     networking.NewNetworkNamespaceManager(mode),
		brManager:     networking.NewBridgeManager(mode),
		tapManager:    networking.NewTAPDeviceManager(mode),
		dhcpManager:   networking.NewDHCPManager(mode),
		sgManager:     sgManager,
		routerManager: networking.NewRouterManager(mode),
	}
}

// RegisterRoutes registers Neutron routes
func (svc *Service) RegisterRoutes(r *gin.RouterGroup) {
	// Version discovery
	r.GET("/v2.0", svc.GetVersion)

	v2 := r.Group("/v2.0")
	{
		// Networks
		v2.GET("/networks", svc.ListNetworks)
		v2.POST("/networks", svc.CreateNetwork)
		v2.GET("/networks/:id", svc.GetNetwork)
		v2.DELETE("/networks/:id", svc.DeleteNetwork)
		v2.PUT("/networks/:id", svc.UpdateNetwork)

		// Subnets
		v2.GET("/subnets", svc.ListSubnets)
		v2.POST("/subnets", svc.CreateSubnet)
		v2.GET("/subnets/:id", svc.GetSubnet)
		v2.DELETE("/subnets/:id", svc.DeleteSubnet)

		// Ports
		v2.GET("/ports", svc.ListPorts)
		v2.POST("/ports", svc.CreatePort)
		v2.GET("/ports/:id", svc.GetPort)
		v2.DELETE("/ports/:id", svc.DeletePort)
		v2.PUT("/ports/:id", svc.UpdatePort)

		// Security Groups
		v2.GET("/security-groups", svc.ListSecurityGroups)
		v2.POST("/security-groups", svc.CreateSecurityGroup)
		v2.GET("/security-groups/:id", svc.GetSecurityGroup)
		v2.DELETE("/security-groups/:id", svc.DeleteSecurityGroup)

		// Security Group Rules
		v2.GET("/security-group-rules", svc.ListSecurityGroupRules)
		v2.POST("/security-group-rules", svc.CreateSecurityGroupRule)
		v2.DELETE("/security-group-rules/:id", svc.DeleteSecurityGroupRule)

		// Routers
		v2.GET("/routers", svc.ListRouters)
		v2.POST("/routers", svc.CreateRouter)
		v2.GET("/routers/:id", svc.GetRouter)
		v2.DELETE("/routers/:id", svc.DeleteRouter)
		v2.PUT("/routers/:id", svc.UpdateRouter)

		// Router Interfaces
		v2.PUT("/routers/:id/add_router_interface", svc.AddRouterInterface)
		v2.PUT("/routers/:id/remove_router_interface", svc.RemoveRouterInterface)

		// Floating IPs
		v2.GET("/floatingips", svc.ListFloatingIPs)
		v2.POST("/floatingips", svc.CreateFloatingIP)
		v2.GET("/floatingips/:id", svc.GetFloatingIP)
		v2.PUT("/floatingips/:id", svc.UpdateFloatingIP)
		v2.DELETE("/floatingips/:id", svc.DeleteFloatingIP)
	}
}

// SetVXLANCoordinator sets the VXLAN coordinator for this service
func (svc *Service) SetVXLANCoordinator(coordinator *VXLANCoordinator) {
	svc.vxlanCoordinator = coordinator
}

// GetNamespaceManager returns the namespace manager (for VXLAN coordinator)
func (svc *Service) GetNamespaceManager() *networking.NetworkNamespaceManager {
	return svc.nsManager
}

// GetVersion returns version details
func (svc *Service) GetVersion(c *gin.Context) {
	c.JSON(200, gin.H{
		"version": gin.H{
			"id":     "v2.0",
			"status": "CURRENT",
			"links": []gin.H{
				{"rel": "self", "href": "http://localhost:9696/v2.0"},
			},
		},
	})
}

// CreateNetworkRequest represents a network creation request
type CreateNetworkRequest struct {
	Network struct {
		Name         string `json:"name" binding:"required"`
		AdminStateUp *bool  `json:"admin_state_up"`
		Shared       *bool  `json:"shared"`
	} `json:"network"`
}

// CreateNetwork creates a new network
func (svc *Service) CreateNetwork(c *gin.Context) {
	var req CreateNetworkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	projectID := c.GetString("project_id")
	networkID := uuid.New().String()
	bridgeName := "br-" + networkID[:8]

	adminStateUp := true
	if req.Network.AdminStateUp != nil {
		adminStateUp = *req.Network.AdminStateUp
	}

	shared := false
	if req.Network.Shared != nil {
		shared = *req.Network.Shared
	}

	// Create namespace for project if it doesn't exist
	if err := svc.nsManager.CreateNamespace(projectID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to create namespace: %v", err)})
		return
	}

	// Create bridge in namespace
	nsName := svc.nsManager.GetNamespaceName(projectID)
	if err := svc.brManager.CreateBridge(bridgeName, true, nsName); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to create bridge: %v", err)})
		return
	}

	// Insert into database
	now := time.Now()

	// Determine network type based on VXLAN coordinator
	networkType := "flat"
	mtu := 1500
	if svc.vxlanCoordinator != nil {
		networkType = "vxlan"
		mtu = 1450 // Account for VXLAN overhead
	}

	_, err := database.DB.Exec(c.Request.Context(), `
		INSERT INTO networks (id, name, project_id, admin_state_up, status, shared, network_type, mtu, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, networkID, req.Network.Name, projectID, adminStateUp, "ACTIVE", shared, networkType, mtu, now, now)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"network": gin.H{
			"id":              networkID,
			"name":            req.Network.Name,
			"tenant_id":       projectID,
			"admin_state_up":  adminStateUp,
			"status":          "ACTIVE",
			"shared":          shared,
			"mtu":             1500,
			"created_at":      now.Format(time.RFC3339),
			"updated_at":      now.Format(time.RFC3339),
		},
	})
}

// ListNetworks lists all networks
func (svc *Service) ListNetworks(c *gin.Context) {
	projectID := c.GetString("project_id")

	rows, err := database.DB.Query(c.Request.Context(), `
		SELECT id, name, admin_state_up, status, shared, mtu, created_at, updated_at
		FROM networks
		WHERE project_id = $1 OR shared = true
		ORDER BY created_at DESC
	`, projectID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var networks []gin.H
	for rows.Next() {
		var id, name, status string
		var adminStateUp, shared bool
		var mtu int
		var createdAt, updatedAt time.Time

		if err := rows.Scan(&id, &name, &adminStateUp, &status, &shared, &mtu, &createdAt, &updatedAt); err != nil {
			continue
		}

		networks = append(networks, gin.H{
			"id":             id,
			"name":           name,
			"tenant_id":      projectID,
			"admin_state_up": adminStateUp,
			"status":         status,
			"shared":         shared,
			"mtu":            mtu,
			"created_at":     createdAt.Format(time.RFC3339),
			"updated_at":     updatedAt.Format(time.RFC3339),
		})
	}

	if networks == nil {
		networks = []gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{"networks": networks})
}

// GetNetwork returns a single network
func (svc *Service) GetNetwork(c *gin.Context) {
	networkID := c.Param("id")
	projectID := c.GetString("project_id")

	var id, name, status string
	var adminStateUp, shared bool
	var mtu int
	var createdAt, updatedAt time.Time

	err := database.DB.QueryRow(c.Request.Context(), `
		SELECT id, name, admin_state_up, status, shared, mtu, created_at, updated_at
		FROM networks
		WHERE (id::text = $1 OR name = $1) AND (project_id = $2 OR shared = true)
		LIMIT 1
	`, networkID, projectID).Scan(&id, &name, &adminStateUp, &status, &shared, &mtu, &createdAt, &updatedAt)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "network not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"network": gin.H{
			"id":             id,
			"name":           name,
			"tenant_id":      projectID,
			"admin_state_up": adminStateUp,
			"status":         status,
			"shared":         shared,
			"mtu":            mtu,
			"created_at":     createdAt.Format(time.RFC3339),
			"updated_at":     updatedAt.Format(time.RFC3339),
		},
	})
}

// DeleteNetwork deletes a network
func (svc *Service) DeleteNetwork(c *gin.Context) {
	networkID := c.Param("id")
	projectID := c.GetString("project_id")

	bridgeName := "br-" + networkID[:8]
	nsName := svc.nsManager.GetNamespaceName(projectID)

	// Delete bridge
	svc.brManager.DeleteBridge(bridgeName, true, nsName)

	// Delete from database
	_, err := database.DB.Exec(c.Request.Context(),
		"DELETE FROM networks WHERE id = $1 AND project_id = $2",
		networkID, projectID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

// UpdateNetwork updates a network
func (svc *Service) UpdateNetwork(c *gin.Context) {
	networkID := c.Param("id")
	projectID := c.GetString("project_id")

	var req struct {
		Network struct {
			Name         *string `json:"name"`
			AdminStateUp *bool   `json:"admin_state_up"`
		} `json:"network"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	updates := []string{}
	args := []interface{}{}
	argID := 1

	if req.Network.Name != nil {
		updates = append(updates, fmt.Sprintf("name = $%d", argID))
		args = append(args, *req.Network.Name)
		argID++
	}

	if req.Network.AdminStateUp != nil {
		updates = append(updates, fmt.Sprintf("admin_state_up = $%d", argID))
		args = append(args, *req.Network.AdminStateUp)
		argID++
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no updates provided"})
		return
	}

	updates = append(updates, fmt.Sprintf("updated_at = $%d", argID))
	args = append(args, time.Now())
	argID++

	args = append(args, networkID, projectID)

	query := fmt.Sprintf("UPDATE networks SET %s WHERE id = $%d AND project_id = $%d",
		updateString(updates), argID, argID+1)

	_, err := database.DB.Exec(c.Request.Context(), query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Return updated network
	svc.GetNetwork(c)
}

// CreateSubnetRequest represents a subnet creation request
type CreateSubnetRequest struct {
	Subnet struct {
		Name           string   `json:"name" binding:"required"`
		NetworkID      string   `json:"network_id" binding:"required"`
		CIDR           string   `json:"cidr" binding:"required"`
		GatewayIP      string   `json:"gateway_ip"`
		IPVersion      int      `json:"ip_version"`
		EnableDHCP     *bool    `json:"enable_dhcp"`
		DNSNameservers []string `json:"dns_nameservers"`
	} `json:"subnet"`
}

// CreateSubnet creates a new subnet
func (svc *Service) CreateSubnet(c *gin.Context) {
	var req CreateSubnetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	projectID := c.GetString("project_id")
	subnetID := uuid.New().String()

	ipVersion := 4
	if req.Subnet.IPVersion > 0 {
		ipVersion = req.Subnet.IPVersion
	}

	enableDHCP := true
	if req.Subnet.EnableDHCP != nil {
		enableDHCP = *req.Subnet.EnableDHCP
	}

	// Parse CIDR to calculate gateway if not provided
	_, ipNet, err := net.ParseCIDR(req.Subnet.CIDR)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid CIDR"})
		return
	}

	gatewayIP := req.Subnet.GatewayIP
	if gatewayIP == "" {
		// Use first IP as gateway
		gatewayIP = incrementIP(ipNet.IP, 1).String()
	}

	// Insert into database
	now := time.Now()
	_, err = database.DB.Exec(c.Request.Context(), `
		INSERT INTO subnets (id, name, network_id, project_id, cidr, gateway_ip, ip_version, enable_dhcp, dns_nameservers, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, subnetID, req.Subnet.Name, req.Subnet.NetworkID, projectID, req.Subnet.CIDR, gatewayIP, ipVersion, enableDHCP, req.Subnet.DNSNameservers, now, now)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Start DHCP server if enabled
	if enableDHCP {
		bridgeName := "br-" + req.Subnet.NetworkID[:8]
		nsName := svc.nsManager.GetNamespaceName(projectID)

		dhcpConfig := networking.DHCPConfig{
			NetworkID:      req.Subnet.NetworkID,
			BridgeName:     bridgeName,
			SubnetCIDR:     req.Subnet.CIDR,
			GatewayIP:      gatewayIP,
			DNSServers:     req.Subnet.DNSNameservers,
			DHCPRangeStart: incrementIP(ipNet.IP, 10).String(),
			DHCPRangeEnd:   incrementIP(ipNet.IP, 250).String(),
			LeaseTime:      "24h",
		}

		go svc.dhcpManager.StartDHCP(dhcpConfig, nsName)
	}

	c.JSON(http.StatusCreated, gin.H{
		"subnet": gin.H{
			"id":              subnetID,
			"name":            req.Subnet.Name,
			"network_id":      req.Subnet.NetworkID,
			"tenant_id":       projectID,
			"cidr":            req.Subnet.CIDR,
			"gateway_ip":      gatewayIP,
			"ip_version":      ipVersion,
			"enable_dhcp":     enableDHCP,
			"dns_nameservers": req.Subnet.DNSNameservers,
			"created_at":      now.Format(time.RFC3339),
			"updated_at":      now.Format(time.RFC3339),
		},
	})
}

// ListSubnets lists all subnets
func (svc *Service) ListSubnets(c *gin.Context) {
	projectID := c.GetString("project_id")

	rows, err := database.DB.Query(c.Request.Context(), `
		SELECT s.id, s.name, s.network_id, s.cidr, s.gateway_ip, s.ip_version, s.enable_dhcp, s.dns_nameservers, s.created_at, s.updated_at
		FROM subnets s
		JOIN networks n ON s.network_id = n.id
		WHERE s.project_id = $1 OR n.shared = true
		ORDER BY s.created_at DESC
	`, projectID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var subnets []gin.H
	for rows.Next() {
		var id, name, networkID, cidr, gatewayIP string
		var ipVersion int
		var enableDHCP bool
		var dnsNameservers []string
		var createdAt, updatedAt time.Time

		if err := rows.Scan(&id, &name, &networkID, &cidr, &gatewayIP, &ipVersion, &enableDHCP, &dnsNameservers, &createdAt, &updatedAt); err != nil {
			continue
		}

		subnets = append(subnets, gin.H{
			"id":              id,
			"name":            name,
			"network_id":      networkID,
			"tenant_id":       projectID,
			"cidr":            cidr,
			"gateway_ip":      gatewayIP,
			"ip_version":      ipVersion,
			"enable_dhcp":     enableDHCP,
			"dns_nameservers": dnsNameservers,
			"created_at":      createdAt.Format(time.RFC3339),
			"updated_at":      updatedAt.Format(time.RFC3339),
		})
	}

	if subnets == nil {
		subnets = []gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{"subnets": subnets})
}

// GetSubnet returns a single subnet
func (svc *Service) GetSubnet(c *gin.Context) {
	subnetID := c.Param("id")
	projectID := c.GetString("project_id")

	var id, name, networkID, cidr, gatewayIP string
	var ipVersion int
	var enableDHCP bool
	var dnsNameservers []string
	var createdAt, updatedAt time.Time

	err := database.DB.QueryRow(c.Request.Context(), `
		SELECT s.id, s.name, s.network_id, s.cidr, s.gateway_ip, s.ip_version, s.enable_dhcp, s.dns_nameservers, s.created_at, s.updated_at
		FROM subnets s
		JOIN networks n ON s.network_id = n.id
		WHERE s.id = $1 AND (s.project_id = $2 OR n.shared = true)
	`, subnetID, projectID).Scan(&id, &name, &networkID, &cidr, &gatewayIP, &ipVersion, &enableDHCP, &dnsNameservers, &createdAt, &updatedAt)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "subnet not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"subnet": gin.H{
			"id":              id,
			"name":            name,
			"network_id":      networkID,
			"tenant_id":       projectID,
			"cidr":            cidr,
			"gateway_ip":      gatewayIP,
			"ip_version":      ipVersion,
			"enable_dhcp":     enableDHCP,
			"dns_nameservers": dnsNameservers,
			"created_at":      createdAt.Format(time.RFC3339),
			"updated_at":      updatedAt.Format(time.RFC3339),
		},
	})
}

// DeleteSubnet deletes a subnet
func (svc *Service) DeleteSubnet(c *gin.Context) {
	subnetID := c.Param("id")
	projectID := c.GetString("project_id")

	// Get network ID to stop DHCP
	var networkID string
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT network_id FROM subnets WHERE id = $1 AND project_id = $2",
		subnetID, projectID,
	).Scan(&networkID)

	if err == nil {
		// Stop DHCP server
		svc.dhcpManager.StopDHCP(networkID)
	}

	// Delete from database
	_, err = database.DB.Exec(c.Request.Context(),
		"DELETE FROM subnets WHERE id = $1 AND project_id = $2",
		subnetID, projectID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

// Helper functions

func incrementIP(ip net.IP, inc uint) net.IP {
	result := make(net.IP, len(ip))
	copy(result, ip)
	for i := len(result) - 1; i >= 0 && inc > 0; i-- {
		sum := uint(result[i]) + inc
		result[i] = byte(sum)
		inc = sum / 256
	}
	return result
}

func updateString(updates []string) string {
	result := ""
	for i, update := range updates {
		if i > 0 {
			result += ", "
		}
		result += update
	}
	return result
}

func generateMAC() string {
	buf := make([]byte, 6)
	rand.Read(buf)
	buf[0] = (buf[0] | 2) & 0xfe // Set local bit, clear multicast bit
	return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x", buf[0], buf[1], buf[2], buf[3], buf[4], buf[5])
}
