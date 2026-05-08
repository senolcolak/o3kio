package keystone

import (
	"errors"
	"fmt"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/common"
	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// ListGroups handles GET /v3/groups
func (svc *Service) ListGroups(c *gin.Context) {
	rows, err := svc.activeDB().Query(c.Request.Context(),
		"SELECT id, name, domain_id, description, created_at FROM groups ORDER BY created_at DESC",
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "list_groups").Msg("Failed to query groups")
		common.SendError(c, common.NewInternalServerError("failed to query groups"))
		return
	}
	defer rows.Close()

	var groups []gin.H
	for rows.Next() {
		var id, name, domainID, description string
		var createdAt time.Time

		if err := rows.Scan(&id, &name, &domainID, &description, &createdAt); err != nil {
			continue
		}

		groups = append(groups, gin.H{
			"id":          id,
			"name":        name,
			"domain_id":   domainID,
			"description": description,
			"links": gin.H{
				"self": c.Request.Host + "/v3/groups/" + id,
			},
		})
	}

	if groups == nil {
		groups = []gin.H{}
	}

	if err := rows.Err(); err != nil {
		log.Error().Err(err).Str("operation", "list_groups").Msg("row iteration error")
		common.SendError(c, common.NewInternalServerError("failed to read groups"))
		return
	}

	c.JSON(200, gin.H{"groups": groups})
}

// CreateGroup handles POST /v3/groups
func (svc *Service) CreateGroup(c *gin.Context) {
	var req struct {
		Group struct {
			Name        string `json:"name" binding:"required"`
			DomainID    string `json:"domain_id"`
			Description string `json:"description"`
		} `json:"group" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Default to "default" domain
	domainID := req.Group.DomainID
	if domainID == "" {
		domainID = "00000000-0000-0000-0000-000000000100" // default domain (corrected UUID)
	}

	groupID := uuid.New().String()
	now := time.Now()

	_, err := svc.activeDB().Exec(c.Request.Context(),
		`INSERT INTO groups (id, name, domain_id, description, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		groupID, req.Group.Name, domainID, req.Group.Description, now, now,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "create_group").Msg("Failed to create group")
		common.SendError(c, common.NewInternalServerError("failed to create group"))
		return
	}

	c.JSON(201, gin.H{
		"group": gin.H{
			"id":          groupID,
			"name":        req.Group.Name,
			"domain_id":   domainID,
			"description": req.Group.Description,
			"links": gin.H{
				"self": c.Request.Host + "/v3/groups/" + groupID,
			},
		},
	})
}

// GetGroup handles GET /v3/groups/:id
func (svc *Service) GetGroup(c *gin.Context) {
	groupID := c.Param("id")

	var name, domainID, description string
	var createdAt time.Time

	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT name, domain_id, description, created_at FROM groups WHERE id = $1",
		groupID,
	).Scan(&name, &domainID, &description, &createdAt)

	if errors.Is(err, database.ErrNoRows) {
		common.SendError(c, common.NewNotFoundError("group"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "get_group").Str("group_id", groupID).Msg("Failed to query group")
		common.SendError(c, common.NewInternalServerError("failed to query group"))
		return
	}

	c.JSON(200, gin.H{
		"group": gin.H{
			"id":          groupID,
			"name":        name,
			"domain_id":   domainID,
			"description": description,
			"links": gin.H{
				"self": c.Request.Host + "/v3/groups/" + groupID,
			},
		},
	})
}

// UpdateGroup handles PATCH /v3/groups/:id
func (svc *Service) UpdateGroup(c *gin.Context) {
	groupID := c.Param("id")

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

	// Build dynamic update query
	updates := []string{}
	args := []interface{}{}
	argIdx := 1

	if req.Group.Name != nil {
		updates = append(updates, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *req.Group.Name)
		argIdx++
	}
	if req.Group.Description != nil {
		updates = append(updates, fmt.Sprintf("description = $%d", argIdx))
		args = append(args, *req.Group.Description)
		argIdx++
	}

	if len(updates) == 0 {
		common.SendError(c, common.NewBadRequestError("no fields to update"))
		return
	}

	updates = append(updates, "updated_at = NOW()")
	args = append(args, groupID)

	query := fmt.Sprintf("UPDATE groups SET %s WHERE id = $%d", joinUpdates(updates), argIdx)
	_, err := svc.activeDB().Exec(c.Request.Context(), query, args...)
	if err != nil {
		log.Error().Err(err).Str("operation", "update_group").Str("group_id", groupID).Msg("Failed to update group")
		common.SendError(c, common.NewInternalServerError("failed to update group"))
		return
	}

	// Fetch updated group
	var name, domainID, description string
	err = svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT name, domain_id, description FROM groups WHERE id = $1",
		groupID,
	).Scan(&name, &domainID, &description)

	if err != nil {
		log.Error().Err(err).Str("operation", "update_group_fetch").Str("group_id", groupID).Msg("Failed to fetch updated group")
		common.SendError(c, common.NewInternalServerError("failed to fetch updated group"))
		return
	}

	c.JSON(200, gin.H{
		"group": gin.H{
			"id":          groupID,
			"name":        name,
			"domain_id":   domainID,
			"description": description,
			"links": gin.H{
				"self": c.Request.Host + "/v3/groups/" + groupID,
			},
		},
	})
}

