package nova

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

	"github.com/cobaltcore-dev/o3k/internal/common"
	"github.com/cobaltcore-dev/o3k/internal/database"
)

// UpdateQuotaRequest represents a quota update request
type UpdateQuotaRequest struct {
	QuotaSet map[string]int `json:"quota_set"`
}

// GetQuotaSet returns quota limits and usage for a project
func (svc *Service) GetQuotaSet(c *gin.Context) {
	projectID := c.Param("id")
	requestingProjectID := c.GetString("project_id")

	// Allow users to query their own project or admins to query any project
	// For now, allow any query (proper RBAC would be implemented in production)
	if projectID == "default" {
		projectID = requestingProjectID
	}

	// Fetch quota limits
	rows, err := svc.activeDB().Query(c.Request.Context(), `
		SELECT resource, hard_limit
		FROM quotas
		WHERE project_id = $1
	`, projectID)

	if err != nil {
		log.Error().Err(err).Str("operation", "get_quota_set").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to get quota set"))
		return
	}
	defer rows.Close()

	quotaSet := gin.H{
		"id": projectID,
	}

	// Default limits if not in database
	defaults := map[string]int{
		"instances":            10,
		"cores":                20,
		"ram":                  51200,
		"volumes":              10,
		"gigabytes":            1000,
		"snapshots":            10,
		"networks":             10,
		"subnets":              10,
		"ports":                50,
		"routers":              10,
		"floatingip":           10,
		"security_groups":      10,
		"security_group_rules": 100,
	}

	// Load limits from database (overrides defaults)
	for rows.Next() {
		var resource string
		var limit int
		if err := rows.Scan(&resource, &limit); err != nil {
			continue
		}
		defaults[resource] = limit
	}
	if err := rows.Err(); err != nil {
		log.Error().Err(err).Str("operation", "get_quota_set").Msg("rows iteration error")
		common.SendError(c, common.NewInternalServerError("failed to get quota set"))
		return
	}

	// Calculate compute usage
	var instanceCount, coreCount, ramCount int
	svc.activeDB().QueryRow(c.Request.Context(), `
		SELECT COUNT(*), COALESCE(SUM(f.vcpus), 0), COALESCE(SUM(f.ram_mb), 0)
		FROM instances i
		LEFT JOIN flavors f ON i.flavor_id = f.id
		WHERE i.project_id = $1
	`, projectID).Scan(&instanceCount, &coreCount, &ramCount)

	// Volume usage
	var volumeCount, gigabyteCount, snapshotCount int
	svc.activeDB().QueryRow(c.Request.Context(), `
		SELECT COUNT(*), COALESCE(SUM(size_gb), 0)
		FROM volumes
		WHERE project_id = $1
	`, projectID).Scan(&volumeCount, &gigabyteCount)

	// Snapshot usage
	svc.activeDB().QueryRow(c.Request.Context(),
		`SELECT COUNT(*) FROM volume_snapshots WHERE project_id = $1`, projectID,
	).Scan(&snapshotCount)

	// Network resource usage
	var networkCount, subnetCount, portCount, routerCount, floatingipCount, sgCount, sgrCount int
	svc.activeDB().QueryRow(c.Request.Context(), `SELECT COUNT(*) FROM networks WHERE project_id = $1`, projectID).Scan(&networkCount)
	svc.activeDB().QueryRow(c.Request.Context(), `SELECT COUNT(*) FROM subnets WHERE project_id = $1`, projectID).Scan(&subnetCount)
	svc.activeDB().QueryRow(c.Request.Context(), `SELECT COUNT(*) FROM ports WHERE project_id = $1`, projectID).Scan(&portCount)
	svc.activeDB().QueryRow(c.Request.Context(), `SELECT COUNT(*) FROM routers WHERE project_id = $1`, projectID).Scan(&routerCount)
	svc.activeDB().QueryRow(c.Request.Context(), `SELECT COUNT(*) FROM floating_ips WHERE project_id = $1`, projectID).Scan(&floatingipCount)
	svc.activeDB().QueryRow(c.Request.Context(), `SELECT COUNT(*) FROM security_groups WHERE project_id = $1`, projectID).Scan(&sgCount)
	svc.activeDB().QueryRow(c.Request.Context(), `SELECT COUNT(*) FROM security_group_rules WHERE security_group_id IN (SELECT id FROM security_groups WHERE project_id = $1)`, projectID).Scan(&sgrCount)

	usages := map[string]int{
		"instances":            instanceCount,
		"cores":                coreCount,
		"ram":                  ramCount,
		"volumes":              volumeCount,
		"gigabytes":            gigabyteCount,
		"snapshots":            snapshotCount,
		"networks":             networkCount,
		"subnets":              subnetCount,
		"ports":                portCount,
		"routers":              routerCount,
		"floatingip":           floatingipCount,
		"security_groups":      sgCount,
		"security_group_rules": sgrCount,
	}

	// Build nested quota format: each resource is {"in_use": N, "limit": M, "reserved": 0}
	for resource, limit := range defaults {
		quotaSet[resource] = gin.H{
			"in_use":   usages[resource],
			"limit":    limit,
			"reserved": 0,
		}
	}

	c.JSON(http.StatusOK, gin.H{"quota_set": quotaSet})
}

