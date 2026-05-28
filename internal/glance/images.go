package glance

import (
	"crypto/md5"
	"crypto/sha512"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/common"
	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/cobaltcore-dev/o3k/pkg/cache"
	"github.com/cobaltcore-dev/o3k/pkg/storage"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
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

// CreateImageRequest represents an image creation request.
//
// Glance v2 surfaces SCS-0102 image-metadata properties as top-level fields
// in both requests and responses. The JSON binding here ignores any unknown
// top-level keys; the SCS bag is collected separately by extractSCSProperties
// from the raw request body.
type CreateImageRequest struct {
	Name            string         `json:"name"`
	DiskFormat      string         `json:"disk_format"`
	ContainerFormat string         `json:"container_format"`
	Visibility      string         `json:"visibility"`
	MinDisk         int            `json:"min_disk"`
	MinRAM          int            `json:"min_ram"`
	Protected       bool           `json:"protected"`
	Tags            []string       `json:"tags"`
	Properties      map[string]any `json:"-"`
}

// fixedImageFields is the set of top-level keys that Glance v2 treats as
// first-class image attributes. Anything else in a create request body is
// considered a property (SCS-0102 or otherwise) and goes into the
// `properties` JSONB column verbatim.
var fixedImageFields = map[string]struct{}{
	"name": {}, "disk_format": {}, "container_format": {},
	"visibility": {}, "min_disk": {}, "min_ram": {},
	"protected": {}, "tags": {},
	// Server-controlled fields a client may echo back.
	"id": {}, "status": {}, "size": {}, "checksum": {},
	"os_hash_algo": {}, "os_hash_value": {},
	"created_at": {}, "updated_at": {},
	"self": {}, "file": {}, "schema": {},
	"owner": {}, "locations": {}, "virtual_size": {},
}

// extractSCSProperties returns the subset of body keys that are not in
// fixedImageFields. This lets SCS-0102 properties (and any other custom
// metadata) ride alongside the fixed Glance fields in the same request.
func extractSCSProperties(body map[string]any) map[string]any {
	if len(body) == 0 {
		return nil
	}
	props := make(map[string]any, len(body))
	for k, v := range body {
		if _, fixed := fixedImageFields[k]; fixed {
			continue
		}
		props[k] = v
	}
	if len(props) == 0 {
		return nil
	}
	return props
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

// checksumOrEmpty returns the string value or "" for a NullString checksum.
func checksumOrEmpty(s sql.NullString) string {
	if s.Valid {
		return s.String
	}
	return ""
}

// parseImageTime tolerates the variety of timestamp formats SQLite and
// Postgres return for created_at/updated_at. SQLite stores TIMESTAMP as TEXT
// using whichever layout the writer supplied (RFC3339 from time.Time, or the
// "YYYY-MM-DD HH:MM:SS" form CURRENT_TIMESTAMP defaults to), and modernc.org's
// driver does not auto-parse on read. Returns zero time on unparseable input.
func parseImageTime(raw string) time.Time {
	if raw == "" {
		return time.Time{}
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
	} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t
		}
	}
	return time.Time{}
}

// nullStringOrEmpty returns the string value or "" for a NullString.
func nullStringOrEmpty(s sql.NullString) string {
	if s.Valid {
		return s.String
	}
	return ""
}

