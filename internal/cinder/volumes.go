package cinder

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/cobaltcore-dev/o3k/pkg/storage"
)

// Service handles Cinder API endpoints
type Service struct {
	mode       string
	cephPool   string
	cephConf   string
	cephClient *storage.CephClient
}

// NewService creates a new Cinder service
func NewService(mode, cephPool, cephConf string) *Service {
	return &Service{
		mode:       mode,
		cephPool:   cephPool,
		cephConf:   cephConf,
		cephClient: storage.NewCephClient(mode, cephPool, cephConf),
	}
}

// RegisterRoutes registers Cinder routes
func (svc *Service) RegisterRoutes(r *gin.RouterGroup) {
	v3 := r.Group("/v3/:project_id")
	{
		// Volumes
		v3.GET("/volumes", svc.ListVolumes)
		v3.GET("/volumes/detail", svc.ListVolumesDetail)
		v3.POST("/volumes", svc.CreateVolume)
		v3.GET("/volumes/:id", svc.GetVolume)
		v3.PATCH("/volumes/:id", svc.UpdateVolume)
		v3.DELETE("/volumes/:id", svc.DeleteVolume)
		v3.POST("/volumes/:id/action", svc.VolumeAction)

		// Volume metadata
		v3.GET("/volumes/:id/metadata", svc.GetVolumeMetadata)
		v3.POST("/volumes/:id/metadata", svc.SetVolumeMetadata)
		v3.GET("/volumes/:id/metadata/:key", svc.GetVolumeMetadataKey)
		v3.PUT("/volumes/:id/metadata/:key", svc.UpdateVolumeMetadataKey)
		v3.DELETE("/volumes/:id/metadata/:key", svc.DeleteVolumeMetadataKey)

		// Snapshots
		v3.GET("/snapshots", svc.ListSnapshots)
		v3.POST("/snapshots", svc.CreateSnapshot)
		v3.GET("/snapshots/:id", svc.GetSnapshot)
		v3.PATCH("/snapshots/:id", svc.UpdateSnapshot)
		v3.DELETE("/snapshots/:id", svc.DeleteSnapshot)

		// Snapshot metadata
		v3.GET("/snapshots/:id/metadata", svc.GetSnapshotMetadata)
		v3.POST("/snapshots/:id/metadata", svc.SetSnapshotMetadata)
		v3.GET("/snapshots/:id/metadata/:key", svc.GetSnapshotMetadataKey)
		v3.PUT("/snapshots/:id/metadata/:key", svc.UpdateSnapshotMetadataKey)
		v3.DELETE("/snapshots/:id/metadata/:key", svc.DeleteSnapshotMetadataKey)

		// Volume types
		v3.GET("/types", svc.ListVolumeTypes)
		v3.GET("/types/:id", svc.GetVolumeType)

		// Limits
		v3.GET("/limits", svc.GetLimits)

		// Services
		v3.GET("/os-services", svc.ListServices)

		// Backups
		v3.GET("/backups", svc.ListBackups)
		v3.GET("/backups/detail", svc.ListBackupsDetail)
		v3.POST("/backups", svc.CreateBackup)
		v3.GET("/backups/:id", svc.GetBackup)
		v3.DELETE("/backups/:id", svc.DeleteBackup)
		v3.POST("/backups/:id/restore", svc.RestoreBackup)
	}
}

// CreateVolumeRequest represents a volume creation request
type CreateVolumeRequest struct {
	Volume struct {
		Name        string `json:"name"`
		Size        int    `json:"size" binding:"required"`
		Description string `json:"description"`
		VolumeType  string `json:"volume_type"`
		SnapshotID  string `json:"snapshot_id"`
		SourceVolID string `json:"source_volid"`
		ImageRef    string `json:"imageRef"`
	} `json:"volume"`
}

