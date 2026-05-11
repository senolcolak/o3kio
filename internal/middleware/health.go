package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/cobaltcore-dev/o3k/internal/database"
)

// RegisterHealthRoutes adds /healthz and /readyz to a Gin router.
// These endpoints bypass authentication and should be registered before auth
// middleware is applied (or on a router that omits auth for these paths).
func RegisterHealthRoutes(r *gin.Engine) {
	r.GET("/healthz", livenessHandler)
	r.GET("/readyz", readinessHandler)
}

// livenessHandler always returns 200. Used by Kubernetes liveness probes to
// determine whether the process is alive and should not be restarted.
func livenessHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// readinessHandler pings the database. Returns 200 when the service is ready
// to accept traffic, 503 when the database is unreachable.
func readinessHandler(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	if err := database.HealthCheck(ctx); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "unavailable",
			"reason": "database unreachable",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
