package cinder

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/common"
	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/cobaltcore-dev/o3k/internal/keystone"
	"github.com/cobaltcore-dev/o3k/pkg/storage"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// Service handles Cinder API endpoints
type Service struct {
	mode       string
	cephPool   string
	cephConf   string
	cephClient *storage.CephClient
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
	db         database.DBIF
}

// NewService creates a new Cinder service
func NewService(mode, cephPool, cephConf string) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	return &Service{
		mode:       mode,
		cephPool:   cephPool,
		cephConf:   cephConf,
		cephClient: storage.NewCephClient(mode, cephPool, cephConf),
		ctx:        ctx,
		cancel:     cancel,
	}
}

// NewServiceWithDB creates a Cinder service with an injected DB for testing.
func NewServiceWithDB(db database.DBIF, mode, cephPool, cephConf string) *Service {
	svc := NewService(mode, cephPool, cephConf)
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

// Shutdown signals all background goroutines to stop and waits for them.
func (svc *Service) Shutdown() {
	svc.cancel()
	svc.wg.Wait()
}

// RegisterRoutes registers Cinder routes
func (svc *Service) RegisterRoutes(r *gin.RouterGroup) {
	// Version discovery endpoint (no auth required)
	r.GET("/", svc.GetVersions)
	r.GET("/v3", svc.GetVersionV3)

	// Limits endpoint without project_id (extracts from token)
	r.GET("/v3/limits", svc.GetLimitsNoProject)

	// Volume transfers without project_id (extracts from token)
	// MUST be registered before v3/:project_id group to avoid route conflict
	r.GET("/v3/os-volume-transfer", svc.ListVolumeTransfersNoProject)
	r.GET("/v3/os-volume-transfer/detail", svc.ListVolumeTransfersNoProject)

	// Volumes operations without project_id (extracts from token)
	r.POST("/v3/volumes", svc.CreateVolume)
	r.GET("/v3/volumes", svc.ListVolumes)
	r.GET("/v3/volumes/detail", svc.ListVolumesDetail)
	r.GET("/v3/volumes/:id", svc.GetVolume)
	r.PATCH("/v3/volumes/:id", svc.UpdateVolume)
	r.PUT("/v3/volumes/:id", svc.UpdateVolume)
	r.DELETE("/v3/volumes/:id", svc.DeleteVolume)
	r.POST("/v3/volumes/:id/action", svc.VolumeAction)

	// Snapshots list without project_id (extracts from token)
	r.GET("/v3/snapshots", svc.ListSnapshots)
	r.GET("/v3/snapshots/detail", svc.ListSnapshotsDetail)
	r.POST("/v3/snapshots", svc.CreateSnapshot)
	r.GET("/v3/snapshots/:id", svc.GetSnapshot)
	r.PUT("/v3/snapshots/:id", svc.UpdateSnapshot)
	r.PATCH("/v3/snapshots/:id", svc.UpdateSnapshot)
	r.DELETE("/v3/snapshots/:id", svc.DeleteSnapshot)

	// Volume types without project_id (extracts from token)
	r.GET("/v3/types", svc.ListVolumeTypes)
	r.GET("/v3/types/default", svc.GetDefaultVolumeType)

	// Volume groups without project_id (extracts from token)
	r.GET("/v3/groups", svc.ListGroups)
	r.POST("/v3/groups", svc.CreateGroup)
	r.GET("/v3/groups/:id", svc.GetGroup)
	r.PUT("/v3/groups/:id", svc.UpdateGroup)
	r.DELETE("/v3/groups/:id", svc.DeleteGroup)

	// QoS Specs without project_id (extracts from token)
	r.GET("/v3/qos-specs", svc.ListQosSpecs)
	r.POST("/v3/qos-specs", svc.CreateQosSpec)
	r.GET("/v3/qos-specs/:id", svc.GetQosSpec)
	r.PUT("/v3/qos-specs/:id", svc.UpdateQosSpec)
	r.DELETE("/v3/qos-specs/:id", svc.DeleteQosSpec)

	// Backups without project_id (extracts from token)
	r.GET("/v3/backups", svc.ListBackups)
	r.GET("/v3/backups/detail", svc.ListBackupsDetail)
	r.POST("/v3/backups", svc.CreateBackup)
	r.GET("/v3/backups/:id", svc.GetBackup)
	r.DELETE("/v3/backups/:id", svc.DeleteBackup)
	r.POST("/v3/backups/:id/action", svc.BackupAction)

	// Availability zones without project_id (extracts from token)
	r.GET("/v3/os-availability-zone", svc.ListAvailabilityZones)

	v3 := r.Group("/v3/:project_id")
	{
		// Volumes (create, list, get by ID, update, delete - need project_id in URL)
		v3.POST("/volumes", svc.CreateVolume)
		v3.GET("/volumes", svc.ListVolumes)
		v3.GET("/volumes/detail", svc.ListVolumesDetail)
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
		v3.GET("/snapshots/detail", svc.ListSnapshotsDetail)
		v3.POST("/snapshots", svc.CreateSnapshot)
		v3.GET("/snapshots/:id", svc.GetSnapshot)
		v3.PUT("/snapshots/:id", svc.UpdateSnapshot)
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
		v3.POST("/types", svc.CreateVolumeType)
		v3.GET("/types/:id", svc.GetVolumeType)
		v3.PUT("/types/:id", svc.UpdateVolumeType)
		v3.DELETE("/types/:id", svc.DeleteVolumeType)
		v3.POST("/types/:id/action", svc.VolumeTypeAction)
		v3.GET("/types/:id/os-volume-type-access", svc.ListVolumeTypeAccess)
		v3.GET("/types/:id/extra_specs", svc.ListVolumeTypeExtraSpecs)
		v3.POST("/types/:id/extra_specs", svc.CreateVolumeTypeExtraSpecs)
		v3.GET("/types/:id/extra_specs/:key", svc.GetVolumeTypeExtraSpecKey)
		v3.PUT("/types/:id/extra_specs/:key", svc.UpdateVolumeTypeExtraSpecKey)
		v3.DELETE("/types/:id/extra_specs/:key", svc.DeleteVolumeTypeExtraSpecKey)

		// Limits
		v3.GET("/limits", svc.GetLimits)

		// Services
		v3.GET("/os-services", svc.ListServices)

		// Volume transfers
		v3.POST("/os-volume-transfer", svc.CreateVolumeTransfer)
		v3.GET("/os-volume-transfer/:id", svc.GetVolumeTransfer)
		v3.POST("/os-volume-transfer/:id/accept", svc.AcceptVolumeTransfer)
		v3.DELETE("/os-volume-transfer/:id", svc.DeleteVolumeTransfer)

		// Volume/Snapshot management
		v3.POST("/os-volume-manage", svc.ManageVolume)
		v3.GET("/manageable_volumes", svc.ListManageableVolumes)
		v3.POST("/os-snapshot-manage", svc.ManageSnapshot)

		// Quotas
		v3.GET("/quota-sets/:id", svc.GetQuotaSet)
		v3.PUT("/quota-sets/:id", svc.UpdateQuotaSet)
		v3.DELETE("/quota-sets/:id", svc.DeleteQuotaSet)
	}
}

// CreateVolumeRequest represents a volume creation request
type CreateVolumeRequest struct {
	Volume struct {
		Name             string `json:"name"`
		Size             int    `json:"size" binding:"required"`
		Description      string `json:"description"`
		VolumeType       string `json:"volume_type"`
		SnapshotID       string `json:"snapshot_id"`
		SourceVolID      string `json:"source_volid"`
		ImageRef         string `json:"imageRef"`
		AvailabilityZone string `json:"availability_zone"`
		Encrypted        bool   `json:"encrypted"`
	} `json:"volume"`
}

// CreateVolume creates a new volume
func (svc *Service) CreateVolume(c *gin.Context) {
	var req CreateVolumeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Try to get project_id from URL param first, then from token context
	projectID := c.Param("project_id")
	if projectID == "" {
		projectID = c.GetString("project_id")
	}
	userID := c.GetString("user_id")
	volumeID := uuid.New().String()

	if req.Volume.Size < 1 {
		common.SendError(c, common.NewBadRequestError("volume size must be at least 1 GB"))
		return
	}

	// Enforce project quota before allocating storage.
	// Quota check and INSERT are done inside a transaction to prevent two concurrent
	// requests from both passing the check and both creating a volume.
	const defaultMaxVolumes = 10
	const defaultMaxGigabytes = 1000

	// Resolve volume_type: use request value or default
	volumeType := req.Volume.VolumeType
	if volumeType == "" {
		volumeType = "__DEFAULT__"
	}

	availabilityZone := req.Volume.AvailabilityZone
	if availabilityZone == "" {
		availabilityZone = "nova"
	}
	encrypted := req.Volume.Encrypted
	now := time.Now()

	tx, err := svc.activeDB().BeginTx(c.Request.Context(), database.TxOptions{})
	if err != nil {
		log.Error().Err(err).Str("operation", "create_volume_begin_tx").Msg("failed to begin transaction")
		common.SendError(c, common.NewInternalServerError("failed to create volume"))
		return
	}
	defer tx.Rollback(c.Request.Context()) //nolint:errcheck

	// Load per-project quota limits inside the transaction (fall back to defaults).
	var quotaVolumes, quotaGigabytes int
	if err := tx.QueryRow(c.Request.Context(),
		`SELECT "limit" FROM cinder_quotas WHERE project_id = $1 AND resource = 'volumes'`,
		projectID,
	).Scan(&quotaVolumes); err != nil {
		quotaVolumes = defaultMaxVolumes
	}
	if err := tx.QueryRow(c.Request.Context(),
		`SELECT "limit" FROM cinder_quotas WHERE project_id = $1 AND resource = 'gigabytes'`,
		projectID,
	).Scan(&quotaGigabytes); err != nil {
		quotaGigabytes = defaultMaxGigabytes
	}

	var usedCount, usedGB int
	if err := tx.QueryRow(c.Request.Context(),
		`SELECT COUNT(*), COALESCE(SUM(size_gb), 0) FROM volumes WHERE project_id = $1 AND status != 'deleted'`,
		projectID,
	).Scan(&usedCount, &usedGB); err != nil {
		log.Error().Err(err).Str("operation", "create_volume_quota_check").Msg("failed to query volume usage")
		common.SendError(c, common.NewInternalServerError("failed to check quota"))
		return
	}

	if usedCount+1 > quotaVolumes {
		common.SendError(c, common.NewQuotaExceededError("VolumeLimitExceeded"))
		return
	}
	if usedGB+req.Volume.Size > quotaGigabytes {
		common.SendError(c, common.NewQuotaExceededError("VolumeLimitExceeded"))
		return
	}

	// Insert volume record within the same transaction so the quota check and
	// row creation are atomic.
	if _, err := tx.Exec(c.Request.Context(), `
		INSERT INTO volumes (id, name, project_id, user_id, size_gb, status, bootable, volume_type, rbd_pool, rbd_image, availability_zone, encrypted, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`, volumeID, req.Volume.Name, projectID, userID, req.Volume.Size, "creating", false, volumeType, svc.cephPool, "volume-"+volumeID, availabilityZone, encrypted, now, now); err != nil {
		log.Error().Err(err).Str("operation", "create_volume_db").Msg("failed to insert volume into database")
		common.SendError(c, common.NewInternalServerError("failed to create volume"))
		return
	}

	if err := tx.Commit(c.Request.Context()); err != nil {
		log.Error().Err(err).Str("operation", "create_volume_commit").Msg("failed to commit volume creation")
		common.SendError(c, common.NewInternalServerError("failed to create volume"))
		return
	}

	// Create RBD volume in Ceph after the DB row is committed.
	// On Ceph failure, the DB row stays in 'creating'; an operator can clean it up.
	if err := svc.cephClient.CreateVolume(c.Request.Context(), volumeID, req.Volume.Size); err != nil {
		log.Error().Err(err).Str("operation", "create_volume_ceph").Msg("failed to create volume in Ceph")
		// Best-effort cleanup of the committed DB row.
		if _, cerr := svc.activeDB().Exec(c.Request.Context(),
			"DELETE FROM volumes WHERE id = $1", volumeID); cerr != nil {
			log.Error().Err(cerr).Str("operation", "create_volume_cleanup").Msg("failed to clean up DB row after Ceph failure")
		}
		common.SendError(c, common.NewServiceUnavailableError("failed to create volume in Ceph"))
		return
	}

	// Update status to available in background
	svc.wg.Add(1)
	go func() {
		defer svc.wg.Done()
		select {
		case <-time.After(100 * time.Millisecond):
		case <-svc.ctx.Done():
			return
		}
		ctx, cancel := context.WithTimeout(svc.ctx, 5*time.Second)
		defer cancel()
		_, _ = svc.activeDB().Exec(ctx,
			"UPDATE volumes SET status = $1, updated_at = $2 WHERE id = $3",
			"available", time.Now(), volumeID)
	}()

	c.JSON(http.StatusAccepted, gin.H{
		"volume": gin.H{
			"id":                volumeID,
			"name":              req.Volume.Name,
			"tenant_id":         projectID,
			"user_id":           userID,
			"size":              req.Volume.Size,
			"status":            "creating",
			"bootable":          false,
			"volume_type":       volumeType,
			"encrypted":         encrypted,
			"availability_zone": availabilityZone,
			"created_at":        now.Format("2006-01-02T15:04:05.000000"),
			"updated_at":        now.Format("2006-01-02T15:04:05.000000"),
			"metadata":          gin.H{},
			"attachments":       []interface{}{},
		},
	})
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

// buildVolumeFilterConditions returns extra WHERE conditions and args for volume list filters.
// queryArgs must already contain the project_id (or be empty for all_tenants admin queries).
// argIdx is the next placeholder index to use. Returns updated conditions, args, and argIdx.
func buildVolumeFilterConditions(c *gin.Context, queryArgs []interface{}, argIdx int) ([]string, []interface{}, int) {
	var extra []string

	if status := c.Query("status"); status != "" {
		extra = append(extra, fmt.Sprintf("status = $%d", argIdx))
		queryArgs = append(queryArgs, status)
		argIdx++
	}
	if name := c.Query("name"); name != "" {
		extra = append(extra, fmt.Sprintf("name = $%d", argIdx))
		queryArgs = append(queryArgs, name)
		argIdx++
	}
	if bootable := c.Query("bootable"); bootable != "" {
		boolVal := bootable == "true"
		extra = append(extra, fmt.Sprintf("bootable = $%d", argIdx))
		queryArgs = append(queryArgs, boolVal)
		argIdx++
	}
	if az := c.Query("availability_zone"); az != "" {
		extra = append(extra, fmt.Sprintf("availability_zone = $%d", argIdx))
		queryArgs = append(queryArgs, az)
		argIdx++
	}

	return extra, queryArgs, argIdx
}

// isAdminRequest returns true when the caller has the admin role.
func isAdminRequest(c *gin.Context) bool {
	roles, _ := c.Get("roles")
	roleList, _ := roles.([]string)
	for _, r := range roleList {
		if r == "admin" {
			return true
		}
	}
	return false
}

// ListVolumes lists all volumes (brief)
func (svc *Service) ListVolumes(c *gin.Context) {
	// Try to get project_id from URL param first, then from token context
	projectID := c.Param("project_id")
	if projectID == "" {
		projectID = c.GetString("project_id")
	}

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

	// Base WHERE: project scope (admin + all_tenants bypasses project filter)
	var conditions []string
	var queryArgs []interface{}
	argIdx := 1

	allTenants := c.Query("all_tenants") == "true" && isAdminRequest(c)
	if !allTenants {
		conditions = append(conditions, fmt.Sprintf("project_id = $%d", argIdx))
		queryArgs = append(queryArgs, projectID)
		argIdx++
	}

	// Marker-based pagination
	if marker := c.Query("marker"); marker != "" {
		var markerCreatedAt time.Time
		markerQuery := "SELECT created_at FROM volumes WHERE id = $1"
		markerArgs := []interface{}{marker}
		if !allTenants {
			markerQuery += " AND project_id = $2"
			markerArgs = append(markerArgs, projectID)
		}
		err := svc.activeDB().QueryRow(c.Request.Context(), markerQuery, markerArgs...).Scan(&markerCreatedAt)
		if err == nil {
			conditions = append(conditions, fmt.Sprintf("created_at < $%d", argIdx))
			queryArgs = append(queryArgs, markerCreatedAt)
			argIdx++
		}
	}

	// Additional filters
	var extra []string
	extra, queryArgs, argIdx = buildVolumeFilterConditions(c, queryArgs, argIdx)
	conditions = append(conditions, extra...)

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + joinConditions(conditions)
	}

	queryArgs = append(queryArgs, limit, offset)

	rows, err := svc.activeDB().Query(c.Request.Context(), fmt.Sprintf(`
		SELECT id, name, size_gb, status, bootable, COALESCE(availability_zone, 'nova')
		FROM volumes
		%s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIdx, argIdx+1), queryArgs...)

	if err != nil {
		log.Error().Err(err).Str("operation", "list_volumes").Msg("failed to query volumes")
		common.SendError(c, common.NewInternalServerError("failed to list volumes"))
		return
	}
	defer rows.Close()

	var volumes []gin.H
	for rows.Next() {
		var id, name, status, availabilityZone string
		var size int
		var bootable bool

		if err := rows.Scan(&id, &name, &size, &status, &bootable, &availabilityZone); err != nil {
			log.Warn().Err(err).Msg("failed to scan volume row")
			continue
		}

		volumes = append(volumes, gin.H{
			"id":                id,
			"name":              name,
			"size":              size,
			"status":            status,
			"bootable":          bootable,
			"availability_zone": availabilityZone,
		})
	}
	if err := rows.Err(); err != nil {
		log.Error().Err(err).Str("operation", "list_volumes").Msg("rows iteration error")
		common.SendError(c, common.NewInternalServerError("failed to list volumes"))
		return
	}

	if volumes == nil {
		volumes = []gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{"volumes": volumes})
}

// ListVolumesDetail lists all volumes (detailed)
func (svc *Service) ListVolumesDetail(c *gin.Context) {
	// Try to get project_id from URL param first, then from token context
	projectID := c.Param("project_id")
	if projectID == "" {
		projectID = c.GetString("project_id")
	}

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

	// Base WHERE: project scope (admin + all_tenants bypasses project filter)
	var conditions []string
	var queryArgs []interface{}
	argIdx := 1

	allTenants := c.Query("all_tenants") == "true" && isAdminRequest(c)
	if !allTenants {
		conditions = append(conditions, fmt.Sprintf("v.project_id = $%d", argIdx))
		queryArgs = append(queryArgs, projectID)
		argIdx++
	}

	// Marker-based pagination
	if marker := c.Query("marker"); marker != "" {
		var markerCreatedAt time.Time
		markerQuery := "SELECT created_at FROM volumes WHERE id = $1"
		markerArgs := []interface{}{marker}
		if !allTenants {
			markerQuery += " AND project_id = $2"
			markerArgs = append(markerArgs, projectID)
		}
		err := svc.activeDB().QueryRow(c.Request.Context(), markerQuery, markerArgs...).Scan(&markerCreatedAt)
		if err == nil {
			conditions = append(conditions, fmt.Sprintf("v.created_at < $%d", argIdx))
			queryArgs = append(queryArgs, markerCreatedAt)
			argIdx++
		}
	}

	// Additional filters (using v. alias)
	if status := c.Query("status"); status != "" {
		conditions = append(conditions, fmt.Sprintf("v.status = $%d", argIdx))
		queryArgs = append(queryArgs, status)
		argIdx++
	}
	if name := c.Query("name"); name != "" {
		conditions = append(conditions, fmt.Sprintf("v.name = $%d", argIdx))
		queryArgs = append(queryArgs, name)
		argIdx++
	}
	if bootable := c.Query("bootable"); bootable != "" {
		conditions = append(conditions, fmt.Sprintf("v.bootable = $%d", argIdx))
		queryArgs = append(queryArgs, bootable == "true")
		argIdx++
	}
	if az := c.Query("availability_zone"); az != "" {
		conditions = append(conditions, fmt.Sprintf("v.availability_zone = $%d", argIdx))
		queryArgs = append(queryArgs, az)
		argIdx++
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + joinConditions(conditions)
	}

	queryArgs = append(queryArgs, limit, offset)

	rows, err := svc.activeDB().Query(c.Request.Context(), fmt.Sprintf(`
		SELECT v.id, v.name, v.size_gb, v.status, v.bootable, v.attached_to_instance_id, v.created_at, v.updated_at, COALESCE(v.volume_type, '__DEFAULT__'), COALESCE(v.availability_zone, 'nova'), COALESCE(v.encrypted, false), v.description, v.user_id::text
		FROM volumes v
		%s
		ORDER BY v.created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIdx, argIdx+1), queryArgs...)

	if err != nil {
		log.Error().Err(err).Str("operation", "list_volumes_detail").Msg("failed to query volumes")
		common.SendError(c, common.NewInternalServerError("failed to list volumes"))
		return
	}
	defer rows.Close()

	var volumes []gin.H
	for rows.Next() {
		var id, name, status, volumeType, availabilityZone string
		var size int
		var bootable, encrypted bool
		var attachedTo sql.NullString
		var description sql.NullString
		var userID sql.NullString
		var createdAt, updatedAt time.Time

		if err := rows.Scan(&id, &name, &size, &status, &bootable, &attachedTo, &createdAt, &updatedAt, &volumeType, &availabilityZone, &encrypted, &description, &userID); err != nil {
			log.Warn().Err(err).Msg("failed to scan volume detail row")
			continue
		}

		attachments := []interface{}{}
		if attachedTo.Valid {
			attachments = append(attachments, map[string]interface{}{
				"id":            id,
				"attachment_id": id,
				"volume_id":     id,
				"server_id":     attachedTo.String,
				"host_name":     "",
				"device":        "/dev/vdb",
				"attached_at":   createdAt.Format("2006-01-02T15:04:05.000000"),
			})
		}

		resolvedUserID := projectID
		if userID.Valid && userID.String != "" {
			resolvedUserID = userID.String
		}

		var descriptionVal interface{}
		if description.Valid {
			descriptionVal = description.String
		}

		volumes = append(volumes, gin.H{
			"id":                  id,
			"name":                name,
			"description":         descriptionVal,
			"tenant_id":           projectID,
			"user_id":             resolvedUserID,
			"size":                size,
			"status":              status,
			"bootable":            fmt.Sprintf("%t", bootable),
			"availability_zone":   availabilityZone,
			"volume_type":         volumeType,
			"encrypted":           encrypted,
			"multiattach":         false,
			"replication_status":  "disabled",
			"migration_status":    nil,
			"consistencygroup_id": nil,
			"source_volid":        nil,
			"snapshot_id":         nil,
			"created_at":          createdAt.Format("2006-01-02T15:04:05.000000"),
			"updated_at":          updatedAt.Format("2006-01-02T15:04:05.000000"),
			"attachments":         attachments,
			"links": []map[string]string{
				{"rel": "self", "href": fmt.Sprintf("/v3/%s/volumes/%s", projectID, id)},
				{"rel": "bookmark", "href": fmt.Sprintf("/volumes/%s", id)},
			},
			"os-vol-host-attr:host":          "localhost",
			"os-vol-mig-status-attr:migstat": nil,
			"os-vol-mig-status-attr:name_id": nil,
			"os-vol-tenant-attr:tenant_id":   projectID,
		})
	}
	if err := rows.Err(); err != nil {
		log.Error().Err(err).Str("operation", "list_volumes_detail").Msg("rows iteration error")
		common.SendError(c, common.NewInternalServerError("failed to list volumes"))
		return
	}

	if volumes == nil {
		volumes = []gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{"volumes": volumes})
}

// GetVolume returns a single volume
func (svc *Service) GetVolume(c *gin.Context) {
	volumeID := c.Param("id")
	// Try to get project_id from URL param first, then from token context
	projectID := c.Param("project_id")
	if projectID == "" {
		projectID = c.GetString("project_id")
	}

	var id, name, status, volumeType, availabilityZone string
	var size int
	var bootable, encrypted bool
	var attachedTo sql.NullString
	var description sql.NullString
	var userID sql.NullString
	var createdAt, updatedAt time.Time

	// Support lookup by ID or name
	err := svc.activeDB().QueryRow(c.Request.Context(), `
		SELECT id, name, size_gb, status, bootable, attached_to_instance_id, created_at, updated_at, COALESCE(volume_type, '__DEFAULT__'), COALESCE(availability_zone, 'nova'), COALESCE(encrypted, false), description, user_id::text
		FROM volumes
		WHERE project_id = $2 AND ((id::text = $1) OR (name = $1))
	`, volumeID, projectID).Scan(&id, &name, &size, &status, &bootable, &attachedTo, &createdAt, &updatedAt, &volumeType, &availabilityZone, &encrypted, &description, &userID)

	if errors.Is(err, database.ErrNoRows) {
		common.SendError(c, common.NewNotFoundError("volume"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "get_volume").Str("volume_id", volumeID).Msg("failed to query volume")
		common.SendError(c, common.NewInternalServerError("failed to get volume"))
		return
	}

	attachments := []interface{}{}
	if attachedTo.Valid {
		attachments = append(attachments, map[string]interface{}{
			"id":            id,
			"attachment_id": id,
			"volume_id":     id,
			"server_id":     attachedTo.String,
			"host_name":     "",
			"device":        "/dev/vdb",
			"attached_at":   createdAt.Format("2006-01-02T15:04:05.000000"),
		})
	}

	// user_id falls back to project_id if not set
	resolvedUserID := projectID
	if userID.Valid && userID.String != "" {
		resolvedUserID = userID.String
	}

	var descriptionVal interface{}
	if description.Valid {
		descriptionVal = description.String
	}

	c.JSON(http.StatusOK, gin.H{
		"volume": gin.H{
			"id":                  id,
			"name":                name,
			"description":         descriptionVal,
			"tenant_id":           projectID,
			"user_id":             resolvedUserID,
			"size":                size,
			"status":              status,
			"bootable":            fmt.Sprintf("%t", bootable),
			"availability_zone":   availabilityZone,
			"volume_type":         volumeType,
			"encrypted":           encrypted,
			"multiattach":         false,
			"replication_status":  "disabled",
			"migration_status":    nil,
			"consistencygroup_id": nil,
			"source_volid":        nil,
			"snapshot_id":         nil,
			"created_at":          createdAt.Format("2006-01-02T15:04:05.000000"),
			"updated_at":          updatedAt.Format("2006-01-02T15:04:05.000000"),
			"attachments":         attachments,
			"links": []map[string]string{
				{"rel": "self", "href": fmt.Sprintf("/v3/%s/volumes/%s", projectID, id)},
				{"rel": "bookmark", "href": fmt.Sprintf("/volumes/%s", id)},
			},
			"os-vol-host-attr:host":              "localhost",
			"os-vol-mig-status-attr:migstat":     nil,
			"os-vol-mig-status-attr:name_id":     nil,
			"os-vol-tenant-attr:tenant_id":        projectID,
		},
	})
}

// DeleteVolume deletes a volume
func (svc *Service) DeleteVolume(c *gin.Context) {
	volumeID := c.Param("id")
	// Try to get project_id from URL param first, then from token context
	projectID := c.Param("project_id")
	if projectID == "" {
		projectID = c.GetString("project_id")
	}

	// Check if volume is attached (support lookup by ID or name)
	var attachedTo sql.NullString
	var actualVolumeID string
	var volumeUserID sql.NullString
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT id, attached_to_instance_id, user_id FROM volumes WHERE project_id = $2 AND ((id::text = $1) OR (name = $1))",
		volumeID, projectID,
	).Scan(&actualVolumeID, &attachedTo, &volumeUserID)

	if errors.Is(err, database.ErrNoRows) {
		common.SendError(c, common.NewNotFoundError("volume"))
		return
	}

	if attachedTo.Valid {
		common.SendError(c, common.NewBadRequestError("volume is attached to an instance"))
		return
	}

	// Policy enforcement
	if keystone.PolicyEngine != nil {
		target := map[string]interface{}{
			"user_id":    volumeUserID.String,
			"project_id": projectID,
		}
		credentials := map[string]interface{}{
			"user_id":    c.GetString("user_id"),
			"project_id": c.GetString("project_id"),
			"roles":      c.GetStringSlice("roles"),
		}
		if !keystone.PolicyEngine.Enforce("volume:delete", target, credentials) {
			common.SendError(c, common.NewForbiddenError("Policy doesn't allow this action"))
			return
		}
	}

	// Delete from database first (recoverable if Ceph cleanup fails)
	_, err = svc.activeDB().Exec(c.Request.Context(),
		"DELETE FROM volumes WHERE id = $1 AND project_id = $2",
		actualVolumeID, projectID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "delete_volume_db").Str("volume_id", volumeID).Msg("failed to delete volume from database")
		common.SendError(c, common.NewInternalServerError("failed to delete volume"))
		return
	}

	// Clean up storage backend (best-effort — orphaned backend data is preferable to data loss)
	// Skip in stub mode: no real backend is available.
	if svc.mode != "stub" {
		if err := svc.cephClient.DeleteVolume(c.Request.Context(), actualVolumeID); err != nil {
			log.Error().Err(err).Str("operation", "delete_volume_ceph").Str("volume_id", actualVolumeID).Msg("failed to delete volume from Ceph (orphaned)")
		}
	}

	c.Status(http.StatusNoContent)
}

