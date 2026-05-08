package cinder

import (
	"errors"
	"net/http"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/common"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

// ListQosSpecs lists all QoS specifications
func (svc *Service) ListQosSpecs(c *gin.Context) {
	projectID := c.GetString("project_id")

	rows, err := svc.activeDB().Query(c.Request.Context(), `
		SELECT id, name, consumer, specs, created_at
		FROM qos_specs
		WHERE project_id = $1
		ORDER BY created_at DESC
	`, projectID)
	if err != nil {
		log.Error().Err(err).Str("operation", "list_qos_specs").Msg("failed to query QoS specs")
		common.SendError(c, common.NewInternalServerError("failed to list QoS specs"))
		return
	}
	defer rows.Close()

	qosSpecs := []map[string]interface{}{}
	for rows.Next() {
		var (
			id        string
			name      string
			consumer  string
			specs     map[string]string
			createdAt time.Time
		)

		err := rows.Scan(&id, &name, &consumer, &specs, &createdAt)
		if err != nil {
			log.Error().Err(err).Str("operation", "list_qos_specs_scan").Msg("failed to scan QoS spec row")
			common.SendError(c, common.NewInternalServerError("failed to list QoS specs"))
			return
		}

		qosSpec := map[string]interface{}{
			"id":       id,
			"name":     name,
			"consumer": consumer,
			"specs":    specs,
		}
		qosSpecs = append(qosSpecs, qosSpec)
	}
	if err := rows.Err(); err != nil {
		log.Error().Err(err).Str("operation", "list_qos_specs").Msg("rows iteration error")
		common.SendError(c, common.NewInternalServerError("failed to list QoS specs"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"qos_specs": qosSpecs})
}

// CreateQosSpec creates a new QoS specification
func (svc *Service) CreateQosSpec(c *gin.Context) {
	var req struct {
		QosSpecs struct {
			Name     string            `json:"name" binding:"required"`
			Consumer string            `json:"consumer"`
			Specs    map[string]string `json:"specs"`
		} `json:"qos_specs" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	if req.QosSpecs.Consumer == "" {
		req.QosSpecs.Consumer = "back-end"
	}

	if req.QosSpecs.Specs == nil {
		req.QosSpecs.Specs = make(map[string]string)
	}

	qosID := uuid.New().String()
	projectID := c.GetString("project_id")
	now := time.Now()

	_, err := svc.activeDB().Exec(c.Request.Context(), `
		INSERT INTO qos_specs (id, name, consumer, specs, project_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, qosID, req.QosSpecs.Name, req.QosSpecs.Consumer, req.QosSpecs.Specs, projectID, now, now)

	if err != nil {
		log.Error().Err(err).Str("operation", "create_qos_spec").Msg("failed to create QoS spec")
		common.SendError(c, common.NewInternalServerError("failed to create QoS spec"))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"qos_specs": map[string]interface{}{
			"id":       qosID,
			"name":     req.QosSpecs.Name,
			"consumer": req.QosSpecs.Consumer,
			"specs":    req.QosSpecs.Specs,
		},
	})
}

// GetQosSpec retrieves a specific QoS specification
func (svc *Service) GetQosSpec(c *gin.Context) {
	qosID := c.Param("id")
	projectID := c.GetString("project_id")

	var (
		name      string
		consumer  string
		specs     map[string]string
		createdAt time.Time
	)

	err := svc.activeDB().QueryRow(c.Request.Context(), `
		SELECT name, consumer, specs, created_at
		FROM qos_specs
		WHERE id = $1 AND project_id = $2
	`, qosID, projectID).Scan(&name, &consumer, &specs, &createdAt)

	if errors.Is(err, pgx.ErrNoRows) {
		common.SendError(c, common.NewNotFoundError("QoS spec"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "get_qos_spec").Str("qos_id", qosID).Msg("failed to query QoS spec")
		common.SendError(c, common.NewInternalServerError("failed to get QoS spec"))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"qos_specs": map[string]interface{}{
			"id":       qosID,
			"name":     name,
			"consumer": consumer,
			"specs":    specs,
		},
	})
}

// UpdateQosSpec updates a QoS specification
func (svc *Service) UpdateQosSpec(c *gin.Context) {
	qosID := c.Param("id")
	projectID := c.GetString("project_id")

	var req struct {
		QosSpecs map[string]string `json:"qos_specs" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Check if QoS spec exists and get current specs
	var currentSpecs map[string]string
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT specs FROM qos_specs WHERE id = $1 AND project_id = $2",
		qosID, projectID,
	).Scan(&currentSpecs)

	if errors.Is(err, pgx.ErrNoRows) {
		common.SendError(c, common.NewNotFoundError("QoS spec"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "update_qos_spec_check").Str("qos_id", qosID).Msg("failed to query QoS spec")
		common.SendError(c, common.NewInternalServerError("failed to update QoS spec"))
		return
	}

	// Merge specs (update existing keys)
	if currentSpecs == nil {
		currentSpecs = make(map[string]string)
	}
	for k, v := range req.QosSpecs {
		currentSpecs[k] = v
	}

	// Update database
	_, err = svc.activeDB().Exec(c.Request.Context(), `
		UPDATE qos_specs
		SET specs = $1, updated_at = $2
		WHERE id = $3 AND project_id = $4
	`, currentSpecs, time.Now(), qosID, projectID)

	if err != nil {
		log.Error().Err(err).Str("operation", "update_qos_spec").Str("qos_id", qosID).Msg("failed to update QoS spec")
		common.SendError(c, common.NewInternalServerError("failed to update QoS spec"))
		return
	}

	// Fetch updated QoS spec
	var (
		name     string
		consumer string
		specs    map[string]string
	)

	err = svc.activeDB().QueryRow(c.Request.Context(), `
		SELECT name, consumer, specs
		FROM qos_specs
		WHERE id = $1
	`, qosID).Scan(&name, &consumer, &specs)

	if err != nil {
		log.Error().Err(err).Str("operation", "update_qos_spec_fetch").Str("qos_id", qosID).Msg("failed to fetch updated QoS spec")
		common.SendError(c, common.NewInternalServerError("failed to fetch updated QoS spec"))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"qos_specs": map[string]interface{}{
			"id":       qosID,
			"name":     name,
			"consumer": consumer,
			"specs":    specs,
		},
	})
}

// DeleteQosSpec deletes a QoS specification
func (svc *Service) DeleteQosSpec(c *gin.Context) {
	qosID := c.Param("id")
	projectID := c.GetString("project_id")

	result, err := svc.activeDB().Exec(c.Request.Context(),
		"DELETE FROM qos_specs WHERE id = $1 AND project_id = $2",
		qosID, projectID,
	)

	if err != nil {
		log.Error().Err(err).Str("operation", "delete_qos_spec").Str("qos_id", qosID).Msg("failed to delete QoS spec")
		common.SendError(c, common.NewInternalServerError("failed to delete QoS spec"))
		return
	}

	if result.RowsAffected() == 0 {
		common.SendError(c, common.NewNotFoundError("QoS spec"))
		return
	}

	c.Status(http.StatusAccepted)
}