// CreateImage creates a new image
func (svc *Service) CreateImage(c *gin.Context) {
	// Read the body once so we can extract both the fixed CreateImageRequest
	// fields and the open-ended SCS property bag from the same payload.
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	var req CreateImageRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	var raw map[string]any
	if len(bodyBytes) > 0 {
		if err := json.Unmarshal(bodyBytes, &raw); err != nil {
			common.SendError(c, common.NewBadRequestError("invalid request body"))
			return
		}
	}
	props := extractSCSProperties(raw)
	if err := validateSCSProperties(props); err != nil {
		common.SendError(c, common.NewBadRequestError(err.Error()))
		return
	}
	req.Properties = props

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

	// Serialise properties to JSON for the JSONB / TEXT column. Empty bag
	// becomes "{}" so the column never holds NULL — keeps GIN index happy
	// on Postgres and JSON parsing trivial on SQLite.
	propsJSON := []byte("{}")
	if len(req.Properties) > 0 {
		propsJSON, err = json.Marshal(req.Properties)
		if err != nil {
			common.SendError(c, common.NewInternalServerError("failed to encode properties"))
			return
		}
	}

	// Insert into database
	now := time.Now()
	_, err = svc.activeDB().Exec(c.Request.Context(), `
		INSERT INTO images (id, name, project_id, status, visibility, disk_format, container_format, min_disk_gb, min_ram_mb, protected, rbd_pool, rbd_image, properties, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`, imageID, req.Name, sql.NullString{String: projectID, Valid: visibility == "private"}, "queued", visibility, diskFormat, containerFormat, req.MinDisk, req.MinRAM, req.Protected, svc.cephPool, "image-"+imageID, string(propsJSON), now, now)

	if err != nil {
		log.Error().Err(err).Str("operation", "create_image").Msg("failed to insert image into database")
		common.SendError(c, common.NewInternalServerError("failed to create image"))
		return
	}

	// Persist tags
	ctx := c.Request.Context()
	for _, tag := range req.Tags {
		_, _ = svc.activeDB().Exec(ctx,
			`INSERT INTO image_tags (image_id, tag) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			imageID, tag)
	}

	resp := gin.H{
		"id":               imageID,
		"name":             req.Name,
		"status":           "queued",
		"visibility":       visibility,
		"disk_format":      diskFormat,
		"container_format": containerFormat,
		"min_disk":         req.MinDisk,
		"min_ram":          req.MinRAM,
		"protected":        req.Protected,
		"tags":             req.Tags,
		"created_at":       now.Format(time.RFC3339),
		"updated_at":       now.Format(time.RFC3339),
		"self":             fmt.Sprintf("/v2/images/%s", imageID),
		"file":             fmt.Sprintf("/v2/images/%s/file", imageID),
		"schema":           "/v2/schemas/image",
	}
	// SCS-0102 properties are surfaced as top-level fields, not nested under
	// "properties" — that's how Glance v2 actually presents them.
	for k, v := range req.Properties {
		resp[k] = v
	}
	c.JSON(http.StatusCreated, resp)
}

// joinConditions joins SQL conditions with AND.
func joinConditions(conditions []string) string {
	result := ""
	for i, c := range conditions {
		if i > 0 {
			result += " AND "
		}
		result += c
	}
	return result
}

// ListImages lists all images
func (svc *Service) ListImages(c *gin.Context) {
	projectID := c.GetString("project_id")

	// Parse pagination parameters
	limit := common.DefaultPaginationLimit
	offset := 0
	if limitParam := c.Query("limit"); limitParam != "" {
		if parsedLimit, err := strconv.Atoi(limitParam); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}
	limit = common.CapLimit(limit)
	if offsetParam := c.Query("offset"); offsetParam != "" {
		if parsedOffset, err := strconv.Atoi(offsetParam); err == nil && parsedOffset >= 0 {
			offset = parsedOffset
		}
	}

	// Build WHERE clause dynamically
	var conditions []string
	var queryArgs []interface{}
	argIdx := 1

	// Visibility / base ownership condition
	if vis := c.Query("visibility"); vis != "" {
		switch vis {
		case "public":
			conditions = append(conditions, "visibility = 'public'")
		case "community":
			conditions = append(conditions, "visibility = 'community'")
		case "private":
			conditions = append(conditions, fmt.Sprintf("(visibility = 'private' AND project_id = $%d)", argIdx))
			queryArgs = append(queryArgs, projectID)
			argIdx++
		case "shared":
			conditions = append(conditions, fmt.Sprintf(
				"(id IN (SELECT image_id FROM image_members WHERE member_id = $%d AND status = 'accepted'))",
				argIdx))
			queryArgs = append(queryArgs, projectID)
			argIdx++
		default:
			// unknown visibility filter: show public + community + owned + shared with this project
			conditions = append(conditions, fmt.Sprintf(
				"(visibility IN ('public', 'community') OR project_id = $%d OR id IN (SELECT image_id FROM image_members WHERE member_id = $%d AND status = 'accepted'))",
				argIdx, argIdx+1))
			queryArgs = append(queryArgs, projectID, projectID)
			argIdx += 2
		}
	} else {
		// No visibility param: show public + community + owned + shared with this project
		conditions = append(conditions, fmt.Sprintf(
			"(visibility IN ('public', 'community') OR project_id = $%d OR id IN (SELECT image_id FROM image_members WHERE member_id = $%d AND status = 'accepted'))",
			argIdx, argIdx+1))
		queryArgs = append(queryArgs, projectID, projectID)
		argIdx += 2
	}

	// Marker-based pagination
	if marker := c.Query("marker"); marker != "" {
		var markerCreatedAt time.Time
		err := svc.activeDB().QueryRow(c.Request.Context(),
			"SELECT created_at FROM images WHERE id = $1 AND (visibility = 'public' OR project_id = $2)",
			marker, projectID,
		).Scan(&markerCreatedAt)
		if err == nil {
			conditions = append(conditions, fmt.Sprintf("created_at < $%d", argIdx))
			queryArgs = append(queryArgs, markerCreatedAt)
			argIdx++
		}
	}

	// Additional filters
	if name := c.Query("name"); name != "" {
		conditions = append(conditions, fmt.Sprintf("name = $%d", argIdx))
		queryArgs = append(queryArgs, name)
		argIdx++
	}
	if status := c.Query("status"); status != "" {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argIdx))
		queryArgs = append(queryArgs, status)
		argIdx++
	}
	if diskFormat := c.Query("disk_format"); diskFormat != "" {
		conditions = append(conditions, fmt.Sprintf("disk_format = $%d", argIdx))
		queryArgs = append(queryArgs, diskFormat)
		argIdx++
	}
	if containerFormat := c.Query("container_format"); containerFormat != "" {
		conditions = append(conditions, fmt.Sprintf("container_format = $%d", argIdx))
		queryArgs = append(queryArgs, containerFormat)
		argIdx++
	}
	if owner := c.Query("owner"); owner != "" {
		conditions = append(conditions, fmt.Sprintf("project_id = $%d", argIdx))
		queryArgs = append(queryArgs, owner)
		argIdx++
	}
	if tag := c.Query("tag"); tag != "" {
		conditions = append(conditions, fmt.Sprintf("id IN (SELECT image_id FROM image_tags WHERE tag = $%d)", argIdx))
		queryArgs = append(queryArgs, tag)
		argIdx++
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + joinConditions(conditions)
	}

	queryArgs = append(queryArgs, limit, offset)

	rows, err := svc.activeDB().Query(c.Request.Context(), fmt.Sprintf(`
		SELECT id, name, status, visibility, size_bytes, disk_format, container_format, min_disk_gb, min_ram_mb, COALESCE(protected, false), created_at, updated_at, COALESCE(project_id, ''), checksum, os_hash_algo, os_hash_value
		FROM images
		%s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIdx, argIdx+1), queryArgs...)

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
		var protected bool
		var createdAt, updatedAt time.Time
		var imageOwner string
		var checksum, osHashAlgo, osHashValue sql.NullString

		if err := rows.Scan(&id, &name, &status, &visibility, &sizeBytes, &diskFormat, &containerFormat, &minDisk, &minRAM, &protected, &createdAt, &updatedAt, &imageOwner, &checksum, &osHashAlgo, &osHashValue); err != nil {
			continue
		}

		image := gin.H{
			"id":               id,
			"name":             name,
			"status":           status,
			"visibility":       visibility,
			"disk_format":      diskFormat,
			"container_format": containerFormat,
			"min_disk":         minDisk,
			"min_ram":          minRAM,
			"owner":            imageOwner,
			"protected":        protected,
			"checksum":         checksumOrEmpty(checksum),
			"os_hash_algo":     nullStringOrEmpty(osHashAlgo),
			"os_hash_value":    nullStringOrEmpty(osHashValue),
			"tags":             []string{},
			"created_at":       createdAt.Format(time.RFC3339),
			"updated_at":       updatedAt.Format(time.RFC3339),
			"self":             fmt.Sprintf("/v2/images/%s", id),
			"file":             fmt.Sprintf("/v2/images/%s/file", id),
			"schema":           "/v2/schemas/image",
		}

		if sizeBytes.Valid {
			image["size"] = sizeBytes.Int64
		}

		images = append(images, image)
	}
	if err := rows.Err(); err != nil {
		log.Error().Err(err).Str("operation", "list_images").Msg("rows iteration error")
		common.SendError(c, common.NewInternalServerError("failed to list images"))
		return
	}

	if images == nil {
		images = []gin.H{}
	}

	resp := gin.H{
		"images": images,
		"schema": "/v2/schemas/images",
		"first":  fmt.Sprintf("/v2/images?limit=%d", limit),
	}

	// Include "next" link only when the page is full (there may be more results)
	if len(images) == limit {
		lastID := images[len(images)-1]["id"].(string)
		resp["next"] = fmt.Sprintf("/v2/images?marker=%s&limit=%d", lastID, limit)
	}

	c.JSON(http.StatusOK, resp)
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
	var checksum, osHashAlgo, osHashValue sql.NullString
	var sizeBytes sql.NullInt64
	var minDisk, minRAM int
	var protected bool
	// SQLite stores TIMESTAMP columns as TEXT and modernc.org/sqlite does not
	// auto-parse them on read, so we scan into strings and convert below.
	var createdAtRaw, updatedAtRaw string
	var imageOwner string
	var propertiesRaw sql.NullString

	// Try by UUID first, then by name if UUID parsing fails
	// Use CAST to handle non-UUID strings gracefully.
	// properties is JSONB on Postgres and TEXT on SQLite — both scan into a
	// NullString once we strip the `::text` cast (pgx returns JSONB bytes
	// that decode to a JSON string).
	// Each $N appears exactly once: the SQLite adapter rewrites $N -> ? naively
	// (no positional reuse), so we pass imageID and projectID twice rather than
	// reuse $1 and $2.
	err := svc.activeDB().QueryRow(ctx, `
		SELECT id, name, status, visibility, size_bytes, disk_format, container_format, min_disk_gb, min_ram_mb, COALESCE(protected, false), checksum, os_hash_algo, os_hash_value, properties, created_at, updated_at, COALESCE(project_id, '')
		FROM images
		WHERE (id::text = $1 OR name = $2) AND (
			visibility IN ('public', 'community') OR
			project_id = $3 OR
			EXISTS (SELECT 1 FROM image_members WHERE image_id = images.id AND member_id = $4 AND status = 'accepted')
		)
		LIMIT 1
	`, imageID, imageID, projectID, projectID).Scan(&id, &name, &status, &visibility, &sizeBytes, &diskFormat, &containerFormat, &minDisk, &minRAM, &protected, &checksum, &osHashAlgo, &osHashValue, &propertiesRaw, &createdAtRaw, &updatedAtRaw, &imageOwner)
	createdAt := parseImageTime(createdAtRaw)
	updatedAt := parseImageTime(updatedAtRaw)

	if errors.Is(err, database.ErrNoRows) {
		common.SendError(c, common.NewNotFoundError("image"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "get_image").Str("image_id", imageID).Msg("failed to query image")
		common.SendError(c, common.NewInternalServerError("failed to get image"))
		return
	}

	image := gin.H{
		"id":               id,
		"name":             name,
		"status":           status,
		"visibility":       visibility,
		"disk_format":      diskFormat,
		"container_format": containerFormat,
		"min_disk":         minDisk,
		"min_ram":          minRAM,
		"owner":            imageOwner,
		"protected":        protected,
		"locations":        []interface{}{},
		"virtual_size":     nil,
		"created_at":       createdAt.Format(time.RFC3339),
		"updated_at":       updatedAt.Format(time.RFC3339),
		"self":             fmt.Sprintf("/v2/images/%s", id),
		"file":             fmt.Sprintf("/v2/images/%s/file", id),
		"schema":           "/v2/schemas/image",
	}

	if sizeBytes.Valid {
		image["size"] = sizeBytes.Int64
	}

	if checksum.Valid && checksum.String != "" {
		image["checksum"] = checksum.String
	}
	if osHashAlgo.Valid && osHashAlgo.String != "" {
		image["os_hash_algo"] = osHashAlgo.String
		image["os_hash_value"] = osHashValue.String
	} else {
		image["os_hash_algo"] = nil
		image["os_hash_value"] = nil
	}

	// Surface SCS-0102 (and other) properties as top-level fields.
	// Glance v2 doesn't nest custom metadata under a "properties" key on the
	// wire — it merges them into the image object alongside name/status/etc.
	if propertiesRaw.Valid && propertiesRaw.String != "" && propertiesRaw.String != "{}" {
		var props map[string]any
		if err := json.Unmarshal([]byte(propertiesRaw.String), &props); err == nil {
			for k, v := range props {
				if _, fixed := fixedImageFields[k]; fixed {
					// Don't let stored properties shadow fixed image attributes.
					continue
				}
				image[k] = v
			}
		}
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
		// rows.Err() non-critical for tags
		if tags == nil {
			tags = []string{}
		}
		image["tags"] = tags
	}

	// Store in cache (1h TTL per config)
	if svc.cache != nil {
		_ = svc.cache.Set(ctx, "image:"+projectID+":"+id, image, 1*time.Hour)
	}

	c.JSON(http.StatusOK, image)
}

