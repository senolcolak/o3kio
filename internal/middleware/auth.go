package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/cobaltcore-dev/o3k/internal/keystone"
)

// AuthMiddleware validates OpenStack tokens
func AuthMiddleware(authService *keystone.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip auth for version discovery endpoints
		if strings.HasSuffix(c.Request.URL.Path, "/v3") ||
			strings.HasSuffix(c.Request.URL.Path, "/v2.1") ||
			strings.HasSuffix(c.Request.URL.Path, "/v2.0") ||
			c.Request.URL.Path == "/" {
			c.Next()
			return
		}

		// Skip auth for token issuance endpoint
		if c.Request.Method == "POST" && strings.HasSuffix(c.Request.URL.Path, "/auth/tokens") {
			c.Next()
			return
		}

		// Get token from X-Auth-Token header
		token := c.GetHeader("X-Auth-Token")
		if token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{
				"message": "authentication required",
				"code":    401,
				"title":   "Unauthorized",
			}})
			c.Abort()
			return
		}

		// Validate token
		claims, err := authService.ValidateToken(token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{
				"message": "invalid or expired token",
				"code":    401,
				"title":   "Unauthorized",
			}})
			c.Abort()
			return
		}

		// Store claims in context
		c.Set("user_id", claims.UserID)
		c.Set("user_name", claims.UserName)
		c.Set("project_id", claims.ProjectID)
		c.Set("roles", claims.Roles)

		c.Next()
	}
}

// RequireProjectScope ensures the token is project-scoped
func RequireProjectScope() gin.HandlerFunc {
	return func(c *gin.Context) {
		projectID, exists := c.Get("project_id")
		if !exists || projectID == "" {
			c.JSON(http.StatusForbidden, gin.H{"error": gin.H{
				"message": "project-scoped token required",
				"code":    403,
				"title":   "Forbidden",
			}})
			c.Abort()
			return
		}
		c.Next()
	}
}

// RequireRole ensures the user has a specific role
func RequireRole(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		roles, exists := c.Get("roles")
		if !exists {
			c.JSON(http.StatusForbidden, gin.H{"error": gin.H{
				"message": "insufficient privileges",
				"code":    403,
				"title":   "Forbidden",
			}})
			c.Abort()
			return
		}

		roleList := roles.([]string)
		for _, r := range roleList {
			if r == role || r == "admin" {
				c.Next()
				return
			}
		}

		c.JSON(http.StatusForbidden, gin.H{"error": gin.H{
			"message": "insufficient privileges",
			"code":    403,
			"title":   "Forbidden",
		}})
		c.Abort()
	}
}