// DeleteGroup handles DELETE /v3/groups/:id
func (svc *Service) DeleteGroup(c *gin.Context) {
	groupID := c.Param("id")

	result, err := svc.activeDB().Exec(c.Request.Context(),
		"DELETE FROM groups WHERE id = $1",
		groupID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "delete_group").Str("group_id", groupID).Msg("Failed to delete group")
		common.SendError(c, common.NewInternalServerError("failed to delete group"))
		return
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		common.SendError(c, common.NewNotFoundError("group"))
		return
	}

	c.Status(204)
}

// ListGroupUsers handles GET /v3/groups/:id/users
func (svc *Service) ListGroupUsers(c *gin.Context) {
	groupID := c.Param("id")

	rows, err := svc.activeDB().Query(c.Request.Context(),
		`SELECT u.id, u.name, u.domain_id, u.enabled
		 FROM users u
		 INNER JOIN group_members gu ON u.id = gu.user_id
		 WHERE gu.group_id = $1`,
		groupID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "list_group_users").Str("group_id", groupID).Msg("Failed to query group users")
		common.SendError(c, common.NewInternalServerError("failed to query group users"))
		return
	}
	defer rows.Close()

	var users []gin.H
	for rows.Next() {
		var id, name, domainID string
		var enabled bool

		if err := rows.Scan(&id, &name, &domainID, &enabled); err != nil {
			continue
		}

		users = append(users, gin.H{
			"id":        id,
			"name":      name,
			"domain_id": domainID,
			"enabled":   enabled,
		})
	}

	if users == nil {
		users = []gin.H{}
	}

	if err := rows.Err(); err != nil {
		log.Error().Err(err).Str("operation", "list_group_users").Msg("row iteration error")
		common.SendError(c, common.NewInternalServerError("failed to read group users"))
		return
	}

	c.JSON(200, gin.H{"users": users})
}

// AddUserToGroup handles PUT /v3/groups/:id/users/:user_id
func (svc *Service) AddUserToGroup(c *gin.Context) {
	groupID := c.Param("id")
	userID := c.Param("user_id")

	_, err := svc.activeDB().Exec(c.Request.Context(),
		`INSERT INTO group_members (group_id, user_id)
		 VALUES ($1, $2)
		 ON CONFLICT (group_id, user_id) DO NOTHING`,
		groupID, userID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "add_user_to_group").Str("group_id", groupID).Str("user_id", userID).Msg("Failed to add user to group")
		common.SendError(c, common.NewInternalServerError("failed to add user to group"))
		return
	}

	c.Status(204)
}

// RemoveUserFromGroup handles DELETE /v3/groups/:id/users/:user_id
func (svc *Service) RemoveUserFromGroup(c *gin.Context) {
	groupID := c.Param("id")
	userID := c.Param("user_id")

	result, err := svc.activeDB().Exec(c.Request.Context(),
		"DELETE FROM group_members WHERE group_id = $1 AND user_id = $2",
		groupID, userID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "remove_user_from_group").Str("group_id", groupID).Str("user_id", userID).Msg("Failed to remove user from group")
		common.SendError(c, common.NewInternalServerError("failed to remove user from group"))
		return
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		common.SendError(c, common.NewNotFoundError("user in group"))
		return
	}

	c.Status(204)
}

// Helper function to join update strings
func joinUpdates(updates []string) string {
	result := ""
	for i, update := range updates {
		if i > 0 {
			result += ", "
		}
		result += update
	}
	return result
}