// CreateVolume creates a new volume
func (svc *Service) CreateVolume(c *gin.Context) {
	var req CreateVolumeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	projectID := c.Param("project_id")
	userID := c.GetString("user_id")
	volumeID := uuid.New().String()

	if req.Volume.Size < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "volume size must be at least 1 GB"})
		return
	}

	// Create RBD volume in Ceph
	if err := svc.cephClient.CreateVolume(c.Request.Context(), volumeID, req.Volume.Size); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": fmt.Sprintf("failed to create volume in Ceph: %v", err)})
		return
	}

	// Insert into database
	now := time.Now()
	_, err := database.DB.Exec(c.Request.Context(), `
		INSERT INTO volumes (id, name, project_id, user_id, size_gb, status, bootable, rbd_pool, rbd_image, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, volumeID, req.Volume.Name, projectID, userID, req.Volume.Size, "creating", false, svc.cephPool, "volume-"+volumeID, now, now)

	if err != nil {
		// Rollback: delete from Ceph
		svc.cephClient.DeleteVolume(c.Request.Context(), volumeID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Update status to available
	go func() {
		time.Sleep(100 * time.Millisecond)
		database.DB.Exec(c.Request.Context(),
			"UPDATE volumes SET status = $1, updated_at = $2 WHERE id = $3",
			"available", time.Now(), volumeID)
	}()

	c.JSON(http.StatusAccepted, gin.H{
		"volume": gin.H{
			"id":          volumeID,
			"name":        req.Volume.Name,
			"tenant_id":   projectID,
			"user_id":     userID,
			"size":        req.Volume.Size,
			"status":      "creating",
			"bootable":    "false",
			"created_at":  now.Format(time.RFC3339),
			"updated_at":  now.Format(time.RFC3339),
			"metadata":    gin.H{},
			"attachments": []interface{}{},
		},
	})
}

// ListVolumes lists all volumes (brief)
func (svc *Service) ListVolumes(c *gin.Context) {
	projectID := c.Param("project_id")

	rows, err := database.DB.Query(c.Request.Context(), `
		SELECT id, name, size_gb
		FROM volumes
		WHERE project_id = $1
		ORDER BY created_at DESC
	`, projectID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var volumes []gin.H
	for rows.Next() {
		var id, name string
		var size int

		if err := rows.Scan(&id, &name, &size); err != nil {
			continue
		}

		volumes = append(volumes, gin.H{
			"id":   id,
			"name": name,
			"size": size,
		})
	}

	if volumes == nil {
		volumes = []gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{"volumes": volumes})
}

// ListVolumesDetail lists all volumes (detailed)
func (svc *Service) ListVolumesDetail(c *gin.Context) {
	projectID := c.Param("project_id")

	rows, err := database.DB.Query(c.Request.Context(), `
		SELECT v.id, v.name, v.size_gb, v.status, v.bootable, v.attached_to_instance_id, v.created_at, v.updated_at
		FROM volumes v
		WHERE v.project_id = $1
		ORDER BY v.created_at DESC
	`, projectID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var volumes []gin.H
	for rows.Next() {
		var id, name, status string
		var size int
		var bootable bool
		var attachedTo sql.NullString
		var createdAt, updatedAt time.Time

		if err := rows.Scan(&id, &name, &size, &status, &bootable, &attachedTo, &createdAt, &updatedAt); err != nil {
			continue
		}

		attachments := []interface{}{}
		if attachedTo.Valid {
			attachments = append(attachments, gin.H{
				"server_id": attachedTo.String,
				"device":    "/dev/vdb",
			})
		}

		volumes = append(volumes, gin.H{
			"id":          id,
			"name":        name,
			"tenant_id":   projectID,
			"size":        size,
			"status":      status,
			"bootable":    fmt.Sprintf("%t", bootable),
			"created_at":  createdAt.Format(time.RFC3339),
			"updated_at":  updatedAt.Format(time.RFC3339),
			"attachments": attachments,
		})
	}

	if volumes == nil {
		volumes = []gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{"volumes": volumes})
}

// GetVolume returns a single volume
func (svc *Service) GetVolume(c *gin.Context) {
	volumeID := c.Param("id")
	projectID := c.Param("project_id")

	var id, name, status string
	var size int
	var bootable bool
	var attachedTo sql.NullString
	var createdAt, updatedAt time.Time

	// Support lookup by ID or name
	err := database.DB.QueryRow(c.Request.Context(), `
		SELECT id, name, size_gb, status, bootable, attached_to_instance_id, created_at, updated_at
		FROM volumes
		WHERE project_id = $2 AND ((id::text = $1) OR (name = $1))
	`, volumeID, projectID).Scan(&id, &name, &size, &status, &bootable, &attachedTo, &createdAt, &updatedAt)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "volume not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	attachments := []interface{}{}
	if attachedTo.Valid {
		attachments = append(attachments, gin.H{
			"server_id": attachedTo.String,
			"device":    "/dev/vdb",
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"volume": gin.H{
			"id":          id,
			"name":        name,
			"tenant_id":   projectID,
			"size":        size,
			"status":      status,
			"bootable":    fmt.Sprintf("%t", bootable),
			"created_at":  createdAt.Format(time.RFC3339),
			"updated_at":  updatedAt.Format(time.RFC3339),
			"attachments": attachments,
		},
	})
}

// DeleteVolume deletes a volume
func (svc *Service) DeleteVolume(c *gin.Context) {
	volumeID := c.Param("id")
	projectID := c.Param("project_id")

	// Check if volume is attached (support lookup by ID or name)
	var attachedTo sql.NullString
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT attached_to_instance_id FROM volumes WHERE project_id = $2 AND ((id::text = $1) OR (name = $1))",
		volumeID, projectID,
	).Scan(&attachedTo)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "volume not found"})
		return
	}

	if attachedTo.Valid {
		c.JSON(http.StatusBadRequest, gin.H{"error": "volume is attached to an instance"})
		return
	}

	// Delete from Ceph
	if err := svc.cephClient.DeleteVolume(c.Request.Context(), volumeID); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": fmt.Sprintf("failed to delete volume from Ceph: %v", err)})
		return
	}

	// Delete from database
	_, err = database.DB.Exec(c.Request.Context(),
		"DELETE FROM volumes WHERE id = $1 AND project_id = $2",
		volumeID, projectID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

// VolumeAction performs an action on a volume
func (svc *Service) VolumeAction(c *gin.Context) {
	volumeID := c.Param("id")
	projectID := c.Param("project_id")

	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Handle attach action
	if attachData, ok := req["os-attach"]; ok {
		attachMap := attachData.(map[string]interface{})
		instanceID := attachMap["instance_uuid"].(string)

		// Update volume to attached status
		_, err := database.DB.Exec(c.Request.Context(), `
			UPDATE volumes
			SET attached_to_instance_id = $1, status = $2, updated_at = $3
			WHERE id = $4 AND project_id = $5
		`, instanceID, "in-use", time.Now(), volumeID, projectID)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.Status(http.StatusAccepted)
		return
	}

	// Handle detach action
	if _, ok := req["os-detach"]; ok {
		// Update volume to available status
		_, err := database.DB.Exec(c.Request.Context(), `
			UPDATE volumes
			SET attached_to_instance_id = NULL, status = $1, updated_at = $2
			WHERE id = $3 AND project_id = $4
		`, "available", time.Now(), volumeID, projectID)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.Status(http.StatusAccepted)
		return
	}

	// Handle extend action
	if extendData, ok := req["os-extend"]; ok {
		extendMap := extendData.(map[string]interface{})
		var newSize int

		// Handle different JSON number types
		newSizeVal := extendMap["new_size"]
		switch v := newSizeVal.(type) {
		case float64:
			newSize = int(v)
		case int:
			newSize = v
		case int64:
			newSize = int(v)
		case json.Number:
			parsed, err := v.Int64()
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid new_size format"})
				return
			}
			newSize = int(parsed)
		case string:
			parsed, err := strconv.Atoi(v)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid new_size format"})
				return
			}
			newSize = parsed
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid new_size type: %T", newSizeVal)})
			return
		}

		// Update volume size
		_, err := database.DB.Exec(c.Request.Context(), `
			UPDATE volumes
			SET size_gb = $1, updated_at = $2
			WHERE id = $3 AND project_id = $4
		`, newSize, time.Now(), volumeID, projectID)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.Status(http.StatusAccepted)
		return
	}

	// Handle retype action
	if retypeData, ok := req["os-retype"]; ok {
		retypeMap := retypeData.(map[string]interface{})
		newType := retypeMap["new_type"].(string)

		// Get or create volume type
		var typeID string
		err := database.DB.QueryRow(c.Request.Context(),
			"SELECT id FROM volume_types WHERE name = $1",
			newType,
		).Scan(&typeID)

		if err == pgx.ErrNoRows {
			// Create new volume type if it doesn't exist
			typeID = uuid.New().String()
			_, err = database.DB.Exec(c.Request.Context(),
				"INSERT INTO volume_types (id, name, description, is_public) VALUES ($1, $2, $3, $4)",
				typeID, newType, "Auto-created volume type", true,
			)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}

		// Update volume type
		_, err = database.DB.Exec(c.Request.Context(), `
			UPDATE volumes
			SET volume_type = $1, updated_at = $2
			WHERE id = $3 AND project_id = $4
		`, newType, time.Now(), volumeID, projectID)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.Status(http.StatusAccepted)
		return
	}

	c.JSON(http.StatusBadRequest, gin.H{"error": "unknown action"})
}

// Snapshot operations

// CreateSnapshotRequest represents a snapshot creation request
type CreateSnapshotRequest struct {
	Snapshot struct {
		Name        string `json:"name"`
		VolumeID    string `json:"volume_id" binding:"required"`
		Description string `json:"description"`
		Force       bool   `json:"force"`
	} `json:"snapshot"`
}

// CreateSnapshot creates a new snapshot
func (svc *Service) CreateSnapshot(c *gin.Context) {
	var req CreateSnapshotRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	projectID := c.Param("project_id")
	snapshotID := uuid.New().String()

	// Get volume info
	var volumeID string
	var size int
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT id, size_gb FROM volumes WHERE id = $1 AND project_id = $2",
		req.Snapshot.VolumeID, projectID,
	).Scan(&volumeID, &size)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "volume not found"})
		return
	}

	// Create snapshot in Ceph
	if err := svc.cephClient.CreateSnapshot(c.Request.Context(), volumeID, snapshotID); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": fmt.Sprintf("failed to create snapshot: %v", err)})
		return
	}

	// Insert into database
	now := time.Now()
	_, err = database.DB.Exec(c.Request.Context(), `
		INSERT INTO snapshots (id, name, volume_id, project_id, size_gb, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, snapshotID, req.Snapshot.Name, volumeID, projectID, size, "creating", now)

	if err != nil {
		svc.cephClient.DeleteSnapshot(c.Request.Context(), volumeID, snapshotID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Update status to available
	go func() {
		time.Sleep(100 * time.Millisecond)
		database.DB.Exec(c.Request.Context(),
			"UPDATE snapshots SET status = $1 WHERE id = $2",
			"available", snapshotID)
	}()

	c.JSON(http.StatusAccepted, gin.H{
		"snapshot": gin.H{
			"id":         snapshotID,
			"name":       req.Snapshot.Name,
			"volume_id":  volumeID,
			"size":       size,
			"status":     "creating",
			"created_at": now.Format(time.RFC3339),
		},
	})
}

// ListSnapshots lists all snapshots
func (svc *Service) ListSnapshots(c *gin.Context) {
	projectID := c.Param("project_id")

	rows, err := database.DB.Query(c.Request.Context(), `
		SELECT id, name, volume_id, size_gb, status, created_at
		FROM snapshots
		WHERE project_id = $1
		ORDER BY created_at DESC
	`, projectID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var snapshots []gin.H
	for rows.Next() {
		var id, name, volumeID, status string
		var size int
		var createdAt time.Time

		if err := rows.Scan(&id, &name, &volumeID, &size, &status, &createdAt); err != nil {
			continue
		}

		snapshots = append(snapshots, gin.H{
			"id":         id,
			"name":       name,
			"volume_id":  volumeID,
			"size":       size,
			"status":     status,
			"created_at": createdAt.Format(time.RFC3339),
		})
	}

	if snapshots == nil {
		snapshots = []gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{"snapshots": snapshots})
}

// GetSnapshot returns a single snapshot
func (svc *Service) GetSnapshot(c *gin.Context) {
	snapshotID := c.Param("id")
	projectID := c.Param("project_id")

	var id, name, volumeID, status string
	var size int
	var createdAt time.Time

	err := database.DB.QueryRow(c.Request.Context(), `
		SELECT id, name, volume_id, size_gb, status, created_at
		FROM snapshots
		WHERE id = $1 AND project_id = $2
	`, snapshotID, projectID).Scan(&id, &name, &volumeID, &size, &status, &createdAt)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "snapshot not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"snapshot": gin.H{
			"id":         id,
			"name":       name,
			"volume_id":  volumeID,
			"size":       size,
			"status":     status,
			"created_at": createdAt.Format(time.RFC3339),
		},
	})
}

