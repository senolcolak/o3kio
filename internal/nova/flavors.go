package nova

import (
	"net/http"

	"github.com/cobaltcore-dev/o3k/internal/common"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rs/zerolog/log"
)

// CreateFlavor handles POST /v2.1/flavors
func (svc *Service) CreateFlavor(c *gin.Context) {
	var req struct {
		Flavor struct {
			Name      string `json:"name" binding:"required"`
			RAM       int    `json:"ram" binding:"required"`
			VCPUs     int    `json:"vcpus" binding:"required"`
			Disk      int    `json:"disk" binding:"required"`
			Ephemeral int    `json:"OS-FLV-EXT-DATA:ephemeral"`
			IsPublic  *bool  `json:"os-flavor-access:is_public"`
		} `json:"flavor" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Validate input ranges
	if req.Flavor.RAM < 0 {
		common.SendError(c, common.NewBadRequestError("RAM must be non-negative"))
		return
	}
	if req.Flavor.VCPUs < 0 {
		common.SendError(c, common.NewBadRequestError("VCPUs must be non-negative"))
		return
	}
	if req.Flavor.Disk < 0 {
		common.SendError(c, common.NewBadRequestError("Disk must be non-negative"))
		return
	}

	flavorID := uuid.New().String()
	ctx := c.Request.Context()

	// Default to public flavor if not specified
	isPublic := true
	if req.Flavor.IsPublic != nil {
		isPublic = *req.Flavor.IsPublic
	}

	_, err := svc.activeDB().Exec(ctx,
		`INSERT INTO flavors (id, name, vcpus, ram_mb, disk_gb, is_public)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		flavorID, req.Flavor.Name, req.Flavor.VCPUs, req.Flavor.RAM, req.Flavor.Disk, isPublic,
	)
	if err != nil {
		// Check for unique constraint violation (duplicate name)
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23505" {
			common.SendError(c, common.NewConflictError("Flavor with name '"+req.Flavor.Name+"' already exists"))
			return
		}
		log.Error().Err(err).Str("operation", "create_flavor").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to create flavor"))
		return
	}

	// Invalidate flavors list cache
	if svc.cache != nil {
		svc.cache.DeletePattern(ctx, "flavors:*")
	}

	c.JSON(http.StatusOK, gin.H{
		"flavor": gin.H{
			"id":                         flavorID,
			"name":                       req.Flavor.Name,
			"vcpus":                      req.Flavor.VCPUs,
			"ram":                        req.Flavor.RAM,
			"disk":                       req.Flavor.Disk,
			"OS-FLV-EXT-DATA:ephemeral":  req.Flavor.Ephemeral,
			"OS-FLV-DISABLED:disabled":   false,
			"os-flavor-access:is_public": isPublic,
			"rxtx_factor":                1.0,
			"swap":                       "",
		},
	})
}

// DeleteFlavor handles DELETE /v2.1/flavors/:id
func (svc *Service) DeleteFlavor(c *gin.Context) {
	flavorID := c.Param("id")
	ctx := c.Request.Context()

	result, err := svc.activeDB().Exec(ctx,
		"DELETE FROM flavors WHERE id = $1",
		flavorID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "delete_flavor").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to delete flavor"))
		return
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		common.SendError(c, common.NewNotFoundError("flavor"))
		return
	}

	// Invalidate cache
	if svc.cache != nil {
		svc.cache.Delete(ctx, "flavor:"+flavorID)
		svc.cache.DeletePattern(ctx, "flavors:*")
	}

	c.Status(http.StatusNoContent)
}

// GetFlavorExtraSpecs handles GET /v2.1/flavors/:id/os-extra_specs
func (svc *Service) GetFlavorExtraSpecs(c *gin.Context) {
	flavorID := c.Param("id")

	rows, err := svc.activeDB().Query(c.Request.Context(),
		"SELECT key, value FROM flavor_extra_specs WHERE flavor_id = $1",
		flavorID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "get_flavor_extra_specs").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to get extra specs"))
		return
	}
	defer rows.Close()

	extraSpecs := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			log.Warn().Err(err).Msg("failed to scan flavor extra spec row")
			continue
		}
		extraSpecs[key] = value
	}

	c.JSON(http.StatusOK, gin.H{"extra_specs": extraSpecs})
}

