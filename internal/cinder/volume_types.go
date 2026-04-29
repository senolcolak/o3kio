package cinder

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/cobaltcore-dev/o3k/internal/common"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

// CreateVolumeType creates a new volume type
func (svc *Service) CreateVolumeType(c *gin.Context) {
	var req struct {
		VolumeType struct {
			Name        string  `json:"name" binding:"required"`
			Description *string `json:"description"`
			IsPublic    *bool   `json:"os-volume-type-access:is_public"`
		} `json:"volume_type" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	typeID := uuid.New().String()
	isPublic := true
	if req.VolumeType.IsPublic != nil {
		isPublic = *req.VolumeType.IsPublic
	}

	_, err := svc.activeDB().Exec(c.Request.Context(), `
		INSERT INTO volume_types (id, name, description, is_public)
		VALUES ($1, $2, $3, $4)
	`, typeID, req.VolumeType.Name, req.VolumeType.Description, isPublic)

	if err != nil {
		log.Error().Err(err).Str("operation", "create_volume_type").Msg("failed to insert volume type")
		common.SendError(c, common.NewInternalServerError("failed to create volume type"))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"volume_type": map[string]interface{}{
			"id":                               typeID,
			"name":                             req.VolumeType.Name,
			"description":                      req.VolumeType.Description,
			"os-volume-type-access:is_public":  isPublic,
			"extra_specs":                      map[string]string{},
		},
	})
}

// UpdateVolumeType updates a volume type
func (svc *Service) UpdateVolumeType(c *gin.Context) {
	typeID := c.Param("id")

	var req struct {
		VolumeType struct {
			Name        *string `json:"name"`
			Description *string `json:"description"`
			IsPublic    *bool   `json:"os-volume-type-access:is_public"`
		} `json:"volume_type" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Check if type exists
	var exists bool
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM volume_types WHERE id = $1)",
		typeID,
	).Scan(&exists)

	if err != nil || !exists {
		common.SendError(c, common.NewNotFoundError("volume type"))
		return
	}

	// Build update query
	updates := []string{}
	args := []interface{}{}
	argPos := 1

	if req.VolumeType.Name != nil {
		updates = append(updates, fmt.Sprintf("name = $%d", argPos))
		args = append(args, *req.VolumeType.Name)
		argPos++
	}

	if req.VolumeType.Description != nil {
		updates = append(updates, fmt.Sprintf("description = $%d", argPos))
		args = append(args, *req.VolumeType.Description)
		argPos++
	}

	if req.VolumeType.IsPublic != nil {
		updates = append(updates, fmt.Sprintf("is_public = $%d", argPos))
		args = append(args, *req.VolumeType.IsPublic)
		argPos++
	}

	if len(updates) == 0 {
		common.SendError(c, common.NewBadRequestError("no fields to update"))
		return
	}

	args = append(args, typeID)
	query := fmt.Sprintf("UPDATE volume_types SET %s WHERE id = $%d",
		strings.Join(updates, ", "), argPos)

	_, err = svc.activeDB().Exec(c.Request.Context(), query, args...)

	if err != nil {
		log.Error().Err(err).Str("operation", "update_volume_type").Msg("failed to update volume type")
		common.SendError(c, common.NewInternalServerError("failed to update volume type"))
		return
	}

	// Return updated type
	svc.GetVolumeType(c)
}

// DeleteVolumeType deletes a volume type
func (svc *Service) DeleteVolumeType(c *gin.Context) {
	typeID := c.Param("id")

	result, err := svc.activeDB().Exec(c.Request.Context(),
		"DELETE FROM volume_types WHERE id = $1",
		typeID,
	)

	if err != nil {
		log.Error().Err(err).Str("operation", "delete_volume_type").Msg("failed to delete volume type")
		common.SendError(c, common.NewInternalServerError("failed to delete volume type"))
		return
	}

	if result.RowsAffected() == 0 {
		common.SendError(c, common.NewNotFoundError("volume type"))
		return
	}

	c.Status(http.StatusAccepted)
}

// ListVolumeTypeExtraSpecs lists extra specs for a volume type
func (svc *Service) ListVolumeTypeExtraSpecs(c *gin.Context) {
	typeID := c.Param("id")

	// Check if type exists
	var extraSpecs map[string]string
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT COALESCE(extra_specs, '{}'::jsonb) FROM volume_types WHERE id = $1",
		typeID,
	).Scan(&extraSpecs)

	if err == pgx.ErrNoRows {
		common.SendError(c, common.NewNotFoundError("volume type"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "list_extra_specs").Msg("failed to query extra specs")
		common.SendError(c, common.NewInternalServerError("failed to list extra specs"))
		return
	}

	if extraSpecs == nil {
		extraSpecs = make(map[string]string)
	}

	c.JSON(http.StatusOK, gin.H{"extra_specs": extraSpecs})
}

