package glance

import (
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/common"
	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/gin-gonic/gin"
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

	if errors.Is(err, database.ErrNoRows) {
		common.SendError(c, common.NewNotFoundError("image"))
		return
	}

	if err != nil {
		common.SendError(c, common.NewInternalServerError("failed to query image"))
		return
	}

	// Can only stage to queued images
	if status != "queued" {
		common.SendError(c, common.NewConflictError("image must be in queued state to stage data"))
		return
	}

	// Limit upload size to 5 GB
	const maxUploadSize int64 = 5 * 1024 * 1024 * 1024
	limitedReader := io.LimitReader(c.Request.Body, maxUploadSize)

	// Upload directly to storage backend so data survives the request
	size, err := svc.imageStore.UploadImage(c.Request.Context(), imageID, limitedReader)
	if err != nil {
		common.SendError(c, common.NewInternalServerError("failed to stage image data"))
		return
	}

	// Mark image as uploading (staged, awaiting import confirmation)
	_, err = svc.activeDB().Exec(c.Request.Context(),
		"UPDATE images SET status = $1, size_bytes = $2, updated_at = $3 WHERE id = $4",
		"uploading", size, time.Now(), imageID,
	)
	if err != nil {
		common.SendError(c, common.NewInternalServerError("failed to update image status"))
		return
	}

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

	if errors.Is(err, database.ErrNoRows) {
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
