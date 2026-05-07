package nova

import (
	"net/http"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/common"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// ListServerGroups handles GET /v2.1/os-server-groups
func (svc *Service) ListServerGroups(c *gin.Context) {
	projectID := c.GetString("project_id")

	rows, err := svc.activeDB().Query(c.Request.Context(),
		`SELECT id, name, policies, members, project_id, created_at, updated_at
		 FROM server_groups WHERE project_id = $1`,
		projectID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "list_server_groups").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to list server groups"))
		return
	}
	defer rows.Close()

	serverGroups := []gin.H{}
	for rows.Next() {
		var id, name, projectID string
		var policies []string
		var members []string
		var createdAt, updatedAt time.Time

		err := rows.Scan(&id, &name, &policies, &members, &projectID, &createdAt, &updatedAt)
		if err != nil {
			continue
		}

		serverGroups = append(serverGroups, gin.H{
			"id":         id,
			"name":       name,
			"policies":   policies,
			"members":    members,
			"metadata":   gin.H{},
			"project_id": projectID,
		})
	}
	if err := rows.Err(); err != nil {
		log.Error().Err(err).Str("operation", "list_server_groups").Msg("rows iteration error")
		common.SendError(c, common.NewInternalServerError("failed to list server groups"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"server_groups": serverGroups})
}

// CreateServerGroup handles POST /v2.1/os-server-groups
func (svc *Service) CreateServerGroup(c *gin.Context) {
	projectID := c.GetString("project_id")

	var req struct {
		ServerGroup struct {
			Name     string   `json:"name" binding:"required"`
			Policies []string `json:"policies" binding:"required"`
		} `json:"server_group" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	groupID := uuid.New().String()
	now := time.Now()

	_, err := svc.activeDB().Exec(c.Request.Context(),
		`INSERT INTO server_groups (id, name, policies, members, project_id, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		groupID, req.ServerGroup.Name, req.ServerGroup.Policies, []string{}, projectID, now, now,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "create_server_group").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to create server group"))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"server_group": gin.H{
			"id":         groupID,
			"name":       req.ServerGroup.Name,
			"policies":   req.ServerGroup.Policies,
			"members":    []string{},
			"metadata":   gin.H{},
			"project_id": projectID,
		},
	})
}

// GetServerGroup handles GET /v2.1/os-server-groups/:id
func (svc *Service) GetServerGroup(c *gin.Context) {
	groupID := c.Param("id")
	projectID := c.GetString("project_id")

	var name, projectIDFromDB string
	var policies []string
	var members []string
	var createdAt, updatedAt time.Time

	err := svc.activeDB().QueryRow(c.Request.Context(),
		`SELECT name, policies, members, project_id, created_at, updated_at
		 FROM server_groups WHERE id = $1 AND project_id = $2`,
		groupID, projectID,
	).Scan(&name, &policies, &members, &projectIDFromDB, &createdAt, &updatedAt)

	if err != nil {
		common.SendError(c, common.NewNotFoundError("server group"))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"server_group": gin.H{
			"id":         groupID,
			"name":       name,
			"policies":   policies,
			"members":    members,
			"metadata":   gin.H{},
			"project_id": projectIDFromDB,
		},
	})
}

// DeleteServerGroup handles DELETE /v2.1/os-server-groups/:id
func (svc *Service) DeleteServerGroup(c *gin.Context) {
	groupID := c.Param("id")
	projectID := c.GetString("project_id")

	result, err := svc.activeDB().Exec(c.Request.Context(),
		"DELETE FROM server_groups WHERE id = $1 AND project_id = $2",
		groupID, projectID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "delete_server_group").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to delete server group"))
		return
	}

	if result.RowsAffected() == 0 {
		common.SendError(c, common.NewNotFoundError("server group"))
		return
	}

	c.Status(http.StatusNoContent)
}
