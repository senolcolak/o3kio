package keystone

import (
	"fmt"
	"net/http"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ListGroups handles GET /v3/groups
func (svc *Service) ListGroups(c *gin.Context) {
	rows, err := database.DB.Query(c.Request.Context(),
		"SELECT id, name, domain_id, description, created_at FROM groups ORDER BY created_at DESC",
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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

	c.JSON(http.StatusOK, gin.H{"groups": groups})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Default to "default" domain
	domainID := req.Group.DomainID
	if domainID == "" {
		domainID = "00000000-0000-0000-0000-000000000001" // default domain
	}

	groupID := uuid.New().String()
	now := time.Now()

	_, err := database.DB.Exec(c.Request.Context(),
		`INSERT INTO groups (id, name, domain_id, description, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		groupID, req.Group.Name, domainID, req.Group.Description, now, now,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
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

	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT name, domain_id, description, created_at FROM groups WHERE id = $1",
		groupID,
	).Scan(&name, &domainID, &description, &createdAt)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"message": "Group not found",
				"code":    404,
				"title":   "Not Found",
			},
		})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "No fields to update"})
		return
	}

	updates = append(updates, "updated_at = NOW()")
	args = append(args, groupID)

	query := fmt.Sprintf("UPDATE groups SET %s WHERE id = $%d", joinUpdates(updates), argIdx)
	_, err := database.DB.Exec(c.Request.Context(), query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Fetch updated group
	var name, domainID, description string
	err = database.DB.QueryRow(c.Request.Context(),
		"SELECT name, domain_id, description FROM groups WHERE id = $1",
		groupID,
	).Scan(&name, &domainID, &description)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
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

	result, err := database.DB.Exec(c.Request.Context(),
		"DELETE FROM groups WHERE id = $1",
		groupID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"message": "Group not found",
				"code":    404,
				"title":   "Not Found",
			},
		})
		return
	}

	c.Status(http.StatusNoContent)
}

// ListGroupUsers handles GET /v3/groups/:id/users
func (svc *Service) ListGroupUsers(c *gin.Context) {
	groupID := c.Param("id")

	rows, err := database.DB.Query(c.Request.Context(),
		`SELECT u.id, u.name, u.domain_id, u.enabled
		 FROM users u
		 INNER JOIN group_users gu ON u.id = gu.user_id
		 WHERE gu.group_id = $1`,
		groupID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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

	c.JSON(http.StatusOK, gin.H{"users": users})
}

// AddUserToGroup handles PUT /v3/groups/:id/users/:user_id
func (svc *Service) AddUserToGroup(c *gin.Context) {
	groupID := c.Param("id")
	userID := c.Param("user_id")

	_, err := database.DB.Exec(c.Request.Context(),
		`INSERT INTO group_users (group_id, user_id)
		 VALUES ($1, $2)
		 ON CONFLICT (group_id, user_id) DO NOTHING`,
		groupID, userID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

// RemoveUserFromGroup handles DELETE /v3/groups/:id/users/:user_id
func (svc *Service) RemoveUserFromGroup(c *gin.Context) {
	groupID := c.Param("id")
	userID := c.Param("user_id")

	result, err := database.DB.Exec(c.Request.Context(),
		"DELETE FROM group_users WHERE group_id = $1 AND user_id = $2",
		groupID, userID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"message": "User not in group",
				"code":    404,
			},
		})
		return
	}

	c.Status(http.StatusNoContent)
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
