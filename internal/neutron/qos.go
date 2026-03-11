package neutron

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ListQoSPolicies lists all QoS policies
func (svc *Service) ListQoSPolicies(c *gin.Context) {
	projectID := c.GetString("project_id")

	rows, err := database.DB.Query(c.Request.Context(), `
		SELECT id, project_id, name, description, shared, created_at, updated_at
		FROM qos_policies
		WHERE project_id = $1 OR shared = true
		ORDER BY created_at DESC
	`, projectID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	policyID := uuid.New().String()
	now := time.Now()

	shared := false
	if req.Policy.Shared != nil {
		shared = *req.Policy.Shared
	}

	_, err := database.DB.Exec(c.Request.Context(), `
		INSERT INTO qos_policies (id, project_id, name, description, shared, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, policyID, projectID, req.Policy.Name, req.Policy.Description, shared, now, now)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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

	err := database.DB.QueryRow(c.Request.Context(), `
		SELECT project_id, name, description, shared, created_at, updated_at
		FROM qos_policies
		WHERE id = $1 AND (project_id = $2 OR shared = true)
	`, policyID, projectID).Scan(&projID, &name, &description, &shared, &createdAt, &updatedAt)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "policy not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if policy exists and belongs to project
	var exists bool
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM qos_policies WHERE id = $1 AND project_id = $2)",
		policyID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "policy not found"})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "no fields to update"})
		return
	}

	args = append(args, time.Now(), policyID)
	query := fmt.Sprintf("UPDATE qos_policies SET %s, updated_at = $%d WHERE id = $%d",
		strings.Join(updates, ", "), argPos, argPos+1)

	_, err = database.DB.Exec(c.Request.Context(), query, args...)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Return updated policy
	svc.GetQoSPolicy(c)
}

// DeleteQoSPolicy deletes a QoS policy
func (svc *Service) DeleteQoSPolicy(c *gin.Context) {
	policyID := c.Param("id")
	projectID := c.GetString("project_id")

	result, err := database.DB.Exec(c.Request.Context(),
		"DELETE FROM qos_policies WHERE id = $1 AND project_id = $2",
		policyID, projectID,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if result.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "policy not found"})
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
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM qos_policies WHERE id = $1 AND (project_id = $2 OR shared = true))",
		policyID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "policy not found"})
		return
	}

	rows, err := database.DB.Query(c.Request.Context(), `
		SELECT id, max_kbps, max_burst_kbps, direction, created_at, updated_at
		FROM qos_bandwidth_limit_rules
		WHERE qos_policy_id = $1
		ORDER BY created_at DESC
	`, policyID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		rule := map[string]interface{}{
			"id":              id,
			"max_kbps":        maxKbps,
			"max_burst_kbps":  maxBurstKbps,
			"direction":       direction,
			"qos_policy_id":   policyID,
		}
		rules = append(rules, rule)
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if policy exists and belongs to project
	var exists bool
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM qos_policies WHERE id = $1 AND project_id = $2)",
		policyID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "policy not found"})
		return
	}

	ruleID := uuid.New().String()
	now := time.Now()

	direction := req.Rule.Direction
	if direction == "" {
		direction = "egress"
	}

	_, err = database.DB.Exec(c.Request.Context(), `
		INSERT INTO qos_bandwidth_limit_rules (id, qos_policy_id, max_kbps, max_burst_kbps, direction, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, ruleID, policyID, req.Rule.MaxKbps, req.Rule.MaxBurstKbps, direction, now, now)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"bandwidth_limit_rule": map[string]interface{}{
			"id":              ruleID,
			"max_kbps":        req.Rule.MaxKbps,
			"max_burst_kbps":  req.Rule.MaxBurstKbps,
			"direction":       direction,
			"qos_policy_id":   policyID,
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
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM qos_policies WHERE id = $1 AND (project_id = $2 OR shared = true))",
		policyID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "policy not found"})
		return
	}

	var (
		maxKbps      int
		maxBurstKbps *int
		direction    string
		createdAt    time.Time
		updatedAt    time.Time
	)

	err = database.DB.QueryRow(c.Request.Context(), `
		SELECT max_kbps, max_burst_kbps, direction, created_at, updated_at
		FROM qos_bandwidth_limit_rules
		WHERE id = $1 AND qos_policy_id = $2
	`, ruleID, policyID).Scan(&maxKbps, &maxBurstKbps, &direction, &createdAt, &updatedAt)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "rule not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"bandwidth_limit_rule": map[string]interface{}{
			"id":              ruleID,
			"max_kbps":        maxKbps,
			"max_burst_kbps":  maxBurstKbps,
			"direction":       direction,
			"qos_policy_id":   policyID,
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if policy exists and belongs to project
	var exists bool
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM qos_policies WHERE id = $1 AND project_id = $2)",
		policyID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "policy not found"})
		return
	}

	// Check if rule exists
	err = database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM qos_bandwidth_limit_rules WHERE id = $1 AND qos_policy_id = $2)",
		ruleID, policyID,
	).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "rule not found"})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "no fields to update"})
		return
	}

	args = append(args, time.Now(), ruleID)
	query := fmt.Sprintf("UPDATE qos_bandwidth_limit_rules SET %s, updated_at = $%d WHERE id = $%d",
		strings.Join(updates, ", "), argPos, argPos+1)

	_, err = database.DB.Exec(c.Request.Context(), query, args...)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM qos_policies WHERE id = $1 AND project_id = $2)",
		policyID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "policy not found"})
		return
	}

	result, err := database.DB.Exec(c.Request.Context(),
		"DELETE FROM qos_bandwidth_limit_rules WHERE id = $1 AND qos_policy_id = $2",
		ruleID, policyID,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if result.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "rule not found"})
		return
	}

	c.Status(http.StatusNoContent)
}
