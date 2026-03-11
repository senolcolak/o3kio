package cinder

import (
	"fmt"
	"net/http"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ListBackups lists all volume backups
func (svc *Service) ListBackups(c *gin.Context) {
	projectID := c.GetString("project_id")

	rows, err := database.DB.Query(c.Request.Context(), `
		SELECT id, project_id, volume_id, name, description, status, size_gb, created_at, updated_at
		FROM volume_backups
		WHERE project_id = $1
		ORDER BY created_at DESC
	`, projectID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	backups := []map[string]interface{}{}
	for rows.Next() {
		var (
			id          string
			projID      string
			volumeID    string
			name        *string
			description *string
			status      string
			sizeGB      int
			createdAt   time.Time
			updatedAt   time.Time
		)

		err := rows.Scan(&id, &projID, &volumeID, &name, &description, &status, &sizeGB, &createdAt, &updatedAt)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		backup := map[string]interface{}{
			"id":          id,
			"volume_id":   volumeID,
			"name":        name,
			"description": description,
			"status":      status,
			"size":        sizeGB,
			"created_at":  createdAt.Format(time.RFC3339),
			"updated_at":  updatedAt.Format(time.RFC3339),
		}
		backups = append(backups, backup)
	}

	c.JSON(http.StatusOK, gin.H{"backups": backups})
}

// ListBackupsDetail lists all volume backups with full details
func (svc *Service) ListBackupsDetail(c *gin.Context) {
	// Same as ListBackups for now
	svc.ListBackups(c)
}

// CreateBackup creates a volume backup
func (svc *Service) CreateBackup(c *gin.Context) {
	projectID := c.GetString("project_id")

	var req struct {
		Backup struct {
			VolumeID    string  `json:"volume_id" binding:"required"`
			Name        *string `json:"name"`
			Description *string `json:"description"`
		} `json:"backup" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if volume exists and get its size
	var volumeSize int
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT size_gb FROM volumes WHERE id = $1 AND project_id = $2",
		req.Backup.VolumeID, projectID,
	).Scan(&volumeSize)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "volume not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	backupID := uuid.New().String()
	now := time.Now()

	_, err = database.DB.Exec(c.Request.Context(), `
		INSERT INTO volume_backups (id, project_id, volume_id, name, description, status, size_gb, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, backupID, projectID, req.Backup.VolumeID, req.Backup.Name, req.Backup.Description, "available", volumeSize, now, now)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"backup": map[string]interface{}{
			"id":          backupID,
			"volume_id":   req.Backup.VolumeID,
			"name":        req.Backup.Name,
			"description": req.Backup.Description,
			"status":      "available",
			"size":        volumeSize,
			"created_at":  now.Format(time.RFC3339),
			"updated_at":  now.Format(time.RFC3339),
		},
	})
}

// GetBackup retrieves a specific backup
func (svc *Service) GetBackup(c *gin.Context) {
	backupID := c.Param("id")
	projectID := c.GetString("project_id")

	var (
		volumeID    string
		name        *string
		description *string
		status      string
		sizeGB      int
		createdAt   time.Time
		updatedAt   time.Time
	)

	err := database.DB.QueryRow(c.Request.Context(), `
		SELECT volume_id, name, description, status, size_gb, created_at, updated_at
		FROM volume_backups
		WHERE id = $1 AND project_id = $2
	`, backupID, projectID).Scan(&volumeID, &name, &description, &status, &sizeGB, &createdAt, &updatedAt)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "backup not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"backup": map[string]interface{}{
			"id":          backupID,
			"volume_id":   volumeID,
			"name":        name,
			"description": description,
			"status":      status,
			"size":        sizeGB,
			"created_at":  createdAt.Format(time.RFC3339),
			"updated_at":  updatedAt.Format(time.RFC3339),
		},
	})
}

// DeleteBackup deletes a backup
func (svc *Service) DeleteBackup(c *gin.Context) {
	backupID := c.Param("id")
	projectID := c.GetString("project_id")

	result, err := database.DB.Exec(c.Request.Context(),
		"DELETE FROM volume_backups WHERE id = $1 AND project_id = $2",
		backupID, projectID,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if result.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "backup not found"})
		return
	}

	c.Status(http.StatusAccepted)
}

// RestoreBackup restores a volume from backup
func (svc *Service) RestoreBackup(c *gin.Context) {
	backupID := c.Param("id")
	projectID := c.GetString("project_id")

	var req struct {
		Restore struct {
			Name     *string `json:"name"`
			VolumeID *string `json:"volume_id"`
		} `json:"restore" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get backup details
	var (
		volumeID string
		sizeGB   int
	)

	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT volume_id, size_gb FROM volume_backups WHERE id = $1 AND project_id = $2",
		backupID, projectID,
	).Scan(&volumeID, &sizeGB)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "backup not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// If volume_id specified, restore to existing volume
	restoredVolumeID := volumeID
	if req.Restore.VolumeID != nil {
		restoredVolumeID = *req.Restore.VolumeID

		// Verify target volume exists
		var exists bool
		err = database.DB.QueryRow(c.Request.Context(),
			"SELECT EXISTS(SELECT 1 FROM volumes WHERE id = $1 AND project_id = $2)",
			restoredVolumeID, projectID,
		).Scan(&exists)

		if err != nil || !exists {
			c.JSON(http.StatusNotFound, gin.H{"error": "target volume not found"})
			return
		}
	} else {
		// Create new volume for restore
		restoredVolumeID = uuid.New().String()
		now := time.Now()

		volumeName := fmt.Sprintf("restored-from-%s", backupID[:8])
		if req.Restore.Name != nil {
			volumeName = *req.Restore.Name
		}

		_, err = database.DB.Exec(c.Request.Context(), `
			INSERT INTO volumes (id, project_id, name, description, size_gb, status, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, restoredVolumeID, projectID, volumeName, "Restored from backup", sizeGB, "available", now, now)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusAccepted, gin.H{
		"restore": map[string]interface{}{
			"backup_id": backupID,
			"volume_id": restoredVolumeID,
		},
	})
}
