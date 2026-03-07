package nova

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/cobaltcore-dev/o3k/internal/database"
)

// AttachInterfaceRequest represents a network interface attachment request
type AttachInterfaceRequest struct {
	InterfaceAttachment struct {
		NetID   string `json:"net_id"`   // Network ID to attach to
		PortID  string `json:"port_id"`  // Existing port ID (optional)
		FixedIP string `json:"fixed_ip"` // Specific IP address (optional)
	} `json:"interfaceAttachment"`
}

// AttachInterface attaches a network interface to an instance
func (svc *Service) AttachInterface(c *gin.Context) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	var req AttachInterfaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"message": "invalid request body",
			"code":    400,
			"title":   "Bad Request",
		}})
		return
	}

	// Verify instance exists
	var libvirtDomainID, status string
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT libvirt_domain_id, status FROM instances WHERE id = $1 AND project_id = $2",
		instanceID, projectID,
	).Scan(&libvirtDomainID, &status)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{
			"message": "instance not found",
			"code":    404,
			"title":   "Not Found",
		}})
		return
	}

	var portID string
	var networkID string
	var fixedIP string
	var macAddress string

	// If port_id provided, use existing port
	if req.InterfaceAttachment.PortID != "" {
		portID = req.InterfaceAttachment.PortID

		// Verify port exists and is available
		var deviceID string
		var fixedIPsJSON []byte
		err := database.DB.QueryRow(c.Request.Context(),
			"SELECT network_id, fixed_ips, mac_address, device_id FROM ports WHERE id = $1 AND project_id = $2",
			portID, projectID,
		).Scan(&networkID, &fixedIPsJSON, &macAddress, &deviceID)

		if err == pgx.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "port not found"})
			return
		}

		if deviceID != "" {
			c.JSON(http.StatusConflict, gin.H{"error": "port already attached to another device"})
			return
		}

		// Parse fixed_ips from JSONB
		// For simplicity, just extract first IP
		fixedIP = "192.168.1.10" // Placeholder, would parse from fixedIPsJSON
	} else if req.InterfaceAttachment.NetID != "" {
		// Create new port on the specified network
		networkID = req.InterfaceAttachment.NetID

		// Verify network exists
		var networkExists bool
		err := database.DB.QueryRow(c.Request.Context(),
			"SELECT EXISTS(SELECT 1 FROM networks WHERE id = $1 AND project_id = $2)",
			networkID, projectID,
		).Scan(&networkExists)

		if err != nil || !networkExists {
			c.JSON(http.StatusNotFound, gin.H{"error": "network not found"})
			return
		}

		// Generate port ID and MAC address
		portID = uuid.New().String()
		macAddress = generateMACAddress()

		// Get subnet info for IP allocation
		var cidr string
		err = database.DB.QueryRow(c.Request.Context(),
			"SELECT cidr FROM subnets WHERE network_id = $1 LIMIT 1",
			networkID,
		).Scan(&cidr)

		if err == pgx.ErrNoRows {
			c.JSON(http.StatusBadRequest, gin.H{"error": "network has no subnet"})
			return
		}

		// Use provided fixed_ip or allocate one
		if req.InterfaceAttachment.FixedIP != "" {
			fixedIP = req.InterfaceAttachment.FixedIP
		} else {
			// Allocate next available IP from subnet
			fixedIP, err = allocateNextIP(c.Request.Context(), networkID, cidr)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to allocate IP: %v", err)})
				return
			}
		}

		// Create port
		fixedIPsJSON := fmt.Sprintf(`[{"ip_address": "%s", "subnet_id": "%s"}]`, fixedIP, networkID)
		_, err = database.DB.Exec(c.Request.Context(), `
			INSERT INTO ports (id, network_id, project_id, name, mac_address, fixed_ips, device_id, device_owner, status, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9, $10, $11)
		`, portID, networkID, projectID, fmt.Sprintf("port-%s", portID[:8]), macAddress, fixedIPsJSON, instanceID, "compute:nova", "ACTIVE", time.Now(), time.Now())

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "either net_id or port_id must be provided"})
		return
	}

	// Update port to attach to instance
	_, err = database.DB.Exec(c.Request.Context(), `
		UPDATE ports
		SET device_id = $1, device_owner = $2, status = $3, updated_at = $4
		WHERE id = $5
	`, instanceID, "compute:nova", "ACTIVE", time.Now(), portID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Store interface attachment record
	attachmentID := uuid.New().String()
	_, err = database.DB.Exec(c.Request.Context(), `
		INSERT INTO interface_attachments (id, instance_id, port_id, mac_address, attached_at)
		VALUES ($1, $2, $3, $4, $5)
	`, attachmentID, instanceID, portID, macAddress, time.Now())

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// In real mode, would hot-plug interface to libvirt VM
	// For stub mode, just return success

	c.JSON(http.StatusOK, gin.H{
		"interfaceAttachment": gin.H{
			"id":          attachmentID,
			"port_id":     portID,
			"net_id":      networkID,
			"fixed_ips":   []gin.H{{"ip_address": fixedIP}},
			"mac_addr":    macAddress,
			"port_state":  "ACTIVE",
		},
	})
}