// UpdateQuotaSet updates quota limits for a project (admin only)
func (svc *Service) UpdateQuotaSet(c *gin.Context) {
	projectID := c.Param("id")

	var req UpdateQuotaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Admin-only operation - check roles
	roles := c.GetStringSlice("roles")
	isAdmin := false
	for _, role := range roles {
		if role == "admin" {
			isAdmin = true
			break
		}
	}
	if !isAdmin {
		common.SendError(c, common.NewForbiddenError("Policy doesn't allow quota updates to be performed. Admin role required."))
		return
	}

	now := time.Now()

	// Update each quota in the request
	for resource, limit := range req.QuotaSet {
		// Skip special fields
		if resource == "id" {
			continue
		}

		_, err := svc.activeDB().Exec(c.Request.Context(), `
			INSERT INTO quotas (project_id, resource, hard_limit, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (project_id, resource)
			DO UPDATE SET hard_limit = $3, updated_at = $5
		`, projectID, resource, limit, now, now)

		if err != nil {
			log.Error().Err(err).Str("operation", "update_quota").Msg("database error")
			common.SendError(c, common.NewInternalServerError("failed to update quota"))
			return
		}
	}

	// Return updated quota set
	svc.GetQuotaSet(c)
}

// GetQuotaSetDefaults returns default quota limits
func (svc *Service) GetQuotaSetDefaults(c *gin.Context) {
	projectID := c.Param("id")

	defaults := map[string]int{
		"instances":            10,
		"cores":                20,
		"ram":                  51200,
		"volumes":              10,
		"gigabytes":            1000,
		"snapshots":            10,
		"networks":             10,
		"subnets":              10,
		"ports":                50,
		"routers":              10,
		"floatingip":           10,
		"security_groups":      10,
		"security_group_rules": 100,
	}

	quotaSet := gin.H{"id": projectID}
	for resource, limit := range defaults {
		quotaSet[resource] = gin.H{"in_use": 0, "limit": limit, "reserved": 0}
	}

	c.JSON(http.StatusOK, gin.H{"quota_set": quotaSet})
}

