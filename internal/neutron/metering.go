package neutron

import (
	"net/http"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ListMeteringLabels handles GET /v2.0/metering/metering-labels
func (svc *Service) ListMeteringLabels(c *gin.Context) {
	projectID := c.GetString("project_id")

	rows, err := database.DB.Query(c.Request.Context(), `
		SELECT id, name, description, project_id, shared, created_at, updated_at
		FROM metering_labels
		WHERE project_id = $1 OR shared = true
		ORDER BY created_at DESC
	`, projectID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	labels := []map[string]interface{}{}
	for rows.Next() {
		var id, projectID uuid.UUID
		var name string
		var description *string
		var shared bool
		var createdAt, updatedAt time.Time

		err := rows.Scan(&id, &name, &description, &projectID, &shared, &createdAt, &updatedAt)
		if err != nil {
			continue
		}

		label := map[string]interface{}{
			"id":         id.String(),
			"name":       name,
			"project_id": projectID.String(),
			"tenant_id":  projectID.String(),
			"shared":     shared,
		}

		if description != nil {
			label["description"] = *description
		}

		labels = append(labels, label)
	}

	c.JSON(http.StatusOK, gin.H{"metering_labels": labels})
}

// CreateMeteringLabel handles POST /v2.0/metering/metering-labels
func (svc *Service) CreateMeteringLabel(c *gin.Context) {
	projectID := c.GetString("project_id")

	var req struct {
		MeteringLabel struct {
			Name        string `json:"name" binding:"required"`
			Description string `json:"description"`
			Shared      bool   `json:"shared"`
		} `json:"metering_label" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	id := uuid.New()
	var description *string
	if req.MeteringLabel.Description != "" {
		description = &req.MeteringLabel.Description
	}

	_, err := database.DB.Exec(c.Request.Context(), `
		INSERT INTO metering_labels (id, name, description, project_id, shared, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, id, req.MeteringLabel.Name, description, projectID, req.MeteringLabel.Shared, time.Now(), time.Now())

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	label := map[string]interface{}{
		"id":         id.String(),
		"name":       req.MeteringLabel.Name,
		"project_id": projectID,
		"tenant_id":  projectID,
		"shared":     req.MeteringLabel.Shared,
	}

	if description != nil {
		label["description"] = *description
	}

	c.JSON(http.StatusCreated, gin.H{"metering_label": label})
}

// GetMeteringLabel handles GET /v2.0/metering/metering-labels/:id
func (svc *Service) GetMeteringLabel(c *gin.Context) {
	projectID := c.GetString("project_id")
	labelID := c.Param("id")

	var id, labelProjectID uuid.UUID
	var name string
	var description *string
	var shared bool
	var createdAt, updatedAt time.Time

	err := database.DB.QueryRow(c.Request.Context(), `
		SELECT id, name, description, project_id, shared, created_at, updated_at
		FROM metering_labels
		WHERE id = $1 AND (project_id = $2 OR shared = true)
	`, labelID, projectID).Scan(&id, &name, &description, &labelProjectID, &shared, &createdAt, &updatedAt)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Metering label not found"})
		return
	}

	label := map[string]interface{}{
		"id":         id.String(),
		"name":       name,
		"project_id": labelProjectID.String(),
		"tenant_id":  labelProjectID.String(),
		"shared":     shared,
	}

	if description != nil {
		label["description"] = *description
	}

	c.JSON(http.StatusOK, gin.H{"metering_label": label})
}

// DeleteMeteringLabel handles DELETE /v2.0/metering/metering-labels/:id
func (svc *Service) DeleteMeteringLabel(c *gin.Context) {
	projectID := c.GetString("project_id")
	labelID := c.Param("id")

	result, err := database.DB.Exec(c.Request.Context(),
		"DELETE FROM metering_labels WHERE id = $1 AND project_id = $2",
		labelID, projectID,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if result.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Metering label not found"})
		return
	}

	c.Status(http.StatusNoContent)
}

// ListMeteringLabelRules handles GET /v2.0/metering/metering-label-rules
func (svc *Service) ListMeteringLabelRules(c *gin.Context) {
	projectID := c.GetString("project_id")

	rows, err := database.DB.Query(c.Request.Context(), `
		SELECT mlr.id, mlr.metering_label_id, mlr.remote_ip_prefix, mlr.direction, mlr.excluded, mlr.created_at
		FROM metering_label_rules mlr
		JOIN metering_labels ml ON mlr.metering_label_id = ml.id
		WHERE ml.project_id = $1 OR ml.shared = true
		ORDER BY mlr.created_at DESC
	`, projectID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	rules := []map[string]interface{}{}
	for rows.Next() {
		var id, labelID uuid.UUID
		var remoteIPPrefix, direction string
		var excluded bool
		var createdAt time.Time

		err := rows.Scan(&id, &labelID, &remoteIPPrefix, &direction, &excluded, &createdAt)
		if err != nil {
			continue
		}

		rule := map[string]interface{}{
			"id":                 id.String(),
			"metering_label_id":  labelID.String(),
			"remote_ip_prefix":   remoteIPPrefix,
			"direction":          direction,
			"excluded":           excluded,
		}

		rules = append(rules, rule)
	}

	c.JSON(http.StatusOK, gin.H{"metering_label_rules": rules})
}

// CreateMeteringLabelRule handles POST /v2.0/metering/metering-label-rules
func (svc *Service) CreateMeteringLabelRule(c *gin.Context) {
	projectID := c.GetString("project_id")

	var req struct {
		MeteringLabelRule struct {
			MeteringLabelID string `json:"metering_label_id" binding:"required"`
			RemoteIPPrefix  string `json:"remote_ip_prefix" binding:"required"`
			Direction       string `json:"direction"`
			Excluded        bool   `json:"excluded"`
		} `json:"metering_label_rule" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Verify label belongs to project
	var labelExists bool
	err := database.DB.QueryRow(c.Request.Context(), `
		SELECT EXISTS(SELECT 1 FROM metering_labels WHERE id = $1 AND (project_id = $2 OR shared = true))
	`, req.MeteringLabelRule.MeteringLabelID, projectID).Scan(&labelExists)

	if err != nil || !labelExists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Metering label not found"})
		return
	}

	// Default direction
	direction := req.MeteringLabelRule.Direction
	if direction == "" {
		direction = "ingress"
	}

	id := uuid.New()

	_, err = database.DB.Exec(c.Request.Context(), `
		INSERT INTO metering_label_rules (id, metering_label_id, remote_ip_prefix, direction, excluded, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, id, req.MeteringLabelRule.MeteringLabelID, req.MeteringLabelRule.RemoteIPPrefix, direction, req.MeteringLabelRule.Excluded, time.Now())

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	rule := map[string]interface{}{
		"id":                 id.String(),
		"metering_label_id":  req.MeteringLabelRule.MeteringLabelID,
		"remote_ip_prefix":   req.MeteringLabelRule.RemoteIPPrefix,
		"direction":          direction,
		"excluded":           req.MeteringLabelRule.Excluded,
	}

	c.JSON(http.StatusCreated, gin.H{"metering_label_rule": rule})
}

// DeleteMeteringLabelRule handles DELETE /v2.0/metering/metering-label-rules/:id
func (svc *Service) DeleteMeteringLabelRule(c *gin.Context) {
	projectID := c.GetString("project_id")
	ruleID := c.Param("id")

	result, err := database.DB.Exec(c.Request.Context(), `
		DELETE FROM metering_label_rules
		WHERE id = $1 AND metering_label_id IN (
			SELECT id FROM metering_labels WHERE project_id = $2
		)
	`, ruleID, projectID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if result.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Metering label rule not found"})
		return
	}

	c.Status(http.StatusNoContent)
}
