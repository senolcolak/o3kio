package nova

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

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
	rows, err := database.DB.Query(c.Request.Context(), `
		SELECT resource, hard_limit
		FROM quotas
		WHERE project_id = $1
	`, projectID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	quotaSet := gin.H{
		"id": projectID,
	}

	// Default values if not in database
	defaults := map[string]int{
		"instances":             10,
		"cores":                 20,
		"ram":                   51200,
		"volumes":               10,
		"gigabytes":             1000,
		"snapshots":             10,
		"networks":              10,
		"subnets":               10,
		"ports":                 50,
		"routers":               10,
		"floatingip":            10,
		"security_groups":       10,
		"security_group_rules":  100,
	}

	// Load limits from database
	for rows.Next() {
		var resource string
		var limit int
		if err := rows.Scan(&resource, &limit); err != nil {
			continue
		}
		quotaSet[resource] = limit
		delete(defaults, resource) // Remove from defaults if found in DB
	}

	// Add any missing defaults
	for resource, limit := range defaults {
		if _, exists := quotaSet[resource]; !exists {
			quotaSet[resource] = limit
		}
	}

	// Calculate usage
	var instanceCount, coreCount, ramCount int
	database.DB.QueryRow(c.Request.Context(), `
		SELECT COUNT(*), COALESCE(SUM(f.vcpus), 0), COALESCE(SUM(f.ram_mb), 0)
		FROM instances i
		LEFT JOIN flavors f ON i.flavor_id = f.id
		WHERE i.project_id = $1
	`, projectID).Scan(&instanceCount, &coreCount, &ramCount)

	// Add usage information (OpenStack includes these as separate keys)
	quotaSet["instances_used"] = instanceCount
	quotaSet["cores_used"] = coreCount
	quotaSet["ram_used"] = ramCount

	// Volume usage
	var volumeCount, gigabyteCount int
	database.DB.QueryRow(c.Request.Context(), `
		SELECT COUNT(*), COALESCE(SUM(size_gb), 0)
		FROM volumes
		WHERE project_id = $1
	`, projectID).Scan(&volumeCount, &gigabyteCount)

	quotaSet["volumes_used"] = volumeCount
	quotaSet["gigabytes_used"] = gigabyteCount

	// Network resource usage
	var networkCount, subnetCount, portCount, routerCount, floatingipCount, sgCount int
	database.DB.QueryRow(c.Request.Context(), `SELECT COUNT(*) FROM networks WHERE project_id = $1`, projectID).Scan(&networkCount)
	database.DB.QueryRow(c.Request.Context(), `SELECT COUNT(*) FROM subnets WHERE project_id = $1`, projectID).Scan(&subnetCount)
	database.DB.QueryRow(c.Request.Context(), `SELECT COUNT(*) FROM ports WHERE project_id = $1`, projectID).Scan(&portCount)
	database.DB.QueryRow(c.Request.Context(), `SELECT COUNT(*) FROM routers WHERE project_id = $1`, projectID).Scan(&routerCount)
	database.DB.QueryRow(c.Request.Context(), `SELECT COUNT(*) FROM floating_ips WHERE project_id = $1`, projectID).Scan(&floatingipCount)
	database.DB.QueryRow(c.Request.Context(), `SELECT COUNT(*) FROM security_groups WHERE project_id = $1`, projectID).Scan(&sgCount)

	quotaSet["networks_used"] = networkCount
	quotaSet["subnets_used"] = subnetCount
	quotaSet["ports_used"] = portCount
	quotaSet["routers_used"] = routerCount
	quotaSet["floatingip_used"] = floatingipCount
	quotaSet["security_groups_used"] = sgCount

	c.JSON(http.StatusOK, gin.H{"quota_set": quotaSet})
}

