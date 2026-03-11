package cinder

import (
	"net/http"

	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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
		err := database.DB.QueryRow(c.Request.Context(),
			"SELECT id FROM projects WHERE name = $1",
			targetProject).Scan(&targetProjectID)
		if err != nil {
			// If not found, use as-is (might be UUID without dashes)
			targetProjectID = targetProject
		}
	}

	// Default quotas
	quotas := map[string]int{
		"volumes":   100,
		"snapshots": 100,
		"gigabytes": 10000,
		"backups":   100,
		"backup_gigabytes": 10000,
		"per_volume_gigabytes": 1000,
	}

	// Load custom quotas from database
	rows, err := database.DB.Query(c.Request.Context(), `
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
		err := database.DB.QueryRow(c.Request.Context(),
			"SELECT id FROM projects WHERE name = $1",
			targetProject).Scan(&targetProjectID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
			return
		}
		projectUUID, _ = uuid.Parse(targetProjectID)
	}

	var req struct {
		QuotaSet map[string]interface{} `json:"quota_set"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Update each quota resource
	for resource, limitVal := range req.QuotaSet {
		// Skip non-integer fields
		var limit int
		switch v := limitVal.(type) {
		case float64:
			limit = int(v)
		case int:
			limit = v
		default:
			continue
		}

		_, err := database.DB.Exec(c.Request.Context(), `
			INSERT INTO cinder_quotas (project_id, resource, "limit")
			VALUES ($1, $2, $3)
			ON CONFLICT (project_id, resource)
			DO UPDATE SET "limit" = $3, updated_at = NOW()
		`, projectUUID, resource, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	// Load and return all quotas (defaults + overrides)
	defaults := map[string]int{
		"volumes":   100,
		"snapshots": 100,
		"gigabytes": 10000,
		"backups":   100,
		"backup_gigabytes": 10000,
		"per_volume_gigabytes": 1000,
	}

	rows, err := database.DB.Query(c.Request.Context(), `
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
		err := database.DB.QueryRow(c.Request.Context(),
			"SELECT id FROM projects WHERE name = $1",
			targetProject).Scan(&targetProjectID)
		if err != nil {
			targetProjectID = targetProject
		}
	}

	_, err := database.DB.Exec(c.Request.Context(),
		"DELETE FROM cinder_quotas WHERE project_id = $1",
		targetProjectID)
	if err != nil && err != pgx.ErrNoRows {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reset quotas"})
		return
	}

	c.Status(http.StatusNoContent)
}