// CheckQuota checks if creating a resource would exceed quota
func (svc *Service) CheckQuota(c *gin.Context, resource string, requestedAmount int) error {
	projectID := c.GetString("project_id")

	// Get quota limit
	var limit int
	err := svc.activeDB().QueryRow(c.Request.Context(), `
		SELECT hard_limit FROM quotas WHERE project_id = $1 AND resource = $2
	`, projectID, resource).Scan(&limit)

	if errors.Is(err, database.ErrNoRows) {
		// No quota set, use defaults
		defaults := map[string]int{
			"instances":            10,
			"cores":                20,
			"ram":                  51200,
			"volumes":              10,
			"gigabytes":            1000,
			"snapshots":            10,
			"networks":             10,
			"subnets":              10,
			"ports":                50,
			"routers":              10,
			"floatingip":           10,
			"security_groups":      10,
			"security_group_rules": 100,
		}
		limit = defaults[resource]
	} else if err != nil {
		return err
	}

	// Get current usage
	var usage int
	switch resource {
	case "instances":
		svc.activeDB().QueryRow(c.Request.Context(), `SELECT COUNT(*) FROM instances WHERE project_id = $1 AND status NOT IN ('DELETED', 'SOFT_DELETED', 'ERROR')`, projectID).Scan(&usage)
	case "cores":
		svc.activeDB().QueryRow(c.Request.Context(), `SELECT COALESCE(SUM(f.vcpus), 0) FROM instances i LEFT JOIN flavors f ON i.flavor_id = f.id WHERE i.project_id = $1 AND i.status NOT IN ('DELETED', 'SOFT_DELETED', 'ERROR')`, projectID).Scan(&usage)
	case "ram":
		svc.activeDB().QueryRow(c.Request.Context(), `SELECT COALESCE(SUM(f.ram_mb), 0) FROM instances i LEFT JOIN flavors f ON i.flavor_id = f.id WHERE i.project_id = $1 AND i.status NOT IN ('DELETED', 'SOFT_DELETED', 'ERROR')`, projectID).Scan(&usage)
	case "volumes":
		svc.activeDB().QueryRow(c.Request.Context(), `SELECT COUNT(*) FROM volumes WHERE project_id = $1`, projectID).Scan(&usage)
	case "gigabytes":
		svc.activeDB().QueryRow(c.Request.Context(), `SELECT COALESCE(SUM(size_gb), 0) FROM volumes WHERE project_id = $1`, projectID).Scan(&usage)
	case "networks":
		svc.activeDB().QueryRow(c.Request.Context(), `SELECT COUNT(*) FROM networks WHERE project_id = $1`, projectID).Scan(&usage)
	case "subnets":
		svc.activeDB().QueryRow(c.Request.Context(), `SELECT COUNT(*) FROM subnets WHERE project_id = $1`, projectID).Scan(&usage)
	case "ports":
		svc.activeDB().QueryRow(c.Request.Context(), `SELECT COUNT(*) FROM ports WHERE project_id = $1`, projectID).Scan(&usage)
	case "routers":
		svc.activeDB().QueryRow(c.Request.Context(), `SELECT COUNT(*) FROM routers WHERE project_id = $1`, projectID).Scan(&usage)
	case "floatingip":
		svc.activeDB().QueryRow(c.Request.Context(), `SELECT COUNT(*) FROM floating_ips WHERE project_id = $1`, projectID).Scan(&usage)
	case "security_groups":
		svc.activeDB().QueryRow(c.Request.Context(), `SELECT COUNT(*) FROM security_groups WHERE project_id = $1`, projectID).Scan(&usage)
	case "security_group_rules":
		svc.activeDB().QueryRow(c.Request.Context(), `SELECT COUNT(*) FROM security_group_rules WHERE security_group_id IN (SELECT id FROM security_groups WHERE project_id = $1)`, projectID).Scan(&usage)
	}

	// Check if adding requested amount would exceed limit
	if usage+requestedAmount > limit {
		return &QuotaExceededError{
			Resource:  resource,
			Limit:     limit,
			Usage:     usage,
			Requested: requestedAmount,
		}
	}

	return nil
}

// QuotaExceededError represents a quota exceeded error
type QuotaExceededError struct {
	Resource  string
	Limit     int
	Usage     int
	Requested int
}

func (e *QuotaExceededError) Error() string {
	return "Quota exceeded"
}
