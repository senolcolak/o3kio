package keystone

import (
	"fmt"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/common"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// ListDomains handles GET /v3/domains
func (svc *Service) ListDomains(c *gin.Context) {
	rows, err := svc.activeDB().Query(c.Request.Context(), `
		SELECT id, name, description, enabled
		FROM domains
		ORDER BY name ASC
	`)

	if err != nil {
		log.Error().Err(err).Str("operation", "list_domains").Msg("Failed to query domains")
		common.SendError(c, common.NewInternalServerError("failed to query domains"))
		return
	}
	defer rows.Close()

	domains := []map[string]interface{}{}
	for rows.Next() {
		var id uuid.UUID
		var name string
		var description *string
		var enabled bool

		err := rows.Scan(&id, &name, &description, &enabled)
		if err != nil {
			continue
		}

		domain := map[string]interface{}{
			"id":      id.String(),
			"name":    name,
			"enabled": enabled,
		}

		if description != nil {
			domain["description"] = *description
		}

		domains = append(domains, domain)
	}

	c.JSON(200, gin.H{"domains": domains})
}

// CreateDomain handles POST /v3/domains
func (svc *Service) CreateDomain(c *gin.Context) {
	var req struct {
		Domain struct {
			Name        string `json:"name" binding:"required"`
			Description string `json:"description"`
			Enabled     *bool  `json:"enabled"`
		} `json:"domain" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	domainID := uuid.New()
	enabled := true
	if req.Domain.Enabled != nil {
		enabled = *req.Domain.Enabled
	}

	var description *string
	if req.Domain.Description != "" {
		description = &req.Domain.Description
	}

	_, err := svc.activeDB().Exec(c.Request.Context(), `
		INSERT INTO domains (id, name, description, enabled, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, domainID, req.Domain.Name, description, enabled, time.Now(), time.Now())

	if err != nil {
		common.SendError(c, common.NewConflictError("domain already exists"))
		return
	}

	domain := map[string]interface{}{
		"id":      domainID.String(),
		"name":    req.Domain.Name,
		"enabled": enabled,
	}

	if description != nil {
		domain["description"] = *description
	}

	c.JSON(201, gin.H{"domain": domain})
}

// GetDomain handles GET /v3/domains/:id
func (svc *Service) GetDomain(c *gin.Context) {
	domainID := c.Param("id")

	var id uuid.UUID
	var name string
	var description *string
	var enabled bool

	err := svc.activeDB().QueryRow(c.Request.Context(), `
		SELECT id, name, description, enabled
		FROM domains
		WHERE id = $1
	`, domainID).Scan(&id, &name, &description, &enabled)

	if err != nil {
		common.SendError(c, common.NewNotFoundError("domain"))
		return
	}

	domain := map[string]interface{}{
		"id":      id.String(),
		"name":    name,
		"enabled": enabled,
	}

	if description != nil {
		domain["description"] = *description
	}

	c.JSON(200, gin.H{"domain": domain})
}

// UpdateDomain handles PATCH /v3/domains/:id
func (svc *Service) UpdateDomain(c *gin.Context) {
	domainID := c.Param("id")

	var req struct {
		Domain map[string]interface{} `json:"domain" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Verify domain exists
	var exists bool
	err := svc.activeDB().QueryRow(c.Request.Context(), `
		SELECT EXISTS(SELECT 1 FROM domains WHERE id = $1)
	`, domainID).Scan(&exists)

	if err != nil || !exists {
		common.SendError(c, common.NewNotFoundError("domain"))
		return
	}

	// Build update query dynamically
	updates := []string{}
	args := []interface{}{}
	argCount := 1

	if name, ok := req.Domain["name"].(string); ok {
		updates = append(updates, fmt.Sprintf("name = $%d", argCount))
		args = append(args, name)
		argCount++
	}

	if description, ok := req.Domain["description"].(string); ok {
		updates = append(updates, fmt.Sprintf("description = $%d", argCount))
		args = append(args, description)
		argCount++
	}

	if enabled, ok := req.Domain["enabled"].(bool); ok {
		updates = append(updates, fmt.Sprintf("enabled = $%d", argCount))
		args = append(args, enabled)
		argCount++
	}

	updates = append(updates, fmt.Sprintf("updated_at = $%d", argCount))
	args = append(args, time.Now())
	argCount++

	args = append(args, domainID)

	if len(updates) > 1 { // More than just updated_at
		query := "UPDATE domains SET "
		for i, update := range updates {
			if i > 0 {
				query += ", "
			}
			query += update
		}
		query += fmt.Sprintf(" WHERE id = $%d", argCount)

		_, err = svc.activeDB().Exec(c.Request.Context(), query, args...)
		if err != nil {
			log.Error().Err(err).Str("operation", "update_domain").Str("domain_id", domainID).Msg("Failed to update domain")
			common.SendError(c, common.NewInternalServerError("failed to update domain"))
			return
		}
	}

	// Fetch updated domain
	var id uuid.UUID
	var name string
	var description *string
	var enabled bool

	err = svc.activeDB().QueryRow(c.Request.Context(), `
		SELECT id, name, description, enabled
		FROM domains
		WHERE id = $1
	`, domainID).Scan(&id, &name, &description, &enabled)

	if err != nil {
		log.Error().Err(err).Str("operation", "update_domain_fetch").Str("domain_id", domainID).Msg("Failed to fetch updated domain")
		common.SendError(c, common.NewInternalServerError("failed to fetch updated domain"))
		return
	}

	domain := map[string]interface{}{
		"id":      id.String(),
		"name":    name,
		"enabled": enabled,
	}

	if description != nil {
		domain["description"] = *description
	}

	c.JSON(200, gin.H{"domain": domain})
}

// DeleteDomain handles DELETE /v3/domains/:id
func (svc *Service) DeleteDomain(c *gin.Context) {
	domainID := c.Param("id")

	// Check if domain is enabled
	var enabled bool
	err := svc.activeDB().QueryRow(c.Request.Context(), `
		SELECT enabled FROM domains WHERE id = $1
	`, domainID).Scan(&enabled)

	if err != nil {
		common.SendError(c, common.NewNotFoundError("domain"))
		return
	}

	if enabled {
		common.SendError(c, common.NewForbiddenError("cannot delete enabled domain; disable it first"))
		return
	}

	result, err := svc.activeDB().Exec(c.Request.Context(),
		"DELETE FROM domains WHERE id = $1",
		domainID,
	)

	if err != nil {
		log.Error().Err(err).Str("operation", "delete_domain").Str("domain_id", domainID).Msg("Failed to delete domain")
		common.SendError(c, common.NewInternalServerError("failed to delete domain"))
		return
	}

	if result.RowsAffected() == 0 {
		common.SendError(c, common.NewNotFoundError("domain"))
		return
	}

	c.Status(204)
}

// GetDomainConfig handles GET /v3/domains/:id/config
func (svc *Service) GetDomainConfig(c *gin.Context) {
	domainID := c.Param("id")

	// Verify domain exists
	var exists bool
	err := svc.activeDB().QueryRow(c.Request.Context(), `
		SELECT EXISTS(SELECT 1 FROM domains WHERE id = $1)
	`, domainID).Scan(&exists)

	if err != nil || !exists {
		common.SendError(c, common.NewNotFoundError("domain"))
		return
	}

	// Return empty config (stub implementation)
	config := map[string]interface{}{
		"identity": map[string]interface{}{},
		"ldap":     map[string]interface{}{},
	}

	c.JSON(200, gin.H{"config": config})
}