// DeleteSnapshot deletes a snapshot
func (svc *Service) DeleteSnapshot(c *gin.Context) {
	snapshotID := c.Param("id")
	projectID := c.Param("project_id")

	// Get volume ID
	var volumeID string
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT volume_id FROM snapshots WHERE id = $1 AND project_id = $2",
		snapshotID, projectID,
	).Scan(&volumeID)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "snapshot not found"})
		return
	}

	// Delete from Ceph
	if err := svc.cephClient.DeleteSnapshot(c.Request.Context(), volumeID, snapshotID); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": fmt.Sprintf("failed to delete snapshot: %v", err)})
		return
	}

	// Delete from database
	_, err = database.DB.Exec(c.Request.Context(),
		"DELETE FROM snapshots WHERE id = $1 AND project_id = $2",
		snapshotID, projectID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

// Volume types

// ListVolumeTypes lists all volume types
func (svc *Service) ListVolumeTypes(c *gin.Context) {
	rows, err := database.DB.Query(c.Request.Context(), `
		SELECT id, name, description, is_public, created_at
		FROM volume_types
		ORDER BY name
	`)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var types []gin.H
	for rows.Next() {
		var id, name, description string
		var isPublic bool
		var createdAt time.Time

		if err := rows.Scan(&id, &name, &description, &isPublic, &createdAt); err != nil {
			continue
		}

		types = append(types, gin.H{
			"id":          id,
			"name":        name,
			"description": description,
			"is_public":   isPublic,
		})
	}

	if types == nil {
		types = []gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{"volume_types": types})
}

// GetVolumeType returns a single volume type
func (svc *Service) GetVolumeType(c *gin.Context) {
	typeID := c.Param("id")

	var id, name, description string
	var isPublic bool

	err := database.DB.QueryRow(c.Request.Context(), `
		SELECT id, name, description, is_public
		FROM volume_types
		WHERE id = $1
	`, typeID).Scan(&id, &name, &description, &isPublic)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "volume type not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"volume_type": gin.H{
			"id":          id,
			"name":        name,
			"description": description,
			"is_public":   isPublic,
		},
	})
}