// VolumeAction performs an action on a volume
func (svc *Service) VolumeAction(c *gin.Context) {
	volumeID := c.Param("id")
	// Try to get project_id from URL param first, then from token context
	projectID := c.Param("project_id")
	if projectID == "" {
		projectID = c.GetString("project_id")
	}

	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Fetch current volume status and size once for all state-guarded actions
	var currentStatus string
	var currentSizeGB int
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT status, size_gb FROM volumes WHERE id = $1 AND project_id = $2",
		volumeID, projectID,
	).Scan(&currentStatus, &currentSizeGB)
	if errors.Is(err, database.ErrNoRows) {
		common.SendError(c, common.NewNotFoundError("volume"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "volume_action_status").Str("volume_id", volumeID).Msg("failed to query volume status")
		common.SendError(c, common.NewInternalServerError("failed to query volume"))
		return
	}

	// Handle attach action
	if attachData, ok := req["os-attach"]; ok {
		attachMap, ok := attachData.(map[string]interface{})
		if !ok {
			common.SendError(c, common.NewBadRequestError("invalid os-attach payload"))
			return
		}
		instanceID, ok := attachMap["instance_uuid"].(string)
		if !ok || instanceID == "" {
			common.SendError(c, common.NewBadRequestError("instance_uuid is required"))
			return
		}

		// Verify the instance belongs to the caller's project.
		var instanceExists bool
		if err := svc.activeDB().QueryRow(c.Request.Context(),
			"SELECT EXISTS(SELECT 1 FROM instances WHERE id = $1 AND project_id = $2)",
			instanceID, projectID,
		).Scan(&instanceExists); err != nil || !instanceExists {
			common.SendError(c, common.NewNotFoundError("instance"))
			return
		}

		// Atomically verify status and transition to 'attaching' then 'in-use'
		// to prevent concurrent attach of the same volume.
		tx, err := svc.activeDB().BeginTx(c.Request.Context(), database.TxOptions{})
		if err != nil {
			log.Error().Err(err).Str("operation", "attach_volume_begin_tx").Str("volume_id", volumeID).Msg("failed to begin transaction")
			common.SendError(c, common.NewInternalServerError("failed to attach volume"))
			return
		}
		defer tx.Rollback(c.Request.Context()) //nolint:errcheck

		var txStatus string
		if err := tx.QueryRow(c.Request.Context(),
			"SELECT status FROM volumes WHERE id = $1 AND project_id = $2",
			volumeID, projectID,
		).Scan(&txStatus); err != nil {
			log.Error().Err(err).Str("operation", "attach_volume_tx_status").Str("volume_id", volumeID).Msg("failed to query volume status in transaction")
			common.SendError(c, common.NewInternalServerError("failed to attach volume"))
			return
		}

		if txStatus != "available" {
			c.JSON(http.StatusConflict, gin.H{
				"conflictingRequest": gin.H{
					"message": fmt.Sprintf("Invalid volume: Volume %s status must be available to attach, currently %s.", volumeID, txStatus),
					"code":    409,
				},
			})
			return
		}

		if _, err := tx.Exec(c.Request.Context(), `
			UPDATE volumes
			SET attached_to_instance_id = $1, status = $2, updated_at = $3
			WHERE id = $4 AND project_id = $5
		`, instanceID, "in-use", time.Now(), volumeID, projectID); err != nil {
			log.Error().Err(err).Str("operation", "attach_volume").Str("volume_id", volumeID).Msg("failed to attach volume")
			common.SendError(c, common.NewInternalServerError("failed to attach volume"))
			return
		}

		if err := tx.Commit(c.Request.Context()); err != nil {
			log.Error().Err(err).Str("operation", "attach_volume_commit").Str("volume_id", volumeID).Msg("failed to commit attach transaction")
			common.SendError(c, common.NewInternalServerError("failed to attach volume"))
			return
		}

		c.Status(http.StatusAccepted)
		return
	}

	// Handle detach action
	if _, ok := req["os-detach"]; ok {
		// Wrap status read+update in a transaction to prevent concurrent detach
		// from reading "in-use" twice and both proceeding.
		tx, err := svc.activeDB().BeginTx(c.Request.Context(), database.TxOptions{})
		if err != nil {
			log.Error().Err(err).Str("operation", "detach_volume_begin_tx").Str("volume_id", volumeID).Msg("failed to begin transaction")
			common.SendError(c, common.NewInternalServerError("failed to detach volume"))
			return
		}
		defer tx.Rollback(c.Request.Context()) //nolint:errcheck

		var txStatus string
		if err := tx.QueryRow(c.Request.Context(),
			"SELECT status FROM volumes WHERE id = $1 AND project_id = $2",
			volumeID, projectID,
		).Scan(&txStatus); err != nil {
			log.Error().Err(err).Str("operation", "detach_volume_tx_status").Str("volume_id", volumeID).Msg("failed to query volume status in transaction")
			common.SendError(c, common.NewInternalServerError("failed to detach volume"))
			return
		}

		if txStatus != "in-use" {
			c.JSON(http.StatusConflict, gin.H{
				"conflictingRequest": gin.H{
					"message": fmt.Sprintf("Invalid volume: Volume %s status must be in-use to detach, currently %s.", volumeID, txStatus),
					"code":    409,
				},
			})
			return
		}

		if _, err := tx.Exec(c.Request.Context(), `
			UPDATE volumes
			SET attached_to_instance_id = NULL, status = $1, updated_at = $2
			WHERE id = $3 AND project_id = $4
		`, "available", time.Now(), volumeID, projectID); err != nil {
			log.Error().Err(err).Str("operation", "detach_volume").Str("volume_id", volumeID).Msg("failed to detach volume")
			common.SendError(c, common.NewInternalServerError("failed to detach volume"))
			return
		}

		if err := tx.Commit(c.Request.Context()); err != nil {
			log.Error().Err(err).Str("operation", "detach_volume_commit").Str("volume_id", volumeID).Msg("failed to commit detach transaction")
			common.SendError(c, common.NewInternalServerError("failed to detach volume"))
			return
		}

		c.Status(http.StatusAccepted)
		return
	}

	// Handle extend action
	if extendData, ok := req["os-extend"]; ok {
		extendMap, ok := extendData.(map[string]interface{})
		if !ok {
			common.SendError(c, common.NewBadRequestError("invalid os-extend payload"))
			return
		}

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
				common.SendError(c, common.NewBadRequestError("invalid new_size format"))
				return
			}
			newSize = int(parsed)
		case string:
			parsed, err := strconv.Atoi(v)
			if err != nil {
				common.SendError(c, common.NewBadRequestError("invalid new_size format"))
				return
			}
			newSize = parsed
		default:
			common.SendError(c, common.NewBadRequestError(fmt.Sprintf("invalid new_size type: %T", newSizeVal)))
			return
		}

		// Wrap status read+size check+update in a transaction to prevent concurrent
		// extends from both reading the same old size and both proceeding.
		tx, err := svc.activeDB().BeginTx(c.Request.Context(), database.TxOptions{})
		if err != nil {
			log.Error().Err(err).Str("operation", "extend_volume_begin_tx").Str("volume_id", volumeID).Msg("failed to begin transaction")
			common.SendError(c, common.NewInternalServerError("failed to extend volume"))
			return
		}
		defer tx.Rollback(c.Request.Context()) //nolint:errcheck

		var txStatus string
		var txSizeGB int
		if err := tx.QueryRow(c.Request.Context(),
			"SELECT status, size_gb FROM volumes WHERE id = $1 AND project_id = $2",
			volumeID, projectID,
		).Scan(&txStatus, &txSizeGB); err != nil {
			log.Error().Err(err).Str("operation", "extend_volume_tx_status").Str("volume_id", volumeID).Msg("failed to query volume in transaction")
			common.SendError(c, common.NewInternalServerError("failed to extend volume"))
			return
		}

		if txStatus != "available" {
			c.JSON(http.StatusConflict, gin.H{
				"conflictingRequest": gin.H{
					"message": fmt.Sprintf("Invalid volume: Volume %s status must be available to extend, currently %s.", volumeID, txStatus),
					"code":    409,
				},
			})
			return
		}

		if newSize <= txSizeGB {
			common.SendError(c, common.NewBadRequestError(
				fmt.Sprintf("New size must be greater than current size (%d GB)", txSizeGB)))
			return
		}

		if _, err := tx.Exec(c.Request.Context(), `
			UPDATE volumes
			SET size_gb = $1, updated_at = $2
			WHERE id = $3 AND project_id = $4
		`, newSize, time.Now(), volumeID, projectID); err != nil {
			log.Error().Err(err).Str("operation", "extend_volume").Str("volume_id", volumeID).Msg("failed to extend volume")
			common.SendError(c, common.NewInternalServerError("failed to extend volume"))
			return
		}

		if err := tx.Commit(c.Request.Context()); err != nil {
			log.Error().Err(err).Str("operation", "extend_volume_commit").Str("volume_id", volumeID).Msg("failed to commit extend transaction")
			common.SendError(c, common.NewInternalServerError("failed to extend volume"))
			return
		}

		c.Status(http.StatusAccepted)
		return
	}

	// Handle retype action
	if retypeData, ok := req["os-retype"]; ok {
		retypeMap, ok := retypeData.(map[string]interface{})
		if !ok {
			common.SendError(c, common.NewBadRequestError("invalid os-retype payload"))
			return
		}
		newType, ok := retypeMap["new_type"].(string)
		if !ok || newType == "" {
			common.SendError(c, common.NewBadRequestError("new_type is required"))
			return
		}

		// Get or create volume type
		var typeID string
		err := svc.activeDB().QueryRow(c.Request.Context(),
			"SELECT id FROM volume_types WHERE name = $1",
			newType,
		).Scan(&typeID)

		if errors.Is(err, database.ErrNoRows) {
			// Create new volume type if it doesn't exist
			typeID = uuid.New().String()
			_, err = svc.activeDB().Exec(c.Request.Context(),
				"INSERT INTO volume_types (id, name, description, is_public) VALUES ($1, $2, $3, $4)",
				typeID, newType, "Auto-created volume type", true,
			)
			if err != nil {
				log.Error().Err(err).Str("operation", "retype_volume_create_type").Msg("failed to create volume type")
				common.SendError(c, common.NewInternalServerError("failed to create volume type"))
				return
			}
		}

		// Update volume type
		_, err = svc.activeDB().Exec(c.Request.Context(), `
			UPDATE volumes
			SET volume_type = $1, updated_at = $2
			WHERE id = $3 AND project_id = $4
		`, newType, time.Now(), volumeID, projectID)

		if err != nil {
			log.Error().Err(err).Str("operation", "retype_volume").Str("volume_id", volumeID).Msg("failed to retype volume")
			common.SendError(c, common.NewInternalServerError("failed to retype volume"))
			return
		}

		c.Status(http.StatusAccepted)
		return
	}

	// Handle unmanage action
	if _, ok := req["os-unmanage"]; ok {
		svc.UnmanageVolume(c, volumeID)
		return
	}

	// Handle update readonly flag action
	if readonlyData, ok := req["os-update_readonly_flag"]; ok {
		readonlyMap, ok := readonlyData.(map[string]interface{})
		if !ok {
			common.SendError(c, common.NewBadRequestError("invalid os-update_readonly_flag payload"))
			return
		}
		readonly, ok := readonlyMap["readonly"].(bool)
		if !ok {
			common.SendError(c, common.NewBadRequestError("readonly must be a boolean"))
			return
		}

		// Update volume readonly flag (stored in metadata or separate field)
		// For now, update in volume_metadata table
		_, err := svc.activeDB().Exec(c.Request.Context(), `
			INSERT INTO volume_metadata (volume_id, meta_key, meta_value)
			VALUES ($1, 'readonly', $2)
			ON CONFLICT (volume_id, meta_key) DO UPDATE SET meta_value = EXCLUDED.meta_value
		`, volumeID, fmt.Sprintf("%t", readonly))

		if err != nil {
			log.Error().Err(err).Str("operation", "update_readonly_flag").Str("volume_id", volumeID).Msg("failed to update readonly flag")
			common.SendError(c, common.NewInternalServerError("failed to update readonly flag"))
			return
		}

		c.Status(http.StatusAccepted)
		return
	}

	// Handle set image metadata action (make volume bootable)
	if imageMetadataData, ok := req["os-set_image_metadata"]; ok {
		imageMetadataMap, ok := imageMetadataData.(map[string]interface{})
		if !ok {
			common.SendError(c, common.NewBadRequestError("invalid os-set_image_metadata payload"))
			return
		}
		metadata, ok := imageMetadataMap["metadata"].(map[string]interface{})
		if !ok {
			common.SendError(c, common.NewBadRequestError("metadata must be an object"))
			return
		}

		// Store image metadata to make volume bootable
		for key, value := range metadata {
			valueStr := fmt.Sprintf("%v", value)
			_, err := svc.activeDB().Exec(c.Request.Context(), `
				INSERT INTO volume_metadata (volume_id, meta_key, meta_value)
				VALUES ($1, $2, $3)
				ON CONFLICT (volume_id, meta_key) DO UPDATE SET meta_value = EXCLUDED.meta_value
			`, volumeID, "volume_image_"+key, valueStr)

			if err != nil {
				log.Error().Err(err).Str("operation", "set_image_metadata").Str("volume_id", volumeID).Msg("failed to set image metadata")
				common.SendError(c, common.NewInternalServerError("failed to set image metadata"))
				return
			}
		}

		c.JSON(http.StatusOK, gin.H{})
		return
	}

	// Handle unset image metadata action
	if unsetImageMetadata, ok := req["os-unset_image_metadata"]; ok {
		unsetMap, ok := unsetImageMetadata.(map[string]interface{})
		if !ok {
			common.SendError(c, common.NewBadRequestError("invalid os-unset_image_metadata payload"))
			return
		}
		key, ok := unsetMap["key"].(string)
		if !ok || key == "" {
			common.SendError(c, common.NewBadRequestError("key is required"))
			return
		}

		// Delete metadata entry with prefixed key
		metadataKey := "volume_image_" + key
		_, err := svc.activeDB().Exec(c.Request.Context(), `
			DELETE FROM volume_metadata
			WHERE volume_id = $1 AND meta_key = $2
		`, volumeID, metadataKey)

		if err != nil {
			log.Error().Err(err).Str("operation", "unset_image_metadata").Str("volume_id", volumeID).Msg("failed to unset image metadata")
			common.SendError(c, common.NewInternalServerError("failed to unset image metadata"))
			return
		}

		c.Status(http.StatusOK)
		return
	}

	// Handle reimage volume action
	if reimageData, ok := req["os-reimage"]; ok {
		reimageMap, ok := reimageData.(map[string]interface{})
		if !ok {
			common.SendError(c, common.NewBadRequestError("invalid os-reimage payload"))
			return
		}
		imageID, ok := reimageMap["image_id"].(string)
		if !ok || imageID == "" {
			common.SendError(c, common.NewBadRequestError("image_id is required"))
			return
		}

		// Update volume to bootable and store image ID
		_, err := svc.activeDB().Exec(c.Request.Context(), `
			UPDATE volumes
			SET bootable = true, updated_at = $1
			WHERE id = $2 AND project_id = $3
		`, time.Now(), volumeID, projectID)

		if err != nil {
			log.Error().Err(err).Str("operation", "reimage_volume").Str("volume_id", volumeID).Msg("failed to reimage volume")
			common.SendError(c, common.NewInternalServerError("failed to reimage volume"))
			return
		}

		// Store image_id as metadata
		_, err = svc.activeDB().Exec(c.Request.Context(), `
			INSERT INTO volume_metadata (volume_id, meta_key, meta_value, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (volume_id, meta_key)
			DO UPDATE SET meta_value = EXCLUDED.meta_value, updated_at = EXCLUDED.updated_at
		`, volumeID, "volume_image_id", imageID, time.Now(), time.Now())

		if err != nil {
			log.Error().Err(err).Str("operation", "reimage_volume_metadata").Str("volume_id", volumeID).Msg("failed to store image metadata")
			common.SendError(c, common.NewInternalServerError("failed to store image metadata"))
			return
		}

		c.Status(http.StatusAccepted)
		return
	}

	// Handle force detach action
	if _, ok := req["os-force_detach"]; ok {
		// Force detach volume from any server
		// In stub mode, just mark as available
		// In real mode, would force detach from hypervisor
		_, err := svc.activeDB().Exec(c.Request.Context(), `
			UPDATE volumes
			SET status = $1, attached_to_instance_id = NULL, updated_at = $2
			WHERE id = $3 AND project_id = $4
		`, "available", time.Now(), volumeID, projectID)

		if err != nil {
			log.Error().Err(err).Str("operation", "force_detach_volume").Str("volume_id", volumeID).Msg("failed to force detach volume")
			common.SendError(c, common.NewInternalServerError("failed to force detach volume"))
			return
		}

		c.Status(http.StatusAccepted)
		return
	}

	// Handle reset status action (admin operation)
	if resetStatusData, ok := req["os-reset_status"]; ok {
		// Verify admin role
		roles, exists := c.Get("roles")
		if !exists {
			common.SendError(c, common.NewForbiddenError("Policy doesn't allow os-reset_status to be performed."))
			return
		}

		roleList, _ := roles.([]string)
		isAdmin := false
		for _, role := range roleList {
			if role == "admin" {
				isAdmin = true
				break
			}
		}

		if !isAdmin {
			common.SendError(c, common.NewForbiddenError("Policy doesn't allow os-reset_status to be performed."))
			return
		}

		resetStatusMap, ok := resetStatusData.(map[string]interface{})
		if !ok {
			common.SendError(c, common.NewBadRequestError("invalid os-reset_status payload"))
			return
		}
		newStatus, ok := resetStatusMap["status"].(string)
		if !ok || newStatus == "" {
			common.SendError(c, common.NewBadRequestError("status is required"))
			return
		}

		// Admin operation to manually set volume status
		_, err := svc.activeDB().Exec(c.Request.Context(), `
			UPDATE volumes
			SET status = $1, updated_at = $2
			WHERE id = $3 AND project_id = $4
		`, newStatus, time.Now(), volumeID, projectID)

		if err != nil {
			log.Error().Err(err).Str("operation", "reset_volume_status").Str("volume_id", volumeID).Msg("failed to reset volume status")
			common.SendError(c, common.NewInternalServerError("failed to reset volume status"))
			return
		}

		c.Status(http.StatusAccepted)
		return
	}

	common.SendError(c, common.NewBadRequestError("unknown action"))
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
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Try to get project_id from URL param first, then from token context
	projectID := c.Param("project_id")
	if projectID == "" {
		projectID = c.GetString("project_id")
	}
	snapshotID := uuid.New().String()

	// Start transaction to lock the volume row during snapshot creation
	tx, err := svc.activeDB().BeginTx(c.Request.Context(), database.TxOptions{})
	if err != nil {
		common.SendError(c, common.NewInternalServerError("failed to begin transaction"))
		return
	}
	defer tx.Rollback(c.Request.Context()) //nolint:errcheck

	// Lock the volume row to prevent concurrent delete/extend
	var volumeID, volStatus string
	var size int
	err = tx.QueryRow(c.Request.Context(),
		"SELECT id, size_gb, status FROM volumes WHERE id = $1 AND project_id = $2 FOR UPDATE",
		req.Snapshot.VolumeID, projectID,
	).Scan(&volumeID, &size, &volStatus)

	if errors.Is(err, database.ErrNoRows) {
		common.SendError(c, common.NewNotFoundError("volume"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "create_snapshot_query_vol").Str("volume_id", req.Snapshot.VolumeID).Msg("failed to query volume")
		common.SendError(c, common.NewInternalServerError("failed to query volume"))
		return
	}
	if volStatus != "available" && volStatus != "in-use" {
		common.SendError(c, common.NewConflictError(
			fmt.Sprintf("Volume %s status must be available or in-use to snapshot, currently %s", req.Snapshot.VolumeID, volStatus)))
		return
	}

	// Create snapshot in Ceph
	if err := svc.cephClient.CreateSnapshot(c.Request.Context(), volumeID, snapshotID); err != nil {
		log.Error().Err(err).Str("operation", "create_snapshot_ceph").Str("volume_id", volumeID).Msg("failed to create snapshot in Ceph")
		common.SendError(c, common.NewServiceUnavailableError("failed to create snapshot"))
		return
	}

	// Insert into database
	now := time.Now()
	_, err = tx.Exec(c.Request.Context(), `
		INSERT INTO snapshots (id, name, volume_id, project_id, size_gb, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, snapshotID, req.Snapshot.Name, volumeID, projectID, size, "creating", now)

	if err != nil {
		_ = svc.cephClient.DeleteSnapshot(c.Request.Context(), volumeID, snapshotID)
		log.Error().Err(err).Str("operation", "create_snapshot_db").Msg("failed to insert snapshot into database")
		common.SendError(c, common.NewInternalServerError("failed to create snapshot"))
		return
	}

	if err := tx.Commit(c.Request.Context()); err != nil {
		_ = svc.cephClient.DeleteSnapshot(c.Request.Context(), volumeID, snapshotID)
		common.SendError(c, common.NewInternalServerError("failed to commit snapshot creation"))
		return
	}

	// Update status to available
	svc.wg.Add(1)
	go func() {
		defer svc.wg.Done()
		select {
		case <-time.After(100 * time.Millisecond):
		case <-svc.ctx.Done():
			return
		}
		ctx, cancel := context.WithTimeout(svc.ctx, 5*time.Second)
		defer cancel()
		if _, err := svc.activeDB().Exec(ctx,
			"UPDATE snapshots SET status = $1, updated_at = $2 WHERE id = $3",
			"available", time.Now(), snapshotID,
		); err != nil {
			log.Error().Err(err).Str("snapshot_id", snapshotID).Msg("CRITICAL: failed to mark snapshot available")
		}
	}()

	c.JSON(http.StatusAccepted, gin.H{
		"snapshot": gin.H{
			"id":         snapshotID,
			"name":       req.Snapshot.Name,
			"volume_id":  volumeID,
			"size":       size,
			"status":     "creating",
			"created_at": now.Format("2006-01-02T15:04:05.000000"),
			"os-extended-snapshot-attributes:progress":   "0%",
			"os-extended-snapshot-attributes:project_id": projectID,
		},
	})
}

// ListSnapshotsDetail is an alias that works with both URL patterns
func (svc *Service) ListSnapshotsDetail(c *gin.Context) {
	// Try to get project_id from URL param first, then from token context
	projectID := c.Param("project_id")
	if projectID == "" {
		projectID = c.GetString("project_id")
	}

	// Parse pagination parameters
	limit := common.DefaultPaginationLimit
	if limitParam := c.Query("limit"); limitParam != "" {
		if parsedLimit, err := strconv.Atoi(limitParam); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}
	limit = common.CapLimit(limit)

	conditions := []string{"project_id = $1"}
	queryArgs := []interface{}{projectID}
	argIdx := 2

	// Marker-based pagination
	if marker := c.Query("marker"); marker != "" {
		var markerCreatedAt time.Time
		err := svc.activeDB().QueryRow(c.Request.Context(),
			"SELECT created_at FROM snapshots WHERE id = $1 AND project_id = $2",
			marker, projectID,
		).Scan(&markerCreatedAt)
		if err == nil {
			conditions = append(conditions, fmt.Sprintf("created_at < $%d", argIdx))
			queryArgs = append(queryArgs, markerCreatedAt)
			argIdx++
		}
	}

	whereClause := "WHERE " + joinConditions(conditions)
	queryArgs = append(queryArgs, limit)

	rows, err := svc.activeDB().Query(c.Request.Context(), fmt.Sprintf(`
		SELECT id, name, volume_id, size_gb, status, created_at
		FROM snapshots
		%s
		ORDER BY created_at DESC
		LIMIT $%d
	`, whereClause, argIdx), queryArgs...)

	if err != nil {
		log.Error().Err(err).Str("operation", "list_snapshots_detail").Msg("failed to query snapshots")
		common.SendError(c, common.NewInternalServerError("failed to list snapshots"))
		return
	}
	defer rows.Close()

	var snapshots []gin.H
	for rows.Next() {
		var id, name, volumeID, status string
		var size int
		var createdAt time.Time

		if err := rows.Scan(&id, &name, &volumeID, &size, &status, &createdAt); err != nil {
			log.Warn().Err(err).Msg("failed to scan snapshot row")
			continue
		}

		snapshots = append(snapshots, gin.H{
			"id":         id,
			"name":       name,
			"volume_id":  volumeID,
			"size":       size,
			"status":     status,
			"created_at": createdAt.Format("2006-01-02T15:04:05.000000"),
			"os-extended-snapshot-attributes:progress":   "100%",
			"os-extended-snapshot-attributes:project_id": projectID,
		})
	}
	if err := rows.Err(); err != nil {
		log.Error().Err(err).Str("operation", "list_snapshots_detail").Msg("rows iteration error")
		common.SendError(c, common.NewInternalServerError("failed to list snapshots"))
		return
	}

	if snapshots == nil {
		snapshots = []gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{"snapshots": snapshots})
}

// GetDefaultVolumeType returns the default volume type
func (svc *Service) GetDefaultVolumeType(c *gin.Context) {
	// Return a default volume type for Horizon compatibility
	c.JSON(http.StatusOK, gin.H{
		"volume_type": gin.H{
			"id":          "default",
			"name":        "default",
			"description": "Default volume type",
			"is_public":   true,
		},
	})
}

// ListSnapshots lists all snapshots
func (svc *Service) ListSnapshots(c *gin.Context) {
	// Try to get project_id from URL param first, then from token context
	projectID := c.Param("project_id")
	if projectID == "" {
		projectID = c.GetString("project_id")
	}

	// Parse pagination parameters
	limit := common.DefaultPaginationLimit
	if limitParam := c.Query("limit"); limitParam != "" {
		if parsedLimit, err := strconv.Atoi(limitParam); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}
	limit = common.CapLimit(limit)

	conditions := []string{"project_id = $1"}
	queryArgs := []interface{}{projectID}
	argIdx := 2

	// Marker-based pagination
	if marker := c.Query("marker"); marker != "" {
		var markerCreatedAt time.Time
		err := svc.activeDB().QueryRow(c.Request.Context(),
			"SELECT created_at FROM snapshots WHERE id = $1 AND project_id = $2",
			marker, projectID,
		).Scan(&markerCreatedAt)
		if err == nil {
			conditions = append(conditions, fmt.Sprintf("created_at < $%d", argIdx))
			queryArgs = append(queryArgs, markerCreatedAt)
			argIdx++
		}
	}

	whereClause := "WHERE " + joinConditions(conditions)
	queryArgs = append(queryArgs, limit)

	rows, err := svc.activeDB().Query(c.Request.Context(), fmt.Sprintf(`
		SELECT id, name, volume_id, size_gb, status, created_at
		FROM snapshots
		%s
		ORDER BY created_at DESC
		LIMIT $%d
	`, whereClause, argIdx), queryArgs...)

	if err != nil {
		log.Error().Err(err).Str("operation", "list_snapshots").Msg("failed to query snapshots")
		common.SendError(c, common.NewInternalServerError("failed to list snapshots"))
		return
	}
	defer rows.Close()

	var snapshots []gin.H
	for rows.Next() {
		var id, name, volumeID, status string
		var size int
		var createdAt time.Time

		if err := rows.Scan(&id, &name, &volumeID, &size, &status, &createdAt); err != nil {
			log.Warn().Err(err).Msg("failed to scan snapshot row")
			continue
		}

		snapshots = append(snapshots, gin.H{
			"id":         id,
			"name":       name,
			"volume_id":  volumeID,
			"size":       size,
			"status":     status,
			"created_at": createdAt.Format("2006-01-02T15:04:05.000000"),
			"os-extended-snapshot-attributes:progress":   "100%",
			"os-extended-snapshot-attributes:project_id": projectID,
		})
	}
	if err := rows.Err(); err != nil {
		log.Error().Err(err).Str("operation", "list_snapshots_detail").Msg("rows iteration error")
		common.SendError(c, common.NewInternalServerError("failed to list snapshots"))
		return
	}

	if snapshots == nil {
		snapshots = []gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{"snapshots": snapshots})
}

// GetSnapshot returns a single snapshot
func (svc *Service) GetSnapshot(c *gin.Context) {
	snapshotID := c.Param("id")
	// Try to get project_id from URL param first, then from token context
	projectID := c.Param("project_id")
	if projectID == "" {
		projectID = c.GetString("project_id")
	}

	var id, name, volumeID, status string
	var size int
	var createdAt time.Time

	err := svc.activeDB().QueryRow(c.Request.Context(), `
		SELECT id, name, volume_id, size_gb, status, created_at
		FROM snapshots
		WHERE id = $1 AND project_id = $2
	`, snapshotID, projectID).Scan(&id, &name, &volumeID, &size, &status, &createdAt)

	if errors.Is(err, database.ErrNoRows) {
		common.SendError(c, common.NewNotFoundError("snapshot"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "get_snapshot").Str("snapshot_id", snapshotID).Msg("failed to query snapshot")
		common.SendError(c, common.NewInternalServerError("failed to get snapshot"))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"snapshot": gin.H{
			"id":         id,
			"name":       name,
			"volume_id":  volumeID,
			"size":       size,
			"status":     status,
			"created_at": createdAt.Format("2006-01-02T15:04:05.000000"),
			"os-extended-snapshot-attributes:progress":   "100%",
			"os-extended-snapshot-attributes:project_id": projectID,
		},
	})
}

// DeleteSnapshot deletes a snapshot
func (svc *Service) DeleteSnapshot(c *gin.Context) {
	snapshotID := c.Param("id")
	// Try to get project_id from URL param first, then from token context
	projectID := c.Param("project_id")
	if projectID == "" {
		projectID = c.GetString("project_id")
	}

	// Get volume ID
	var volumeID string
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT volume_id FROM snapshots WHERE id = $1 AND project_id = $2",
		snapshotID, projectID,
	).Scan(&volumeID)

	if errors.Is(err, database.ErrNoRows) {
		common.SendError(c, common.NewNotFoundError("snapshot"))
		return
	}

	// Delete from database first (recoverable if Ceph cleanup fails)
	_, err = svc.activeDB().Exec(c.Request.Context(),
		"DELETE FROM snapshots WHERE id = $1 AND project_id = $2",
		snapshotID, projectID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "delete_snapshot_db").Str("snapshot_id", snapshotID).Msg("failed to delete snapshot from database")
		common.SendError(c, common.NewInternalServerError("failed to delete snapshot"))
		return
	}

	// Clean up Ceph (best-effort — orphaned backend data is preferable to data loss)
	if err := svc.cephClient.DeleteSnapshot(c.Request.Context(), volumeID, snapshotID); err != nil {
		log.Error().Err(err).Str("operation", "delete_snapshot_ceph").Str("snapshot_id", snapshotID).Msg("failed to delete snapshot from Ceph (orphaned)")
	}

	c.Status(http.StatusNoContent)
}

// Volume types

// ListVolumeTypes lists all volume types
func (svc *Service) ListVolumeTypes(c *gin.Context) {
	rows, err := svc.activeDB().Query(c.Request.Context(), `
		SELECT id, name, description, is_public, created_at
		FROM volume_types
		ORDER BY name
	`)

	if err != nil {
		log.Error().Err(err).Str("operation", "list_volume_types").Msg("failed to query volume types")
		common.SendError(c, common.NewInternalServerError("failed to list volume types"))
		return
	}
	defer rows.Close()

	var types []gin.H
	for rows.Next() {
		var id, name, description string
		var isPublic bool
		var createdAt time.Time

		if err := rows.Scan(&id, &name, &description, &isPublic, &createdAt); err != nil {
			log.Warn().Err(err).Msg("failed to scan volume type row")
			continue
		}

		types = append(types, gin.H{
			"id":                              id,
			"name":                            name,
			"description":                     description,
			"is_public":                       isPublic,
			"extra_specs":                     map[string]string{},
			"qos_specs_id":                    nil,
			"os-volume-type-access:is_public": isPublic,
		})
	}
	if err := rows.Err(); err != nil {
		log.Error().Err(err).Str("operation", "list_volume_types").Msg("rows iteration error")
		common.SendError(c, common.NewInternalServerError("failed to list volume types"))
		return
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

	err := svc.activeDB().QueryRow(c.Request.Context(), `
		SELECT id, name, COALESCE(description, ''), is_public
		FROM volume_types
		WHERE id = $1
	`, typeID).Scan(&id, &name, &description, &isPublic)

	if errors.Is(err, database.ErrNoRows) {
		common.SendError(c, common.NewNotFoundError("volume type"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "get_volume_type").Str("type_id", typeID).Msg("failed to query volume type")
		common.SendError(c, common.NewInternalServerError("failed to get volume type"))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"volume_type": gin.H{
			"id":                              id,
			"name":                            name,
			"description":                     description,
			"is_public":                       isPublic,
			"extra_specs":                     map[string]string{},
			"qos_specs_id":                    nil,
			"os-volume-type-access:is_public": isPublic,
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
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Check volume exists and belongs to project
	var currentName, currentDesc string
	var sizeGB int
	var status string
	var bootable bool
	var attachedTo sql.NullString
	var existingVolumeType, existingAZ string
	var existingEncrypted bool
	var createdAt, updatedAt time.Time
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT name, COALESCE(description, ''), size_gb, status, bootable, attached_to_instance_id, COALESCE(volume_type, '__DEFAULT__'), created_at, updated_at, COALESCE(availability_zone, 'nova'), COALESCE(encrypted, false) FROM volumes WHERE id = $1 AND project_id = $2",
		volumeID, projectID,
	).Scan(&currentName, &currentDesc, &sizeGB, &status, &bootable, &attachedTo, &existingVolumeType, &createdAt, &updatedAt, &existingAZ, &existingEncrypted)

	if errors.Is(err, database.ErrNoRows) {
		common.SendError(c, common.NewNotFoundError("volume"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "update_volume_check").Str("volume_id", volumeID).Msg("failed to query volume")
		common.SendError(c, common.NewInternalServerError("failed to update volume"))
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
	_, err = svc.activeDB().Exec(c.Request.Context(),
		"UPDATE volumes SET name = $1, description = $2, updated_at = $3 WHERE id = $4",
		currentName, currentDesc, now, volumeID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "update_volume").Str("volume_id", volumeID).Msg("failed to update volume")
		common.SendError(c, common.NewInternalServerError("failed to update volume"))
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
			"id":                volumeID,
			"name":              currentName,
			"description":       currentDesc,
			"tenant_id":         projectID,
			"size":              sizeGB,
			"status":            status,
			"bootable":          bootable,
			"availability_zone": existingAZ,
			"volume_type":       existingVolumeType,
			"encrypted":         existingEncrypted,
			"attachments":       attachments,
			"metadata":          gin.H{},
			"created_at":        createdAt.Format("2006-01-02T15:04:05.000000"),
			"updated_at":        now.Format("2006-01-02T15:04:05.000000"),
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
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Check snapshot exists and belongs to project
	var currentName, currentDesc string
	var volumeID string
	var sizeGB int
	var status string
	var createdAt time.Time
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT name, COALESCE(description, ''), volume_id, size_gb, status, created_at FROM snapshots WHERE id = $1 AND project_id = $2",
		snapshotID, projectID,
	).Scan(&currentName, &currentDesc, &volumeID, &sizeGB, &status, &createdAt)

	if errors.Is(err, database.ErrNoRows) {
		common.SendError(c, common.NewNotFoundError("snapshot"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "update_snapshot_check").Str("snapshot_id", snapshotID).Msg("failed to query snapshot")
		common.SendError(c, common.NewInternalServerError("failed to update snapshot"))
		return
	}

	// Apply updates
	if req.Snapshot.Name != nil {
		currentName = *req.Snapshot.Name
	}
	if req.Snapshot.Description != nil {
		currentDesc = *req.Snapshot.Description
	}

	_, err = svc.activeDB().Exec(c.Request.Context(),
		"UPDATE snapshots SET name = $1, description = $2 WHERE id = $3",
		currentName, currentDesc, snapshotID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "update_snapshot").Str("snapshot_id", snapshotID).Msg("failed to update snapshot")
		common.SendError(c, common.NewInternalServerError("failed to update snapshot"))
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
			"created_at":  createdAt.Format("2006-01-02T15:04:05.000000"),
			"os-extended-snapshot-attributes:progress":   "100%",
			"os-extended-snapshot-attributes:project_id": projectID,
		},
	})
}

// GetVolumeMetadata returns all metadata for a volume
func (svc *Service) GetVolumeMetadata(c *gin.Context) {
	volumeID := c.Param("id")
	projectID := c.GetString("project_id")

	// Check volume exists and belongs to project
	var exists bool
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM volumes WHERE id = $1 AND project_id = $2)",
		volumeID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		common.SendError(c, common.NewNotFoundError("volume"))
		return
	}

	rows, err := svc.activeDB().Query(c.Request.Context(),
		"SELECT meta_key, meta_value FROM volume_metadata WHERE volume_id = $1",
		volumeID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "get_volume_metadata").Str("volume_id", volumeID).Msg("failed to query volume metadata")
		common.SendError(c, common.NewInternalServerError("failed to get volume metadata"))
		return
	}
	defer rows.Close()

	metadata := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			log.Warn().Err(err).Msg("failed to scan metadata row")
			continue
		}
		metadata[key] = value
	}
	if err := rows.Err(); err != nil {
		log.Error().Err(err).Str("operation", "get_metadata").Msg("rows iteration error")
		common.SendError(c, common.NewInternalServerError("failed to get metadata"))
		return
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
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Check volume exists and belongs to project
	var exists bool
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM volumes WHERE id = $1 AND project_id = $2)",
		volumeID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		common.SendError(c, common.NewNotFoundError("volume"))
		return
	}

	// Delete existing metadata then insert new metadata atomically
	if err = database.WithTx(c.Request.Context(), func(tx database.Tx) error {
		if _, err := tx.Exec(c.Request.Context(),
			"DELETE FROM volume_metadata WHERE volume_id = $1",
			volumeID,
		); err != nil {
			return fmt.Errorf("delete_volume_metadata: %w", err)
		}
		for key, value := range req.Metadata {
			if _, err := tx.Exec(c.Request.Context(), `
				INSERT INTO volume_metadata (volume_id, meta_key, meta_value)
				VALUES ($1, $2, $3)
			`, volumeID, key, value); err != nil {
				return fmt.Errorf("insert_volume_metadata: %w", err)
			}
		}
		return nil
	}); err != nil {
		log.Error().Err(err).Str("operation", "set_volume_metadata").Str("volume_id", volumeID).Msg("failed to set volume metadata")
		common.SendError(c, common.NewInternalServerError("failed to set volume metadata"))
		return
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
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM volumes WHERE id = $1 AND project_id = $2)",
		volumeID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		common.SendError(c, common.NewNotFoundError("volume"))
		return
	}

	var value string
	err = svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT meta_value FROM volume_metadata WHERE volume_id = $1 AND meta_key = $2",
		volumeID, key,
	).Scan(&value)

	if errors.Is(err, database.ErrNoRows) {
		common.SendError(c, common.NewNotFoundError("metadata key"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "get_volume_metadata_key").Str("volume_id", volumeID).Msg("failed to query volume metadata key")
		common.SendError(c, common.NewInternalServerError("failed to get metadata key"))
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
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	value, ok := req.Meta[key]
	if !ok {
		common.SendError(c, common.NewBadRequestError("key not found in request body"))
		return
	}

	// Check volume exists and belongs to project
	var exists bool
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM volumes WHERE id = $1 AND project_id = $2)",
		volumeID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		common.SendError(c, common.NewNotFoundError("volume"))
		return
	}

	// Upsert metadata
	_, err = svc.activeDB().Exec(c.Request.Context(), `
		INSERT INTO volume_metadata (volume_id, meta_key, meta_value, updated_at)
		VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
		ON CONFLICT (volume_id, meta_key)
		DO UPDATE SET meta_value = $3, updated_at = CURRENT_TIMESTAMP
	`, volumeID, key, value)

	if err != nil {
		log.Error().Err(err).Str("operation", "update_volume_metadata_key").Str("volume_id", volumeID).Msg("failed to update volume metadata key")
		common.SendError(c, common.NewInternalServerError("failed to update metadata key"))
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
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM volumes WHERE id = $1 AND project_id = $2)",
		volumeID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		common.SendError(c, common.NewNotFoundError("volume"))
		return
	}

	_, err = svc.activeDB().Exec(c.Request.Context(),
		"DELETE FROM volume_metadata WHERE volume_id = $1 AND meta_key = $2",
		volumeID, key,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "delete_volume_metadata_key").Str("volume_id", volumeID).Msg("failed to delete volume metadata key")
		common.SendError(c, common.NewInternalServerError("failed to delete metadata key"))
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
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM snapshots WHERE id = $1 AND project_id = $2)",
		snapshotID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		common.SendError(c, common.NewNotFoundError("snapshot"))
		return
	}

	rows, err := svc.activeDB().Query(c.Request.Context(),
		"SELECT meta_key, meta_value FROM snapshot_metadata WHERE snapshot_id = $1",
		snapshotID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "get_snapshot_metadata").Str("snapshot_id", snapshotID).Msg("failed to query snapshot metadata")
		common.SendError(c, common.NewInternalServerError("failed to get snapshot metadata"))
		return
	}
	defer rows.Close()

	metadata := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			log.Warn().Err(err).Msg("failed to scan metadata row")
			continue
		}
		metadata[key] = value
	}
	if err := rows.Err(); err != nil {
		log.Error().Err(err).Str("operation", "get_metadata").Msg("rows iteration error")
		common.SendError(c, common.NewInternalServerError("failed to get metadata"))
		return
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
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Check snapshot exists and belongs to project
	var exists bool
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM snapshots WHERE id = $1 AND project_id = $2)",
		snapshotID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		common.SendError(c, common.NewNotFoundError("snapshot"))
		return
	}

	// Delete existing metadata then insert new metadata atomically
	if err = database.WithTx(c.Request.Context(), func(tx database.Tx) error {
		if _, err := tx.Exec(c.Request.Context(),
			"DELETE FROM snapshot_metadata WHERE snapshot_id = $1",
			snapshotID,
		); err != nil {
			return fmt.Errorf("delete_snapshot_metadata: %w", err)
		}
		for key, value := range req.Metadata {
			if _, err := tx.Exec(c.Request.Context(), `
				INSERT INTO snapshot_metadata (snapshot_id, meta_key, meta_value)
				VALUES ($1, $2, $3)
			`, snapshotID, key, value); err != nil {
				return fmt.Errorf("insert_snapshot_metadata: %w", err)
			}
		}
		return nil
	}); err != nil {
		log.Error().Err(err).Str("operation", "set_snapshot_metadata").Str("snapshot_id", snapshotID).Msg("failed to set snapshot metadata")
		common.SendError(c, common.NewInternalServerError("failed to set snapshot metadata"))
		return
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
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM snapshots WHERE id = $1 AND project_id = $2)",
		snapshotID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		common.SendError(c, common.NewNotFoundError("snapshot"))
		return
	}

	var value string
	err = svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT meta_value FROM snapshot_metadata WHERE snapshot_id = $1 AND meta_key = $2",
		snapshotID, key,
	).Scan(&value)

	if errors.Is(err, database.ErrNoRows) {
		common.SendError(c, common.NewNotFoundError("metadata key"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "get_snapshot_metadata_key").Str("snapshot_id", snapshotID).Msg("failed to query snapshot metadata key")
		common.SendError(c, common.NewInternalServerError("failed to get metadata key"))
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
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	value, ok := req.Meta[key]
	if !ok {
		common.SendError(c, common.NewBadRequestError("key not found in request body"))
		return
	}

	// Check snapshot exists and belongs to project
	var exists bool
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM snapshots WHERE id = $1 AND project_id = $2)",
		snapshotID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		common.SendError(c, common.NewNotFoundError("snapshot"))
		return
	}

	// Upsert metadata
	_, err = svc.activeDB().Exec(c.Request.Context(), `
		INSERT INTO snapshot_metadata (snapshot_id, meta_key, meta_value, updated_at)
		VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
		ON CONFLICT (snapshot_id, meta_key)
		DO UPDATE SET meta_value = $3, updated_at = CURRENT_TIMESTAMP
	`, snapshotID, key, value)

	if err != nil {
		log.Error().Err(err).Str("operation", "update_snapshot_metadata_key").Str("snapshot_id", snapshotID).Msg("failed to update snapshot metadata key")
		common.SendError(c, common.NewInternalServerError("failed to update metadata key"))
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
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM snapshots WHERE id = $1 AND project_id = $2)",
		snapshotID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		common.SendError(c, common.NewNotFoundError("snapshot"))
		return
	}

	_, err = svc.activeDB().Exec(c.Request.Context(),
		"DELETE FROM snapshot_metadata WHERE snapshot_id = $1 AND meta_key = $2",
		snapshotID, key,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "delete_snapshot_metadata_key").Str("snapshot_id", snapshotID).Msg("failed to delete snapshot metadata key")
		common.SendError(c, common.NewInternalServerError("failed to delete metadata key"))
		return
	}

	c.Status(http.StatusNoContent)
}

// GetLimits returns volume service limits and quotas
func (svc *Service) GetLimits(c *gin.Context) {
	projectID := c.Param("project_id")

	// Query current usage from database
	var volumesUsed, snapshotsUsed, gigabytesUsed int

	_ = svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT COUNT(*), COALESCE(SUM(size_gb), 0) FROM volumes WHERE project_id = $1 AND status != 'deleted'",
		projectID,
	).Scan(&volumesUsed, &gigabytesUsed)

	_ = svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT COUNT(*) FROM snapshots WHERE project_id = $1 AND status != 'deleted'",
		projectID,
	).Scan(&snapshotsUsed)

	// Return limits response
	c.JSON(200, gin.H{
		"limits": gin.H{
			"rate": []gin.H{}, // No rate limiting
			"absolute": gin.H{
				"maxTotalVolumes":          1000,
				"maxTotalSnapshots":        1000,
				"maxTotalVolumeGigabytes":  10000,
				"maxTotalBackups":          100,
				"maxTotalBackupGigabytes":  5000,
				"totalVolumesUsed":         volumesUsed,
				"totalSnapshotsUsed":       snapshotsUsed,
				"totalGigabytesUsed":       gigabytesUsed,
				"totalBackupsUsed":         0,
				"totalBackupGigabytesUsed": 0,
			},
		},
	})
}

// GetLimitsNoProject returns volume service limits without project_id in URL (extracts from token)
func (svc *Service) GetLimitsNoProject(c *gin.Context) {
	// Extract project_id from JWT token (set by auth middleware)
	projectID := c.GetString("project_id")

	// Query current usage from database
	var volumesUsed, snapshotsUsed, gigabytesUsed int

	_ = svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT COUNT(*), COALESCE(SUM(size_gb), 0) FROM volumes WHERE project_id = $1 AND status != 'deleted'",
		projectID,
	).Scan(&volumesUsed, &gigabytesUsed)

	_ = svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT COUNT(*) FROM snapshots WHERE project_id = $1 AND status != 'deleted'",
		projectID,
	).Scan(&snapshotsUsed)

	// Return limits response
	c.JSON(200, gin.H{
		"limits": gin.H{
			"rate": []gin.H{}, // No rate limiting
			"absolute": gin.H{
				"maxTotalVolumes":          1000,
				"maxTotalSnapshots":        1000,
				"maxTotalVolumeGigabytes":  10000,
				"maxTotalBackups":          100,
				"maxTotalBackupGigabytes":  5000,
				"totalVolumesUsed":         volumesUsed,
				"totalSnapshotsUsed":       snapshotsUsed,
				"totalGigabytesUsed":       gigabytesUsed,
				"totalBackupsUsed":         0,
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
				"binary":          "cinder-volume",
				"host":            "o3k-volume-1",
				"zone":            "nova",
				"status":          "enabled",
				"state":           "up",
				"updated_at":      now,
				"disabled_reason": nil,
			},
			{
				"binary":          "cinder-scheduler",
				"host":            "o3k-controller",
				"zone":            "internal",
				"status":          "enabled",
				"state":           "up",
				"updated_at":      now,
				"disabled_reason": nil,
			},
			{
				"binary":          "cinder-backup",
				"host":            "o3k-backup-1",
				"zone":            "nova",
				"status":          "enabled",
				"state":           "up",
				"updated_at":      now,
				"disabled_reason": nil,
			},
		},
	})
}

// GetVersions returns the root version discovery response
func (svc *Service) GetVersions(c *gin.Context) {
	scheme := "http"
	if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	baseURL := fmt.Sprintf("%s://%s/v3/", scheme, c.Request.Host)
	c.JSON(http.StatusOK, gin.H{
		"versions": []gin.H{
			{
				"id":          "v3.0",
				"status":      "CURRENT",
				"version":     "3.70",
				"min_version": "3.0",
				"updated":     "2021-04-07T00:00:00Z",
				"links": []gin.H{
					{
						"rel":  "self",
						"href": baseURL,
					},
				},
				"media-types": []gin.H{
					{
						"base": "application/json",
						"type": "application/vnd.openstack.volume+json;version=3",
					},
				},
			},
		},
	})
}

// GetVersionV3 returns the v3 version information
func (svc *Service) GetVersionV3(c *gin.Context) {
	scheme := "http"
	if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	baseURL := fmt.Sprintf("%s://%s/v3/", scheme, c.Request.Host)
	c.JSON(http.StatusOK, gin.H{
		"version": gin.H{
			"id":          "v3.0",
			"status":      "CURRENT",
			"version":     "3.71",
			"min_version": "3.0",
			"updated":     "2021-04-07T00:00:00Z",
			"links": []gin.H{
				{
					"rel":  "self",
					"href": baseURL,
				},
			},
			"media-types": []gin.H{
				{
					"base": "application/json",
					"type": "application/vnd.openstack.volume+json;version=3",
				},
			},
		},
	})
}
