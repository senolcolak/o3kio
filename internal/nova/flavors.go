package nova

import (
	"net/http"

	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// CreateFlavor handles POST /v2.1/flavors
func (svc *Service) CreateFlavor(c *gin.Context) {
	var req struct {
		Flavor struct {
			Name      string `json:"name" binding:"required"`
			RAM       int    `json:"ram" binding:"required"`
			VCPUs     int    `json:"vcpus" binding:"required"`
			Disk      int    `json:"disk" binding:"required"`
			Ephemeral int    `json:"OS-FLV-EXT-DATA:ephemeral"`
			IsPublic  *bool  `json:"os-flavor-access:is_public"`
		} `json:"flavor" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"badRequest": gin.H{
			"message": err.Error(),
			"code":    400,
		}})
		return
	}

	flavorID := uuid.New().String()

	// Default to public flavor if not specified
	isPublic := true
	if req.Flavor.IsPublic != nil {
		isPublic = *req.Flavor.IsPublic
	}

	_, err := database.DB.Exec(c.Request.Context(),
		`INSERT INTO flavors (id, name, vcpus, ram_mb, disk_gb, is_public)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		flavorID, req.Flavor.Name, req.Flavor.VCPUs, req.Flavor.RAM, req.Flavor.Disk, isPublic,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"flavor": gin.H{
			"id":                         flavorID,
			"name":                       req.Flavor.Name,
			"vcpus":                      req.Flavor.VCPUs,
			"ram":                        req.Flavor.RAM,
			"disk":                       req.Flavor.Disk,
			"OS-FLV-EXT-DATA:ephemeral":  req.Flavor.Ephemeral,
			"OS-FLV-DISABLED:disabled":   false,
			"os-flavor-access:is_public": isPublic,
			"rxtx_factor":                1.0,
			"swap":                       "",
		},
	})
}

// DeleteFlavor handles DELETE /v2.1/flavors/:id
func (svc *Service) DeleteFlavor(c *gin.Context) {
	flavorID := c.Param("id")

	result, err := database.DB.Exec(c.Request.Context(),
		"DELETE FROM flavors WHERE id = $1",
		flavorID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"itemNotFound": gin.H{
			"message": "Flavor not found",
			"code":    404,
		}})
		return
	}

	c.Status(http.StatusNoContent)
}

// GetFlavorExtraSpecs handles GET /v2.1/flavors/:id/os-extra_specs
func (svc *Service) GetFlavorExtraSpecs(c *gin.Context) {
	flavorID := c.Param("id")

	rows, err := database.DB.Query(c.Request.Context(),
		"SELECT key, value FROM flavor_extra_specs WHERE flavor_id = $1",
		flavorID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	extraSpecs := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			continue
		}
		extraSpecs[key] = value
	}

	c.JSON(http.StatusOK, gin.H{"extra_specs": extraSpecs})
}

// CreateFlavorExtraSpecs handles POST /v2.1/flavors/:id/os-extra_specs
func (svc *Service) CreateFlavorExtraSpecs(c *gin.Context) {
	flavorID := c.Param("id")

	var req struct {
		ExtraSpecs map[string]string `json:"extra_specs" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"badRequest": gin.H{
			"message": err.Error(),
			"code":    400,
		}})
		return
	}

	// Insert or update extra specs
	for key, value := range req.ExtraSpecs {
		_, err := database.DB.Exec(c.Request.Context(),
			`INSERT INTO flavor_extra_specs (flavor_id, key, value)
			 VALUES ($1, $2, $3)
			 ON CONFLICT (flavor_id, key) DO UPDATE SET value = $3`,
			flavorID, key, value,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"extra_specs": req.ExtraSpecs})
}