// UpdateVolume updates volume properties
func (svc *Service) UpdateVolume(c *gin.Context) {
	volumeID := c.Param("id")
	projectID := c.GetString("project_id")

	var req struct {
		Volume struct {
			Name        *string `json:"name"`
			Description *string `json:"description"`
		} `json:"volume"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Check volume exists and belongs to project
	var currentName, currentDesc string
	var sizeGB int
	var status string
	var createdAt, updatedAt time.Time
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT name, COALESCE(description, ''), size_gb, status, created_at, updated_at FROM volumes WHERE id = $1 AND project_id = $2",
		volumeID, projectID,
	).Scan(&currentName, &currentDesc, &sizeGB, &status, &createdAt, &updatedAt)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "volume not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Apply updates
	if req.Volume.Name != nil {
		currentName = *req.Volume.Name
	}
	if req.Volume.Description != nil {
		currentDesc = *req.Volume.Description
	}

	now := time.Now()
	_, err = database.DB.Exec(c.Request.Context(),
		"UPDATE volumes SET name = $1, description = $2, updated_at = $3 WHERE id = $4",
		currentName, currentDesc, now, volumeID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"volume": gin.H{
			"id":          volumeID,
			"name":        currentName,
			"description": currentDesc,
			"size":        sizeGB,
			"status":      status,
			"created_at":  createdAt.Format(time.RFC3339),
			"updated_at":  now.Format(time.RFC3339),
		},
	})
}

// UpdateSnapshot updates snapshot properties
func (svc *Service) UpdateSnapshot(c *gin.Context) {
	snapshotID := c.Param("id")
	projectID := c.GetString("project_id")

	var req struct {
		Snapshot struct {
			Name        *string `json:"name"`
			Description *string `json:"description"`
		} `json:"snapshot"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Check snapshot exists and belongs to project
	var currentName, currentDesc string
	var volumeID string
	var sizeGB int
	var status string
	var createdAt time.Time
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT name, COALESCE(description, ''), volume_id, size_gb, status, created_at FROM snapshots WHERE id = $1 AND project_id = $2",
		snapshotID, projectID,
	).Scan(&currentName, &currentDesc, &volumeID, &sizeGB, &status, &createdAt)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "snapshot not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Apply updates
	if req.Snapshot.Name != nil {
		currentName = *req.Snapshot.Name
	}
	if req.Snapshot.Description != nil {
		currentDesc = *req.Snapshot.Description
	}

	_, err = database.DB.Exec(c.Request.Context(),
		"UPDATE snapshots SET name = $1, description = $2 WHERE id = $3",
		currentName, currentDesc, snapshotID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"snapshot": gin.H{
			"id":          snapshotID,
			"name":        currentName,
			"description": currentDesc,
			"volume_id":   volumeID,
			"size":        sizeGB,
			"status":      status,
			"created_at":  createdAt.Format(time.RFC3339),
		},
	})
}

