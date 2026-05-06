package glance

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
	"github.com/cobaltcore-dev/o3k/internal/common"
	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/cobaltcore-dev/o3k/pkg/cache"
	"github.com/cobaltcore-dev/o3k/pkg/storage"
)

// Service handles Glance API endpoints
type Service struct {
	mode        string
	storageMode string
	cephPool    string
	cephConf    string
	s3Bucket    string
	s3Region    string
	s3Endpoint  string
	imageStore  *storage.ImageStore
	cache       *cache.Cache
	db          database.DBIF
}

// NewService creates a new Glance service
func NewService(mode, cephPool, cephConf, s3Bucket, s3Region, s3Endpoint string, cacheInstance *cache.Cache) *Service {
	return &Service{
		mode:        mode,
		storageMode: mode, // storageMode same as mode for backward compatibility
		cephPool:    cephPool,
		cephConf:    cephConf,
		s3Bucket:    s3Bucket,
		s3Region:    s3Region,
		s3Endpoint:  s3Endpoint,
		imageStore:  storage.NewImageStore(mode, cephPool, cephConf, s3Bucket, s3Region, s3Endpoint),
		cache:       cacheInstance,
	}
}

// NewServiceWithDB creates a Glance service with an injected DB for testing.
func NewServiceWithDB(db database.DBIF, mode, cephPool, cephConf, s3Bucket, s3Region, s3Endpoint string, cacheInstance *cache.Cache) *Service {
	svc := NewService(mode, cephPool, cephConf, s3Bucket, s3Region, s3Endpoint, cacheInstance)
	svc.db = db
	return svc
}

// activeDB returns the injected DB or falls back to the global.
func (svc *Service) activeDB() database.DBIF {
	if svc.db != nil {
		return svc.db
	}
	return database.DB
}

// RegisterRoutes registers Glance routes (excluding version discovery which is handled separately)
func (svc *Service) RegisterRoutes(r *gin.RouterGroup) {
	// Note: Version discovery (GET / and GET /v2) are registered separately
	// in main.go without auth middleware to comply with OpenStack spec
	//
	// gophercloud's NewImageServiceV2() ALWAYS appends /v2 to the catalog endpoint,
	// so we register routes WITHOUT /v2 prefix to avoid /v2/v2 doubling

	// Images
	r.GET("/images", svc.ListImages)
	r.POST("/images", svc.CreateImage)
	r.GET("/images/:id", svc.GetImage)
	r.DELETE("/images/:id", svc.DeleteImage)
	r.PATCH("/images/:id", svc.UpdateImage)

	// Image data
	r.PUT("/images/:id/file", svc.UploadImageData)
	r.GET("/images/:id/file", svc.DownloadImageData)

	// Image import
	r.POST("/images/:id/stage", svc.StageImageData)
	r.POST("/images/:id/import", svc.ImportImage)
	r.GET("/images/:id/import", svc.GetImageImportInfo)

	// Image members (sharing)
	r.POST("/images/:id/members", svc.CreateImageMember)
	r.GET("/images/:id/members", svc.ListImageMembers)
	r.GET("/images/:id/members/:member_id", svc.GetImageMember)
	r.PUT("/images/:id/members/:member_id", svc.UpdateImageMember)
	r.DELETE("/images/:id/members/:member_id", svc.DeleteImageMember)

	// Image tags
	r.PUT("/images/:id/tags/:tag", svc.AddImageTag)
	r.DELETE("/images/:id/tags/:tag", svc.DeleteImageTag)

	// Image actions
	r.POST("/images/:id/actions/deactivate", svc.DeactivateImage)
	r.POST("/images/:id/actions/reactivate", svc.ReactivateImage)

	// Schemas
	r.GET("/schemas/image", svc.GetImageSchema)
	r.GET("/schemas/images", svc.GetImagesSchema)
	r.GET("/schemas/member", svc.GetMemberSchema)
	r.GET("/schemas/members", svc.GetMembersSchema)

	// Tasks and import
	r.POST("/tasks", svc.CreateTask)
	r.GET("/tasks", svc.ListTasks)
	r.GET("/tasks/:id", svc.GetTask)

	// Stores
	r.GET("/stores", svc.ListStores)
	r.GET("/stores/info", svc.GetStoresInfo)

	// Metadefs
	r.GET("/metadefs/namespaces", svc.ListMetadefNamespaces)
	r.POST("/metadefs/namespaces", svc.CreateMetadefNamespace)
	r.GET("/metadefs/namespaces/:namespace", svc.GetMetadefNamespace)
	r.PUT("/metadefs/namespaces/:namespace", svc.UpdateMetadefNamespace)
	r.DELETE("/metadefs/namespaces/:namespace", svc.DeleteMetadefNamespace)
	r.GET("/metadefs/resource_types", svc.ListMetadefResourceTypes)

	// Cache management
	r.GET("/cache/images", svc.ListCachedImages)
	r.DELETE("/cache/images", svc.ClearCache)
	r.PUT("/cache/images/:id", svc.PrefetchImage)
	r.DELETE("/cache/images/:id", svc.DeleteCachedImage)
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
						"href": fmt.Sprintf("%s/", common.BaseURL(c, 9292)),
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
					"href": fmt.Sprintf("%s/", common.BaseURL(c, 9292)),
				},
			},
		},
	})
}

