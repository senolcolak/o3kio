package cinder

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/cobaltcore-dev/o3k/internal/common"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/rs/zerolog/log"
)

// GetQuotaSet returns quota limits for a project
func (svc *Service) GetQuotaSet(c *gin.Context) {
	targetProject := c.Param("id")

	// Resolve project name to UUID if needed
	var targetProjectID string
	if len(targetProject) == 36 && targetProject[8] == '-' {
		// Already a UUID
		targetProjectID = targetProject
	} else {
		// Look up by name
		err := svc.activeDB().QueryRow(c.Request.Context(),
			"SELECT id FROM projects WHERE name = $1",
			targetProject).Scan(&targetProjectID)
		if err != nil {
			// If not found, use as-is (might be UUID without dashes)
			targetProjectID = targetProject
		}
	}

	// Default quotas
	quotas := map[string]int{
		"volumes":              100,
		"snapshots":            100,
		"gigabytes":            10000,
		"backups":              100,
		"backup_gigabytes":     10000,
		"per_volume_gigabytes": 1000,
	}

	// Load custom quotas from database
	rows, err := svc.activeDB().Query(c.Request.Context(), `
		SELECT resource, "limit"
		FROM cinder_quotas
		WHERE project_id = $1
	`, targetProjectID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var resource string
			var limit int
			if err := rows.Scan(&resource, &limit); err == nil {
				quotas[resource] = limit
			}
		}
		// Best-effort: log but don't fail if iteration error on quotas
		if iterErr := rows.Err(); iterErr != nil {
			log.Warn().Err(iterErr).Str("operation", "get_quota_set").Msg("rows iteration error")
		}
	}

	// Add project ID to response
	quotaSet := make(map[string]interface{})
	quotaSet["id"] = targetProjectID
	for k, v := range quotas {
		quotaSet[k] = v
	}

	c.JSON(http.StatusOK, gin.H{"quota_set": quotaSet})
}

// UpdateQuotaSet updates quota limits for a project
func (svc *Service) UpdateQuotaSet(c *gin.Context) {
	targetProject := c.Param("id")

	// Resolve project name to UUID if needed
	var projectUUID uuid.UUID
	if len(targetProject) == 36 && targetProject[8] == '-' {
		projectUUID, _ = uuid.Parse(targetProject)
	} else {
		// Look up by name
		var targetProjectID string
		err := svc.activeDB().QueryRow(c.Request.Context(),
			"SELECT id FROM projects WHERE name = $1",
			targetProject).Scan(&targetProjectID)
		if err != nil {
			common.SendError(c, common.NewNotFoundError("project"))
			return
		}
		projectUUID, _ = uuid.Parse(targetProjectID)
	}

	var req struct {
		QuotaSet map[string]interface{} `json:"quota_set"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Update each quota resource
	for resource, limitVal := range req.QuotaSet {
		// Skip non-integer fields like "id"
		if resource == "id" {
			continue
		}

		var limit int
		switch v := limitVal.(type) {
		case float64:
			limit = int(v)
		case int:
			limit = v
		case json.Number:
			// Handle json.Number from interface{} unmarshaling
			val, err := v.Int64()
			if err != nil {
				continue
			}
			limit = int(val)
		default:
			continue
		}

		_, err := svc.activeDB().Exec(c.Request.Context(), `
			INSERT INTO cinder_quotas (project_id, resource, "limit")
			VALUES ($1, $2, $3)
			ON CONFLICT (project_id, resource)
			DO UPDATE SET "limit" = $3, updated_at = NOW()
		`, projectUUID, resource, limit)
		if err != nil {
			log.Error().Err(err).Str("operation", "update_quota").Str("resource", resource).Msg("failed to update quota")
			common.SendError(c, common.NewInternalServerError("failed to update quota"))
			return
		}
	}

	// Load and return all quotas (defaults + overrides)
	defaults := map[string]int{
		"volumes":              100,
		"snapshots":            100,
		"gigabytes":            10000,
		"backups":              100,
		"backup_gigabytes":     10000,
		"per_volume_gigabytes": 1000,
	}

	rows, err := svc.activeDB().Query(c.Request.Context(), `
		SELECT resource, "limit"
		FROM cinder_quotas
		WHERE project_id = $1
	`, projectUUID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var resource string
			var limit int
			if err := rows.Scan(&resource, &limit); err == nil {
				defaults[resource] = limit
			}
		}
		if iterErr := rows.Err(); iterErr != nil {
			log.Warn().Err(iterErr).Str("operation", "get_quota_set_defaults").Msg("rows iteration error")
		}
	}

	quotaSet := make(map[string]interface{})
	quotaSet["id"] = projectUUID.String()
	for k, v := range defaults {
		quotaSet[k] = v
	}

	c.JSON(http.StatusOK, gin.H{"quota_set": quotaSet})
}

// DeleteQuotaSet resets quotas to defaults for a project
func (svc *Service) DeleteQuotaSet(c *gin.Context) {
	targetProject := c.Param("id")

	// Resolve project name to UUID if needed
	var targetProjectID string
	if len(targetProject) == 36 && targetProject[8] == '-' {
		targetProjectID = targetProject
	} else {
		err := svc.activeDB().QueryRow(c.Request.Context(),
			"SELECT id FROM projects WHERE name = $1",
			targetProject).Scan(&targetProjectID)
		if err != nil {
			targetProjectID = targetProject
		}
	}

	_, err := svc.activeDB().Exec(c.Request.Context(),
		"DELETE FROM cinder_quotas WHERE project_id = $1",
		targetProjectID)
	if err != nil && !errors.Is(err, database.ErrNoRows) {
		log.Error().Err(err).Str("operation", "delete_quota_set").Msg("failed to reset quotas")
		common.SendError(c, common.NewInternalServerError("failed to reset quotas"))
		return
	}

	c.Status(http.StatusNoContent)
}