// DeleteImage deletes an image
func (svc *Service) DeleteImage(c *gin.Context) {
	imageID := c.Param("id")
	projectID := c.GetString("project_id")
	isAdmin := c.GetBool("is_admin")
	ctx := c.Request.Context()

	// Check if image exists and determine ownership + protection status
	var ownerProjectID sql.NullString
	var protected bool
	err := svc.activeDB().QueryRow(ctx,
		"SELECT project_id, COALESCE(protected, false) FROM images WHERE id = $1",
		imageID,
	).Scan(&ownerProjectID, &protected)
	if err != nil {
		common.SendError(c, common.NewNotFoundError("image"))
		return
	}

	// Only the owner or admin can delete; public visibility does not grant delete access
	if (!ownerProjectID.Valid || ownerProjectID.String != projectID) && !isAdmin {
		common.SendError(c, common.NewForbiddenError("you do not have permission to delete this image"))
		return
	}

	// Protected images cannot be deleted
	if protected {
		c.JSON(http.StatusForbidden, gin.H{
			"message": fmt.Sprintf("Image %s is protected and cannot be deleted.", imageID),
			"code":    403,
		})
		return
	}

	// Delete from database
	_, err = svc.activeDB().Exec(ctx,
		"DELETE FROM images WHERE id = $1",
		imageID,
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
		_ = svc.cache.Delete(ctx, "image:"+projectID+":"+imageID)
		_ = svc.cache.DeletePattern(ctx, "images:*")
	}

	c.Status(http.StatusNoContent)
}

