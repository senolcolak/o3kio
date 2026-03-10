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

// RegisterRoutes registers Glance routes (excluding version discovery which is handled separately)
func (svc *Service) RegisterRoutes(r *gin.RouterGroup) {
	// Note: Version discovery (GET / and GET /v2) are registered separately
	// in main.go without auth middleware to comply with OpenStack spec

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

		// Image members (sharing)
		v2.POST("/images/:id/members", svc.CreateImageMember)
		v2.GET("/images/:id/members", svc.ListImageMembers)
		v2.GET("/images/:id/members/:member_id", svc.GetImageMember)
		v2.PUT("/images/:id/members/:member_id", svc.UpdateImageMember)
		v2.DELETE("/images/:id/members/:member_id", svc.DeleteImageMember)

		// Image tags
		v2.PUT("/images/:id/tags/:tag", svc.AddImageTag)
		v2.DELETE("/images/:id/tags/:tag", svc.DeleteImageTag)

		// Image actions
		v2.POST("/images/:id/actions/deactivate", svc.DeactivateImage)
		v2.POST("/images/:id/actions/reactivate", svc.ReactivateImage)

		// Schemas
		v2.GET("/schemas/image", svc.GetImageSchema)
		v2.GET("/schemas/images", svc.GetImagesSchema)
		v2.GET("/schemas/member", svc.GetMemberSchema)
		v2.GET("/schemas/members", svc.GetMembersSchema)
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

	// Load tags
	rows, err := database.DB.Query(c.Request.Context(),
		"SELECT tag FROM image_tags WHERE image_id = $1 ORDER BY tag",
		id,
	)
	if err == nil {
		defer rows.Close()
		var tags []string
		for rows.Next() {
			var tag string
			if rows.Scan(&tag) == nil {
				tags = append(tags, tag)
			}
		}
		if tags == nil {
			tags = []string{}
		}
		image["tags"] = tags
	} else {
		image["tags"] = []string{}
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
		"SELECT status FROM images WHERE id = $1 AND (visibility = 'public' OR project_id = $2)",
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

// CreateImageMember creates a new image member (share image with another project)
func (svc *Service) CreateImageMember(c *gin.Context) {
	imageID := c.Param("id")
	projectID := c.GetString("project_id")

	var req struct {
		Member string `json:"member"` // member_id is the project ID to share with
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Check if image exists and is owned by requester
	var ownerID sql.NullString
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT project_id FROM images WHERE id = $1",
		imageID,
	).Scan(&ownerID)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "image not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if !ownerID.Valid || ownerID.String != projectID {
		c.JSON(http.StatusForbidden, gin.H{"error": "not authorized to share this image"})
		return
	}

	// Insert image member
	memberID := uuid.New().String()
	now := time.Now()
	_, err = database.DB.Exec(c.Request.Context(), `
		INSERT INTO image_members (id, image_id, member_id, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, memberID, imageID, req.Member, "pending", now, now)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"member_id":  req.Member,
		"image_id":   imageID,
		"status":     "pending",
		"created_at": now.Format(time.RFC3339),
		"updated_at": now.Format(time.RFC3339),
		"schema":     "/v2/schemas/member",
	})
}

// ListImageMembers lists all members for an image
func (svc *Service) ListImageMembers(c *gin.Context) {
	imageID := c.Param("id")
	projectID := c.GetString("project_id")

	// Check if image exists and requester has access
	var ownerID sql.NullString
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT project_id FROM images WHERE id = $1",
		imageID,
	).Scan(&ownerID)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "image not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Only owner can list members
	if !ownerID.Valid || ownerID.String != projectID {
		c.JSON(http.StatusForbidden, gin.H{"error": "not authorized to view image members"})
		return
	}

	rows, err := database.DB.Query(c.Request.Context(), `
		SELECT member_id, status, created_at, updated_at
		FROM image_members
		WHERE image_id = $1
		ORDER BY created_at DESC
	`, imageID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var members []gin.H
	for rows.Next() {
		var memberID, status string
		var createdAt, updatedAt time.Time

		if err := rows.Scan(&memberID, &status, &createdAt, &updatedAt); err != nil {
			continue
		}

		members = append(members, gin.H{
			"member_id":  memberID,
			"image_id":   imageID,
			"status":     status,
			"created_at": createdAt.Format(time.RFC3339),
			"updated_at": updatedAt.Format(time.RFC3339),
			"schema":     "/v2/schemas/member",
		})
	}

	if members == nil {
		members = []gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{
		"members": members,
		"schema":  "/v2/schemas/members",
	})
}

// GetImageMember gets a specific image member
func (svc *Service) GetImageMember(c *gin.Context) {
	imageID := c.Param("id")
	memberID := c.Param("member_id")
	projectID := c.GetString("project_id")

	// Check if image exists and requester has access
	var ownerID sql.NullString
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT project_id FROM images WHERE id = $1",
		imageID,
	).Scan(&ownerID)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "image not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Only owner or member can view membership
	if (!ownerID.Valid || ownerID.String != projectID) && memberID != projectID {
		c.JSON(http.StatusForbidden, gin.H{"error": "not authorized"})
		return
	}

	var status string
	var createdAt, updatedAt time.Time
	err = database.DB.QueryRow(c.Request.Context(), `
		SELECT status, created_at, updated_at
		FROM image_members
		WHERE image_id = $1 AND member_id = $2
	`, imageID, memberID).Scan(&status, &createdAt, &updatedAt)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "member not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"member_id":  memberID,
		"image_id":   imageID,
		"status":     status,
		"created_at": createdAt.Format(time.RFC3339),
		"updated_at": updatedAt.Format(time.RFC3339),
		"schema":     "/v2/schemas/member",
	})
}

// UpdateImageMember updates image member status
func (svc *Service) UpdateImageMember(c *gin.Context) {
	imageID := c.Param("id")
	memberID := c.Param("member_id")
	projectID := c.GetString("project_id")

	var req struct {
		Status string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Validate status
	if req.Status != "accepted" && req.Status != "rejected" && req.Status != "pending" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
		return
	}

	// Check if image exists
	var ownerID sql.NullString
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT project_id FROM images WHERE id = $1",
		imageID,
	).Scan(&ownerID)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "image not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Both owner and member can update status:
	// - Owner can set to "pending"
	// - Member can set to "accepted" or "rejected"
	isOwner := ownerID.Valid && ownerID.String == projectID
	isMember := memberID == projectID

	if !isOwner && !isMember {
		c.JSON(http.StatusForbidden, gin.H{"error": "not authorized"})
		return
	}

	// Owner can only set status to "pending"
	if isOwner && !isMember && req.Status != "pending" {
		c.JSON(http.StatusForbidden, gin.H{"error": "owner can only set status to pending"})
		return
	}

	// Update member status
	_, err = database.DB.Exec(c.Request.Context(), `
		UPDATE image_members
		SET status = $1, updated_at = $2
		WHERE image_id = $3 AND member_id = $4
	`, req.Status, time.Now(), imageID, memberID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Return updated member
	var status string
	var createdAt, updatedAt time.Time
	err = database.DB.QueryRow(c.Request.Context(), `
		SELECT status, created_at, updated_at
		FROM image_members
		WHERE image_id = $1 AND member_id = $2
	`, imageID, memberID).Scan(&status, &createdAt, &updatedAt)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"member_id":  memberID,
		"image_id":   imageID,
		"status":     status,
		"created_at": createdAt.Format(time.RFC3339),
		"updated_at": updatedAt.Format(time.RFC3339),
		"schema":     "/v2/schemas/member",
	})
}

// DeleteImageMember deletes an image member (unshare)
func (svc *Service) DeleteImageMember(c *gin.Context) {
	imageID := c.Param("id")
	memberID := c.Param("member_id")
	projectID := c.GetString("project_id")

	// Check if image exists and requester is owner
	var ownerID sql.NullString
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT project_id FROM images WHERE id = $1",
		imageID,
	).Scan(&ownerID)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "image not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Only owner can delete members
	if !ownerID.Valid || ownerID.String != projectID {
		c.JSON(http.StatusForbidden, gin.H{"error": "not authorized"})
		return
	}

	// Delete member
	_, err = database.DB.Exec(c.Request.Context(),
		"DELETE FROM image_members WHERE image_id = $1 AND member_id = $2",
		imageID, memberID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

// AddImageTag adds a tag to an image
func (svc *Service) AddImageTag(c *gin.Context) {
	imageID := c.Param("id")
	tag := c.Param("tag")
	projectID := c.GetString("project_id")

	// Check image exists and user has permission
	var ownerID sql.NullString
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT project_id FROM images WHERE id = $1",
		imageID,
	).Scan(&ownerID)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "image not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Only owner can add tags
	if !ownerID.Valid || ownerID.String != projectID {
		c.JSON(http.StatusForbidden, gin.H{"error": "not authorized"})
		return
	}

	// Add tag (ignore if already exists due to UNIQUE constraint)
	_, err = database.DB.Exec(c.Request.Context(), `
		INSERT INTO image_tags (image_id, tag)
		VALUES ($1, $2)
		ON CONFLICT (image_id, tag) DO NOTHING
	`, imageID, tag)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

// DeleteImageTag removes a tag from an image
func (svc *Service) DeleteImageTag(c *gin.Context) {
	imageID := c.Param("id")
	tag := c.Param("tag")
	projectID := c.GetString("project_id")

	// Check image exists and user has permission
	var ownerID sql.NullString
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT project_id FROM images WHERE id = $1",
		imageID,
	).Scan(&ownerID)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "image not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Only owner can delete tags
	if !ownerID.Valid || ownerID.String != projectID {
		c.JSON(http.StatusForbidden, gin.H{"error": "not authorized"})
		return
	}

	// Delete tag
	_, err = database.DB.Exec(c.Request.Context(),
		"DELETE FROM image_tags WHERE image_id = $1 AND tag = $2",
		imageID, tag,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

// DeactivateImage deactivates an image
func (svc *Service) DeactivateImage(c *gin.Context) {
	imageID := c.Param("id")
	projectID := c.GetString("project_id")

	// Check image exists and user has permission
	var ownerID sql.NullString
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT project_id FROM images WHERE id = $1",
		imageID,
	).Scan(&ownerID)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "image not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Only owner can deactivate
	if !ownerID.Valid || ownerID.String != projectID {
		c.JSON(http.StatusForbidden, gin.H{"error": "not authorized"})
		return
	}

	// Update status to deactivated
	_, err = database.DB.Exec(c.Request.Context(),
		"UPDATE images SET status = $1, updated_at = $2 WHERE id = $3",
		"deactivated", time.Now(), imageID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

// ReactivateImage reactivates a deactivated image
func (svc *Service) ReactivateImage(c *gin.Context) {
	imageID := c.Param("id")
	projectID := c.GetString("project_id")

	// Check image exists and user has permission
	var ownerID sql.NullString
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT project_id FROM images WHERE id = $1",
		imageID,
	).Scan(&ownerID)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "image not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Only owner can reactivate
	if !ownerID.Valid || ownerID.String != projectID {
		c.JSON(http.StatusForbidden, gin.H{"error": "not authorized"})
		return
	}

	// Update status to active
	_, err = database.DB.Exec(c.Request.Context(),
		"UPDATE images SET status = $1, updated_at = $2 WHERE id = $3",
		"active", time.Now(), imageID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

// GetMemberSchema returns the member schema
func (svc *Service) GetMemberSchema(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"name": "member",
		"properties": gin.H{
			"created_at": gin.H{
				"type":        "string",
				"description": "Date and time of member creation",
				"readOnly":    true,
			},
			"image_id": gin.H{
				"type":        "string",
				"description": "An identifier for the image",
				"pattern":     "^([0-9a-fA-F]){8}-([0-9a-fA-F]){4}-([0-9a-fA-F]){4}-([0-9a-fA-F]){4}-([0-9a-fA-F]){12}$",
			},
			"member_id": gin.H{
				"type":        "string",
				"description": "An identifier for the image member (tenant ID)",
			},
			"status": gin.H{
				"type":        "string",
				"description": "The status of this image member",
				"enum":        []string{"pending", "accepted", "rejected"},
			},
			"updated_at": gin.H{
				"type":        "string",
				"description": "Date and time of last member update",
				"readOnly":    true,
			},
			"schema": gin.H{
				"type":        "string",
				"description": "The URL for the schema describing this member",
				"readOnly":    true,
			},
		},
	})
}

// GetMembersSchema returns the members list schema
func (svc *Service) GetMembersSchema(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"name": "members",
		"properties": gin.H{
			"members": gin.H{
				"type": "array",
				"items": gin.H{
					"type": "object",
					"name": "member",
					"properties": gin.H{
						"created_at": gin.H{
							"type":        "string",
							"description": "Date and time of member creation",
						},
						"image_id": gin.H{
							"type":        "string",
							"description": "An identifier for the image",
						},
						"member_id": gin.H{
							"type":        "string",
							"description": "An identifier for the image member",
						},
						"status": gin.H{
							"type":        "string",
							"description": "The status of this image member",
						},
						"updated_at": gin.H{
							"type":        "string",
							"description": "Date and time of last member update",
						},
						"schema": gin.H{
							"type": "string",
						},
					},
				},
			},
			"schema": gin.H{
				"type": "string",
			},
		},
	})
}