// GetVolumeMetadata returns all metadata for a volume
func (svc *Service) GetVolumeMetadata(c *gin.Context) {
	volumeID := c.Param("id")
	projectID := c.GetString("project_id")

	// Check volume exists and belongs to project
	var exists bool
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM volumes WHERE id = $1 AND project_id = $2)",
		volumeID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "volume not found"})
		return
	}

	rows, err := database.DB.Query(c.Request.Context(),
		"SELECT meta_key, meta_value FROM volume_metadata WHERE volume_id = $1",
		volumeID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	metadata := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			continue
		}
		metadata[key] = value
	}

	c.JSON(http.StatusOK, gin.H{"metadata": metadata})
}

// SetVolumeMetadata sets/replaces all metadata for a volume
func (svc *Service) SetVolumeMetadata(c *gin.Context) {
	volumeID := c.Param("id")
	projectID := c.GetString("project_id")

	var req struct {
		Metadata map[string]string `json:"metadata"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Check volume exists and belongs to project
	var exists bool
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM volumes WHERE id = $1 AND project_id = $2)",
		volumeID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "volume not found"})
		return
	}

	// Delete existing metadata
	_, err = database.DB.Exec(c.Request.Context(),
		"DELETE FROM volume_metadata WHERE volume_id = $1",
		volumeID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Insert new metadata
	for key, value := range req.Metadata {
		_, err = database.DB.Exec(c.Request.Context(), `
			INSERT INTO volume_metadata (volume_id, meta_key, meta_value)
			VALUES ($1, $2, $3)
		`, volumeID, key, value)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"metadata": req.Metadata})
}

// GetVolumeMetadataKey returns a single metadata key
func (svc *Service) GetVolumeMetadataKey(c *gin.Context) {
	volumeID := c.Param("id")
	key := c.Param("key")
	projectID := c.GetString("project_id")

	// Check volume exists and belongs to project
	var exists bool
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM volumes WHERE id = $1 AND project_id = $2)",
		volumeID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "volume not found"})
		return
	}

	var value string
	err = database.DB.QueryRow(c.Request.Context(),
		"SELECT meta_value FROM volume_metadata WHERE volume_id = $1 AND meta_key = $2",
		volumeID, key,
	).Scan(&value)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "metadata key not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"meta": map[string]string{key: value}})
}

// UpdateVolumeMetadataKey updates a single metadata key
func (svc *Service) UpdateVolumeMetadataKey(c *gin.Context) {
	volumeID := c.Param("id")
	key := c.Param("key")
	projectID := c.GetString("project_id")

	var req struct {
		Meta map[string]string `json:"meta"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	value, ok := req.Meta[key]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "key not found in request body"})
		return
	}

	// Check volume exists and belongs to project
	var exists bool
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM volumes WHERE id = $1 AND project_id = $2)",
		volumeID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "volume not found"})
		return
	}

	// Upsert metadata
	_, err = database.DB.Exec(c.Request.Context(), `
		INSERT INTO volume_metadata (volume_id, meta_key, meta_value, updated_at)
		VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
		ON CONFLICT (volume_id, meta_key)
		DO UPDATE SET meta_value = $3, updated_at = CURRENT_TIMESTAMP
	`, volumeID, key, value)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"meta": map[string]string{key: value}})
}

