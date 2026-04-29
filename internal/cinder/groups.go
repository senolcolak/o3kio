package cinder

import (
	"net/http"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/common"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

// ListGroups lists all volume groups
func (svc *Service) ListGroups(c *gin.Context) {
	projectID := c.GetString("project_id")

	rows, err := svc.activeDB().Query(c.Request.Context(), `
		SELECT id, project_id, name, description, status, group_type, created_at, updated_at
		FROM volume_groups
		WHERE project_id = $1
		ORDER BY created_at DESC
	`, projectID)
	if err != nil {
		log.Error().Err(err).Str("operation", "list_groups").Msg("failed to query volume groups")
		common.SendError(c, common.NewInternalServerError("failed to list groups"))
		return
	}
	defer rows.Close()

	groups := []map[string]interface{}{}
	for rows.Next() {
		var (
			id          string
			projID      string
			name        *string
			description *string
			status      string
			groupType   string
			createdAt   time.Time
			updatedAt   time.Time
		)

		err := rows.Scan(&id, &projID, &name, &description, &status, &groupType, &createdAt, &updatedAt)
		if err != nil {
			log.Error().Err(err).Str("operation", "scan_group").Msg("failed to scan group row")
			common.SendError(c, common.NewInternalServerError("failed to read group data"))
			return
		}

		group := map[string]interface{}{
			"id":          id,
			"name":        name,
			"description": description,
			"status":      status,
			"group_type":  groupType,
			"created_at":  createdAt.Format(time.RFC3339),
			"updated_at":  updatedAt.Format(time.RFC3339),
		}
		groups = append(groups, group)
	}

	c.JSON(http.StatusOK, gin.H{"groups": groups})
}

// CreateGroup creates a new volume group
func (svc *Service) CreateGroup(c *gin.Context) {
	projectID := c.GetString("project_id")

	var req struct {
		Group struct {
			Name        *string `json:"name"`
			Description *string `json:"description"`
			GroupType   string  `json:"group_type" binding:"required"`
		} `json:"group" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	groupID := uuid.New().String()
	now := time.Now()

	_, err := svc.activeDB().Exec(c.Request.Context(), `
		INSERT INTO volume_groups (id, project_id, name, description, status, group_type, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, groupID, projectID, req.Group.Name, req.Group.Description, "available", req.Group.GroupType, now, now)

	if err != nil {
		log.Error().Err(err).Str("operation", "create_group").Msg("failed to insert volume group")
		common.SendError(c, common.NewInternalServerError("failed to create group"))
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"group": map[string]interface{}{
			"id":          groupID,
			"name":        req.Group.Name,
			"description": req.Group.Description,
			"status":      "available",
			"group_type":  req.Group.GroupType,
			"created_at":  now.Format(time.RFC3339),
			"updated_at":  now.Format(time.RFC3339),
		},
	})
}

// GetGroup retrieves a specific group
func (svc *Service) GetGroup(c *gin.Context) {
	groupID := c.Param("id")
	projectID := c.GetString("project_id")

	var (
		name        *string
		description *string
		status      string
		groupType   string
		createdAt   time.Time
		updatedAt   time.Time
	)

	err := svc.activeDB().QueryRow(c.Request.Context(), `
		SELECT name, description, status, group_type, created_at, updated_at
		FROM volume_groups
		WHERE id = $1 AND project_id = $2
	`, groupID, projectID).Scan(&name, &description, &status, &groupType, &createdAt, &updatedAt)

	if err == pgx.ErrNoRows {
		common.SendError(c, common.NewNotFoundError("group"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "get_group").Msg("failed to query volume group")
		common.SendError(c, common.NewInternalServerError("failed to get group"))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"group": map[string]interface{}{
			"id":          groupID,
			"name":        name,
			"description": description,
			"status":      status,
			"group_type":  groupType,
			"created_at":  createdAt.Format(time.RFC3339),
			"updated_at":  updatedAt.Format(time.RFC3339),
		},
	})
}

// UpdateGroup updates a group
func (svc *Service) UpdateGroup(c *gin.Context) {
	groupID := c.Param("id")
	projectID := c.GetString("project_id")

	var req struct {
		Group struct {
			Name        *string `json:"name"`
			Description *string `json:"description"`
		} `json:"group" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Check if group exists
	var exists bool
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM volume_groups WHERE id = $1 AND project_id = $2)",
		groupID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		common.SendError(c, common.NewNotFoundError("group"))
		return
	}

	// Update group
	_, err = svc.activeDB().Exec(c.Request.Context(), `
		UPDATE volume_groups
		SET name = COALESCE($1, name),
		    description = COALESCE($2, description),
		    updated_at = $3
		WHERE id = $4 AND project_id = $5
	`, req.Group.Name, req.Group.Description, time.Now(), groupID, projectID)

	if err != nil {
		log.Error().Err(err).Str("operation", "update_group").Msg("failed to update volume group")
		common.SendError(c, common.NewInternalServerError("failed to update group"))
		return
	}

	// Fetch updated group
	var (
		name        *string
		description *string
		status      string
		groupType   string
		createdAt   time.Time
		updatedAt   time.Time
	)

	err = svc.activeDB().QueryRow(c.Request.Context(), `
		SELECT name, description, status, group_type, created_at, updated_at
		FROM volume_groups
		WHERE id = $1 AND project_id = $2
	`, groupID, projectID).Scan(&name, &description, &status, &groupType, &createdAt, &updatedAt)

	if err != nil {
		log.Error().Err(err).Str("operation", "fetch_updated_group").Msg("failed to fetch updated group")
		common.SendError(c, common.NewInternalServerError("failed to fetch updated group"))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"group": map[string]interface{}{
			"id":          groupID,
			"name":        name,
			"description": description,
			"status":      status,
			"group_type":  groupType,
			"created_at":  createdAt.Format(time.RFC3339),
			"updated_at":  updatedAt.Format(time.RFC3339),
		},
	})
}

// DeleteGroup deletes a group
func (svc *Service) DeleteGroup(c *gin.Context) {
	groupID := c.Param("id")
	projectID := c.GetString("project_id")

	result, err := svc.activeDB().Exec(c.Request.Context(),
		"DELETE FROM volume_groups WHERE id = $1 AND project_id = $2",
		groupID, projectID,
	)

	if err != nil {
		log.Error().Err(err).Str("operation", "delete_group").Msg("failed to delete volume group")
		common.SendError(c, common.NewInternalServerError("failed to delete group"))
		return
	}

	if result.RowsAffected() == 0 {
		common.SendError(c, common.NewNotFoundError("group"))
		return
	}

	c.Status(http.StatusAccepted)
}