// CreateVolumeTypeExtraSpecs handles POST /v3/:project_id/types/:id/extra_specs
func (svc *Service) CreateVolumeTypeExtraSpecs(c *gin.Context) {
	typeID := c.Param("id")

	var req struct {
		ExtraSpecs map[string]string `json:"extra_specs" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Convert to JSONB
	extraSpecsJSON, err := json.Marshal(req.ExtraSpecs)
	if err != nil {
		log.Error().Err(err).Str("operation", "create_extra_specs").Msg("failed to marshal extra specs")
		common.SendError(c, common.NewInternalServerError("failed to create extra specs"))
		return
	}

	_, err = svc.activeDB().Exec(c.Request.Context(),
		"UPDATE volume_types SET extra_specs = $1 WHERE id = $2",
		extraSpecsJSON, typeID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "create_extra_specs").Msg("failed to update extra specs")
		common.SendError(c, common.NewInternalServerError("failed to create extra specs"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"extra_specs": req.ExtraSpecs})
}

// GetVolumeTypeExtraSpecKey handles GET /v3/:project_id/types/:id/extra_specs/:key
func (svc *Service) GetVolumeTypeExtraSpecKey(c *gin.Context) {
	typeID := c.Param("id")
	key := c.Param("key")

	var extraSpecs map[string]string
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT COALESCE(extra_specs, '{}'::jsonb) FROM volume_types WHERE id = $1",
		typeID,
	).Scan(&extraSpecs)

	if err == pgx.ErrNoRows {
		common.SendError(c, common.NewNotFoundError("volume type"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "get_extra_spec_key").Msg("failed to query extra specs")
		common.SendError(c, common.NewInternalServerError("failed to get extra spec"))
		return
	}

	value, ok := extraSpecs[key]
	if !ok {
		common.SendError(c, common.NewNotFoundError("extra spec key"))
		return
	}

	c.JSON(http.StatusOK, gin.H{key: value})
}

// UpdateVolumeTypeExtraSpecKey handles PUT /v3/:project_id/types/:id/extra_specs/:key
func (svc *Service) UpdateVolumeTypeExtraSpecKey(c *gin.Context) {
	typeID := c.Param("id")
	key := c.Param("key")

	var req map[string]string
	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	value, ok := req[key]
	if !ok {
		common.SendError(c, common.NewBadRequestError("key not found in request body"))
		return
	}

	// Get existing extra specs
	var extraSpecs map[string]string
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT COALESCE(extra_specs, '{}'::jsonb) FROM volume_types WHERE id = $1",
		typeID,
	).Scan(&extraSpecs)

	if err == pgx.ErrNoRows {
		common.SendError(c, common.NewNotFoundError("volume type"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "update_extra_spec_key").Msg("failed to query extra specs")
		common.SendError(c, common.NewInternalServerError("failed to update extra spec"))
		return
	}

	if extraSpecs == nil {
		extraSpecs = make(map[string]string)
	}

	// Update the key
	extraSpecs[key] = value

	// Save back to database
	extraSpecsJSON, err := json.Marshal(extraSpecs)
	if err != nil {
		log.Error().Err(err).Str("operation", "update_extra_spec_key").Msg("failed to marshal extra specs")
		common.SendError(c, common.NewInternalServerError("failed to update extra spec"))
		return
	}

	_, err = svc.activeDB().Exec(c.Request.Context(),
		"UPDATE volume_types SET extra_specs = $1 WHERE id = $2",
		extraSpecsJSON, typeID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "update_extra_spec_key").Msg("failed to save extra specs")
		common.SendError(c, common.NewInternalServerError("failed to update extra spec"))
		return
	}

	c.JSON(http.StatusOK, gin.H{key: value})
}

// DeleteVolumeTypeExtraSpecKey handles DELETE /v3/:project_id/types/:id/extra_specs/:key
func (svc *Service) DeleteVolumeTypeExtraSpecKey(c *gin.Context) {
	typeID := c.Param("id")
	key := c.Param("key")

	// Get existing extra specs
	var extraSpecs map[string]string
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT COALESCE(extra_specs, '{}'::jsonb) FROM volume_types WHERE id = $1",
		typeID,
	).Scan(&extraSpecs)

	if err == pgx.ErrNoRows {
		common.SendError(c, common.NewNotFoundError("volume type"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "delete_extra_spec_key").Msg("failed to query extra specs")
		common.SendError(c, common.NewInternalServerError("failed to delete extra spec"))
		return
	}

	if extraSpecs == nil {
		extraSpecs = make(map[string]string)
	}

	// Check if key exists
	if _, ok := extraSpecs[key]; !ok {
		common.SendError(c, common.NewNotFoundError("extra spec key"))
		return
	}

	// Delete the key
	delete(extraSpecs, key)

	// Save back to database
	extraSpecsJSON, err := json.Marshal(extraSpecs)
	if err != nil {
		log.Error().Err(err).Str("operation", "delete_extra_spec_key").Msg("failed to marshal extra specs")
		common.SendError(c, common.NewInternalServerError("failed to delete extra spec"))
		return
	}

	_, err = svc.activeDB().Exec(c.Request.Context(),
		"UPDATE volume_types SET extra_specs = $1 WHERE id = $2",
		extraSpecsJSON, typeID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "delete_extra_spec_key").Msg("failed to save extra specs")
		common.SendError(c, common.NewInternalServerError("failed to delete extra spec"))
		return
	}

	c.Status(http.StatusNoContent)
}
