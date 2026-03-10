package nova

import (
	"net/http"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/gin-gonic/gin"
)

// GetServerDiagnostics handles GET /v2.1/servers/:id/diagnostics
func (svc *Service) GetServerDiagnostics(c *gin.Context) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	// Query instance to verify it exists and belongs to project
	var status, flavorID string
	var vcpus, memoryMB, diskGB int
	var createdAt time.Time

	err := database.DB.QueryRow(c.Request.Context(),
		`SELECT i.status, i.created_at, f.vcpus, f.ram_mb, f.disk_gb, i.flavor_id
		 FROM instances i
		 LEFT JOIN flavors f ON i.flavor_id = f.id
		 WHERE i.id = $1 AND i.project_id = $2`,
		instanceID, projectID,
	).Scan(&status, &createdAt, &vcpus, &memoryMB, &diskGB, &flavorID)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"itemNotFound": gin.H{
				"message": "Instance not found",
				"code":    404,
			},
		})
		return
	}

	// Calculate uptime
	uptime := time.Since(createdAt).Seconds()

	// Return diagnostic information
	// In stub mode, return mock values; in real mode, these would come from libvirt
	c.JSON(http.StatusOK, gin.H{
		"state":           status,
		"driver":          "libvirt",
		"hypervisor":      "kvm",
		"uptime":          int(uptime),
		"num_cpus":        vcpus,
		"num_nics":        1,
		"num_disks":       1,
		"memory":          memoryMB,
		"memory-actual":   memoryMB,
		"memory-rss":      memoryMB / 2,
		"cpu0_time":       int(uptime * 1000000000), // nanoseconds
		"vda_read":        1024 * 1024,              // bytes
		"vda_write":       512 * 1024,               // bytes
		"vda_read_req":    100,
		"vda_write_req":   50,
		"vnet0_rx":        2048 * 1024, // bytes
		"vnet0_tx":        1024 * 1024,
		"vnet0_rx_packets": 1000,
		"vnet0_tx_packets": 500,
	})
}
