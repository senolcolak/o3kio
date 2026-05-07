package neutron

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/common"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

// CreatePortForwardingRequest represents a port forwarding creation request
type CreatePortForwardingRequest struct {
	PortForwarding struct {
		InternalPortID    string `json:"internal_port_id" binding:"required"`
		InternalIPAddress string `json:"internal_ip_address" binding:"required"`
		ExternalPort      int    `json:"external_port" binding:"required"`
		InternalPort      int    `json:"internal_port" binding:"required"`
		Protocol          string `json:"protocol" binding:"required"`
		Description       string `json:"description"`
	} `json:"port_forwarding"`
}

// UpdatePortForwardingRequest represents a port forwarding update request
type UpdatePortForwardingRequest struct {
	PortForwarding struct {
		InternalPortID    *string `json:"internal_port_id"`
		InternalIPAddress *string `json:"internal_ip_address"`
		InternalPort      *int    `json:"internal_port"`
		Description       *string `json:"description"`
	} `json:"port_forwarding"`
}

// PortForwarding represents a port forwarding rule
type PortForwarding struct {
	ID                string
	ProjectID         string
	FloatingIPID      string
	InternalPortID    string
	InternalIPAddress string
	ExternalPort      int
	InternalPort      int
	Protocol          string
	Status            string
	Description       sql.NullString
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// ListPortForwardings handles GET /v2.0/floatingips/:id/port_forwardings
func (svc *Service) ListPortForwardings(c *gin.Context) {
	floatingIPID := c.Param("id")
	projectID := c.GetString("project_id")

	// Verify floating IP exists and belongs to project
	var exists bool
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM floating_ips WHERE id = $1 AND project_id = $2)",
		floatingIPID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		common.SendError(c, common.NewNotFoundError("floating IP"))
		return
	}

	// List port forwardings
	rows, err := svc.activeDB().Query(c.Request.Context(), `
		SELECT id, project_id, floatingip_id, internal_port_id, internal_ip_address,
		       external_port, internal_port, protocol, status, description,
		       created_at, updated_at
		FROM port_forwardings
		WHERE floatingip_id = $1 AND project_id = $2
		ORDER BY external_port ASC
	`, floatingIPID, projectID)

	if err != nil {
		log.Error().Err(err).Str("operation", "list_port_forwardings").Msg("failed to query port forwardings")
		common.SendError(c, common.NewInternalServerError("failed to list port forwardings"))
		return
	}
	defer rows.Close()

	var forwardings []gin.H
	for rows.Next() {
		var pf PortForwarding
		if err := rows.Scan(&pf.ID, &pf.ProjectID, &pf.FloatingIPID, &pf.InternalPortID,
			&pf.InternalIPAddress, &pf.ExternalPort, &pf.InternalPort, &pf.Protocol,
			&pf.Status, &pf.Description, &pf.CreatedAt, &pf.UpdatedAt); err != nil {
			continue
		}

		result := gin.H{
			"id":                  pf.ID,
			"internal_port_id":    pf.InternalPortID,
			"internal_ip_address": pf.InternalIPAddress,
			"external_port":       pf.ExternalPort,
			"internal_port":       pf.InternalPort,
			"protocol":            pf.Protocol,
			"created_at":          pf.CreatedAt.Format(time.RFC3339),
			"updated_at":          pf.UpdatedAt.Format(time.RFC3339),
		}

		if pf.Description.Valid {
			result["description"] = pf.Description.String
		}

		forwardings = append(forwardings, result)
	}
	if err := rows.Err(); err != nil {
		log.Error().Err(err).Str("operation", "list_port_forwardings").Msg("rows iteration error")
		common.SendError(c, common.NewInternalServerError("failed to list port forwardings"))
		return
	}

	if forwardings == nil {
		forwardings = []gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{"port_forwardings": forwardings})
}

