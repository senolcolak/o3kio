package nova

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/cobaltcore-dev/o3k/internal/database"
)

// GetConsoleRequest represents a console access request
type GetConsoleRequest struct {
	RemoteConsole struct {
		Protocol string `json:"protocol" binding:"required"` // "vnc" or "serial"
		Type     string `json:"type" binding:"required"`     // "novnc", "xvpvnc", "serial"
	} `json:"remote_console"`
}

// GetRemoteConsole returns console connection details
func (svc *Service) GetRemoteConsole(c *gin.Context) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	var req GetConsoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"message": "invalid request body",
			"code":    400,
			"title":   "Bad Request",
		}})
		return
	}

	// Verify instance exists
	var libvirtDomainID string
	var vncPort int
	var vncPassword string
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT libvirt_domain_id, console_vnc_port, console_vnc_password FROM instances WHERE id = $1 AND project_id = $2",
		instanceID, projectID,
	).Scan(&libvirtDomainID, &vncPort, &vncPassword)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{
			"message": "instance not found",
			"code":    404,
			"title":   "Not Found",
		}})
		return
	}

	// If VNC not configured yet, set it up
	if vncPort == 0 || vncPassword == "" {
		vncPort, vncPassword, err = svc.setupVNCConsole(c.Request.Context(), instanceID, libvirtDomainID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to setup console: %v", err)})
			return
		}
	}

	// Build console URL based on protocol/type
	var consoleURL string
	switch req.RemoteConsole.Type {
	case "novnc":
		// noVNC is a web-based VNC client
		// Format: http://nova-novncproxy:6080/vnc_auto.html?token=<token>
		token := generateConsoleToken(instanceID)
		consoleURL = fmt.Sprintf("http://localhost:6080/vnc_auto.html?token=%s", token)
	case "xvpvnc":
		// XVP VNC (legacy)
		consoleURL = fmt.Sprintf("http://localhost:6081/console?token=%s", generateConsoleToken(instanceID))
	case "serial":
		// Serial console
		consoleURL = fmt.Sprintf("ws://localhost:6083/?token=%s", generateConsoleToken(instanceID))
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"message": fmt.Sprintf("unsupported console type: %s", req.RemoteConsole.Type),
			"code":    400,
			"title":   "Bad Request",
		}})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"remote_console": gin.H{
			"protocol": req.RemoteConsole.Protocol,
			"type":     req.RemoteConsole.Type,
			"url":      consoleURL,
		},
	})
}

// GetVNCConsole returns VNC console details (legacy API)
func (svc *Service) GetVNCConsole(c *gin.Context) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	var req struct {
		GetVNCConsole struct {
			Type string `json:"type"` // "novnc" or "xvpvnc"
		} `json:"os-getVNCConsole"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	svc.getVNCConsoleResponse(c, instanceID, projectID, req.GetVNCConsole.Type)
}

// GetVNCConsoleAction handles VNC console requests from the action API
func (svc *Service) GetVNCConsoleAction(c *gin.Context, vncConsole interface{}) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	// Parse the already-parsed action body
	consoleMap, ok := vncConsole.(map[string]interface{})
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid console request"})
		return
	}

	consoleType, ok := consoleMap["type"].(string)
	if !ok {
		consoleType = "novnc" // default
	}

	svc.getVNCConsoleResponse(c, instanceID, projectID, consoleType)
}

// getVNCConsoleResponse is the shared logic for returning console details
func (svc *Service) getVNCConsoleResponse(c *gin.Context, instanceID, projectID, consoleType string) {

	// Verify instance exists
	var libvirtDomainID string
	var vncPort int
	var vncPassword string
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT libvirt_domain_id, console_vnc_port, console_vnc_password FROM instances WHERE id = $1 AND project_id = $2",
		instanceID, projectID,
	).Scan(&libvirtDomainID, &vncPort, &vncPassword)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
		return
	}

	// If VNC not configured yet, set it up
	if vncPort == 0 || vncPassword == "" {
		vncPort, vncPassword, err = svc.setupVNCConsole(c.Request.Context(), instanceID, libvirtDomainID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to setup console: %v", err)})
			return
		}
	}

	// Generate console URL
	token := generateConsoleToken(instanceID)
	consoleURL := fmt.Sprintf("http://localhost:6080/vnc_auto.html?token=%s", token)

	c.JSON(http.StatusOK, gin.H{
		"console": gin.H{
			"type": consoleType,
			"url":  consoleURL,
		},
	})
}

// setupVNCConsole configures VNC access for an instance
func (svc *Service) setupVNCConsole(ctx context.Context, instanceID, libvirtDomainID string) (int, string, error) {
	// Generate random VNC password
	password := generateVNCPassword()

	// In real mode, we would query libvirt for the actual VNC port
	// For now, use a calculated port based on instance ID hash
	vncPort := 5900 + (hashString(instanceID) % 1000)

	// In stub mode or if libvirt not available, just store the values
	// In real mode, we could configure VNC on the libvirt domain

	// Update database
	_, err := database.DB.Exec(ctx, `
		UPDATE instances
		SET console_vnc_port = $1, console_vnc_password = $2, updated_at = $3
		WHERE id = $4
	`, vncPort, password, time.Now(), instanceID)

	if err != nil {
		return 0, "", err
	}

	return vncPort, password, nil
}

// generateConsoleToken creates a temporary token for console access
func generateConsoleToken(instanceID string) string {
	// In production, this would be stored in a cache with expiration
	// For now, just create a deterministic token based on instance ID
	return fmt.Sprintf("token-%s-%d", instanceID[:8], time.Now().Unix())
}

// generateVNCPassword creates a random VNC password
func generateVNCPassword() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// hashString creates a simple hash of a string for port calculation
func hashString(s string) int {
	hash := 0
	for _, c := range s {
		hash = (hash * 31) + int(c)
	}
	if hash < 0 {
		hash = -hash
	}
	return hash
}
