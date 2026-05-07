package neutron

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/common"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

// ListQoSPolicies lists all QoS policies
func (svc *Service) ListQoSPolicies(c *gin.Context) {
	projectID := c.GetString("project_id")

	rows, err := svc.activeDB().Query(c.Request.Context(), `
		SELECT id, project_id, name, description, shared, created_at, updated_at
		FROM qos_policies
		WHERE project_id = $1 OR shared = true
		ORDER BY created_at DESC
	`, projectID)
	if err != nil {
		log.Error().Err(err).Str("operation", "list_qos_policies").Msg("failed to query QoS policies")
		common.SendError(c, common.NewInternalServerError("failed to list QoS policies"))
		return
	}
	defer rows.Close()

	policies := []map[string]interface{}{}
	for rows.Next() {
		var (
			id          string
			projID      string
			name        string
			description *string
			shared      bool
			createdAt   time.Time
			updatedAt   time.Time
		)

		err := rows.Scan(&id, &projID, &name, &description, &shared, &createdAt, &updatedAt)
		if err != nil {
			log.Error().Err(err).Str("operation", "list_qos_policies_scan").Msg("failed to scan QoS policy row")
			common.SendError(c, common.NewInternalServerError("failed to read QoS policy data"))
			return
		}

		policy := map[string]interface{}{
			"id":          id,
			"project_id":  projID,
			"tenant_id":   projID,
			"name":        name,
			"description": description,
			"shared":      shared,
			"created_at":  createdAt.Format(time.RFC3339),
			"updated_at":  updatedAt.Format(time.RFC3339),
		}
		policies = append(policies, policy)
	}
	if err := rows.Err(); err != nil {
		log.Error().Err(err).Str("operation", "list_qos_policies").Msg("rows iteration error")
		common.SendError(c, common.NewInternalServerError("failed to list QoS policies"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"policies": policies})
}

// CreateQoSPolicy creates a new QoS policy
func (svc *Service) CreateQoSPolicy(c *gin.Context) {
	projectID := c.GetString("project_id")

	var req struct {
		Policy struct {
			Name        string  `json:"name" binding:"required"`
			Description *string `json:"description"`
			Shared      *bool   `json:"shared"`
		} `json:"policy" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	policyID := uuid.New().String()
	now := time.Now()

	shared := false
	if req.Policy.Shared != nil {
		shared = *req.Policy.Shared
	}

	_, err := svc.activeDB().Exec(c.Request.Context(), `
		INSERT INTO qos_policies (id, project_id, name, description, shared, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, policyID, projectID, req.Policy.Name, req.Policy.Description, shared, now, now)

	if err != nil {
		log.Error().Err(err).Str("operation", "create_qos_policy").Msg("failed to create QoS policy")
		common.SendError(c, common.NewInternalServerError("failed to create QoS policy"))
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"policy": map[string]interface{}{
			"id":          policyID,
			"project_id":  projectID,
			"tenant_id":   projectID,
			"name":        req.Policy.Name,
			"description": req.Policy.Description,
			"shared":      shared,
			"created_at":  now.Format(time.RFC3339),
			"updated_at":  now.Format(time.RFC3339),
		},
	})
}

// GetQoSPolicy retrieves a specific QoS policy
func (svc *Service) GetQoSPolicy(c *gin.Context) {
	policyID := c.Param("id")
	projectID := c.GetString("project_id")

	var (
		projID      string
		name        string
		description *string
		shared      bool
		createdAt   time.Time
		updatedAt   time.Time
	)

	err := svc.activeDB().QueryRow(c.Request.Context(), `
		SELECT project_id, name, description, shared, created_at, updated_at
		FROM qos_policies
		WHERE id = $1 AND (project_id = $2 OR shared = true)
	`, policyID, projectID).Scan(&projID, &name, &description, &shared, &createdAt, &updatedAt)

	if err == pgx.ErrNoRows {
		common.SendError(c, common.NewNotFoundError("policy"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "get_qos_policy").Msg("failed to get QoS policy")
		common.SendError(c, common.NewInternalServerError("failed to get QoS policy"))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"policy": map[string]interface{}{
			"id":          policyID,
			"project_id":  projID,
			"tenant_id":   projID,
			"name":        name,
			"description": description,
			"shared":      shared,
			"created_at":  createdAt.Format(time.RFC3339),
			"updated_at":  updatedAt.Format(time.RFC3339),
		},
	})
}

// UpdateQoSPolicy updates a QoS policy
func (svc *Service) UpdateQoSPolicy(c *gin.Context) {
	policyID := c.Param("id")
	projectID := c.GetString("project_id")

	var req struct {
		Policy struct {
			Name        *string `json:"name"`
			Description *string `json:"description"`
			Shared      *bool   `json:"shared"`
		} `json:"policy" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Check if policy exists and belongs to project
	var exists bool
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM qos_policies WHERE id = $1 AND project_id = $2)",
		policyID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		common.SendError(c, common.NewNotFoundError("policy"))
		return
	}

	// Build update query
	updates := []string{}
	args := []interface{}{}
	argPos := 1

	if req.Policy.Name != nil {
		updates = append(updates, fmt.Sprintf("name = $%d", argPos))
		args = append(args, *req.Policy.Name)
		argPos++
	}

	if req.Policy.Description != nil {
		updates = append(updates, fmt.Sprintf("description = $%d", argPos))
		args = append(args, *req.Policy.Description)
		argPos++
	}

	if req.Policy.Shared != nil {
		updates = append(updates, fmt.Sprintf("shared = $%d", argPos))
		args = append(args, *req.Policy.Shared)
		argPos++
	}

	if len(updates) == 0 {
		common.SendError(c, common.NewBadRequestError("no fields to update"))
		return
	}

	args = append(args, time.Now(), policyID)
	query := fmt.Sprintf("UPDATE qos_policies SET %s, updated_at = $%d WHERE id = $%d",
		strings.Join(updates, ", "), argPos, argPos+1)

	_, err = svc.activeDB().Exec(c.Request.Context(), query, args...)

	if err != nil {
		log.Error().Err(err).Str("operation", "update_qos_policy").Msg("failed to update QoS policy")
		common.SendError(c, common.NewInternalServerError("failed to update QoS policy"))
		return
	}

	// Return updated policy
	svc.GetQoSPolicy(c)
}

// DeleteQoSPolicy deletes a QoS policy
func (svc *Service) DeleteQoSPolicy(c *gin.Context) {
	policyID := c.Param("id")
	projectID := c.GetString("project_id")

	result, err := svc.activeDB().Exec(c.Request.Context(),
		"DELETE FROM qos_policies WHERE id = $1 AND project_id = $2",
		policyID, projectID,
	)

	if err != nil {
		log.Error().Err(err).Str("operation", "delete_qos_policy").Msg("failed to delete QoS policy")
		common.SendError(c, common.NewInternalServerError("failed to delete QoS policy"))
		return
	}

	if result.RowsAffected() == 0 {
		common.SendError(c, common.NewNotFoundError("policy"))
		return
	}

	c.Status(http.StatusNoContent)
}

// ListBandwidthLimitRules lists bandwidth limit rules for a policy
func (svc *Service) ListBandwidthLimitRules(c *gin.Context) {
	policyID := c.Param("id")
	projectID := c.GetString("project_id")

	// Check if policy exists and is accessible
	var exists bool
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM qos_policies WHERE id = $1 AND (project_id = $2 OR shared = true))",
		policyID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		common.SendError(c, common.NewNotFoundError("policy"))
		return
	}

	rows, err := svc.activeDB().Query(c.Request.Context(), `
		SELECT id, max_kbps, max_burst_kbps, direction, created_at, updated_at
		FROM qos_bandwidth_limit_rules
		WHERE qos_policy_id = $1
		ORDER BY created_at DESC
	`, policyID)
	if err != nil {
		log.Error().Err(err).Str("operation", "list_bandwidth_limit_rules").Msg("failed to query bandwidth limit rules")
		common.SendError(c, common.NewInternalServerError("failed to list bandwidth limit rules"))
		return
	}
	defer rows.Close()

	rules := []map[string]interface{}{}
	for rows.Next() {
		var (
			id           string
			maxKbps      int
			maxBurstKbps *int
			direction    string
			createdAt    time.Time
			updatedAt    time.Time
		)

		err := rows.Scan(&id, &maxKbps, &maxBurstKbps, &direction, &createdAt, &updatedAt)
		if err != nil {
			log.Error().Err(err).Str("operation", "list_bandwidth_limit_rules_scan").Msg("failed to scan bandwidth limit rule row")
			common.SendError(c, common.NewInternalServerError("failed to read bandwidth limit rule data"))
			return
		}

		rule := map[string]interface{}{
			"id":             id,
			"max_kbps":       maxKbps,
			"max_burst_kbps": maxBurstKbps,
			"direction":      direction,
			"qos_policy_id":  policyID,
		}
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		log.Error().Err(err).Str("operation", "list_bandwidth_limit_rules").Msg("rows iteration error")
		common.SendError(c, common.NewInternalServerError("failed to list bandwidth limit rules"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"bandwidth_limit_rules": rules})
}

// CreateBandwidthLimitRule creates a bandwidth limit rule
func (svc *Service) CreateBandwidthLimitRule(c *gin.Context) {
	policyID := c.Param("id")
	projectID := c.GetString("project_id")

	var req struct {
		Rule struct {
			MaxKbps      int    `json:"max_kbps" binding:"required"`
			MaxBurstKbps *int   `json:"max_burst_kbps"`
			Direction    string `json:"direction"`
		} `json:"bandwidth_limit_rule" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Check if policy exists and belongs to project
	var exists bool
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM qos_policies WHERE id = $1 AND project_id = $2)",
		policyID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		common.SendError(c, common.NewNotFoundError("policy"))
		return
	}

	ruleID := uuid.New().String()
	now := time.Now()

	direction := req.Rule.Direction
	if direction == "" {
		direction = "egress"
	}

	_, err = svc.activeDB().Exec(c.Request.Context(), `
		INSERT INTO qos_bandwidth_limit_rules (id, qos_policy_id, max_kbps, max_burst_kbps, direction, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, ruleID, policyID, req.Rule.MaxKbps, req.Rule.MaxBurstKbps, direction, now, now)

	if err != nil {
		log.Error().Err(err).Str("operation", "create_bandwidth_limit_rule").Msg("failed to create bandwidth limit rule")
		common.SendError(c, common.NewInternalServerError("failed to create bandwidth limit rule"))
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"bandwidth_limit_rule": map[string]interface{}{
			"id":             ruleID,
			"max_kbps":       req.Rule.MaxKbps,
			"max_burst_kbps": req.Rule.MaxBurstKbps,
			"direction":      direction,
			"qos_policy_id":  policyID,
		},
	})
}

// GetBandwidthLimitRule retrieves a specific bandwidth limit rule
func (svc *Service) GetBandwidthLimitRule(c *gin.Context) {
	policyID := c.Param("id")
	ruleID := c.Param("rule_id")
	projectID := c.GetString("project_id")

	// Check if policy exists and is accessible
	var exists bool
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM qos_policies WHERE id = $1 AND (project_id = $2 OR shared = true))",
		policyID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		common.SendError(c, common.NewNotFoundError("policy"))
		return
	}

	var (
		maxKbps      int
		maxBurstKbps *int
		direction    string
		createdAt    time.Time
		updatedAt    time.Time
	)

	err = svc.activeDB().QueryRow(c.Request.Context(), `
		SELECT max_kbps, max_burst_kbps, direction, created_at, updated_at
		FROM qos_bandwidth_limit_rules
		WHERE id = $1 AND qos_policy_id = $2
	`, ruleID, policyID).Scan(&maxKbps, &maxBurstKbps, &direction, &createdAt, &updatedAt)

	if err == pgx.ErrNoRows {
		common.SendError(c, common.NewNotFoundError("rule"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "get_bandwidth_limit_rule").Msg("failed to get bandwidth limit rule")
		common.SendError(c, common.NewInternalServerError("failed to get bandwidth limit rule"))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"bandwidth_limit_rule": map[string]interface{}{
			"id":             ruleID,
			"max_kbps":       maxKbps,
			"max_burst_kbps": maxBurstKbps,
			"direction":      direction,
			"qos_policy_id":  policyID,
		},
	})
}

// UpdateBandwidthLimitRule updates a bandwidth limit rule
func (svc *Service) UpdateBandwidthLimitRule(c *gin.Context) {
	policyID := c.Param("id")
	ruleID := c.Param("rule_id")
	projectID := c.GetString("project_id")

	var req struct {
		Rule struct {
			MaxKbps      *int    `json:"max_kbps"`
			MaxBurstKbps *int    `json:"max_burst_kbps"`
			Direction    *string `json:"direction"`
		} `json:"bandwidth_limit_rule" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Check if policy exists and belongs to project
	var exists bool
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM qos_policies WHERE id = $1 AND project_id = $2)",
		policyID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		common.SendError(c, common.NewNotFoundError("policy"))
		return
	}

	// Check if rule exists
	err = svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM qos_bandwidth_limit_rules WHERE id = $1 AND qos_policy_id = $2)",
		ruleID, policyID,
	).Scan(&exists)

	if err != nil || !exists {
		common.SendError(c, common.NewNotFoundError("rule"))
		return
	}

	// Build update query
	updates := []string{}
	args := []interface{}{}
	argPos := 1

	if req.Rule.MaxKbps != nil {
		updates = append(updates, fmt.Sprintf("max_kbps = $%d", argPos))
		args = append(args, *req.Rule.MaxKbps)
		argPos++
	}

	if req.Rule.MaxBurstKbps != nil {
		updates = append(updates, fmt.Sprintf("max_burst_kbps = $%d", argPos))
		args = append(args, *req.Rule.MaxBurstKbps)
		argPos++
	}

	if req.Rule.Direction != nil {
		updates = append(updates, fmt.Sprintf("direction = $%d", argPos))
		args = append(args, *req.Rule.Direction)
		argPos++
	}

	if len(updates) == 0 {
		common.SendError(c, common.NewBadRequestError("no fields to update"))
		return
	}

	args = append(args, time.Now(), ruleID)
	query := fmt.Sprintf("UPDATE qos_bandwidth_limit_rules SET %s, updated_at = $%d WHERE id = $%d",
		strings.Join(updates, ", "), argPos, argPos+1)

	_, err = svc.activeDB().Exec(c.Request.Context(), query, args...)

	if err != nil {
		log.Error().Err(err).Str("operation", "update_bandwidth_limit_rule").Msg("failed to update bandwidth limit rule")
		common.SendError(c, common.NewInternalServerError("failed to update bandwidth limit rule"))
		return
	}

	// Return updated rule
	svc.GetBandwidthLimitRule(c)
}

// DeleteBandwidthLimitRule deletes a bandwidth limit rule
func (svc *Service) DeleteBandwidthLimitRule(c *gin.Context) {
	policyID := c.Param("id")
	ruleID := c.Param("rule_id")
	projectID := c.GetString("project_id")

	// Check if policy exists and belongs to project
	var exists bool
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM qos_policies WHERE id = $1 AND project_id = $2)",
		policyID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		common.SendError(c, common.NewNotFoundError("policy"))
		return
	}

	result, err := svc.activeDB().Exec(c.Request.Context(),
		"DELETE FROM qos_bandwidth_limit_rules WHERE id = $1 AND qos_policy_id = $2",
		ruleID, policyID,
	)

	if err != nil {
		log.Error().Err(err).Str("operation", "delete_bandwidth_limit_rule").Msg("failed to delete bandwidth limit rule")
		common.SendError(c, common.NewInternalServerError("failed to delete bandwidth limit rule"))
		return
	}

	if result.RowsAffected() == 0 {
		common.SendError(c, common.NewNotFoundError("rule"))
		return
	}

	c.Status(http.StatusNoContent)
}
