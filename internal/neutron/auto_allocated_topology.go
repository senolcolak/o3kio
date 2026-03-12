package neutron

import (
	"net/http"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// GetAutoAllocatedTopology returns the auto-allocated network topology for a project
func (svc *Service) GetAutoAllocatedTopology(c *gin.Context) {
	projectIDParam := c.Param("project")
	projectID := c.GetString("project_id")

	// In multi-project setup, would validate projectIDParam matches auth
	_ = projectIDParam

	// Check if auto-allocated network exists for this project
	var networkID, networkName string
	err := database.DB.QueryRow(c.Request.Context(), `
		SELECT id, name
		FROM networks
		WHERE project_id = $1 AND name = 'auto-allocated-network'
		LIMIT 1
	`, projectID).Scan(&networkID, &networkName)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{
			"NeutronError": gin.H{
				"type":    "AutoAllocationNotAvailable",
				"message": "auto-allocated topology not available",
				"detail":  "",
			},
		})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"auto_allocated_topology": gin.H{
			"id": networkID,
		},
	})
}

// CreateAutoAllocatedTopology creates an auto-allocated network topology for a project
func (svc *Service) CreateAutoAllocatedTopology(c *gin.Context) {
	projectIDParam := c.Param("project")
	projectID := c.GetString("project_id")

	_ = projectIDParam

	// Check if auto-allocated network already exists
	var existingNetworkID string
	err := database.DB.QueryRow(c.Request.Context(), `
		SELECT id
		FROM networks
		WHERE project_id = $1 AND name = 'auto-allocated-network'
		LIMIT 1
	`, projectID).Scan(&existingNetworkID)

	if err == nil {
		// Already exists, return it
		c.JSON(http.StatusOK, gin.H{
			"auto_allocated_topology": gin.H{
				"id": existingNetworkID,
			},
		})
		return
	}

	// Create auto-allocated network
	networkID := uuid.New().String()
	networkName := "auto-allocated-network"
	now := time.Now()

	_, err = database.DB.Exec(c.Request.Context(), `
		INSERT INTO networks (id, name, project_id, admin_state_up, status, shared, network_type, mtu, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, networkID, networkName, projectID, true, "ACTIVE", false, "flat", 1500, now, now)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Create auto-allocated subnet
	subnetID := uuid.New().String()
	subnetCIDR := "192.168.100.0/24"
	gatewayIP := "192.168.100.1"

	_, err = database.DB.Exec(c.Request.Context(), `
		INSERT INTO subnets (id, name, network_id, project_id, cidr, gateway_ip, ip_version, enable_dhcp, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, subnetID, "auto-allocated-subnet", networkID, projectID, subnetCIDR, gatewayIP, 4, true, now, now)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Create namespace and bridge if in real mode
	if svc.mode != "stub" {
		if err := svc.nsManager.CreateNamespace(projectID); err == nil {
			bridgeName := "br-" + networkID[:8]
			nsName := svc.nsManager.GetNamespaceName(projectID)
			svc.brManager.CreateBridge(bridgeName, true, nsName)
		}
	}

	c.JSON(http.StatusCreated, gin.H{
		"auto_allocated_topology": gin.H{
			"id": networkID,
		},
	})
}

// DeleteAutoAllocatedTopology deletes the auto-allocated network topology for a project
func (svc *Service) DeleteAutoAllocatedTopology(c *gin.Context) {
	projectIDParam := c.Param("project")
	projectID := c.GetString("project_id")

	_ = projectIDParam

	// Find auto-allocated network
	var networkID string
	err := database.DB.QueryRow(c.Request.Context(), `
		SELECT id
		FROM networks
		WHERE project_id = $1 AND name = 'auto-allocated-network'
		LIMIT 1
	`, projectID).Scan(&networkID)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "auto-allocated topology not found"})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Delete subnets first (cascade will handle ports)
	_, err = database.DB.Exec(c.Request.Context(), `
		DELETE FROM subnets WHERE network_id = $1
	`, networkID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Delete network
	_, err = database.DB.Exec(c.Request.Context(), `
		DELETE FROM networks WHERE id = $1
	`, networkID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Clean up bridge if in real mode
	if svc.mode != "stub" {
		bridgeName := "br-" + networkID[:8]
		nsName := svc.nsManager.GetNamespaceName(projectID)
		svc.brManager.DeleteBridge(bridgeName, true, nsName)
	}

	c.Status(http.StatusNoContent)
}
