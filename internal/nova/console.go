package nova

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/common"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
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
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Verify instance exists
	var libvirtDomainID string
	var vncPort int
	var vncPassword string
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT libvirt_domain_id, console_vnc_port, console_vnc_password FROM instances WHERE id = $1 AND project_id = $2",
		instanceID, projectID,
	).Scan(&libvirtDomainID, &vncPort, &vncPassword)

	if err == pgx.ErrNoRows {
		common.SendError(c, common.NewNotFoundError("instance"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "get_console").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to get instance"))
		return
	}

	// If VNC not configured yet, set it up
	if vncPort == 0 || vncPassword == "" {
		vncPort, vncPassword, err = svc.setupVNCConsole(c.Request.Context(), instanceID, libvirtDomainID)
		if err != nil {
			log.Error().Err(err).Str("operation", "setup_vnc_console").Msg("console setup error")
			common.SendError(c, common.NewInternalServerError("failed to setup console"))
			return
		}
	}

	// Build console URL based on protocol/type
	consoleHost := svc.NoVNCProxyHost
	if consoleHost == "" {
		consoleHost = strings.Split(c.Request.Host, ":")[0]
	}
	var consoleURL string
	switch req.RemoteConsole.Type {
	case "novnc":
		// noVNC is a web-based VNC client
		// Format: http://nova-novncproxy:6080/vnc_auto.html?token=<token>
		token := generateConsoleToken(instanceID)
		consoleURL = fmt.Sprintf("http://%s:6080/vnc_auto.html?token=%s", consoleHost, token)
	case "xvpvnc":
		// XVP VNC (legacy)
		consoleURL = fmt.Sprintf("http://%s:6081/console?token=%s", consoleHost, generateConsoleToken(instanceID))
	case "serial":
		// Serial console
		consoleURL = fmt.Sprintf("ws://%s:6083/?token=%s", consoleHost, generateConsoleToken(instanceID))
	default:
		common.SendError(c, common.NewBadRequestError(fmt.Sprintf("unsupported console type: %s", req.RemoteConsole.Type)))
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
		common.SendError(c, common.NewBadRequestError("invalid request body"))
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
		common.SendError(c, common.NewBadRequestError("invalid console request"))
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
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT libvirt_domain_id, console_vnc_port, console_vnc_password FROM instances WHERE id = $1 AND project_id = $2",
		instanceID, projectID,
	).Scan(&libvirtDomainID, &vncPort, &vncPassword)

	if err == pgx.ErrNoRows {
		common.SendError(c, common.NewNotFoundError("instance"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "get_vnc_console").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to get instance"))
		return
	}

	// If VNC not configured yet, set it up
	if vncPort == 0 || vncPassword == "" {
		vncPort, vncPassword, err = svc.setupVNCConsole(c.Request.Context(), instanceID, libvirtDomainID)
		if err != nil {
			log.Error().Err(err).Str("operation", "setup_vnc_console_action").Msg("console setup error")
			common.SendError(c, common.NewInternalServerError("failed to setup console"))
			return
		}
	}

	// Generate console URL
	consoleHost := svc.NoVNCProxyHost
	if consoleHost == "" {
		consoleHost = strings.Split(c.Request.Host, ":")[0]
	}
	token := generateConsoleToken(instanceID)
	consoleURL := fmt.Sprintf("http://%s:6080/vnc_auto.html?token=%s", consoleHost, token)

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
	_, err := svc.activeDB().Exec(ctx, `
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

// GetConsoleOutputAction handles console output requests (os-getConsoleOutput action)
func (svc *Service) GetConsoleOutputAction(c *gin.Context, consoleOutput interface{}) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	// Verify instance exists
	var id string
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT id FROM instances WHERE id = $1 AND project_id = $2",
		instanceID, projectID,
	).Scan(&id)

	if err == pgx.ErrNoRows {
		common.SendError(c, common.NewNotFoundError("instance"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "get_console_output").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to get instance"))
		return
	}

	// Parse length parameter
	length := 0
	if consoleMap, ok := consoleOutput.(map[string]interface{}); ok {
		if lengthVal, ok := consoleMap["length"].(float64); ok {
			length = int(lengthVal)
		}
	}

	// In stub mode or without actual libvirt connection, return fake console output
	output := fmt.Sprintf("Console output for instance %s\n", instanceID)
	output += "Booting from Hard Disk...\n"
	output += "Cloud-init v. 22.1-14 running 'init-local' at Wed, 11 Mar 2026 10:00:00 +0000\n"
	output += "Cloud-init v. 22.1-14 running 'init' at Wed, 11 Mar 2026 10:00:05 +0000\n"
	output += "System is ready.\n"

	// Trim to requested length if specified
	if length > 0 && len(output) > length {
		output = output[len(output)-length:]
	}

	c.JSON(http.StatusOK, gin.H{
		"output": output,
	})
}

// GetSerialConsoleAction handles serial console requests (os-getSerialConsole action)
func (svc *Service) GetSerialConsoleAction(c *gin.Context, serialConsole interface{}) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	// Verify instance exists
	var id string
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT id FROM instances WHERE id = $1 AND project_id = $2",
		instanceID, projectID,
	).Scan(&id)

	if err == pgx.ErrNoRows {
		common.SendError(c, common.NewNotFoundError("instance"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "get_serial_console").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to get instance"))
		return
	}

	// Parse console type
	consoleType := "serial"
	if consoleMap, ok := serialConsole.(map[string]interface{}); ok {
		if typeVal, ok := consoleMap["type"].(string); ok {
			consoleType = typeVal
		}
	}

	// Generate serial console URL
	consoleHost := svc.NoVNCProxyHost
	if consoleHost == "" {
		consoleHost = strings.Split(c.Request.Host, ":")[0]
	}
	token := generateConsoleToken(instanceID)
	consoleURL := fmt.Sprintf("ws://%s:6083/?token=%s", consoleHost, token)

	c.JSON(http.StatusOK, gin.H{
		"console": gin.H{
			"type": consoleType,
			"url":  consoleURL,
		},
	})
}

// GetSPICEConsoleAction handles SPICE console requests (os-getSPICEConsole action)
func (svc *Service) GetSPICEConsoleAction(c *gin.Context, spiceConsole interface{}) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	// Verify instance exists
	var id string
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT id FROM instances WHERE id = $1 AND project_id = $2",
		instanceID, projectID,
	).Scan(&id)

	if err == pgx.ErrNoRows {
		common.SendError(c, common.NewNotFoundError("instance"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "get_spice_console").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to get instance"))
		return
	}

	// Parse console type
	consoleType := "spice-html5"
	if consoleMap, ok := spiceConsole.(map[string]interface{}); ok {
		if typeVal, ok := consoleMap["type"].(string); ok {
			consoleType = typeVal
		}
	}

	// Generate SPICE console URL
	consoleHost := svc.NoVNCProxyHost
	if consoleHost == "" {
		consoleHost = strings.Split(c.Request.Host, ":")[0]
	}
	token := generateConsoleToken(instanceID)
	consoleURL := fmt.Sprintf("http://%s:6082/spice_auto.html?token=%s", consoleHost, token)

	c.JSON(http.StatusOK, gin.H{
		"console": gin.H{
			"type": consoleType,
			"url":  consoleURL,
		},
	})
}

// GetRDPConsoleAction handles RDP console requests (os-getRDPConsole action)
func (svc *Service) GetRDPConsoleAction(c *gin.Context, rdpConsole interface{}) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	// Verify instance exists
	var id string
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT id FROM instances WHERE id = $1 AND project_id = $2",
		instanceID, projectID,
	).Scan(&id)

	if err == pgx.ErrNoRows {
		common.SendError(c, common.NewNotFoundError("instance"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "get_rdp_console").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to get instance"))
		return
	}

	// Parse console type
	consoleType := "rdp-html5"
	if consoleMap, ok := rdpConsole.(map[string]interface{}); ok {
		if typeVal, ok := consoleMap["type"].(string); ok {
			consoleType = typeVal
		}
	}

	// Generate RDP console URL
	consoleHost := svc.NoVNCProxyHost
	if consoleHost == "" {
		consoleHost = strings.Split(c.Request.Host, ":")[0]
	}
	token := generateConsoleToken(instanceID)
	consoleURL := fmt.Sprintf("http://%s:6084/rdp.html?token=%s", consoleHost, token)

	c.JSON(http.StatusOK, gin.H{
		"console": gin.H{
			"type": consoleType,
			"url":  consoleURL,
		},
	})
}
