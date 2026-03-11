package cinder

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	typeID := uuid.New().String()
	isPublic := true
	if req.VolumeType.IsPublic != nil {
		isPublic = *req.VolumeType.IsPublic
	}

	_, err := database.DB.Exec(c.Request.Context(), `
		INSERT INTO volume_types (id, name, description, is_public)
		VALUES ($1, $2, $3, $4)
	`, typeID, req.VolumeType.Name, req.VolumeType.Description, isPublic)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"volume_type": map[string]interface{}{
			"id":                                typeID,
			"name":                              req.VolumeType.Name,
			"description":                       req.VolumeType.Description,
			"os-volume-type-access:is_public":  isPublic,
			"extra_specs":                       map[string]string{},
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if type exists
	var exists bool
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM volume_types WHERE id = $1)",
		typeID,
	).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "volume type not found"})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "no fields to update"})
		return
	}

	args = append(args, typeID)
	query := fmt.Sprintf("UPDATE volume_types SET %s WHERE id = $%d",
		strings.Join(updates, ", "), argPos)

	_, err = database.DB.Exec(c.Request.Context(), query, args...)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Return updated type
	svc.GetVolumeType(c)
}

// DeleteVolumeType deletes a volume type
func (svc *Service) DeleteVolumeType(c *gin.Context) {
	typeID := c.Param("id")

	result, err := database.DB.Exec(c.Request.Context(),
		"DELETE FROM volume_types WHERE id = $1",
		typeID,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if result.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "volume type not found"})
		return
	}

	c.Status(http.StatusAccepted)
}

// ListVolumeTypeExtraSpecs lists extra specs for a volume type
func (svc *Service) ListVolumeTypeExtraSpecs(c *gin.Context) {
	typeID := c.Param("id")

	// Check if type exists
	var extraSpecs map[string]string
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT COALESCE(extra_specs, '{}'::jsonb) FROM volume_types WHERE id = $1",
		typeID,
	).Scan(&extraSpecs)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "volume type not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if extraSpecs == nil {
		extraSpecs = make(map[string]string)
	}

	c.JSON(http.StatusOK, gin.H{"extra_specs": extraSpecs})
}