// DeleteVolumeMetadataKey deletes a single metadata key
func (svc *Service) DeleteVolumeMetadataKey(c *gin.Context) {
	volumeID := c.Param("id")
	key := c.Param("key")
	projectID := c.GetString("project_id")

	// Check volume exists and belongs to project
	var exists bool
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM volumes WHERE id = $1 AND project_id = $2)",
		volumeID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "volume not found"})
		return
	}

	_, err = database.DB.Exec(c.Request.Context(),
		"DELETE FROM volume_metadata WHERE volume_id = $1 AND meta_key = $2",
		volumeID, key,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

// GetSnapshotMetadata returns all metadata for a snapshot
func (svc *Service) GetSnapshotMetadata(c *gin.Context) {
	snapshotID := c.Param("id")
	projectID := c.GetString("project_id")

	// Check snapshot exists and belongs to project
	var exists bool
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM snapshots WHERE id = $1 AND project_id = $2)",
		snapshotID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "snapshot not found"})
		return
	}

	rows, err := database.DB.Query(c.Request.Context(),
		"SELECT meta_key, meta_value FROM snapshot_metadata WHERE snapshot_id = $1",
		snapshotID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	metadata := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			continue
		}
		metadata[key] = value
	}

	c.JSON(http.StatusOK, gin.H{"metadata": metadata})
}

