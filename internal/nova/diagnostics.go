package nova

import (
	"net/http"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/common"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// GetServerDiagnostics handles GET /v2.1/servers/:id/diagnostics
func (svc *Service) GetServerDiagnostics(c *gin.Context) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	// Query instance to verify it exists and belongs to project
	var status, flavorID string
	var vcpus, memoryMB, diskGB int
	var createdAt time.Time

	err := svc.activeDB().QueryRow(c.Request.Context(),
		`SELECT i.status, i.created_at, f.vcpus, f.ram_mb, f.disk_gb, i.flavor_id
		 FROM instances i
		 LEFT JOIN flavors f ON i.flavor_id = f.id
		 WHERE i.id = $1 AND i.project_id = $2`,
		instanceID, projectID,
	).Scan(&status, &createdAt, &vcpus, &memoryMB, &diskGB, &flavorID)

	if err != nil {
		common.SendError(c, common.NewNotFoundError("instance"))
		return
	}

	// Calculate uptime
	uptime := time.Since(createdAt).Seconds()

	// Return diagnostic information
	// In stub mode, return mock values; in real mode, these would come from libvirt
	c.JSON(http.StatusOK, gin.H{
		"state":            status,
		"driver":           "libvirt",
		"hypervisor":       "kvm",
		"uptime":           int(uptime),
		"num_cpus":         vcpus,
		"num_nics":         1,
		"num_disks":        1,
		"memory":           memoryMB,
		"memory-actual":    memoryMB,
		"memory-rss":       memoryMB / 2,
		"cpu0_time":        int(uptime * 1000000000), // nanoseconds
		"vda_read":         1024 * 1024,              // bytes
		"vda_write":        512 * 1024,               // bytes
		"vda_read_req":     100,
		"vda_write_req":    50,
		"vnet0_rx":         2048 * 1024, // bytes
		"vnet0_tx":         1024 * 1024,
		"vnet0_rx_packets": 1000,
		"vnet0_tx_packets": 500,
	})
}

// ListInstanceActions handles GET /v2.1/servers/:id/os-instance-actions
func (svc *Service) ListInstanceActions(c *gin.Context) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	// Verify instance exists and belongs to project
	var exists bool
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM instances WHERE id = $1 AND project_id = $2)",
		instanceID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		common.SendError(c, common.NewNotFoundError("instance"))
		return
	}

	// Query instance actions
	rows, err := svc.activeDB().Query(c.Request.Context(),
		`SELECT id, action, request_id, user_id, project_id, start_time, message
		 FROM instance_actions
		 WHERE instance_id = $1
		 ORDER BY start_time DESC`,
		instanceID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "list_instance_actions").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to list instance actions"))
		return
	}
	defer rows.Close()

	actions := []gin.H{}
	for rows.Next() {
		var id, action, requestID, userID, projectIDStr, message string
		var startTime time.Time

		if err := rows.Scan(&id, &action, &requestID, &userID, &projectIDStr, &startTime, &message); err != nil {
			log.Warn().Err(err).Msg("failed to scan instance action row")
			continue
		}

		actions = append(actions, gin.H{
			"action":     action,
			"request_id": requestID,
			"user_id":    userID,
			"project_id": projectIDStr,
			"start_time": startTime.Format(time.RFC3339),
			"message":    message,
		})
	}
	if err := rows.Err(); err != nil {
		log.Error().Err(err).Str("operation", "list_instance_actions").Msg("rows iteration error")
		common.SendError(c, common.NewInternalServerError("failed to list instance actions"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"instanceActions": actions})
}

// GetInstanceAction handles GET /v2.1/servers/:id/os-instance-actions/:request_id
func (svc *Service) GetInstanceAction(c *gin.Context) {
	instanceID := c.Param("id")
	requestID := c.Param("request_id")
	projectID := c.GetString("project_id")

	// Verify instance exists and belongs to project
	var exists bool
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM instances WHERE id = $1 AND project_id = $2)",
		instanceID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		common.SendError(c, common.NewNotFoundError("instance"))
		return
	}

	// Query specific action
	var action, userID, projectIDStr, message string
	var startTime time.Time

	err = svc.activeDB().QueryRow(c.Request.Context(),
		`SELECT action, user_id, project_id, start_time, message
		 FROM instance_actions
		 WHERE instance_id = $1 AND request_id = $2`,
		instanceID, requestID,
	).Scan(&action, &userID, &projectIDStr, &startTime, &message)

	if err != nil {
		common.SendError(c, common.NewNotFoundError("action"))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"instanceAction": gin.H{
			"action":     action,
			"request_id": requestID,
			"user_id":    userID,
			"project_id": projectIDStr,
			"start_time": startTime.Format(time.RFC3339),
			"message":    message,
		},
	})
}
