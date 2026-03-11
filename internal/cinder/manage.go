package cinder

import (
	"net/http"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ManageVolume handles POST /v3/:project_id/os-volume-manage
func (svc *Service) ManageVolume(c *gin.Context) {
	projectID := c.GetString("project_id")

	var req struct {
		Volume struct {
			Host       string                 `json:"host" binding:"required"`
			Ref        map[string]interface{} `json:"ref" binding:"required"`
			Name       string                 `json:"name"`
			VolumeType string                 `json:"volume_type"`
			Metadata   map[string]string      `json:"metadata"`
		} `json:"volume" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Create volume from existing storage
	volumeID := uuid.New()

	// Default size for managed volumes (would query backend in real implementation)
	sizeGB := 1

	_, err := database.DB.Exec(c.Request.Context(), `
		INSERT INTO volumes (id, name, size_gb, status, project_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, volumeID, req.Volume.Name, sizeGB, "available", projectID, time.Now(), time.Now())

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	volume := map[string]interface{}{
		"id":                volumeID.String(),
		"name":              req.Volume.Name,
		"size":              sizeGB,
		"status":            "available",
		"volume_type":       req.Volume.VolumeType,
		"host":              req.Volume.Host,
		"availability_zone": "nova",
		"created_at":        time.Now().Format(time.RFC3339),
	}

	c.JSON(http.StatusAccepted, gin.H{"volume": volume})
}

// ListManageableVolumes handles GET /v3/:project_id/manageable_volumes
func (svc *Service) ListManageableVolumes(c *gin.Context) {
	host := c.Query("host")

	if host == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "host parameter is required"})
		return
	}

	// In stub mode, return empty list
	// In real mode, would query storage backend for unmanaged volumes
	manageableVolumes := []map[string]interface{}{}

	c.JSON(http.StatusOK, gin.H{"manageable-volumes": manageableVolumes})
}

// UnmanageVolume handles volume action: os-unmanage
func (svc *Service) UnmanageVolume(c *gin.Context, volumeID string) {
	projectID := c.GetString("project_id")

	// Verify volume exists and belongs to project
	var id uuid.UUID
	var status string

	err := database.DB.QueryRow(c.Request.Context(), `
		SELECT id, status
		FROM volumes
		WHERE id = $1 AND project_id = $2
	`, volumeID, projectID).Scan(&id, &status)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Volume not found"})
		return
	}

	// Cannot unmanage attached volumes
	if status == "in-use" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot unmanage volume in use"})
		return
	}

	// Remove from database (in real implementation, would leave on backend)
	_, err = database.DB.Exec(c.Request.Context(),
		"DELETE FROM volumes WHERE id = $1 AND project_id = $2",
		volumeID, projectID,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusAccepted)
}

// ManageSnapshot handles POST /v3/:project_id/os-snapshot-manage
func (svc *Service) ManageSnapshot(c *gin.Context) {
	projectID := c.GetString("project_id")

	var req struct {
		Snapshot struct {
			VolumeID string                 `json:"volume_id" binding:"required"`
			Ref      map[string]interface{} `json:"ref" binding:"required"`
			Name     string                 `json:"name"`
			Metadata map[string]string      `json:"metadata"`
		} `json:"snapshot" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Verify volume exists
	var volumeExists bool
	err := database.DB.QueryRow(c.Request.Context(), `
		SELECT EXISTS(SELECT 1 FROM volumes WHERE id = $1 AND project_id = $2)
	`, req.Snapshot.VolumeID, projectID).Scan(&volumeExists)

	if err != nil || !volumeExists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Volume not found"})
		return
	}

	// Create snapshot from existing storage
	snapshotID := uuid.New()

	// Default size (would query backend in real implementation)
	sizeGB := 1

	_, err = database.DB.Exec(c.Request.Context(), `
		INSERT INTO snapshots (id, name, volume_id, size_gb, status, project_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, snapshotID, req.Snapshot.Name, req.Snapshot.VolumeID, sizeGB, "available", projectID, time.Now(), time.Now())

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	snapshot := map[string]interface{}{
		"id":         snapshotID.String(),
		"name":       req.Snapshot.Name,
		"volume_id":  req.Snapshot.VolumeID,
		"size":       sizeGB,
		"status":     "available",
		"created_at": time.Now().Format(time.RFC3339),
	}

	c.JSON(http.StatusAccepted, gin.H{"snapshot": snapshot})
}
