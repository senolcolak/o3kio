package nova

import (
	"net/http"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/common"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// ListTenantUsage handles GET /v2.1/os-simple-tenant-usage
func (svc *Service) ListTenantUsage(c *gin.Context) {
	// Query all projects with their instance counts and uptime
	rows, err := svc.activeDB().Query(c.Request.Context(),
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
	if err != nil {
		log.Error().Err(err).Str("operation", "list_tenant_usage").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to list tenant usage"))
		return
	}
	defer rows.Close()

	var tenantUsages []gin.H
	for rows.Next() {
		var projectID string
		var totalInstances int
		var totalHours, totalVCPUs, totalMemoryMB, totalLocalGB float64

		if err := rows.Scan(&projectID, &totalInstances, &totalHours, &totalVCPUs, &totalMemoryMB, &totalLocalGB); err != nil {
			continue
		}

		tenantUsages = append(tenantUsages, gin.H{
			"tenant_id":               projectID,
			"total_local_gb_usage":    totalLocalGB,
			"total_vcpus_usage":       totalVCPUs,
			"total_memory_mb_usage":   totalMemoryMB,
			"total_hours":             totalHours,
			"start":                   time.Now().Add(-30 * 24 * time.Hour).Format("2006-01-02T15:04:05.000000"),
			"stop":                    time.Now().Format("2006-01-02T15:04:05.000000"),
			"server_usages":           []gin.H{},
		})
	}

	if tenantUsages == nil {
		tenantUsages = []gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{"tenant_usages": tenantUsages})
}

// GetTenantUsage handles GET /v2.1/os-simple-tenant-usage/:id
func (svc *Service) GetTenantUsage(c *gin.Context) {
	tenantID := c.Param("id")

	// Query specific tenant usage
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
		// Return zero usage if no instances found
		totalInstances = 0
		totalHours = 0
		totalVCPUs = 0
		totalMemoryMB = 0
		totalLocalGB = 0
	}

	c.JSON(http.StatusOK, gin.H{
		"tenant_usage": gin.H{
			"tenant_id":               tenantID,
			"total_local_gb_usage":    totalLocalGB,
			"total_vcpus_usage":       totalVCPUs,
			"total_memory_mb_usage":   totalMemoryMB,
			"total_hours":             totalHours,
			"start":                   time.Now().Add(-30 * 24 * time.Hour).Format("2006-01-02T15:04:05.000000"),
			"stop":                    time.Now().Format("2006-01-02T15:04:05.000000"),
			"server_usages":           []gin.H{},
		},
	})
}