var imageUpdateFields = map[string]string{
	"/name":       "name",
	"/visibility": "visibility",
	"/min_disk":   "min_disk_gb",
	"/min_ram":    "min_ram_mb",
	"/protected":  "protected",
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

	// Load current row: protected flag + properties bag. We need the bag in
	// memory to apply patch ops on SCS-0102 properties; the column is JSONB
	// on Postgres and TEXT on SQLite, but in both cases we treat it as JSON.
	var protected bool
	var propertiesRaw sql.NullString
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT COALESCE(protected, false), properties FROM images WHERE id = $1 AND project_id = $2",
		imageID, projectID,
	).Scan(&protected, &propertiesRaw)
	if errors.Is(err, database.ErrNoRows) {
		common.SendError(c, common.NewNotFoundError("image"))
		return
	}
	if err != nil {
		common.SendError(c, common.NewInternalServerError("failed to query image"))
		return
	}

	props := map[string]any{}
	if propertiesRaw.Valid && propertiesRaw.String != "" {
		_ = json.Unmarshal([]byte(propertiesRaw.String), &props)
	}

	if protected {
		// Only allow unprotecting the image (single replace of /protected to false)
		allowUnprotect := len(updates) == 1 &&
			updates[0]["op"] == "replace" &&
			updates[0]["path"] == "/protected" &&
			updates[0]["value"] == false
		if !allowUnprotect {
			common.SendError(c, common.NewForbiddenError("image is protected and cannot be modified"))
			return
		}
	}

	propsTouched := false
	for _, update := range updates {
		op, ok1 := update["op"].(string)
		path, ok2 := update["path"].(string)
		if !ok1 || !ok2 {
			continue
		}
		value := update["value"]

		// First-class column path (e.g. /name, /visibility): use the allowlist.
		if field, ok := allowedImageUpdateField(path); ok {
			if op != "replace" {
				continue
			}
			query := fmt.Sprintf("UPDATE images SET %s = $1, updated_at = $2 WHERE id = $3 AND project_id = $4", field)
			if _, err := svc.activeDB().Exec(c.Request.Context(), query, value, time.Now(), imageID, projectID); err != nil {
				log.Error().Err(err).Str("field", field).Str("image_id", imageID).Msg("failed to update image field")
				common.SendError(c, common.NewInternalServerError("failed to update image"))
				return
			}
			continue
		}

		// Property path: anything else with a single leading slash and no
		// further slashes is treated as a custom/SCS property mutation on
		// the JSON bag. The Glance v2 wire format surfaces these as
		// top-level fields, so the JSON Patch path is /<key>.
		if len(path) < 2 || path[0] != '/' {
			continue
		}
		key := path[1:]
		if _, fixed := fixedImageFields[key]; fixed {
			// Reserved keys can't be mutated as properties.
			continue
		}
		switch op {
		case "add", "replace":
			props[key] = value
			propsTouched = true
		case "remove":
			delete(props, key)
			propsTouched = true
		}
	}

	if propsTouched {
		if err := validateSCSProperties(props); err != nil {
			common.SendError(c, common.NewBadRequestError(err.Error()))
			return
		}
		blob, err := json.Marshal(props)
		if err != nil {
			common.SendError(c, common.NewInternalServerError("failed to encode properties"))
			return
		}
		if _, err := svc.activeDB().Exec(c.Request.Context(),
			"UPDATE images SET properties = $1, updated_at = $2 WHERE id = $3 AND project_id = $4",
			string(blob), time.Now(), imageID, projectID,
		); err != nil {
			log.Error().Err(err).Str("image_id", imageID).Msg("failed to update image properties")
			common.SendError(c, common.NewInternalServerError("failed to update image properties"))
			return
		}
	}

	// Invalidate cache so GetImage re-reads the row we just mutated.
	if svc.cache != nil {
		_ = svc.cache.Delete(c.Request.Context(), "image:"+projectID+":"+imageID)
		_ = svc.cache.DeletePattern(c.Request.Context(), "images:*")
	}

	// Return updated image
	svc.GetImage(c)
}

