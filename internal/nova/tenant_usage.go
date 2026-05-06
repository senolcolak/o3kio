package nova

import (
	"net/http"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/common"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

// isAdminContext returns true if the request context carries the "admin" role.
func isAdminContext(c *gin.Context) bool {
	roles, _ := c.Get("roles")
	if roleList, ok := roles.([]string); ok {
		for _, r := range roleList {
			if r == "admin" {
				return true
			}
		}
	}
	return false
}

// ListTenantUsage handles GET /v2.1/os-simple-tenant-usage
// Admins see all projects; non-admins see only their own project.
func (svc *Service) ListTenantUsage(c *gin.Context) {
	ctx := c.Request.Context()

	var (
		rows pgx.Rows
		err  error
	)

	if isAdminContext(c) {
		rows, err = svc.activeDB().Query(ctx,
			`SELECT
				i.project_id,
				COUNT(*) as total_instances,
				COALESCE(SUM(EXTRACT(EPOCH FROM (NOW() - i.created_at)) / 3600), 0) as total_hours,
				COALESCE(SUM(f.vcpus), 0) as total_vcpus_usage,
				COALESCE(SUM(f.ram_mb), 0) as total_memory_mb_usage,
				COALESCE(SUM(f.disk_gb), 0) as total_local_gb_usage
			 FROM instances i
			 LEFT JOIN flavors f ON i.flavor_id = f.id
			 GROUP BY i.project_id`,
		)
	} else {
		projectID := c.GetString("project_id")
		rows, err = svc.activeDB().Query(ctx,
			`SELECT
				i.project_id,
				COUNT(*) as total_instances,
				COALESCE(SUM(EXTRACT(EPOCH FROM (NOW() - i.created_at)) / 3600), 0) as total_hours,
				COALESCE(SUM(f.vcpus), 0) as total_vcpus_usage,
				COALESCE(SUM(f.ram_mb), 0) as total_memory_mb_usage,
				COALESCE(SUM(f.disk_gb), 0) as total_local_gb_usage
			 FROM instances i
			 LEFT JOIN flavors f ON i.flavor_id = f.id
			 WHERE i.project_id = $1
			 GROUP BY i.project_id`,
			projectID,
		)
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "list_tenant_usage").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to list tenant usage"))
		return
	}
	defer rows.Close()

	var tenantUsages []gin.H
	for rows.Next() {
		var pid string
		var totalInstances int
		var totalHours, totalVCPUs, totalMemoryMB, totalLocalGB float64

		if err := rows.Scan(&pid, &totalInstances, &totalHours, &totalVCPUs, &totalMemoryMB, &totalLocalGB); err != nil {
			continue
		}

		tenantUsages = append(tenantUsages, gin.H{
			"tenant_id":             pid,
			"total_local_gb_usage":  totalLocalGB,
			"total_vcpus_usage":     totalVCPUs,
			"total_memory_mb_usage": totalMemoryMB,
			"total_hours":           totalHours,
			"start":                 time.Now().Add(-30 * 24 * time.Hour).Format("2006-01-02T15:04:05.000000"),
			"stop":                  time.Now().Format("2006-01-02T15:04:05.000000"),
			"server_usages":         []gin.H{},
		})
	}

	if tenantUsages == nil {
		tenantUsages = []gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{"tenant_usages": tenantUsages})
}

// GetTenantUsage handles GET /v2.1/os-simple-tenant-usage/:id
// Admins may query any tenant; non-admins may only query their own project.
func (svc *Service) GetTenantUsage(c *gin.Context) {
	tenantID := c.Param("id")

	if !isAdminContext(c) {
		if projectID := c.GetString("project_id"); tenantID != projectID {
			common.SendError(c, common.NewForbiddenError("access denied to other tenant's usage"))
			return
		}
	}

	var totalInstances int
	var totalHours, totalVCPUs, totalMemoryMB, totalLocalGB float64

	err := svc.activeDB().QueryRow(c.Request.Context(),
		`SELECT
			COUNT(*) as total_instances,
			COALESCE(SUM(EXTRACT(EPOCH FROM (NOW() - i.created_at)) / 3600), 0) as total_hours,
			COALESCE(SUM(f.vcpus), 0) as total_vcpus_usage,
			COALESCE(SUM(f.ram_mb), 0) as total_memory_mb_usage,
			COALESCE(SUM(f.disk_gb), 0) as total_local_gb_usage
		 FROM instances i
		 LEFT JOIN flavors f ON i.flavor_id = f.id
		 WHERE i.project_id = $1
		 GROUP BY i.project_id`,
		tenantID,
	).Scan(&totalInstances, &totalHours, &totalVCPUs, &totalMemoryMB, &totalLocalGB)

	if err != nil {
		// No instances found — return zeros rather than an error.
		totalInstances = 0
		totalHours = 0
		totalVCPUs = 0
		totalMemoryMB = 0
		totalLocalGB = 0
	}

	c.JSON(http.StatusOK, gin.H{
		"tenant_usage": gin.H{
			"tenant_id":             tenantID,
			"total_local_gb_usage":  totalLocalGB,
			"total_vcpus_usage":     totalVCPUs,
			"total_memory_mb_usage": totalMemoryMB,
			"total_hours":           totalHours,
			"start":                 time.Now().Add(-30 * 24 * time.Hour).Format("2006-01-02T15:04:05.000000"),
			"stop":                  time.Now().Format("2006-01-02T15:04:05.000000"),
			"server_usages":         []gin.H{},
		},
	})
}
