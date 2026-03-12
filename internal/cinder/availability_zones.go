package cinder

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ListAvailabilityZones lists availability zones for Cinder
func (svc *Service) ListAvailabilityZones(c *gin.Context) {
	// In stub mode, return a single default availability zone
	// In real mode, would query actual storage backend zones

	zones := []gin.H{
		{
			"zoneName": "nova",
			"zoneState": gin.H{
				"available": true,
			},
		},
	}

	c.JSON(http.StatusOK, gin.H{"availabilityZoneInfo": zones})
}