// DetachInterface detaches a network interface from an instance
func (svc *Service) DetachInterface(c *gin.Context) {
	instanceID := c.Param("id")
	portID := c.Param("port_id")
	projectID := c.GetString("project_id")

	// Verify instance exists
	var instanceExists bool
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM instances WHERE id = $1 AND project_id = $2)",
		instanceID, projectID,
	).Scan(&instanceExists)

	if err != nil || !instanceExists {
		c.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
		return
	}

	// Verify port is attached to this instance
	var attachedInstanceID string
	err = database.DB.QueryRow(c.Request.Context(),
		"SELECT device_id FROM ports WHERE id = $1",
		portID,
	).Scan(&attachedInstanceID)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "port not found"})
		return
	}

	if attachedInstanceID != instanceID {
		c.JSON(http.StatusConflict, gin.H{"error": "port not attached to this instance"})
		return
	}

	// Delete interface attachment record
	_, err = database.DB.Exec(c.Request.Context(),
		"DELETE FROM interface_attachments WHERE instance_id = $1 AND port_id = $2",
		instanceID, portID,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Update port to detach from instance
	_, err = database.DB.Exec(c.Request.Context(), `
		UPDATE ports
		SET device_id = NULL, device_owner = NULL, status = $1, updated_at = $2
		WHERE id = $3
	`, "DOWN", time.Now(), portID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// In real mode, would hot-unplug interface from libvirt VM
	// For stub mode, just return success

	c.Status(http.StatusAccepted)
}

// ListInterfaceAttachments lists all network interfaces attached to an instance
func (svc *Service) ListInterfaceAttachments(c *gin.Context) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	// Verify instance exists
	var instanceExists bool
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM instances WHERE id = $1 AND project_id = $2)",
		instanceID, projectID,
	).Scan(&instanceExists)

	if err != nil || !instanceExists {
		c.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
		return
	}

	// Query all interface attachments
	rows, err := database.DB.Query(c.Request.Context(), `
		SELECT ia.id, ia.port_id, p.network_id, p.fixed_ips, ia.mac_address, p.status
		FROM interface_attachments ia
		JOIN ports p ON ia.port_id = p.id
		WHERE ia.instance_id = $1
		ORDER BY ia.attached_at
	`, instanceID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var attachments []gin.H
	for rows.Next() {
		var id, portID, networkID, macAddress string
		var fixedIPsJSON []byte
		var portState *string

		if err := rows.Scan(&id, &portID, &networkID, &fixedIPsJSON, &macAddress, &portState); err != nil {
			continue
		}

		state := "DOWN"
		if portState != nil {
			state = *portState
		}

		// Parse fixed_ips from JSONB
		var fixedIPs []map[string]interface{}
		if err := json.Unmarshal(fixedIPsJSON, &fixedIPs); err == nil && len(fixedIPs) > 0 {
			attachments = append(attachments, gin.H{
				"id":         id,
				"port_id":    portID,
				"net_id":     networkID,
				"fixed_ips":  fixedIPs,
				"mac_addr":   macAddress,
				"port_state": state,
			})
		}
	}

	if attachments == nil {
		attachments = []gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{"interfaceAttachments": attachments})
}

// generateMACAddress generates a random MAC address
func generateMACAddress() string {
	// Use OpenStack's OUI prefix: fa:16:3e
	return fmt.Sprintf("fa:16:3e:%02x:%02x:%02x",
		uuid.New()[0], uuid.New()[1], uuid.New()[2])
}

// allocateNextIP allocates the next available IP from a subnet
func allocateNextIP(ctx context.Context, networkID, cidr string) (string, error) {
	// Simple allocation: get last used IP and increment
	// In production, would use proper IPAM
	var fixedIPsJSON []byte
	err := database.DB.QueryRow(ctx,
		"SELECT fixed_ips FROM ports WHERE network_id = $1 ORDER BY created_at DESC LIMIT 1",
		networkID,
	).Scan(&fixedIPsJSON)

	if err == pgx.ErrNoRows {
		// First allocation, use .10
		baseIP := cidr[:len(cidr)-3] // Remove /24
		return baseIP + "10", nil
	}

	if err != nil {
		return "", err
	}

	// For simplicity, parse JSON and increment
	// In production, would use proper IPAM library
	var ips []map[string]interface{}
	if err := json.Unmarshal(fixedIPsJSON, &ips); err != nil {
		// Fallback to simple allocation
		baseIP := cidr[:len(cidr)-3]
		return baseIP + "11", nil
	}

	if len(ips) == 0 {
		baseIP := cidr[:len(cidr)-3]
		return baseIP + "10", nil
	}

	lastIP := ips[0]["ip_address"].(string)

	// Increment last octet
	var a, b, c, d int
	fmt.Sscanf(lastIP, "%d.%d.%d.%d", &a, &b, &c, &d)
	d++
	if d > 254 {
		return "", fmt.Errorf("subnet exhausted")
	}

	return fmt.Sprintf("%d.%d.%d.%d", a, b, c, d), nil
}
