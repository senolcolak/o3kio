package glance

import (
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/cobaltcore-dev/o3k/pkg/storage"
)

// Service handles Glance API endpoints
type Service struct {
	mode       string
	cephPool   string
	cephConf   string
	s3Bucket   string
	s3Region   string
	s3Endpoint string
	imageStore *storage.ImageStore
}

// NewService creates a new Glance service
func NewService(mode, cephPool, cephConf, s3Bucket, s3Region, s3Endpoint string) *Service {
	return &Service{
		mode:       mode,
		cephPool:   cephPool,
		cephConf:   cephConf,
		s3Bucket:   s3Bucket,
		s3Region:   s3Region,
		s3Endpoint: s3Endpoint,
		imageStore: storage.NewImageStore(mode, cephPool, cephConf, s3Bucket, s3Region, s3Endpoint),
	}
}

// RegisterRoutes registers Glance routes
func (svc *Service) RegisterRoutes(r *gin.RouterGroup) {
	// Version discovery
	r.GET("/", svc.GetVersions)
	r.GET("/v2", svc.GetVersionV2)

	v2 := r.Group("/v2")
	{
		// Images
		v2.GET("/images", svc.ListImages)
		v2.POST("/images", svc.CreateImage)
		v2.GET("/images/:id", svc.GetImage)
		v2.DELETE("/images/:id", svc.DeleteImage)
		v2.PATCH("/images/:id", svc.UpdateImage)

		// Image data
		v2.PUT("/images/:id/file", svc.UploadImageData)
		v2.GET("/images/:id/file", svc.DownloadImageData)

		// Schemas
		v2.GET("/schemas/image", svc.GetImageSchema)
		v2.GET("/schemas/images", svc.GetImagesSchema)
	}
}

// CreateImageRequest represents an image creation request
type CreateImageRequest struct {
	Name            string  `json:"name"`
	DiskFormat      string  `json:"disk_format"`
	ContainerFormat string  `json:"container_format"`
	Visibility      string  `json:"visibility"`
	MinDisk         int     `json:"min_disk"`
	MinRAM          int     `json:"min_ram"`
	Protected       bool    `json:"protected"`
	Tags            []string `json:"tags"`
}

// GetVersions returns all available API versions
func (svc *Service) GetVersions(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"versions": []gin.H{
			{
				"id":     "v2.9",
				"status": "CURRENT",
				"links": []gin.H{
					{
						"rel":  "self",
						"href": "http://localhost:9292/v2/",
					},
				},
			},
		},
	})
}

// GetVersionV2 returns v2 API version details
func (svc *Service) GetVersionV2(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"version": gin.H{
			"id":     "v2.9",
			"status": "CURRENT",
			"links": []gin.H{
				{
					"rel":  "self",
					"href": "http://localhost:9292/v2/",
				},
			},
		},
	})
}

// CreateImage creates a new image
func (svc *Service) CreateImage(c *gin.Context) {
	var req CreateImageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	projectID := c.GetString("project_id")
	imageID := uuid.New().String()

	// Set defaults
	visibility := "private"
	if req.Visibility != "" {
		visibility = req.Visibility
	}

	diskFormat := "qcow2"
	if req.DiskFormat != "" {
		diskFormat = req.DiskFormat
	}

	containerFormat := "bare"
	if req.ContainerFormat != "" {
		containerFormat = req.ContainerFormat
	}

	// Insert into database
	now := time.Now()
	_, err := database.DB.Exec(c.Request.Context(), `
		INSERT INTO images (id, name, project_id, status, visibility, disk_format, container_format, min_disk_gb, min_ram_mb, rbd_pool, rbd_image, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`, imageID, req.Name, sql.NullString{String: projectID, Valid: visibility == "private"}, "queued", visibility, diskFormat, containerFormat, req.MinDisk, req.MinRAM, svc.cephPool, "image-"+imageID, now, now)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":                imageID,
		"name":              req.Name,
		"status":            "queued",
		"visibility":        visibility,
		"disk_format":       diskFormat,
		"container_format":  containerFormat,
		"min_disk":          req.MinDisk,
		"min_ram":           req.MinRAM,
		"protected":         req.Protected,
		"tags":              req.Tags,
		"created_at":        now.Format(time.RFC3339),
		"updated_at":        now.Format(time.RFC3339),
		"self":              fmt.Sprintf("/v2/images/%s", imageID),
		"file":              fmt.Sprintf("/v2/images/%s/file", imageID),
		"schema":            "/v2/schemas/image",
	})
}