// CreateImage creates a new image
func (svc *Service) CreateImage(c *gin.Context) {
	var req CreateImageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
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
	_, err := svc.activeDB().Exec(c.Request.Context(), `
		INSERT INTO images (id, name, project_id, status, visibility, disk_format, container_format, min_disk_gb, min_ram_mb, rbd_pool, rbd_image, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`, imageID, req.Name, sql.NullString{String: projectID, Valid: visibility == "private"}, "queued", visibility, diskFormat, containerFormat, req.MinDisk, req.MinRAM, svc.cephPool, "image-"+imageID, now, now)

	if err != nil {
		log.Error().Err(err).Str("operation", "create_image").Msg("failed to insert image into database")
		common.SendError(c, common.NewInternalServerError("failed to create image"))
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

	// Parse pagination parameters
	limit := 1000
	offset := 0
	if limitParam := c.Query("limit"); limitParam != "" {
		if parsedLimit, err := strconv.Atoi(limitParam); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}
	if offsetParam := c.Query("offset"); offsetParam != "" {
		if parsedOffset, err := strconv.Atoi(offsetParam); err == nil && parsedOffset >= 0 {
			offset = parsedOffset
		}
	}

	// Marker-based pagination
	var markerCondition string
	var queryArgs []interface{}
	queryArgs = append(queryArgs, projectID)
	argIdx := 2

	if marker := c.Query("marker"); marker != "" {
		var markerCreatedAt time.Time
		err := svc.activeDB().QueryRow(c.Request.Context(),
			"SELECT created_at FROM images WHERE id = $1 AND (visibility = 'public' OR project_id = $2)",
			marker, projectID,
		).Scan(&markerCreatedAt)
		if err == nil {
			markerCondition = fmt.Sprintf(" AND created_at < $%d", argIdx)
			queryArgs = append(queryArgs, markerCreatedAt)
			argIdx++
		}
	}

	queryArgs = append(queryArgs, limit, offset)

	rows, err := svc.activeDB().Query(c.Request.Context(), fmt.Sprintf(`
		SELECT id, name, status, visibility, size_bytes, disk_format, container_format, min_disk_gb, min_ram_mb, created_at, updated_at
		FROM images
		WHERE (visibility = 'public' OR project_id = $1)%s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, markerCondition, argIdx, argIdx+1), queryArgs...)

	if err != nil {
		log.Error().Err(err).Str("operation", "list_images").Msg("failed to query images")
		common.SendError(c, common.NewInternalServerError("failed to list images"))
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
	ctx := c.Request.Context()

	// Try cache first
	if svc.cache != nil {
		cacheKey := "image:" + projectID + ":" + imageID
		var cached gin.H
		if err := svc.cache.Get(ctx, cacheKey, &cached); err == nil {
			c.JSON(http.StatusOK, cached)
			return
		}
	}

	// Cache miss - query database
	var id, name, status, visibility, diskFormat, containerFormat string
	var checksum sql.NullString
	var sizeBytes sql.NullInt64
	var minDisk, minRAM int
	var createdAt, updatedAt time.Time

	// Try by UUID first, then by name if UUID parsing fails
	// Use CAST to handle non-UUID strings gracefully
	err := svc.activeDB().QueryRow(ctx, `
		SELECT id, name, status, visibility, size_bytes, disk_format, container_format, min_disk_gb, min_ram_mb, checksum, created_at, updated_at
		FROM images
		WHERE (id::text = $1 OR name = $1) AND (visibility = 'public' OR project_id = $2)
		LIMIT 1
	`, imageID, projectID).Scan(&id, &name, &status, &visibility, &sizeBytes, &diskFormat, &containerFormat, &minDisk, &minRAM, &checksum, &createdAt, &updatedAt)

	if err == pgx.ErrNoRows {
		common.SendError(c, common.NewNotFoundError("image"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "get_image").Str("image_id", imageID).Msg("failed to query image")
		common.SendError(c, common.NewInternalServerError("failed to get image"))
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
	rows, err := svc.activeDB().Query(ctx,
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
	}

	// Store in cache (1h TTL per config)
	if svc.cache != nil {
		svc.cache.Set(ctx, "image:"+projectID+":"+id, image, 1*time.Hour)
	}

	c.JSON(http.StatusOK, image)
}

// DeleteImage deletes an image
func (svc *Service) DeleteImage(c *gin.Context) {
	imageID := c.Param("id")
	projectID := c.GetString("project_id")
	ctx := c.Request.Context()

	// Check if image exists in database (and user has access)
	var exists bool
	err := svc.activeDB().QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM images WHERE id = $1 AND (visibility = 'public' OR project_id = $2))",
		imageID, projectID,
	).Scan(&exists)
	if err != nil {
		log.Error().Err(err).Str("operation", "delete_image_check").Str("image_id", imageID).Msg("failed to check image existence")
		common.SendError(c, common.NewInternalServerError("failed to delete image"))
		return
	}
	if !exists {
		common.SendError(c, common.NewNotFoundError("image"))
		return
	}

	// Delete from database first
	_, err = svc.activeDB().Exec(ctx,
		"DELETE FROM images WHERE id = $1 AND (visibility = 'public' OR project_id = $2)",
		imageID, projectID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "delete_image_db").Str("image_id", imageID).Msg("failed to delete image from database")
		common.SendError(c, common.NewInternalServerError("failed to delete image"))
		return
	}

	// Delete from storage (best effort - image metadata gone is success)
	_ = svc.imageStore.DeleteImage(ctx, imageID)

	// Invalidate cache
	if svc.cache != nil {
		svc.cache.Delete(ctx, "image:"+imageID)
		svc.cache.DeletePattern(ctx, "images:*")
	}

	c.Status(http.StatusNoContent)
}

var imageUpdateFields = map[string]string{
	"/name":       "name",
	"/visibility": "visibility",
	"/min_disk":   "min_disk_gb",
	"/min_ram":    "min_ram_mb",
}

func allowedImageUpdateField(path string) (string, bool) {
	col, ok := imageUpdateFields[path]
	return col, ok
}

// UpdateImage updates an image
func (svc *Service) UpdateImage(c *gin.Context) {
	imageID := c.Param("id")
	projectID := c.GetString("project_id")

	// Parse JSON Patch operations
	var updates []map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Apply updates (simplified - only handles replace operations)
	for _, update := range updates {
		op, ok1 := update["op"].(string)
		path, ok2 := update["path"].(string)
		if !ok1 || !ok2 {
			continue
		}
		value := update["value"]

		if op == "replace" {
			field, ok := allowedImageUpdateField(path)
			if !ok {
				continue
			}
			// field is now a validated column name from the allowlist
			query := fmt.Sprintf("UPDATE images SET %s = $1, updated_at = $2 WHERE id = $3 AND project_id = $4", field)
			if _, err := svc.activeDB().Exec(c.Request.Context(), query, value, time.Now(), imageID, projectID); err != nil {
				log.Error().Err(err).Str("field", field).Str("image_id", imageID).Msg("failed to update image field")
				common.SendError(c, common.NewInternalServerError("failed to update image"))
				return
			}
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
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT status FROM images WHERE id = $1 AND project_id = $2",
		imageID, projectID,
	).Scan(&status)

	if err == pgx.ErrNoRows {
		common.SendError(c, common.NewNotFoundError("image"))
		return
	}

	if status != "queued" {
		common.SendError(c, common.NewConflictError("image data already exists"))
		return
	}

	// Update status to saving
	svc.activeDB().Exec(c.Request.Context(),
		"UPDATE images SET status = $1, updated_at = $2 WHERE id = $3",
		"saving", time.Now(), imageID)

	// Upload to storage (limit to 5GB)
	const maxImageUpload int64 = 5 * 1024 * 1024 * 1024
	limitedBody := io.LimitReader(c.Request.Body, maxImageUpload)
	size, err := svc.imageStore.UploadImage(c.Request.Context(), imageID, limitedBody)
	if err != nil {
		svc.activeDB().Exec(c.Request.Context(),
			"UPDATE images SET status = $1 WHERE id = $2",
			"queued", imageID)
		log.Error().Err(err).Str("operation", "upload_image").Str("image_id", imageID).Msg("failed to upload image to storage")
		common.SendError(c, common.NewServiceUnavailableError("failed to upload image"))
		return
	}

	// Update status to active and set size
	svc.activeDB().Exec(c.Request.Context(),
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
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT status, size_bytes FROM images WHERE id = $1 AND (visibility = 'public' OR project_id = $2)",
		imageID, projectID,
	).Scan(&status, &sizeBytes)

	if err == pgx.ErrNoRows {
		common.SendError(c, common.NewNotFoundError("image"))
		return
	}

	if status != "active" {
		common.SendError(c, common.NewConflictError("image is not active"))
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
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Check if image exists and is owned by requester
	var ownerID sql.NullString
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT project_id FROM images WHERE id = $1",
		imageID,
	).Scan(&ownerID)

	if err == pgx.ErrNoRows {
		common.SendError(c, common.NewNotFoundError("image"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "create_image_member_check").Str("image_id", imageID).Msg("failed to query image")
		common.SendError(c, common.NewInternalServerError("failed to create image member"))
		return
	}

	if !ownerID.Valid || ownerID.String != projectID {
		common.SendError(c, common.NewForbiddenError("not authorized to share this image"))
		return
	}

	// Insert image member
	memberID := uuid.New().String()
	now := time.Now()
	_, err = svc.activeDB().Exec(c.Request.Context(), `
		INSERT INTO image_members (id, image_id, member_id, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, memberID, imageID, req.Member, "pending", now, now)

	if err != nil {
		log.Error().Err(err).Str("operation", "create_image_member").Str("image_id", imageID).Msg("failed to insert image member")
		common.SendError(c, common.NewInternalServerError("failed to create image member"))
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
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT project_id FROM images WHERE id = $1",
		imageID,
	).Scan(&ownerID)

	if err == pgx.ErrNoRows {
		common.SendError(c, common.NewNotFoundError("image"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "list_image_members_check").Str("image_id", imageID).Msg("failed to query image")
		common.SendError(c, common.NewInternalServerError("failed to list image members"))
		return
	}

	// Only owner can list members
	if !ownerID.Valid || ownerID.String != projectID {
		common.SendError(c, common.NewForbiddenError("not authorized to view image members"))
		return
	}

	rows, err := svc.activeDB().Query(c.Request.Context(), `
		SELECT member_id, status, created_at, updated_at
		FROM image_members
		WHERE image_id = $1
		ORDER BY created_at DESC
	`, imageID)

	if err != nil {
		log.Error().Err(err).Str("operation", "list_image_members").Str("image_id", imageID).Msg("failed to query image members")
		common.SendError(c, common.NewInternalServerError("failed to list image members"))
		return
	}
	defer rows.Close()

	var members []gin.H
	for rows.Next() {
		var memberID, status string
		var createdAt, updatedAt time.Time

		if err := rows.Scan(&memberID, &status, &createdAt, &updatedAt); err != nil {
			log.Warn().Err(err).Msg("failed to scan image member row")
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
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT project_id FROM images WHERE id = $1",
		imageID,
	).Scan(&ownerID)

	if err == pgx.ErrNoRows {
		common.SendError(c, common.NewNotFoundError("image"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "get_image_member_check").Str("image_id", imageID).Msg("failed to query image")
		common.SendError(c, common.NewInternalServerError("failed to get image member"))
		return
	}

	// Only owner or member can view membership
	if (!ownerID.Valid || ownerID.String != projectID) && memberID != projectID {
		common.SendError(c, common.NewForbiddenError("not authorized"))
		return
	}

	var status string
	var createdAt, updatedAt time.Time
	err = svc.activeDB().QueryRow(c.Request.Context(), `
		SELECT status, created_at, updated_at
		FROM image_members
		WHERE image_id = $1 AND member_id = $2
	`, imageID, memberID).Scan(&status, &createdAt, &updatedAt)

	if err == pgx.ErrNoRows {
		common.SendError(c, common.NewNotFoundError("member"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "get_image_member").Str("image_id", imageID).Msg("failed to query image member")
		common.SendError(c, common.NewInternalServerError("failed to get image member"))
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
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Validate status
	if req.Status != "accepted" && req.Status != "rejected" && req.Status != "pending" {
		common.SendError(c, common.NewBadRequestError("invalid status"))
		return
	}

	// Check if image exists
	var ownerID sql.NullString
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT project_id FROM images WHERE id = $1",
		imageID,
	).Scan(&ownerID)

	if err == pgx.ErrNoRows {
		common.SendError(c, common.NewNotFoundError("image"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "update_image_member_check").Str("image_id", imageID).Msg("failed to query image")
		common.SendError(c, common.NewInternalServerError("failed to update image member"))
		return
	}

	// Both owner and member can update status:
	// - Owner can set to "pending"
	// - Member can set to "accepted" or "rejected"
	isOwner := ownerID.Valid && ownerID.String == projectID
	isMember := memberID == projectID

	if !isOwner && !isMember {
		common.SendError(c, common.NewForbiddenError("not authorized"))
		return
	}

	// Owner can only set status to "pending"
	if isOwner && !isMember && req.Status != "pending" {
		common.SendError(c, common.NewForbiddenError("owner can only set status to pending"))
		return
	}

	// Update member status
	_, err = svc.activeDB().Exec(c.Request.Context(), `
		UPDATE image_members
		SET status = $1, updated_at = $2
		WHERE image_id = $3 AND member_id = $4
	`, req.Status, time.Now(), imageID, memberID)

	if err != nil {
		log.Error().Err(err).Str("operation", "update_image_member").Str("image_id", imageID).Msg("failed to update image member")
		common.SendError(c, common.NewInternalServerError("failed to update image member"))
		return
	}

	// Return updated member
	var status string
	var createdAt, updatedAt time.Time
	err = svc.activeDB().QueryRow(c.Request.Context(), `
		SELECT status, created_at, updated_at
		FROM image_members
		WHERE image_id = $1 AND member_id = $2
	`, imageID, memberID).Scan(&status, &createdAt, &updatedAt)

	if err != nil {
		log.Error().Err(err).Str("operation", "update_image_member_fetch").Str("image_id", imageID).Msg("failed to fetch updated image member")
		common.SendError(c, common.NewInternalServerError("failed to fetch updated image member"))
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
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT project_id FROM images WHERE id = $1",
		imageID,
	).Scan(&ownerID)

	if err == pgx.ErrNoRows {
		common.SendError(c, common.NewNotFoundError("image"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "delete_image_member_check").Str("image_id", imageID).Msg("failed to query image owner")
		common.SendError(c, common.NewInternalServerError("failed to delete image member"))
		return
	}

	// Only owner can delete members
	if !ownerID.Valid || ownerID.String != projectID {
		common.SendError(c, common.NewForbiddenError("not authorized"))
		return
	}

	// Delete member
	_, err = svc.activeDB().Exec(c.Request.Context(),
		"DELETE FROM image_members WHERE image_id = $1 AND member_id = $2",
		imageID, memberID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "delete_image_member").Str("image_id", imageID).Msg("failed to delete image member")
		common.SendError(c, common.NewInternalServerError("failed to delete image member"))
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
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT project_id FROM images WHERE id = $1",
		imageID,
	).Scan(&ownerID)

	if err == pgx.ErrNoRows {
		common.SendError(c, common.NewNotFoundError("image"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "add_image_tag_check").Str("image_id", imageID).Msg("failed to query image owner")
		common.SendError(c, common.NewInternalServerError("failed to add image tag"))
		return
	}

	// Only owner can add tags
	if !ownerID.Valid || ownerID.String != projectID {
		common.SendError(c, common.NewForbiddenError("not authorized"))
		return
	}

	// Add tag (ignore if already exists due to UNIQUE constraint)
	_, err = svc.activeDB().Exec(c.Request.Context(), `
		INSERT INTO image_tags (image_id, tag)
		VALUES ($1, $2)
		ON CONFLICT (image_id, tag) DO NOTHING
	`, imageID, tag)

	if err != nil {
		log.Error().Err(err).Str("operation", "add_image_tag").Str("image_id", imageID).Msg("failed to add image tag")
		common.SendError(c, common.NewInternalServerError("failed to add image tag"))
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
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT project_id FROM images WHERE id = $1",
		imageID,
	).Scan(&ownerID)

	if err == pgx.ErrNoRows {
		common.SendError(c, common.NewNotFoundError("image"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "delete_image_tag_check").Str("image_id", imageID).Msg("failed to query image owner")
		common.SendError(c, common.NewInternalServerError("failed to delete image tag"))
		return
	}

	// Only owner can delete tags
	if !ownerID.Valid || ownerID.String != projectID {
		common.SendError(c, common.NewForbiddenError("not authorized"))
		return
	}

	// Delete tag
	_, err = svc.activeDB().Exec(c.Request.Context(),
		"DELETE FROM image_tags WHERE image_id = $1 AND tag = $2",
		imageID, tag,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "delete_image_tag").Str("image_id", imageID).Msg("failed to delete image tag")
		common.SendError(c, common.NewInternalServerError("failed to delete image tag"))
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
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT project_id FROM images WHERE id = $1",
		imageID,
	).Scan(&ownerID)

	if err == pgx.ErrNoRows {
		common.SendError(c, common.NewNotFoundError("image"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "deactivate_image_check").Str("image_id", imageID).Msg("failed to query image owner")
		common.SendError(c, common.NewInternalServerError("failed to deactivate image"))
		return
	}

	// Only owner can deactivate
	if !ownerID.Valid || ownerID.String != projectID {
		common.SendError(c, common.NewForbiddenError("not authorized"))
		return
	}

	// Update status to deactivated
	_, err = svc.activeDB().Exec(c.Request.Context(),
		"UPDATE images SET status = $1, updated_at = $2 WHERE id = $3",
		"deactivated", time.Now(), imageID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "deactivate_image").Str("image_id", imageID).Msg("failed to deactivate image")
		common.SendError(c, common.NewInternalServerError("failed to deactivate image"))
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
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT project_id FROM images WHERE id = $1",
		imageID,
	).Scan(&ownerID)

	if err == pgx.ErrNoRows {
		common.SendError(c, common.NewNotFoundError("image"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "reactivate_image_check").Str("image_id", imageID).Msg("failed to query image owner")
		common.SendError(c, common.NewInternalServerError("failed to reactivate image"))
		return
	}

	// Only owner can reactivate
	if !ownerID.Valid || ownerID.String != projectID {
		common.SendError(c, common.NewForbiddenError("not authorized"))
		return
	}

	// Update status to active
	_, err = svc.activeDB().Exec(c.Request.Context(),
		"UPDATE images SET status = $1, updated_at = $2 WHERE id = $3",
		"active", time.Now(), imageID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "reactivate_image").Str("image_id", imageID).Msg("failed to reactivate image")
		common.SendError(c, common.NewInternalServerError("failed to reactivate image"))
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