// UpdateQuotaSet updates quota limits for a project (admin only)
func (svc *Service) UpdateQuotaSet(c *gin.Context) {
	projectID := c.Param("id")

	var req UpdateQuotaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// TODO: Check if user is admin
	// For now, allow any user to update quotas

	now := time.Now()

	// Update each quota in the request
	for resource, limit := range req.QuotaSet {
		// Skip special fields
		if resource == "id" {
			continue
		}

		_, err := database.DB.Exec(c.Request.Context(), `
			INSERT INTO quotas (project_id, resource, hard_limit, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (project_id, resource)
			DO UPDATE SET hard_limit = $3, updated_at = $5
		`, projectID, resource, limit, now, now)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	// Return updated quota set
	svc.GetQuotaSet(c)
}

// GetQuotaSetDefaults returns default quota limits
func (svc *Service) GetQuotaSetDefaults(c *gin.Context) {
	projectID := c.Param("id")

	quotaSet := gin.H{
		"id":                    projectID,
		"instances":             10,
		"cores":                 20,
		"ram":                   51200,
		"volumes":               10,
		"gigabytes":             1000,
		"snapshots":             10,
		"networks":              10,
		"subnets":               10,
		"ports":                 50,
		"routers":               10,
		"floatingip":            10,
		"security_groups":       10,
		"security_group_rules":  100,
	}

	c.JSON(http.StatusOK, gin.H{"quota_set": quotaSet})
}

// CheckQuota checks if creating a resource would exceed quota
func CheckQuota(c *gin.Context, resource string, requestedAmount int) error {
	projectID := c.GetString("project_id")

	// Get quota limit
	var limit int
	err := database.DB.QueryRow(c.Request.Context(), `
		SELECT hard_limit FROM quotas WHERE project_id = $1 AND resource = $2
	`, projectID, resource).Scan(&limit)

	if err == pgx.ErrNoRows {
		// No quota set, use defaults
		defaults := map[string]int{
			"instances":             10,
			"cores":                 20,
			"ram":                   51200,
			"volumes":               10,
			"gigabytes":             1000,
			"snapshots":             10,
			"networks":              10,
			"subnets":               10,
			"ports":                 50,
			"routers":               10,
			"floatingip":            10,
			"security_groups":       10,
			"security_group_rules":  100,
		}
		limit = defaults[resource]
	} else if err != nil {
		return err
	}

	// Get current usage
	var usage int
	switch resource {
	case "instances":
		database.DB.QueryRow(c.Request.Context(), `SELECT COUNT(*) FROM instances WHERE project_id = $1`, projectID).Scan(&usage)
	case "cores":
		database.DB.QueryRow(c.Request.Context(), `SELECT COALESCE(SUM(f.vcpus), 0) FROM instances i LEFT JOIN flavors f ON i.flavor_id = f.id WHERE i.project_id = $1`, projectID).Scan(&usage)
	case "ram":
		database.DB.QueryRow(c.Request.Context(), `SELECT COALESCE(SUM(f.ram_mb), 0) FROM instances i LEFT JOIN flavors f ON i.flavor_id = f.id WHERE i.project_id = $1`, projectID).Scan(&usage)
	case "volumes":
		database.DB.QueryRow(c.Request.Context(), `SELECT COUNT(*) FROM volumes WHERE project_id = $1`, projectID).Scan(&usage)
	case "gigabytes":
		database.DB.QueryRow(c.Request.Context(), `SELECT COALESCE(SUM(size_gb), 0) FROM volumes WHERE project_id = $1`, projectID).Scan(&usage)
	case "networks":
		database.DB.QueryRow(c.Request.Context(), `SELECT COUNT(*) FROM networks WHERE project_id = $1`, projectID).Scan(&usage)
	case "subnets":
		database.DB.QueryRow(c.Request.Context(), `SELECT COUNT(*) FROM subnets WHERE project_id = $1`, projectID).Scan(&usage)
	case "ports":
		database.DB.QueryRow(c.Request.Context(), `SELECT COUNT(*) FROM ports WHERE project_id = $1`, projectID).Scan(&usage)
	case "routers":
		database.DB.QueryRow(c.Request.Context(), `SELECT COUNT(*) FROM routers WHERE project_id = $1`, projectID).Scan(&usage)
	case "floatingip":
		database.DB.QueryRow(c.Request.Context(), `SELECT COUNT(*) FROM floating_ips WHERE project_id = $1`, projectID).Scan(&usage)
	case "security_groups":
		database.DB.QueryRow(c.Request.Context(), `SELECT COUNT(*) FROM security_groups WHERE project_id = $1`, projectID).Scan(&usage)
	case "security_group_rules":
		database.DB.QueryRow(c.Request.Context(), `SELECT COUNT(*) FROM security_group_rules WHERE security_group_id IN (SELECT id FROM security_groups WHERE project_id = $1)`, projectID).Scan(&usage)
	}

	// Check if adding requested amount would exceed limit
	if usage+requestedAmount > limit {
		return &QuotaExceededError{
			Resource: resource,
			Limit:    limit,
			Usage:    usage,
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