// SetSnapshotMetadata sets/replaces all metadata for a snapshot
func (svc *Service) SetSnapshotMetadata(c *gin.Context) {
	snapshotID := c.Param("id")
	projectID := c.GetString("project_id")

	var req struct {
		Metadata map[string]string `json:"metadata"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Check snapshot exists and belongs to project
	var exists bool
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM snapshots WHERE id = $1 AND project_id = $2)",
		snapshotID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "snapshot not found"})
		return
	}

	// Delete existing metadata
	_, err = database.DB.Exec(c.Request.Context(),
		"DELETE FROM snapshot_metadata WHERE snapshot_id = $1",
		snapshotID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Insert new metadata
	for key, value := range req.Metadata {
		_, err = database.DB.Exec(c.Request.Context(), `
			INSERT INTO snapshot_metadata (snapshot_id, meta_key, meta_value)
			VALUES ($1, $2, $3)
		`, snapshotID, key, value)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"metadata": req.Metadata})
}

// GetSnapshotMetadataKey returns a single metadata key
func (svc *Service) GetSnapshotMetadataKey(c *gin.Context) {
	snapshotID := c.Param("id")
	key := c.Param("key")
	projectID := c.GetString("project_id")

	// Check snapshot exists and belongs to project
	var exists bool
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM snapshots WHERE id = $1 AND project_id = $2)",
		snapshotID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "snapshot not found"})
		return
	}

	var value string
	err = database.DB.QueryRow(c.Request.Context(),
		"SELECT meta_value FROM snapshot_metadata WHERE snapshot_id = $1 AND meta_key = $2",
		snapshotID, key,
	).Scan(&value)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "metadata key not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"meta": map[string]string{key: value}})
}

// UpdateSnapshotMetadataKey updates a single metadata key
func (svc *Service) UpdateSnapshotMetadataKey(c *gin.Context) {
	snapshotID := c.Param("id")
	key := c.Param("key")
	projectID := c.GetString("project_id")

	var req struct {
		Meta map[string]string `json:"meta"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	value, ok := req.Meta[key]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "key not found in request body"})
		return
	}

	// Check snapshot exists and belongs to project
	var exists bool
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM snapshots WHERE id = $1 AND project_id = $2)",
		snapshotID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "snapshot not found"})
		return
	}

	// Upsert metadata
	_, err = database.DB.Exec(c.Request.Context(), `
		INSERT INTO snapshot_metadata (snapshot_id, meta_key, meta_value, updated_at)
		VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
		ON CONFLICT (snapshot_id, meta_key)
		DO UPDATE SET meta_value = $3, updated_at = CURRENT_TIMESTAMP
	`, snapshotID, key, value)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"meta": map[string]string{key: value}})
}

