package nova

import (
	"net/http"

	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/gin-gonic/gin"
)

// ListServerTags returns all tags for a server
func (svc *Service) ListServerTags(c *gin.Context) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	// Verify server exists
	var exists bool
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM instances WHERE id = $1 AND project_id = $2)",
		instanceID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}

	// Get tags
	rows, err := database.DB.Query(c.Request.Context(),
		"SELECT tag FROM server_tags WHERE instance_id = $1 ORDER BY tag",
		instanceID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Verify server exists
	var exists bool
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM instances WHERE id = $1 AND project_id = $2)",
		instanceID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}

	// Delete all existing tags
	_, err = database.DB.Exec(c.Request.Context(),
		"DELETE FROM server_tags WHERE instance_id = $1",
		instanceID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Insert new tags
	for _, tag := range req.Tags {
		_, err = database.DB.Exec(c.Request.Context(),
			"INSERT INTO server_tags (instance_id, tag) VALUES ($1, $2) ON CONFLICT DO NOTHING",
			instanceID, tag,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM instances WHERE id = $1 AND project_id = $2)",
		instanceID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}

	// Insert tag
	_, err = database.DB.Exec(c.Request.Context(),
		"INSERT INTO server_tags (instance_id, tag) VALUES ($1, $2) ON CONFLICT DO NOTHING",
		instanceID, tag,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM instances WHERE id = $1 AND project_id = $2)",
		instanceID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}

	// Delete tag
	result, err := database.DB.Exec(c.Request.Context(),
		"DELETE FROM server_tags WHERE instance_id = $1 AND tag = $2",
		instanceID, tag,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if result.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "tag not found"})
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
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM instances WHERE id = $1 AND project_id = $2)",
		instanceID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}

	// Delete all tags
	_, err = database.DB.Exec(c.Request.Context(),
		"DELETE FROM server_tags WHERE instance_id = $1",
		instanceID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}
