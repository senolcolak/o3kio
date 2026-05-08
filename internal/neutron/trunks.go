package neutron

import (
	"fmt"
	"net/http"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/common"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// ListTrunks handles GET /v2.0/trunks
func (svc *Service) ListTrunks(c *gin.Context) {
	projectID := c.GetString("project_id")

	rows, err := svc.activeDB().Query(c.Request.Context(), `
		SELECT id, name, description, project_id, port_id, admin_state_up, status, created_at, updated_at
		FROM trunks
		WHERE project_id = $1
	`, projectID)
	if err != nil {
		log.Error().Err(err).Str("operation", "list_trunks").Msg("failed to query trunks")
		common.SendError(c, common.NewInternalServerError("failed to list trunks"))
		return
	}
	defer rows.Close()

	trunks := []map[string]interface{}{}
	for rows.Next() {
		var id, portID uuid.UUID
		var projectIDStr uuid.UUID
		var name, description, status string
		var adminStateUp bool
		var createdAt, updatedAt time.Time

		err := rows.Scan(&id, &name, &description, &projectIDStr, &portID, &adminStateUp, &status, &createdAt, &updatedAt)
		if err != nil {
			continue
		}

		// Get subports
		subPortRows, _ := svc.activeDB().Query(c.Request.Context(), `
			SELECT port_id, segmentation_type, segmentation_id
			FROM trunk_subports
			WHERE trunk_id = $1
		`, id)

		subPorts := []map[string]interface{}{}
		if subPortRows != nil {
			for subPortRows.Next() {
				var subPortID uuid.UUID
				var segmentationType string
				var segmentationID int
				if subPortRows.Scan(&subPortID, &segmentationType, &segmentationID) == nil {
					subPorts = append(subPorts, map[string]interface{}{
						"port_id":           subPortID.String(),
						"segmentation_type": segmentationType,
						"segmentation_id":   segmentationID,
					})
				}
			}
			subPortRows.Close()
		}

		trunk := map[string]interface{}{
			"id":             id.String(),
			"name":           name,
			"description":    description,
			"project_id":     projectIDStr.String(),
			"tenant_id":      projectIDStr.String(),
			"port_id":        portID.String(),
			"admin_state_up": adminStateUp,
			"status":         status,
			"sub_ports":      subPorts,
			"created_at":     createdAt.Format(time.RFC3339),
			"updated_at":     updatedAt.Format(time.RFC3339),
		}
		trunks = append(trunks, trunk)
	}
	if err := rows.Err(); err != nil {
		log.Error().Err(err).Str("operation", "list_trunks").Msg("rows iteration error")
		common.SendError(c, common.NewInternalServerError("failed to list trunks"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"trunks": trunks})
}

// CreateTrunk handles POST /v2.0/trunks
func (svc *Service) CreateTrunk(c *gin.Context) {
	projectID := c.GetString("project_id")

	var req struct {
		Trunk struct {
			Name         string `json:"name"`
			Description  string `json:"description"`
			PortID       string `json:"port_id" binding:"required"`
			AdminStateUp *bool  `json:"admin_state_up"`
		} `json:"trunk" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	adminStateUp := true
	if req.Trunk.AdminStateUp != nil {
		adminStateUp = *req.Trunk.AdminStateUp
	}

	trunkID := uuid.New()
	portID, err := uuid.Parse(req.Trunk.PortID)
	if err != nil {
		common.SendError(c, common.NewBadRequestError("invalid port_id"))
		return
	}

	_, err = svc.activeDB().Exec(c.Request.Context(), `
		INSERT INTO trunks (id, name, description, project_id, port_id, admin_state_up, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, trunkID, req.Trunk.Name, req.Trunk.Description, projectID, portID, adminStateUp, "ACTIVE", time.Now(), time.Now())

	if err != nil {
		log.Error().Err(err).Str("operation", "create_trunk").Msg("failed to create trunk")
		common.SendError(c, common.NewInternalServerError("failed to create trunk"))
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"trunk": map[string]interface{}{
			"id":             trunkID.String(),
			"name":           req.Trunk.Name,
			"description":    req.Trunk.Description,
			"project_id":     projectID,
			"tenant_id":      projectID,
			"port_id":        req.Trunk.PortID,
			"admin_state_up": adminStateUp,
			"status":         "ACTIVE",
			"sub_ports":      []interface{}{},
			"created_at":     time.Now().Format(time.RFC3339),
			"updated_at":     time.Now().Format(time.RFC3339),
		},
	})
}

// GetTrunk handles GET /v2.0/trunks/:id
func (svc *Service) GetTrunk(c *gin.Context) {
	trunkID := c.Param("id")
	projectID := c.GetString("project_id")

	var id, portID uuid.UUID
	var projectIDStr uuid.UUID
	var name, description, status string
	var adminStateUp bool
	var createdAt, updatedAt time.Time

	err := svc.activeDB().QueryRow(c.Request.Context(), `
		SELECT id, name, description, project_id, port_id, admin_state_up, status, created_at, updated_at
		FROM trunks
		WHERE id = $1 AND project_id = $2
	`, trunkID, projectID).Scan(&id, &name, &description, &projectIDStr, &portID, &adminStateUp, &status, &createdAt, &updatedAt)

	if err != nil {
		common.SendError(c, common.NewNotFoundError("trunk"))
		return
	}

	// Get subports
	subPortRows, _ := svc.activeDB().Query(c.Request.Context(), `
		SELECT port_id, segmentation_type, segmentation_id
		FROM trunk_subports
		WHERE trunk_id = $1
	`, id)

	subPorts := []map[string]interface{}{}
	if subPortRows != nil {
		defer subPortRows.Close()
		for subPortRows.Next() {
			var subPortID uuid.UUID
			var segmentationType string
			var segmentationID int
			if subPortRows.Scan(&subPortID, &segmentationType, &segmentationID) == nil {
				subPorts = append(subPorts, map[string]interface{}{
					"port_id":           subPortID.String(),
					"segmentation_type": segmentationType,
					"segmentation_id":   segmentationID,
				})
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"trunk": map[string]interface{}{
			"id":             id.String(),
			"name":           name,
			"description":    description,
			"project_id":     projectIDStr.String(),
			"tenant_id":      projectIDStr.String(),
			"port_id":        portID.String(),
			"admin_state_up": adminStateUp,
			"status":         status,
			"sub_ports":      subPorts,
			"created_at":     createdAt.Format(time.RFC3339),
			"updated_at":     updatedAt.Format(time.RFC3339),
		},
	})
}

// UpdateTrunk handles PUT /v2.0/trunks/:id
func (svc *Service) UpdateTrunk(c *gin.Context) {
	trunkID := c.Param("id")
	projectID := c.GetString("project_id")

	var req struct {
		Trunk map[string]interface{} `json:"trunk" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Verify trunk exists
	var exists bool
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM trunks WHERE id = $1 AND project_id = $2)",
		trunkID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		common.SendError(c, common.NewNotFoundError("trunk"))
		return
	}

	// Build UPDATE query dynamically
	updates := []string{}
	args := []interface{}{}
	argPos := 1

	if name, ok := req.Trunk["name"].(string); ok {
		updates = append(updates, fmt.Sprintf("name = $%d", argPos))
		args = append(args, name)
		argPos++
	}

	if description, ok := req.Trunk["description"].(string); ok {
		updates = append(updates, fmt.Sprintf("description = $%d", argPos))
		args = append(args, description)
		argPos++
	}

	if adminStateUp, ok := req.Trunk["admin_state_up"].(bool); ok {
		updates = append(updates, fmt.Sprintf("admin_state_up = $%d", argPos))
		args = append(args, adminStateUp)
		argPos++
	}

	if len(updates) > 0 {
		updates = append(updates, fmt.Sprintf("updated_at = $%d", argPos))
		args = append(args, time.Now())
		argPos++

		args = append(args, trunkID, projectID)

		query := "UPDATE trunks SET " + updates[0]
		for i := 1; i < len(updates); i++ {
			query += ", " + updates[i]
		}
		query += fmt.Sprintf(" WHERE id = $%d AND project_id = $%d", argPos, argPos+1)

		_, err = svc.activeDB().Exec(c.Request.Context(), query, args...)
		if err != nil {
			log.Error().Err(err).Str("operation", "update_trunk").Msg("failed to update trunk")
			common.SendError(c, common.NewInternalServerError("failed to update trunk"))
			return
		}
	}

	// Fetch updated trunk
	var id, portID uuid.UUID
	var projectIDStr uuid.UUID
	var name, description, status string
	var adminStateUp bool
	var createdAt, updatedAt time.Time

	err = svc.activeDB().QueryRow(c.Request.Context(), `
		SELECT id, name, description, project_id, port_id, admin_state_up, status, created_at, updated_at
		FROM trunks
		WHERE id = $1 AND project_id = $2
	`, trunkID, projectID).Scan(&id, &name, &description, &projectIDStr, &portID, &adminStateUp, &status, &createdAt, &updatedAt)

	if err != nil {
		log.Error().Err(err).Str("operation", "update_trunk_fetch").Msg("failed to fetch updated trunk")
		common.SendError(c, common.NewInternalServerError("failed to fetch updated trunk"))
		return
	}

	// Get subports
	subPortRows, _ := svc.activeDB().Query(c.Request.Context(), `
		SELECT port_id, segmentation_type, segmentation_id
		FROM trunk_subports
		WHERE trunk_id = $1
	`, id)

	subPorts := []map[string]interface{}{}
	if subPortRows != nil {
		defer subPortRows.Close()
		for subPortRows.Next() {
			var subPortID uuid.UUID
			var segmentationType string
			var segmentationID int
			if subPortRows.Scan(&subPortID, &segmentationType, &segmentationID) == nil {
				subPorts = append(subPorts, map[string]interface{}{
					"port_id":           subPortID.String(),
					"segmentation_type": segmentationType,
					"segmentation_id":   segmentationID,
				})
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"trunk": map[string]interface{}{
			"id":             id.String(),
			"name":           name,
			"description":    description,
			"project_id":     projectIDStr.String(),
			"tenant_id":      projectIDStr.String(),
			"port_id":        portID.String(),
			"admin_state_up": adminStateUp,
			"status":         status,
			"sub_ports":      subPorts,
			"created_at":     createdAt.Format(time.RFC3339),
			"updated_at":     updatedAt.Format(time.RFC3339),
		},
	})
}

// DeleteTrunk handles DELETE /v2.0/trunks/:id
func (svc *Service) DeleteTrunk(c *gin.Context) {
	trunkID := c.Param("id")
	projectID := c.GetString("project_id")

	result, err := svc.activeDB().Exec(c.Request.Context(),
		"DELETE FROM trunks WHERE id = $1 AND project_id = $2",
		trunkID, projectID,
	)

	if err != nil {
		log.Error().Err(err).Str("operation", "delete_trunk").Msg("failed to delete trunk")
		common.SendError(c, common.NewInternalServerError("failed to delete trunk"))
		return
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		common.SendError(c, common.NewNotFoundError("trunk"))
		return
	}

	c.Status(http.StatusNoContent)
}

// AddSubports handles PUT /v2.0/trunks/:id/add_subports
func (svc *Service) AddSubports(c *gin.Context) {
	trunkID := c.Param("id")
	projectID := c.GetString("project_id")

	var req struct {
		SubPorts []struct {
			PortID           string `json:"port_id" binding:"required"`
			SegmentationType string `json:"segmentation_type" binding:"required"`
			SegmentationID   int    `json:"segmentation_id" binding:"required"`
		} `json:"sub_ports" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Verify trunk exists
	var exists bool
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM trunks WHERE id = $1 AND project_id = $2)",
		trunkID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		common.SendError(c, common.NewNotFoundError("trunk"))
		return
	}

	// Add subports
	for _, subPort := range req.SubPorts {
		portID, err := uuid.Parse(subPort.PortID)
		if err != nil {
			continue
		}

		_, err = svc.activeDB().Exec(c.Request.Context(), `
			INSERT INTO trunk_subports (trunk_id, port_id, segmentation_type, segmentation_id, created_at)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (trunk_id, port_id) DO NOTHING
		`, trunkID, portID, subPort.SegmentationType, subPort.SegmentationID, time.Now())

		if err != nil {
			log.Error().Err(err).Str("operation", "add_subports").Msg("failed to add subport")
			common.SendError(c, common.NewInternalServerError("failed to add subport"))
			return
		}
	}

	// Update trunk updated_at
	_, _ = svc.activeDB().Exec(c.Request.Context(),
		"UPDATE trunks SET updated_at = $1 WHERE id = $2",
		time.Now(), trunkID,
	)

	// Fetch updated trunk
	var id, portID uuid.UUID
	var projectIDStr uuid.UUID
	var name, description, status string
	var adminStateUp bool
	var createdAt, updatedAt time.Time

	err = svc.activeDB().QueryRow(c.Request.Context(), `
		SELECT id, name, description, project_id, port_id, admin_state_up, status, created_at, updated_at
		FROM trunks
		WHERE id = $1 AND project_id = $2
	`, trunkID, projectID).Scan(&id, &name, &description, &projectIDStr, &portID, &adminStateUp, &status, &createdAt, &updatedAt)

	if err != nil {
		log.Error().Err(err).Str("operation", "add_subports_fetch").Msg("failed to fetch trunk after adding subports")
		common.SendError(c, common.NewInternalServerError("failed to fetch trunk"))
		return
	}

	// Get subports
	subPortRows, _ := svc.activeDB().Query(c.Request.Context(), `
		SELECT port_id, segmentation_type, segmentation_id
		FROM trunk_subports
		WHERE trunk_id = $1
	`, id)

	subPorts := []map[string]interface{}{}
	if subPortRows != nil {
		defer subPortRows.Close()
		for subPortRows.Next() {
			var subPortID uuid.UUID
			var segmentationType string
			var segmentationID int
			if subPortRows.Scan(&subPortID, &segmentationType, &segmentationID) == nil {
				subPorts = append(subPorts, map[string]interface{}{
					"port_id":           subPortID.String(),
					"segmentation_type": segmentationType,
					"segmentation_id":   segmentationID,
				})
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"trunk": map[string]interface{}{
			"id":             id.String(),
			"name":           name,
			"description":    description,
			"project_id":     projectIDStr.String(),
			"tenant_id":      projectIDStr.String(),
			"port_id":        portID.String(),
			"admin_state_up": adminStateUp,
			"status":         status,
			"sub_ports":      subPorts,
			"created_at":     createdAt.Format(time.RFC3339),
			"updated_at":     updatedAt.Format(time.RFC3339),
		},
	})
}

// RemoveSubports handles PUT /v2.0/trunks/:id/remove_subports
func (svc *Service) RemoveSubports(c *gin.Context) {
	trunkID := c.Param("id")
	projectID := c.GetString("project_id")

	var req struct {
		SubPorts []struct {
			PortID string `json:"port_id" binding:"required"`
		} `json:"sub_ports" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Verify trunk exists
	var exists bool
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM trunks WHERE id = $1 AND project_id = $2)",
		trunkID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		common.SendError(c, common.NewNotFoundError("trunk"))
		return
	}

	// Remove subports
	for _, subPort := range req.SubPorts {
		_, err = svc.activeDB().Exec(c.Request.Context(),
			"DELETE FROM trunk_subports WHERE trunk_id = $1 AND port_id = $2",
			trunkID, subPort.PortID,
		)

		if err != nil {
			log.Error().Err(err).Str("operation", "remove_subports").Msg("failed to remove subport")
			common.SendError(c, common.NewInternalServerError("failed to remove subport"))
			return
		}
	}

	// Update trunk updated_at
	_, _ = svc.activeDB().Exec(c.Request.Context(),
		"UPDATE trunks SET updated_at = $1 WHERE id = $2",
		time.Now(), trunkID,
	)

	// Fetch updated trunk
	var id, portID uuid.UUID
	var projectIDStr uuid.UUID
	var name, description, status string
	var adminStateUp bool
	var createdAt, updatedAt time.Time

	err = svc.activeDB().QueryRow(c.Request.Context(), `
		SELECT id, name, description, project_id, port_id, admin_state_up, status, created_at, updated_at
		FROM trunks
		WHERE id = $1 AND project_id = $2
	`, trunkID, projectID).Scan(&id, &name, &description, &projectIDStr, &portID, &adminStateUp, &status, &createdAt, &updatedAt)

	if err != nil {
		log.Error().Err(err).Str("operation", "remove_subports_fetch").Msg("failed to fetch trunk after removing subports")
		common.SendError(c, common.NewInternalServerError("failed to fetch trunk"))
		return
	}

	// Get subports
	subPortRows, _ := svc.activeDB().Query(c.Request.Context(), `
		SELECT port_id, segmentation_type, segmentation_id
		FROM trunk_subports
		WHERE trunk_id = $1
	`, id)

	subPorts := []map[string]interface{}{}
	if subPortRows != nil {
		defer subPortRows.Close()
		for subPortRows.Next() {
			var subPortID uuid.UUID
			var segmentationType string
			var segmentationID int
			if subPortRows.Scan(&subPortID, &segmentationType, &segmentationID) == nil {
				subPorts = append(subPorts, map[string]interface{}{
					"port_id":           subPortID.String(),
					"segmentation_type": segmentationType,
					"segmentation_id":   segmentationID,
				})
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"trunk": map[string]interface{}{
			"id":             id.String(),
			"name":           name,
			"description":    description,
			"project_id":     projectIDStr.String(),
			"tenant_id":      projectIDStr.String(),
			"port_id":        portID.String(),
			"admin_state_up": adminStateUp,
			"status":         status,
			"sub_ports":      subPorts,
			"created_at":     createdAt.Format(time.RFC3339),
			"updated_at":     updatedAt.Format(time.RFC3339),
		},
	})
}
