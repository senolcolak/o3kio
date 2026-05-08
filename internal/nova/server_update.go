package nova

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/common"
	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// UpdateServer handles PATCH /v2.1/servers/:id
func (svc *Service) UpdateServer(c *gin.Context) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	var req struct {
		Server struct {
			Name *string `json:"name"`
		} `json:"server"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Verify server exists and get current details
	var (
		currentName     string
		currentStatus   string
		currentFlavorID string
		currentImageID  *string
		createdAt       time.Time
		updatedAt       time.Time
	)

	err := svc.activeDB().QueryRow(c.Request.Context(), `
		SELECT name, status, flavor_id, image_id, created_at, updated_at
		FROM instances
		WHERE id = $1 AND project_id = $2
	`, instanceID, projectID).Scan(
		&currentName, &currentStatus,
		&currentFlavorID, &currentImageID, &createdAt, &updatedAt,
	)

	if errors.Is(err, database.ErrNoRows) {
		common.SendError(c, common.NewNotFoundError("instance"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "get_server_for_update").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to get server"))
		return
	}

	// Build update query dynamically based on what's being updated
	updates := []string{}
	params := []interface{}{instanceID, projectID}
	paramIndex := 3

	if req.Server.Name != nil {
		updates = append(updates, fmt.Sprintf("name = $%d", paramIndex))
		params = append(params, *req.Server.Name)
		currentName = *req.Server.Name
		paramIndex++
	}

	if len(updates) == 0 {
		// No updates requested, just return current state
	} else {
		// Add updated_at
		updates = append(updates, fmt.Sprintf("updated_at = $%d", paramIndex))
		params = append(params, time.Now())

		// Build and execute update query
		query := "UPDATE instances SET "
		for i, update := range updates {
			if i > 0 {
				query += ", "
			}
			query += update
		}
		query += " WHERE id = $1 AND project_id = $2"

		_, err = svc.activeDB().Exec(c.Request.Context(), query, params...)
		if err != nil {
			log.Error().Err(err).Str("operation", "update_server").Msg("database error")
			common.SendError(c, common.NewInternalServerError("failed to update server"))
			return
		}
	}

	// Build response with full server details
	server := gin.H{
		"id":        instanceID,
		"name":      currentName,
		"status":    currentStatus,
		"tenant_id": projectID,
		"user_id":   projectID, // Simplified
		"created":   createdAt.Format(time.RFC3339),
		"updated":   time.Now().Format(time.RFC3339),
		"hostId":    "",
		"addresses": gin.H{},
		"links": []gin.H{
			{
				"href": c.Request.Host + "/v2.1/servers/" + instanceID,
				"rel":  "self",
			},
		},
		"image": gin.H{
			"id": "",
		},
		"flavor": gin.H{
			"id": currentFlavorID,
		},
	}

	if currentImageID != nil {
		server["image"] = gin.H{"id": *currentImageID}
	}

	c.JSON(http.StatusOK, gin.H{"server": server})
}
