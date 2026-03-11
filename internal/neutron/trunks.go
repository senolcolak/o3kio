package neutron

import (
	"net/http"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ListTrunks handles GET /v2.0/trunks
func (svc *Service) ListTrunks(c *gin.Context) {
	projectID := c.GetString("project_id")

	rows, err := database.DB.Query(c.Request.Context(), `
		SELECT id, name, description, project_id, port_id, admin_state_up, status, created_at, updated_at
		FROM trunks
		WHERE project_id = $1
	`, projectID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
		subPortRows, _ := database.DB.Query(c.Request.Context(), `
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
			"id":              id.String(),
			"name":            name,
			"description":     description,
			"project_id":      projectIDStr.String(),
			"tenant_id":       projectIDStr.String(),
			"port_id":         portID.String(),
			"admin_state_up":  adminStateUp,
			"status":          status,
			"sub_ports":       subPorts,
			"created_at":      createdAt.Format(time.RFC3339),
			"updated_at":      updatedAt.Format(time.RFC3339),
		}
		trunks = append(trunks, trunk)
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	adminStateUp := true
	if req.Trunk.AdminStateUp != nil {
		adminStateUp = *req.Trunk.AdminStateUp
	}

	trunkID := uuid.New()
	portID, err := uuid.Parse(req.Trunk.PortID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid port_id"})
		return
	}

	_, err = database.DB.Exec(c.Request.Context(), `
		INSERT INTO trunks (id, name, description, project_id, port_id, admin_state_up, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, trunkID, req.Trunk.Name, req.Trunk.Description, projectID, portID, adminStateUp, "ACTIVE", time.Now(), time.Now())

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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

	err := database.DB.QueryRow(c.Request.Context(), `
		SELECT id, name, description, project_id, port_id, admin_state_up, status, created_at, updated_at
		FROM trunks
		WHERE id = $1 AND project_id = $2
	`, trunkID, projectID).Scan(&id, &name, &description, &projectIDStr, &portID, &adminStateUp, &status, &createdAt, &updatedAt)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Trunk not found"})
		return
	}

	// Get subports
	subPortRows, _ := database.DB.Query(c.Request.Context(), `
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Verify trunk exists
	var exists bool
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM trunks WHERE id = $1 AND project_id = $2)",
		trunkID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Trunk not found"})
		return
	}

	// Build UPDATE query dynamically
	updates := []string{}
	args := []interface{}{}
	argPos := 1

	if name, ok := req.Trunk["name"].(string); ok {
		updates = append(updates, "name = $"+string(rune(argPos+'0')))
		args = append(args, name)
		argPos++
	}

	if description, ok := req.Trunk["description"].(string); ok {
		updates = append(updates, "description = $"+string(rune(argPos+'0')))
		args = append(args, description)
		argPos++
	}

	if adminStateUp, ok := req.Trunk["admin_state_up"].(bool); ok {
		updates = append(updates, "admin_state_up = $"+string(rune(argPos+'0')))
		args = append(args, adminStateUp)
		argPos++
	}

	if len(updates) > 0 {
		updates = append(updates, "updated_at = $"+string(rune(argPos+'0')))
		args = append(args, time.Now())
		argPos++

		args = append(args, trunkID, projectID)

		query := "UPDATE trunks SET " + updates[0]
		for i := 1; i < len(updates); i++ {
			query += ", " + updates[i]
		}
		query += " WHERE id = $" + string(rune(argPos+'0')) + " AND project_id = $" + string(rune(argPos+'1'))

		_, err = database.DB.Exec(c.Request.Context(), query, args...)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	// Fetch updated trunk
	var id, portID uuid.UUID
	var projectIDStr uuid.UUID
	var name, description, status string
	var adminStateUp bool
	var createdAt, updatedAt time.Time

	err = database.DB.QueryRow(c.Request.Context(), `
		SELECT id, name, description, project_id, port_id, admin_state_up, status, created_at, updated_at
		FROM trunks
		WHERE id = $1 AND project_id = $2
	`, trunkID, projectID).Scan(&id, &name, &description, &projectIDStr, &portID, &adminStateUp, &status, &createdAt, &updatedAt)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Get subports
	subPortRows, _ := database.DB.Query(c.Request.Context(), `
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

	result, err := database.DB.Exec(c.Request.Context(),
		"DELETE FROM trunks WHERE id = $1 AND project_id = $2",
		trunkID, projectID,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Trunk not found"})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Verify trunk exists
	var exists bool
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM trunks WHERE id = $1 AND project_id = $2)",
		trunkID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Trunk not found"})
		return
	}

	// Add subports
	for _, subPort := range req.SubPorts {
		portID, err := uuid.Parse(subPort.PortID)
		if err != nil {
			continue
		}

		_, err = database.DB.Exec(c.Request.Context(), `
			INSERT INTO trunk_subports (trunk_id, port_id, segmentation_type, segmentation_id, created_at)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (trunk_id, port_id) DO NOTHING
		`, trunkID, portID, subPort.SegmentationType, subPort.SegmentationID, time.Now())

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	// Update trunk updated_at
	_, err = database.DB.Exec(c.Request.Context(),
		"UPDATE trunks SET updated_at = $1 WHERE id = $2",
		time.Now(), trunkID,
	)

	// Fetch updated trunk
	var id, portID uuid.UUID
	var projectIDStr uuid.UUID
	var name, description, status string
	var adminStateUp bool
	var createdAt, updatedAt time.Time

	err = database.DB.QueryRow(c.Request.Context(), `
		SELECT id, name, description, project_id, port_id, admin_state_up, status, created_at, updated_at
		FROM trunks
		WHERE id = $1 AND project_id = $2
	`, trunkID, projectID).Scan(&id, &name, &description, &projectIDStr, &portID, &adminStateUp, &status, &createdAt, &updatedAt)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Get subports
	subPortRows, _ := database.DB.Query(c.Request.Context(), `
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Verify trunk exists
	var exists bool
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM trunks WHERE id = $1 AND project_id = $2)",
		trunkID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Trunk not found"})
		return
	}

	// Remove subports
	for _, subPort := range req.SubPorts {
		_, err = database.DB.Exec(c.Request.Context(),
			"DELETE FROM trunk_subports WHERE trunk_id = $1 AND port_id = $2",
			trunkID, subPort.PortID,
		)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	// Update trunk updated_at
	_, err = database.DB.Exec(c.Request.Context(),
		"UPDATE trunks SET updated_at = $1 WHERE id = $2",
		time.Now(), trunkID,
	)

	// Fetch updated trunk
	var id, portID uuid.UUID
	var projectIDStr uuid.UUID
	var name, description, status string
	var adminStateUp bool
	var createdAt, updatedAt time.Time

	err = database.DB.QueryRow(c.Request.Context(), `
		SELECT id, name, description, project_id, port_id, admin_state_up, status, created_at, updated_at
		FROM trunks
		WHERE id = $1 AND project_id = $2
	`, trunkID, projectID).Scan(&id, &name, &description, &projectIDStr, &portID, &adminStateUp, &status, &createdAt, &updatedAt)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Get subports
	subPortRows, _ := database.DB.Query(c.Request.Context(), `
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