// CreatePortForwarding handles POST /v2.0/floatingips/:id/port_forwardings
func (svc *Service) CreatePortForwarding(c *gin.Context) {
	floatingIPID := c.Param("id")
	projectID := c.GetString("project_id")

	var req CreatePortForwardingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Validate protocol
	protocol := strings.ToLower(req.PortForwarding.Protocol)
	if protocol != "tcp" && protocol != "udp" {
		common.SendError(c, common.NewBadRequestError("protocol must be tcp or udp"))
		return
	}

	// Validate port ranges
	if req.PortForwarding.ExternalPort < 1 || req.PortForwarding.ExternalPort > 65535 {
		common.SendError(c, common.NewBadRequestError("external_port must be between 1 and 65535"))
		return
	}
	if req.PortForwarding.InternalPort < 1 || req.PortForwarding.InternalPort > 65535 {
		common.SendError(c, common.NewBadRequestError("internal_port must be between 1 and 65535"))
		return
	}

	// Fetch floating IP details (for NAT configuration)
	var floatingIP, routerID sql.NullString
	err := svc.activeDB().QueryRow(c.Request.Context(), `
		SELECT floating_ip_address, router_id
		FROM floating_ips
		WHERE id = $1 AND project_id = $2
	`, floatingIPID, projectID).Scan(&floatingIP, &routerID)

	if err == pgx.ErrNoRows {
		common.SendError(c, common.NewNotFoundError("floating IP"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "create_port_forwarding_fetch_fip").Msg("failed to fetch floating IP")
		common.SendError(c, common.NewInternalServerError("failed to create port forwarding"))
		return
	}

	if !routerID.Valid {
		common.SendError(c, common.NewBadRequestError("floating IP must be associated with a router"))
		return
	}

	// Check for duplicate (floatingip_id, external_port, protocol)
	var dupExists bool
	err = svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM port_forwardings WHERE floatingip_id = $1 AND external_port = $2 AND protocol = $3)",
		floatingIPID, req.PortForwarding.ExternalPort, protocol,
	).Scan(&dupExists)

	if err != nil {
		log.Error().Err(err).Str("operation", "create_port_forwarding_check_dup").Msg("failed to check for duplicate port forwarding")
		common.SendError(c, common.NewInternalServerError("failed to create port forwarding"))
		return
	}
	if dupExists {
		common.SendError(c, common.NewConflictError(fmt.Sprintf("port forwarding for %s:%d already exists", protocol, req.PortForwarding.ExternalPort)))
		return
	}

	// Create port forwarding in database
	pfID := uuid.New().String()
	now := time.Now()

	_, err = svc.activeDB().Exec(c.Request.Context(), `
		INSERT INTO port_forwardings (id, project_id, floatingip_id, internal_port_id,
			internal_ip_address, external_port, internal_port, protocol, status, description,
			created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, pfID, projectID, floatingIPID, req.PortForwarding.InternalPortID,
		req.PortForwarding.InternalIPAddress, req.PortForwarding.ExternalPort,
		req.PortForwarding.InternalPort, protocol, "ACTIVE",
		req.PortForwarding.Description, now, now)

	if err != nil {
		log.Error().Err(err).Str("operation", "create_port_forwarding").Msg("failed to create port forwarding")
		common.SendError(c, common.NewInternalServerError("failed to create port forwarding"))
		return
	}

	// Configure NAT rule via RouterManager
	externalInterface := "qg-ext-" + routerID.String[:7]
	if err := svc.routerManager.AddPortForwarding(
		routerID.String,
		floatingIP.String,
		req.PortForwarding.ExternalPort,
		req.PortForwarding.InternalIPAddress,
		req.PortForwarding.InternalPort,
		protocol,
		externalInterface,
	); err != nil {
		fmt.Printf("Warning: failed to configure port forwarding NAT rule: %v\n", err)
	}

	// Return created resource
	result := gin.H{
		"id":                  pfID,
		"internal_port_id":    req.PortForwarding.InternalPortID,
		"internal_ip_address": req.PortForwarding.InternalIPAddress,
		"external_port":       req.PortForwarding.ExternalPort,
		"internal_port":       req.PortForwarding.InternalPort,
		"protocol":            protocol,
		"created_at":          now.Format(time.RFC3339),
		"updated_at":          now.Format(time.RFC3339),
	}

	if req.PortForwarding.Description != "" {
		result["description"] = req.PortForwarding.Description
	}

	c.JSON(http.StatusCreated, gin.H{"port_forwarding": result})
}

// GetPortForwarding handles GET /v2.0/floatingips/:id/port_forwardings/:pf_id
func (svc *Service) GetPortForwarding(c *gin.Context) {
	floatingIPID := c.Param("id")
	pfID := c.Param("pf_id")
	projectID := c.GetString("project_id")

	var pf PortForwarding
	err := svc.activeDB().QueryRow(c.Request.Context(), `
		SELECT id, project_id, floatingip_id, internal_port_id, internal_ip_address,
		       external_port, internal_port, protocol, status, description,
		       created_at, updated_at
		FROM port_forwardings
		WHERE id = $1 AND floatingip_id = $2 AND project_id = $3
	`, pfID, floatingIPID, projectID).Scan(&pf.ID, &pf.ProjectID, &pf.FloatingIPID,
		&pf.InternalPortID, &pf.InternalIPAddress, &pf.ExternalPort, &pf.InternalPort,
		&pf.Protocol, &pf.Status, &pf.Description, &pf.CreatedAt, &pf.UpdatedAt)

	if err == pgx.ErrNoRows {
		common.SendError(c, common.NewNotFoundError("port forwarding"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "get_port_forwarding").Msg("failed to get port forwarding")
		common.SendError(c, common.NewInternalServerError("failed to get port forwarding"))
		return
	}

	result := gin.H{
		"id":                  pf.ID,
		"internal_port_id":    pf.InternalPortID,
		"internal_ip_address": pf.InternalIPAddress,
		"external_port":       pf.ExternalPort,
		"internal_port":       pf.InternalPort,
		"protocol":            pf.Protocol,
		"created_at":          pf.CreatedAt.Format(time.RFC3339),
		"updated_at":          pf.UpdatedAt.Format(time.RFC3339),
	}

	if pf.Description.Valid {
		result["description"] = pf.Description.String
	}

	c.JSON(http.StatusOK, gin.H{"port_forwarding": result})
}

// UpdatePortForwarding handles PUT /v2.0/floatingips/:id/port_forwardings/:pf_id
func (svc *Service) UpdatePortForwarding(c *gin.Context) {
	floatingIPID := c.Param("id")
	pfID := c.Param("pf_id")
	projectID := c.GetString("project_id")

	var req UpdatePortForwardingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Fetch current state
	var currentPF PortForwarding
	var floatingIP, routerID sql.NullString
	err := svc.activeDB().QueryRow(c.Request.Context(), `
		SELECT pf.id, pf.internal_port_id, pf.internal_ip_address, pf.external_port,
		       pf.internal_port, pf.protocol, fi.floating_ip_address, fi.router_id
		FROM port_forwardings pf
		JOIN floating_ips fi ON pf.floatingip_id = fi.id
		WHERE pf.id = $1 AND pf.floatingip_id = $2 AND pf.project_id = $3
	`, pfID, floatingIPID, projectID).Scan(&currentPF.ID, &currentPF.InternalPortID,
		&currentPF.InternalIPAddress, &currentPF.ExternalPort, &currentPF.InternalPort,
		&currentPF.Protocol, &floatingIP, &routerID)

	if err == pgx.ErrNoRows {
		common.SendError(c, common.NewNotFoundError("port forwarding"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "update_port_forwarding_fetch").Msg("failed to fetch port forwarding")
		common.SendError(c, common.NewInternalServerError("failed to update port forwarding"))
		return
	}

	// Build dynamic UPDATE query
	updates := []string{}
	args := []interface{}{}
	argID := 1

	needsNATUpdate := false

	if req.PortForwarding.InternalPortID != nil {
		updates = append(updates, fmt.Sprintf("internal_port_id = $%d", argID))
		args = append(args, *req.PortForwarding.InternalPortID)
		argID++
	}
	if req.PortForwarding.InternalIPAddress != nil {
		updates = append(updates, fmt.Sprintf("internal_ip_address = $%d", argID))
		args = append(args, *req.PortForwarding.InternalIPAddress)
		argID++
		needsNATUpdate = true
	}
	if req.PortForwarding.InternalPort != nil {
		updates = append(updates, fmt.Sprintf("internal_port = $%d", argID))
		args = append(args, *req.PortForwarding.InternalPort)
		argID++
		needsNATUpdate = true
	}
	if req.PortForwarding.Description != nil {
		updates = append(updates, fmt.Sprintf("description = $%d", argID))
		args = append(args, *req.PortForwarding.Description)
		argID++
	}

	if len(updates) == 0 {
		svc.GetPortForwarding(c) // No updates, return current state
		return
	}

	// Update NAT rules if internal IP or port changed
	if needsNATUpdate && routerID.Valid {
		externalInterface := "qg-ext-" + routerID.String[:7]
		// Remove old NAT rule
		svc.routerManager.RemovePortForwarding(
			routerID.String,
			floatingIP.String,
			currentPF.ExternalPort,
			currentPF.InternalIPAddress,
			currentPF.InternalPort,
			currentPF.Protocol,
			externalInterface,
		)

		// Add new NAT rule with updated values
		newInternalIP := currentPF.InternalIPAddress
		if req.PortForwarding.InternalIPAddress != nil {
			newInternalIP = *req.PortForwarding.InternalIPAddress
		}
		newInternalPort := currentPF.InternalPort
		if req.PortForwarding.InternalPort != nil {
			newInternalPort = *req.PortForwarding.InternalPort
		}

		if err := svc.routerManager.AddPortForwarding(
			routerID.String,
			floatingIP.String,
			currentPF.ExternalPort,
			newInternalIP,
			newInternalPort,
			currentPF.Protocol,
			externalInterface,
		); err != nil {
			fmt.Printf("Warning: failed to update port forwarding NAT rule: %v\n", err)
		}
	}

	updates = append(updates, fmt.Sprintf("updated_at = $%d", argID))
	args = append(args, time.Now())
	argID++

	args = append(args, pfID, floatingIPID, projectID)

	query := fmt.Sprintf("UPDATE port_forwardings SET %s WHERE id = $%d AND floatingip_id = $%d AND project_id = $%d",
		updateString(updates), argID, argID+1, argID+2)

	_, err = svc.activeDB().Exec(c.Request.Context(), query, args...)
	if err != nil {
		log.Error().Err(err).Str("operation", "update_port_forwarding").Msg("failed to update port forwarding")
		common.SendError(c, common.NewInternalServerError("failed to update port forwarding"))
		return
	}

	// Return updated resource
	svc.GetPortForwarding(c)
}

// DeletePortForwarding handles DELETE /v2.0/floatingips/:id/port_forwardings/:pf_id
func (svc *Service) DeletePortForwarding(c *gin.Context) {
	floatingIPID := c.Param("id")
	pfID := c.Param("pf_id")
	projectID := c.GetString("project_id")

	// Fetch port forwarding details (for NAT cleanup)
	var externalPort, internalPort int
	var protocol, internalIP string
	var floatingIP, routerID sql.NullString

	err := svc.activeDB().QueryRow(c.Request.Context(), `
		SELECT pf.external_port, pf.internal_port, pf.protocol, pf.internal_ip_address,
		       fi.floating_ip_address, fi.router_id
		FROM port_forwardings pf
		JOIN floating_ips fi ON pf.floatingip_id = fi.id
		WHERE pf.id = $1 AND pf.floatingip_id = $2 AND pf.project_id = $3
	`, pfID, floatingIPID, projectID).Scan(&externalPort, &internalPort, &protocol,
		&internalIP, &floatingIP, &routerID)

	if err == pgx.ErrNoRows {
		common.SendError(c, common.NewNotFoundError("port forwarding"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "delete_port_forwarding_fetch").Msg("failed to fetch port forwarding")
		common.SendError(c, common.NewInternalServerError("failed to delete port forwarding"))
		return
	}

	// Remove NAT rule
	if routerID.Valid {
		externalInterface := "qg-ext-" + routerID.String[:7]
		if err := svc.routerManager.RemovePortForwarding(
			routerID.String,
			floatingIP.String,
			externalPort,
			internalIP,
			internalPort,
			protocol,
			externalInterface,
		); err != nil {
			fmt.Printf("Warning: failed to remove port forwarding NAT rule: %v\n", err)
		}
	}

	// Delete from database
	_, err = svc.activeDB().Exec(c.Request.Context(),
		"DELETE FROM port_forwardings WHERE id = $1 AND floatingip_id = $2 AND project_id = $3",
		pfID, floatingIPID, projectID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "delete_port_forwarding").Msg("failed to delete port forwarding")
		common.SendError(c, common.NewInternalServerError("failed to delete port forwarding"))
		return
	}

	c.Status(http.StatusNoContent)
}
