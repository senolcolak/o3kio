package keystone

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/common"
	"github.com/cobaltcore-dev/o3k/internal/keystone/policy"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

// PolicyEngine is the global policy engine instance.
var PolicyEngine *policy.Engine

func init() {
	PolicyEngine = policy.NewEngine()
}

// LoadPoliciesFromDB reads all policies from the database and loads them into the engine.
func (svc *Service) LoadPoliciesFromDB(ctx context.Context) error {
	rows, err := svc.activeDB().Query(ctx, "SELECT blob FROM keystone_policies ORDER BY created_at ASC")
	if err != nil {
		return fmt.Errorf("query keystone_policies: %w", err)
	}
	defer rows.Close()

	allPolicies := make(map[string]string)
	for rows.Next() {
		var blob string
		if err := rows.Scan(&blob); err != nil {
			continue
		}
		var policies map[string]string
		if err := json.Unmarshal([]byte(blob), &policies); err != nil {
			log.Warn().Err(err).Msg("failed to parse policy blob")
			continue
		}
		for k, v := range policies {
			allPolicies[k] = v
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate keystone_policies: %w", err)
	}

	PolicyEngine.LoadPolicy(allPolicies)
	return nil
}

// ListPolicies returns all policies (GET /v3/policies).
func (svc *Service) ListPolicies(c *gin.Context) {
	rows, err := svc.activeDB().Query(c.Request.Context(),
		"SELECT id, type, blob, created_at, updated_at FROM keystone_policies ORDER BY created_at")
	if err != nil {
		common.SendError(c, common.NewInternalServerError("failed to list policies"))
		return
	}
	defer rows.Close()

	var policies []gin.H
	for rows.Next() {
		var id, pType, blob string
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&id, &pType, &blob, &createdAt, &updatedAt); err != nil {
			continue
		}
		policies = append(policies, gin.H{
			"id":         id,
			"type":       pType,
			"blob":       blob,
			"links":      gin.H{"self": "/v3/policies/" + id},
			"created_at": createdAt.Format(time.RFC3339),
			"updated_at": updatedAt.Format(time.RFC3339),
		})
	}
	if policies == nil {
		policies = []gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{
		"policies": policies,
		"links":    gin.H{"self": "/v3/policies", "next": nil, "previous": nil},
	})
}

// CreatePolicy creates a new policy (POST /v3/policies).
func (svc *Service) CreatePolicy(c *gin.Context) {
	var req struct {
		Policy struct {
			Type string `json:"type"`
			Blob string `json:"blob"`
		} `json:"policy"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	if req.Policy.Blob == "" {
		common.SendError(c, common.NewBadRequestError("policy blob is required"))
		return
	}

	var rules map[string]string
	if err := json.Unmarshal([]byte(req.Policy.Blob), &rules); err != nil {
		common.SendError(c, common.NewBadRequestError("policy blob must be valid JSON with string values"))
		return
	}

	pType := req.Policy.Type
	if pType == "" {
		pType = "application/json"
	}

	id := uuid.New().String()
	now := time.Now()

	_, err := svc.activeDB().Exec(c.Request.Context(),
		"INSERT INTO keystone_policies (id, type, blob, created_at, updated_at) VALUES ($1, $2, $3, $4, $5)",
		id, pType, req.Policy.Blob, now, now)
	if err != nil {
		common.SendError(c, common.NewInternalServerError("failed to create policy"))
		return
	}

	if err := svc.LoadPoliciesFromDB(c.Request.Context()); err != nil {
		log.Warn().Err(err).Msg("failed to reload policies after create")
	}

	c.JSON(http.StatusCreated, gin.H{
		"policy": gin.H{
			"id":         id,
			"type":       pType,
			"blob":       req.Policy.Blob,
			"links":      gin.H{"self": "/v3/policies/" + id},
			"created_at": now.Format(time.RFC3339),
			"updated_at": now.Format(time.RFC3339),
		},
	})
}

// GetPolicy returns a single policy (GET /v3/policies/:policy_id).
func (svc *Service) GetPolicy(c *gin.Context) {
	policyID := c.Param("policy_id")

	var id, pType, blob string
	var createdAt, updatedAt time.Time
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT id, type, blob, created_at, updated_at FROM keystone_policies WHERE id = $1",
		policyID).Scan(&id, &pType, &blob, &createdAt, &updatedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		common.SendError(c, common.NewNotFoundError("policy"))
		return
	}
	if err != nil {
		common.SendError(c, common.NewInternalServerError("failed to get policy"))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"policy": gin.H{
			"id":         id,
			"type":       pType,
			"blob":       blob,
			"links":      gin.H{"self": "/v3/policies/" + id},
			"created_at": createdAt.Format(time.RFC3339),
			"updated_at": updatedAt.Format(time.RFC3339),
		},
	})
}

// UpdatePolicy updates a policy (PATCH /v3/policies/:policy_id).
func (svc *Service) UpdatePolicy(c *gin.Context) {
	policyID := c.Param("policy_id")

	var req struct {
		Policy struct {
			Type string `json:"type"`
			Blob string `json:"blob"`
		} `json:"policy"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	if req.Policy.Blob != "" {
		var rules map[string]string
		if err := json.Unmarshal([]byte(req.Policy.Blob), &rules); err != nil {
			common.SendError(c, common.NewBadRequestError("policy blob must be valid JSON"))
			return
		}
	}

	now := time.Now()
	result, err := svc.activeDB().Exec(c.Request.Context(),
		"UPDATE keystone_policies SET blob = COALESCE(NULLIF($1, ''), blob), type = COALESCE(NULLIF($2, ''), type), updated_at = $3 WHERE id = $4",
		req.Policy.Blob, req.Policy.Type, now, policyID)
	if err != nil {
		common.SendError(c, common.NewInternalServerError("failed to update policy"))
		return
	}
	if result.RowsAffected() == 0 {
		common.SendError(c, common.NewNotFoundError("policy"))
		return
	}

	if err := svc.LoadPoliciesFromDB(c.Request.Context()); err != nil {
		log.Warn().Err(err).Msg("failed to reload policies after update")
	}

	svc.GetPolicy(c)
}

// DeletePolicy deletes a policy (DELETE /v3/policies/:policy_id).
func (svc *Service) DeletePolicy(c *gin.Context) {
	policyID := c.Param("policy_id")

	result, err := svc.activeDB().Exec(c.Request.Context(),
		"DELETE FROM keystone_policies WHERE id = $1", policyID)
	if err != nil {
		common.SendError(c, common.NewInternalServerError("failed to delete policy"))
		return
	}
	if result.RowsAffected() == 0 {
		common.SendError(c, common.NewNotFoundError("policy"))
		return
	}

	if err := svc.LoadPoliciesFromDB(c.Request.Context()); err != nil {
		log.Warn().Err(err).Msg("failed to reload policies after delete")
	}

	c.Status(http.StatusNoContent)
}
