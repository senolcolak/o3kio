package placement

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Service handles Placement API endpoints (stub implementation)
type Service struct{}

// NewService creates a new Placement service
func NewService() *Service {
	return &Service{}
}

// RegisterRoutes registers Placement routes
func (svc *Service) RegisterRoutes(r *gin.RouterGroup) {
	// Version discovery (no auth required)
	r.GET("/", svc.GetVersions)

	// Root versions endpoint
	r.GET("/v1", svc.GetVersion)

	// Placement API v1.0 endpoints (minimal stub for Horizon compatibility)
	// These endpoints return empty results - full implementation not required for basic Horizon functionality
	r.GET("/resource_providers", svc.ListResourceProviders)
	r.GET("/resource_classes", svc.ListResourceClasses)
	r.GET("/traits", svc.ListTraits)
}

// GetVersions returns the root version discovery response
func (svc *Service) GetVersions(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"versions": []gin.H{
			{
				"id":      "v1.0",
				"status":  "CURRENT",
				"min_version": "1.0",
				"max_version": "1.39",
				"links": []gin.H{
					{
						"rel":  "self",
						"href": "http://o3k:8778/",
					},
				},
			},
		},
	})
}

// GetVersion returns v1 version information
func (svc *Service) GetVersion(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"version": gin.H{
			"id":      "v1.0",
			"status":  "CURRENT",
			"min_version": "1.0",
			"max_version": "1.39",
			"links": []gin.H{
				{
					"rel":  "self",
					"href": "http://o3k:8778/",
				},
			},
		},
	})
}

// ListResourceProviders returns empty list (stub)
func (svc *Service) ListResourceProviders(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"resource_providers": []gin.H{},
	})
}

// ListResourceClasses returns empty list (stub)
func (svc *Service) ListResourceClasses(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"resource_classes": []gin.H{},
	})
}

// ListTraits returns empty list (stub)
func (svc *Service) ListTraits(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"traits": []gin.H{},
	})
}
