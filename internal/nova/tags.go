package nova

import (
	"net/http"

	"github.com/cobaltcore-dev/o3k/internal/common"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// ListServerTags returns all tags for a server
func (svc *Service) ListServerTags(c *gin.Context) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	// Verify server exists
	var exists bool
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM instances WHERE id = $1 AND project_id = $2)",
		instanceID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		common.SendError(c, common.NewNotFoundError("server"))
		return
	}

	// Get tags
	rows, err := svc.activeDB().Query(c.Request.Context(),
		"SELECT tag FROM server_tags WHERE instance_id = $1 ORDER BY tag",
		instanceID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "list_server_tags").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to list server tags"))
		return
	}
	defer rows.Close()

	tags := []string{}
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err == nil {
			tags = append(tags, tag)
		}
	}
	if err := rows.Err(); err != nil {
		log.Error().Err(err).Str("operation", "list_server_tags").Msg("rows iteration error")
		common.SendError(c, common.NewInternalServerError("failed to list server tags"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"tags": tags})
}

// ReplaceServerTags replaces all server tags
func (svc *Service) ReplaceServerTags(c *gin.Context) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	var req struct {
		Tags []string `json:"tags" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Verify server exists
	var exists bool
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM instances WHERE id = $1 AND project_id = $2)",
		instanceID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		common.SendError(c, common.NewNotFoundError("server"))
		return
	}

	// Delete all existing tags
	_, err = svc.activeDB().Exec(c.Request.Context(),
		"DELETE FROM server_tags WHERE instance_id = $1",
		instanceID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "delete_server_tags").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to delete server tags"))
		return
	}

	// Insert new tags
	for _, tag := range req.Tags {
		_, err = svc.activeDB().Exec(c.Request.Context(),
			"INSERT INTO server_tags (instance_id, tag) VALUES ($1, $2) ON CONFLICT DO NOTHING",
			instanceID, tag,
		)
		if err != nil {
			log.Error().Err(err).Str("operation", "insert_server_tag").Msg("database error")
			common.SendError(c, common.NewInternalServerError("failed to insert server tag"))
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"tags": req.Tags})
}

// AddServerTag adds a single tag to a server
func (svc *Service) AddServerTag(c *gin.Context) {
	instanceID := c.Param("id")
	tag := c.Param("tag")
	projectID := c.GetString("project_id")

	// Verify server exists
	var exists bool
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM instances WHERE id = $1 AND project_id = $2)",
		instanceID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		common.SendError(c, common.NewNotFoundError("server"))
		return
	}

	// Insert tag
	_, err = svc.activeDB().Exec(c.Request.Context(),
		"INSERT INTO server_tags (instance_id, tag) VALUES ($1, $2) ON CONFLICT DO NOTHING",
		instanceID, tag,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "add_server_tag").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to add server tag"))
		return
	}

	c.Status(http.StatusCreated)
}

// DeleteServerTag removes a single tag from a server
func (svc *Service) DeleteServerTag(c *gin.Context) {
	instanceID := c.Param("id")
	tag := c.Param("tag")
	projectID := c.GetString("project_id")

	// Verify server exists
	var exists bool
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM instances WHERE id = $1 AND project_id = $2)",
		instanceID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		common.SendError(c, common.NewNotFoundError("server"))
		return
	}

	// Delete tag
	result, err := svc.activeDB().Exec(c.Request.Context(),
		"DELETE FROM server_tags WHERE instance_id = $1 AND tag = $2",
		instanceID, tag,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "delete_server_tag").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to delete server tag"))
		return
	}

	if result.RowsAffected() == 0 {
		common.SendError(c, common.NewNotFoundError("tag"))
		return
	}

	c.Status(http.StatusNoContent)
}

// DeleteAllServerTags removes all tags from a server
func (svc *Service) DeleteAllServerTags(c *gin.Context) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	// Verify server exists
	var exists bool
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM instances WHERE id = $1 AND project_id = $2)",
		instanceID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		common.SendError(c, common.NewNotFoundError("server"))
		return
	}

	// Delete all tags
	_, err = svc.activeDB().Exec(c.Request.Context(),
		"DELETE FROM server_tags WHERE instance_id = $1",
		instanceID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "delete_all_server_tags").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to delete server tags"))
		return
	}

	c.Status(http.StatusNoContent)
}