// ListImages lists all images
func (svc *Service) ListImages(c *gin.Context) {
	projectID := c.GetString("project_id")

	rows, err := database.DB.Query(c.Request.Context(), `
		SELECT id, name, status, visibility, size_bytes, disk_format, container_format, min_disk_gb, min_ram_mb, created_at, updated_at
		FROM images
		WHERE visibility = 'public' OR project_id = $1
		ORDER BY created_at DESC
	`, projectID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var images []gin.H
	for rows.Next() {
		var id, name, status, visibility, diskFormat, containerFormat string
		var sizeBytes sql.NullInt64
		var minDisk, minRAM int
		var createdAt, updatedAt time.Time

		if err := rows.Scan(&id, &name, &status, &visibility, &sizeBytes, &diskFormat, &containerFormat, &minDisk, &minRAM, &createdAt, &updatedAt); err != nil {
			continue
		}

		image := gin.H{
			"id":                id,
			"name":              name,
			"status":            status,
			"visibility":        visibility,
			"disk_format":       diskFormat,
			"container_format":  containerFormat,
			"min_disk":          minDisk,
			"min_ram":           minRAM,
			"created_at":        createdAt.Format(time.RFC3339),
			"updated_at":        updatedAt.Format(time.RFC3339),
			"self":              fmt.Sprintf("/v2/images/%s", id),
			"file":              fmt.Sprintf("/v2/images/%s/file", id),
			"schema":            "/v2/schemas/image",
		}

		if sizeBytes.Valid {
			image["size"] = sizeBytes.Int64
		}

		images = append(images, image)
	}

	if images == nil {
		images = []gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{
		"images": images,
		"schema":  "/v2/schemas/images",
		"first":   "/v2/images",
	})
}

// GetImage returns a single image
func (svc *Service) GetImage(c *gin.Context) {
	imageID := c.Param("id")
	projectID := c.GetString("project_id")

	var id, name, status, visibility, diskFormat, containerFormat string
	var checksum sql.NullString
	var sizeBytes sql.NullInt64
	var minDisk, minRAM int
	var createdAt, updatedAt time.Time

	// Try by UUID first, then by name if UUID parsing fails
	// Use CAST to handle non-UUID strings gracefully
	err := database.DB.QueryRow(c.Request.Context(), `
		SELECT id, name, status, visibility, size_bytes, disk_format, container_format, min_disk_gb, min_ram_mb, checksum, created_at, updated_at
		FROM images
		WHERE (id::text = $1 OR name = $1) AND (visibility = 'public' OR project_id = $2)
		LIMIT 1
	`, imageID, projectID).Scan(&id, &name, &status, &visibility, &sizeBytes, &diskFormat, &containerFormat, &minDisk, &minRAM, &checksum, &createdAt, &updatedAt)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "image not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	image := gin.H{
		"id":                id,
		"name":              name,
		"status":            status,
		"visibility":        visibility,
		"disk_format":       diskFormat,
		"container_format":  containerFormat,
		"min_disk":          minDisk,
		"min_ram":           minRAM,
		"created_at":        createdAt.Format(time.RFC3339),
		"updated_at":        updatedAt.Format(time.RFC3339),
		"self":              fmt.Sprintf("/v2/images/%s", id),
		"file":              fmt.Sprintf("/v2/images/%s/file", id),
		"schema":            "/v2/schemas/image",
	}

	if sizeBytes.Valid {
		image["size"] = sizeBytes.Int64
	}

	if checksum.Valid && checksum.String != "" {
		image["checksum"] = checksum.String
	}

	c.JSON(http.StatusOK, image)
}