// UploadImageData uploads image data
func (svc *Service) UploadImageData(c *gin.Context) {
	imageID := c.Param("id")
	projectID := c.GetString("project_id")

	// Atomically transition status from queued to saving to prevent concurrent uploads
	result, err := svc.activeDB().Exec(c.Request.Context(),
		"UPDATE images SET status = $1, updated_at = $2 WHERE id = $3 AND project_id = $4 AND status = 'queued'",
		"saving", time.Now(), imageID, projectID)
	if err != nil {
		log.Error().Err(err).Str("operation", "upload_image").Str("image_id", imageID).Msg("failed to update image status")
		common.SendError(c, common.NewInternalServerError("failed to update image status"))
		return
	}
	if result.RowsAffected() == 0 {
		// Either image doesn't exist or it's not in queued state
		var status string
		checkErr := svc.activeDB().QueryRow(c.Request.Context(),
			"SELECT status FROM images WHERE id = $1 AND project_id = $2", imageID, projectID,
		).Scan(&status)
		if errors.Is(checkErr, database.ErrNoRows) {
			common.SendError(c, common.NewNotFoundError("image"))
		} else {
			common.SendError(c, common.NewConflictError("image data already exists"))
		}
		return
	}

	// Upload to storage (limit to 5GB), tee through MD5 and SHA-512 hashers
	const maxImageUpload int64 = 5 * 1024 * 1024 * 1024
	md5h := md5.New()
	sha512h := sha512.New()
	multiWriter := io.MultiWriter(md5h, sha512h)
	limitedBody := io.LimitReader(io.TeeReader(c.Request.Body, multiWriter), maxImageUpload)
	size, err := svc.imageStore.UploadImage(c.Request.Context(), imageID, limitedBody)
	if err != nil {
		_, _ = svc.activeDB().Exec(c.Request.Context(),
			"UPDATE images SET status = $1 WHERE id = $2",
			"queued", imageID)
		log.Error().Err(err).Str("operation", "upload_image").Str("image_id", imageID).Msg("failed to upload image to storage")
		common.SendError(c, common.NewServiceUnavailableError("failed to upload image"))
		return
	}

	checksum := hex.EncodeToString(md5h.Sum(nil))
	osHashValue := hex.EncodeToString(sha512h.Sum(nil))

	// Update status to active, set size, MD5 checksum and SHA-512 hash
	if _, err := svc.activeDB().Exec(c.Request.Context(),
		"UPDATE images SET status = $1, size_bytes = $2, checksum = $3, os_hash_algo = $4, os_hash_value = $5, updated_at = $6 WHERE id = $7",
		"active", size, checksum, "sha512", osHashValue, time.Now(), imageID,
	); err != nil {
		log.Error().Err(err).Str("image_id", imageID).Msg("CRITICAL: failed to finalize image status after successful upload")
		common.SendError(c, common.NewInternalServerError("failed to finalize image upload"))
		return
	}

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

	if errors.Is(err, database.ErrNoRows) {
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
		"name": "image",
		"properties": gin.H{
			"id":               gin.H{"type": "string"},
			"name":             gin.H{"type": "string"},
			"status":           gin.H{"type": "string"},
			"visibility":       gin.H{"type": "string"},
			"size":             gin.H{"type": "integer"},
			"disk_format":      gin.H{"type": "string"},
			"container_format": gin.H{"type": "string"},
			"min_disk":         gin.H{"type": "integer"},
			"min_ram":          gin.H{"type": "integer"},
			"created_at":       gin.H{"type": "string"},
			"updated_at":       gin.H{"type": "string"},
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

	if errors.Is(err, database.ErrNoRows) {
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

	if errors.Is(err, database.ErrNoRows) {
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
	if err := rows.Err(); err != nil {
		log.Error().Err(err).Str("operation", "list_image_members").Msg("rows iteration error")
		common.SendError(c, common.NewInternalServerError("failed to list image members"))
		return
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

	if errors.Is(err, database.ErrNoRows) {
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

	if errors.Is(err, database.ErrNoRows) {
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

	if errors.Is(err, database.ErrNoRows) {
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

	if errors.Is(err, database.ErrNoRows) {
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

	if errors.Is(err, database.ErrNoRows) {
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

	if errors.Is(err, database.ErrNoRows) {
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

	if errors.Is(err, database.ErrNoRows) {
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

	if errors.Is(err, database.ErrNoRows) {
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
