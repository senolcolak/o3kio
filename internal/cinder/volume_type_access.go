package cinder

import (
	"net/http"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// VolumeTypeAction handles POST /v3/:project_id/types/:id/action
func (svc *Service) VolumeTypeAction(c *gin.Context) {
	typeID := c.Param("id")

	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Handle addProjectAccess action
	if addAccess, ok := req["addProjectAccess"]; ok {
		addMap := addAccess.(map[string]interface{})
		projectID := addMap["project"].(string)

		// Verify volume type exists and is private
		var isPublic bool
		err := database.DB.QueryRow(c.Request.Context(),
			"SELECT is_public FROM volume_types WHERE id = $1",
			typeID,
		).Scan(&isPublic)

		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Volume type not found"})
			return
		}

		if isPublic {
			c.JSON(http.StatusConflict, gin.H{"error": "Cannot add access to public volume type"})
			return
		}

		// Add access
		_, err = database.DB.Exec(c.Request.Context(), `
			INSERT INTO volume_type_access (volume_type_id, project_id, created_at)
			VALUES ($1, $2, $3)
			ON CONFLICT (volume_type_id, project_id) DO NOTHING
		`, typeID, projectID, time.Now())

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.Status(http.StatusAccepted)
		return
	}

	// Handle removeProjectAccess action
	if removeAccess, ok := req["removeProjectAccess"]; ok {
		removeMap := removeAccess.(map[string]interface{})
		projectID := removeMap["project"].(string)

		// Remove access
		result, err := database.DB.Exec(c.Request.Context(),
			"DELETE FROM volume_type_access WHERE volume_type_id = $1 AND project_id = $2",
			typeID, projectID,
		)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		if result.RowsAffected() == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "Project access not found"})
			return
		}

		c.Status(http.StatusAccepted)
		return
	}

	c.JSON(http.StatusBadRequest, gin.H{"error": "Unknown action"})
}

// ListVolumeTypeAccess handles GET /v3/:project_id/types/:id/os-volume-type-access
func (svc *Service) ListVolumeTypeAccess(c *gin.Context) {
	typeID := c.Param("id")

	// Verify volume type exists and is private
	var isPublic bool
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT is_public FROM volume_types WHERE id = $1",
		typeID,
	).Scan(&isPublic)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Volume type not found"})
		return
	}

	if isPublic {
		// Public types don't have access control
		c.JSON(http.StatusOK, gin.H{"volume_type_access": []interface{}{}})
		return
	}

	// Get access list
	rows, err := database.DB.Query(c.Request.Context(), `
		SELECT volume_type_id, project_id
		FROM volume_type_access
		WHERE volume_type_id = $1
	`, typeID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	access := []map[string]interface{}{}
	for rows.Next() {
		var volumeTypeID, projectID uuid.UUID
		if rows.Scan(&volumeTypeID, &projectID) == nil {
			access = append(access, map[string]interface{}{
				"volume_type_id": volumeTypeID.String(),
				"project_id":     projectID.String(),
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{"volume_type_access": access})
}