// CreateFlavorExtraSpecs handles POST /v2.1/flavors/:id/os-extra_specs
func (svc *Service) CreateFlavorExtraSpecs(c *gin.Context) {
	flavorID := c.Param("id")

	var req struct {
		ExtraSpecs map[string]string `json:"extra_specs" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Insert or update extra specs
	for key, value := range req.ExtraSpecs {
		_, err := svc.activeDB().Exec(c.Request.Context(),
			`INSERT INTO flavor_extra_specs (flavor_id, key, value)
			 VALUES ($1, $2, $3)
			 ON CONFLICT (flavor_id, key) DO UPDATE SET value = $3`,
			flavorID, key, value,
		)
		if err != nil {
			log.Error().Err(err).Str("operation", "create_flavor_extra_specs").Msg("database error")
			common.SendError(c, common.NewInternalServerError("failed to create extra spec"))
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"extra_specs": req.ExtraSpecs})
}

// GetFlavorExtraSpecKey handles GET /v2.1/flavors/:id/os-extra_specs/:key
func (svc *Service) GetFlavorExtraSpecKey(c *gin.Context) {
	flavorID := c.Param("id")
	key := c.Param("key")

	var value string
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT value FROM flavor_extra_specs WHERE flavor_id = $1 AND key = $2",
		flavorID, key,
	).Scan(&value)

	if err != nil {
		common.SendError(c, common.NewNotFoundError("extra spec key"))
		return
	}

	c.JSON(http.StatusOK, gin.H{key: value})
}

// UpdateFlavorExtraSpecKey handles PUT /v2.1/flavors/:id/os-extra_specs/:key
func (svc *Service) UpdateFlavorExtraSpecKey(c *gin.Context) {
	flavorID := c.Param("id")
	key := c.Param("key")

	var req map[string]string
	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Extract value for the specific key
	value, ok := req[key]
	if !ok {
		common.SendError(c, common.NewBadRequestError("Key in URL must match key in request body"))
		return
	}

	_, err := svc.activeDB().Exec(c.Request.Context(),
		`INSERT INTO flavor_extra_specs (flavor_id, key, value)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (flavor_id, key) DO UPDATE SET value = $3`,
		flavorID, key, value,
	)

	if err != nil {
		log.Error().Err(err).Str("operation", "update_flavor_extra_spec_key").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to update extra spec"))
		return
	}

	c.JSON(http.StatusOK, gin.H{key: value})
}

// DeleteFlavorExtraSpecKey handles DELETE /v2.1/flavors/:id/os-extra_specs/:key
func (svc *Service) DeleteFlavorExtraSpecKey(c *gin.Context) {
	flavorID := c.Param("id")
	key := c.Param("key")

	result, err := svc.activeDB().Exec(c.Request.Context(),
		"DELETE FROM flavor_extra_specs WHERE flavor_id = $1 AND key = $2",
		flavorID, key,
	)

	if err != nil {
		log.Error().Err(err).Str("operation", "delete_flavor_extra_spec_key").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to delete extra spec"))
		return
	}

	if result.RowsAffected() == 0 {
		common.SendError(c, common.NewNotFoundError("extra spec key"))
		return
	}

	c.Status(http.StatusNoContent)
}

// FlavorAction handles POST /v2.1/flavors/:id/action
func (svc *Service) FlavorAction(c *gin.Context) {
	flavorID := c.Param("id")

	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Handle addTenantAccess action
	if addAccess, ok := req["addTenantAccess"].(map[string]interface{}); ok {
		tenantID, ok := addAccess["tenant"].(string)
		if !ok {
			common.SendError(c, common.NewBadRequestError("tenant is required"))
			return
		}

		_, err := svc.activeDB().Exec(c.Request.Context(),
			`INSERT INTO flavor_access (flavor_id, project_id)
			 VALUES ($1, $2)
			 ON CONFLICT (flavor_id, project_id) DO NOTHING`,
			flavorID, tenantID,
		)

		if err != nil {
			log.Error().Err(err).Str("operation", "add_tenant_access").Msg("database error")
			common.SendError(c, common.NewInternalServerError("failed to add tenant access"))
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"flavor_access": gin.H{
				"flavor_id": flavorID,
				"tenant_id": tenantID,
			},
		})
		return
	}

	// Handle removeTenantAccess action
	if removeAccess, ok := req["removeTenantAccess"].(map[string]interface{}); ok {
		tenantID, ok := removeAccess["tenant"].(string)
		if !ok {
			common.SendError(c, common.NewBadRequestError("tenant is required"))
			return
		}

		_, err := svc.activeDB().Exec(c.Request.Context(),
			"DELETE FROM flavor_access WHERE flavor_id = $1 AND project_id = $2",
			flavorID, tenantID,
		)

		if err != nil {
			log.Error().Err(err).Str("operation", "remove_tenant_access").Msg("database error")
			common.SendError(c, common.NewInternalServerError("failed to remove tenant access"))
			return
		}

		c.Status(http.StatusOK)
		return
	}

	common.SendError(c, common.NewBadRequestError("Unknown action"))
}

// GetFlavorAccess handles GET /v2.1/flavors/:id/os-flavor-access
func (svc *Service) GetFlavorAccess(c *gin.Context) {
	flavorID := c.Param("id")

	rows, err := svc.activeDB().Query(c.Request.Context(),
		"SELECT flavor_id, project_id FROM flavor_access WHERE flavor_id = $1",
		flavorID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "get_flavor_access").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to get flavor access"))
		return
	}
	defer rows.Close()

	flavorAccess := []gin.H{}
	for rows.Next() {
		var fID, pID string
		if err := rows.Scan(&fID, &pID); err != nil {
			log.Warn().Err(err).Msg("failed to scan flavor access row")
			continue
		}
		flavorAccess = append(flavorAccess, gin.H{
			"flavor_id": fID,
			"tenant_id": pID,
		})
	}

	c.JSON(http.StatusOK, gin.H{"flavor_access": flavorAccess})
}
