package glance

import (
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// StageImageData stages image data before import
func (svc *Service) StageImageData(c *gin.Context) {
	imageID := c.Param("id")
	projectID := c.GetString("project_id")

	// Verify image exists and is owned by project
	var status string
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT status FROM images WHERE id = $1 AND project_id = $2",
		imageID, projectID,
	).Scan(&status)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"message": "Image not found"})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	// Can only stage to queued images
	if status != "queued" {
		c.JSON(http.StatusConflict, gin.H{"message": "Image is not in queued state"})
		return
	}

	// Read staged data (in real implementation, would save to staging area)
	stagedData, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Failed to read image data"})
		return
	}

	// Update image status to uploading
	_, err = svc.activeDB().Exec(c.Request.Context(), `
		UPDATE images
		SET status = $1, size_bytes = $2, updated_at = $3
		WHERE id = $4
	`, "uploading", len(stagedData), time.Now(), imageID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	// In stub mode, data is discarded
	// In real mode, would save to staging location
	_ = stagedData

	c.Status(http.StatusNoContent)
}

// ImportImage imports staged image data
func (svc *Service) ImportImage(c *gin.Context) {
	imageID := c.Param("id")
	projectID := c.GetString("project_id")

	// Parse import method from body
	var req struct {
		Method struct {
			Name string `json:"name"`
		} `json:"method"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid request body"})
		return
	}

	// Verify image exists
	var status string
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT status FROM images WHERE id = $1 AND project_id = $2",
		imageID, projectID,
	).Scan(&status)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"message": "Image not found"})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	// Import from staging to active storage
	// In stub mode, just mark as active
	// In real mode, would move from staging to backend storage
	_, err = svc.activeDB().Exec(c.Request.Context(), `
		UPDATE images
		SET status = $1, updated_at = $2
		WHERE id = $3
	`, "active", time.Now(), imageID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	c.Status(http.StatusAccepted)
}

// GetImageImportInfo returns available import methods for an image
func (svc *Service) GetImageImportInfo(c *gin.Context) {
	imageID := c.Param("id")
	projectID := c.GetString("project_id")

	// Verify image exists
	var exists bool
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM images WHERE id = $1 AND project_id = $2)",
		imageID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"message": "Image not found"})
		return
	}

	// Return available import methods
	c.JSON(http.StatusOK, gin.H{
		"import-methods": gin.H{
			"description": "Import methods available.",
			"type":        "array",
			"value": []string{
				"glance-direct",
				"web-download",
			},
		},
	})
}