// DeleteSnapshotMetadataKey deletes a single metadata key
func (svc *Service) DeleteSnapshotMetadataKey(c *gin.Context) {
	snapshotID := c.Param("id")
	key := c.Param("key")
	projectID := c.GetString("project_id")

	// Check snapshot exists and belongs to project
	var exists bool
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM snapshots WHERE id = $1 AND project_id = $2)",
		snapshotID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "snapshot not found"})
		return
	}

	_, err = database.DB.Exec(c.Request.Context(),
		"DELETE FROM snapshot_metadata WHERE snapshot_id = $1 AND meta_key = $2",
		snapshotID, key,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

// GetLimits returns volume service limits and quotas
func (svc *Service) GetLimits(c *gin.Context) {
	projectID := c.Param("project_id")

	// Query current usage from database
	var volumesUsed, snapshotsUsed, gigabytesUsed int

	database.DB.QueryRow(c.Request.Context(),
		"SELECT COUNT(*), COALESCE(SUM(size_gb), 0) FROM volumes WHERE project_id = $1 AND status != 'deleted'",
		projectID,
	).Scan(&volumesUsed, &gigabytesUsed)

	database.DB.QueryRow(c.Request.Context(),
		"SELECT COUNT(*) FROM snapshots WHERE project_id = $1 AND status != 'deleted'",
		projectID,
	).Scan(&snapshotsUsed)

	// Return limits response
	c.JSON(200, gin.H{
		"limits": gin.H{
			"rate": []gin.H{}, // No rate limiting
			"absolute": gin.H{
				"maxTotalVolumes":       1000,
				"maxTotalSnapshots":     1000,
				"maxTotalVolumeGigabytes": 10000,
				"maxTotalBackups":       100,
				"maxTotalBackupGigabytes": 5000,
				"totalVolumesUsed":      volumesUsed,
				"totalSnapshotsUsed":    snapshotsUsed,
				"totalGigabytesUsed":    gigabytesUsed,
				"totalBackupsUsed":      0,
				"totalBackupGigabytesUsed": 0,
			},
		},
	})
}

// ListServices returns list of volume services
func (svc *Service) ListServices(c *gin.Context) {
	// Format: OpenStack timestamp without Z
	now := time.Now().Format("2006-01-02T15:04:05.000000")

	// Return list of volume services for Horizon System Info
	c.JSON(200, gin.H{
		"services": []gin.H{
			{
				"binary":         "cinder-volume",
				"host":           "o3k-volume-1",
				"zone":           "nova",
				"status":         "enabled",
				"state":          "up",
				"updated_at":     now,
				"disabled_reason": nil,
			},
			{
				"binary":         "cinder-scheduler",
				"host":           "o3k-controller",
				"zone":           "internal",
				"status":         "enabled",
				"state":          "up",
				"updated_at":     now,
				"disabled_reason": nil,
			},
			{
				"binary":         "cinder-backup",
				"host":           "o3k-backup-1",
				"zone":           "nova",
				"status":         "enabled",
				"state":          "up",
				"updated_at":     now,
				"disabled_reason": nil,
			},
		},
	})
}