// DeleteImage deletes an image
func (svc *Service) DeleteImage(c *gin.Context) {
	imageID := c.Param("id")
	projectID := c.GetString("project_id")

	// Delete from storage
	if err := svc.imageStore.DeleteImage(c.Request.Context(), imageID); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": fmt.Sprintf("failed to delete image from storage: %v", err)})
		return
	}

	// Delete from database
	_, err := database.DB.Exec(c.Request.Context(),
		"DELETE FROM images WHERE id = $1 AND (visibility = 'public' OR project_id = $2)",
		imageID, projectID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

// UpdateImage updates an image
func (svc *Service) UpdateImage(c *gin.Context) {
	imageID := c.Param("id")
	projectID := c.GetString("project_id")

	// Parse JSON Patch operations
	var updates []map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Apply updates (simplified - only handles replace operations)
	for _, update := range updates {
		op := update["op"].(string)
		path := update["path"].(string)
		value := update["value"]

		if op == "replace" {
			var field string
			switch path {
			case "/name":
				field = "name"
			case "/visibility":
				field = "visibility"
			case "/min_disk":
				field = "min_disk_gb"
			case "/min_ram":
				field = "min_ram_mb"
			default:
				continue
			}

			query := fmt.Sprintf("UPDATE images SET %s = $1, updated_at = $2 WHERE id = $3 AND (visibility != 'public' OR project_id = $4)", field)
			database.DB.Exec(c.Request.Context(), query, value, time.Now(), imageID, projectID)
		}
	}

	// Return updated image
	svc.GetImage(c)
}

// UploadImageData uploads image data
func (svc *Service) UploadImageData(c *gin.Context) {
	imageID := c.Param("id")
	projectID := c.GetString("project_id")

	// Check if image exists and is in queued status
	var status string
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT status FROM images WHERE id = $1 AND project_id = $2",
		imageID, projectID,
	).Scan(&status)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "image not found"})
		return
	}

	if status != "queued" {
		c.JSON(http.StatusConflict, gin.H{"error": "image data already exists"})
		return
	}

	// Update status to saving
	database.DB.Exec(c.Request.Context(),
		"UPDATE images SET status = $1, updated_at = $2 WHERE id = $3",
		"saving", time.Now(), imageID)

	// Upload to storage
	size, err := svc.imageStore.UploadImage(c.Request.Context(), imageID, c.Request.Body)
	if err != nil {
		database.DB.Exec(c.Request.Context(),
			"UPDATE images SET status = $1 WHERE id = $2",
			"queued", imageID)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": fmt.Sprintf("failed to upload image: %v", err)})
		return
	}

	// Update status to active and set size
	database.DB.Exec(c.Request.Context(),
		"UPDATE images SET status = $1, size_bytes = $2, updated_at = $3 WHERE id = $4",
		"active", size, time.Now(), imageID)

	c.Status(http.StatusNoContent)
}

// DownloadImageData downloads image data
func (svc *Service) DownloadImageData(c *gin.Context) {
	imageID := c.Param("id")
	projectID := c.GetString("project_id")

	// Check if image exists and is active
	var status string
	var sizeBytes sql.NullInt64
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT status, size_bytes FROM images WHERE id = $1 AND (visibility = 'public' OR project_id = $2)",
		imageID, projectID,
	).Scan(&status, &sizeBytes)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "image not found"})
		return
	}

	if status != "active" {
		c.JSON(http.StatusConflict, gin.H{"error": "image is not active"})
		return
	}

	// Set headers
	c.Header("Content-Type", "application/octet-stream")
	if sizeBytes.Valid {
		c.Header("Content-Length", fmt.Sprintf("%d", sizeBytes.Int64))
	}

	// Download from storage
	writer := c.Writer
	if err := svc.imageStore.DownloadImage(c.Request.Context(), imageID, writer); err != nil {
		// Cannot send JSON error here as we've already started sending data
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
}

// GetImageSchema returns the image schema
func (svc *Service) GetImageSchema(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"name":       "image",
		"properties": gin.H{
			"id":                gin.H{"type": "string"},
			"name":              gin.H{"type": "string"},
			"status":            gin.H{"type": "string"},
			"visibility":        gin.H{"type": "string"},
			"size":              gin.H{"type": "integer"},
			"disk_format":       gin.H{"type": "string"},
			"container_format":  gin.H{"type": "string"},
			"min_disk":          gin.H{"type": "integer"},
			"min_ram":           gin.H{"type": "integer"},
			"created_at":        gin.H{"type": "string"},
			"updated_at":        gin.H{"type": "string"},
		},
	})
}

// GetImagesSchema returns the images list schema
func (svc *Service) GetImagesSchema(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"name": "images",
		"properties": gin.H{
			"images": gin.H{
				"type": "array",
				"items": gin.H{
					"type": "object",
					"name": "image",
				},
			},
		},
	})
}
