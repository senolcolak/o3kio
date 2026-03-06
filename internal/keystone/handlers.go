package keystone

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/cobaltcore-dev/o3k/internal/common"
	"github.com/cobaltcore-dev/o3k/internal/database"
)

// Service handles Keystone API endpoints
type Service struct {
	authService *AuthService
}

// NewService creates a new Keystone service
func NewService(authService *AuthService) *Service {
	return &Service{
		authService: authService,
	}
}

// RegisterRoutes registers Keystone routes
func (svc *Service) RegisterRoutes(r *gin.RouterGroup) {
	v3 := r.Group("/v3")
	{
		// Version discovery
		v3.GET("", svc.GetVersion)

		// Authentication
		v3.POST("/auth/tokens", svc.AuthenticateToken)
		v3.GET("/auth/tokens", svc.ValidateToken)
		v3.DELETE("/auth/tokens", svc.RevokeToken)

		// Users
		v3.GET("/users", svc.ListUsers)
		v3.GET("/users/:id", svc.GetUser)

		// Projects
		v3.GET("/projects", svc.ListProjects)
		v3.GET("/projects/:id", svc.GetProject)

		// Roles
		v3.GET("/roles", svc.ListRoles)
	}
}

// GetVersion returns Keystone API version info
func (svc *Service) GetVersion(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"version": gin.H{
			"id":     "v3.14",
			"status": "stable",
			"links": []gin.H{
				{
					"rel":  "self",
					"href": "http://localhost:5000/v3",
				},
			},
			"media-types": []gin.H{
				{
					"base":      "application/json",
					"type":      "application/vnd.openstack.identity-v3+json",
				},
			},
		},
	})
}

// AuthenticateToken handles token authentication (POST /v3/auth/tokens)
func (svc *Service) AuthenticateToken(c *gin.Context) {
	var req AuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"message": "invalid request body",
			"code":    400,
			"title":   "Bad Request",
		}})
		return
	}

	// Handle password authentication
	resp, tokenString, err := svc.authService.AuthenticatePassword(c.Request.Context(), &req)
	if err != nil {
		if osErr, ok := err.(*common.OpenStackError); ok {
			c.JSON(osErr.StatusCode, gin.H{"error": gin.H{
				"message": osErr.Message,
				"code":    osErr.StatusCode,
				"title":   osErr.Code,
			}})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"message": err.Error(),
			"code":    500,
			"title":   "Internal Server Error",
		}})
		return
	}

	// Return token in X-Subject-Token header
	c.Header("X-Subject-Token", tokenString)
	c.JSON(http.StatusCreated, resp)
}

// ValidateToken validates a token (GET /v3/auth/tokens)
func (svc *Service) ValidateToken(c *gin.Context) {
	tokenString := c.GetHeader("X-Subject-Token")
	if tokenString == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"message": "X-Subject-Token header required",
			"code":    400,
			"title":   "Bad Request",
		}})
		return
	}

	claims, err := svc.authService.ValidateToken(tokenString)
	if err != nil {
		if osErr, ok := err.(*common.OpenStackError); ok {
			c.JSON(osErr.StatusCode, gin.H{"error": gin.H{
				"message": osErr.Message,
				"code":    osErr.StatusCode,
				"title":   osErr.Code,
			}})
			return
		}
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{
			"message": "invalid token",
			"code":    401,
			"title":   "Unauthorized",
		}})
		return
	}

	// Return token info
	c.JSON(http.StatusOK, gin.H{
		"token": gin.H{
			"user": gin.H{
				"id":   claims.UserID,
				"name": claims.UserName,
			},
			"project_id": claims.ProjectID,
			"roles":      claims.Roles,
		},
	})
}

// RevokeToken revokes a token (DELETE /v3/auth/tokens)
func (svc *Service) RevokeToken(c *gin.Context) {
	// In JWT implementation, we don't maintain token blacklist
	// Tokens expire naturally based on TTL
	c.Status(http.StatusNoContent)
}

// ListUsers lists all users
func (svc *Service) ListUsers(c *gin.Context) {
	rows, err := database.DB.Query(c.Request.Context(),
		"SELECT id, name, enabled FROM users ORDER BY name",
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var users []gin.H
	for rows.Next() {
		var id, name string
		var enabled bool
		if err := rows.Scan(&id, &name, &enabled); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		users = append(users, gin.H{
			"id":      id,
			"name":    name,
			"enabled": enabled,
			"domain_id": "default",
		})
	}

	c.JSON(http.StatusOK, gin.H{"users": users})
}

// GetUser returns a single user
func (svc *Service) GetUser(c *gin.Context) {
	userID := c.Param("id")

	var id, name string
	var enabled bool
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT id, name, enabled FROM users WHERE id = $1",
		userID,
	).Scan(&id, &name, &enabled)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{
			"message": "user not found",
			"code":    404,
			"title":   "Not Found",
		}})
		return
	}

	c.JSON(http.StatusOK, gin.H{"user": gin.H{
		"id":      id,
		"name":    name,
		"enabled": enabled,
		"domain_id": "default",
	}})
}

// ListProjects lists all projects
func (svc *Service) ListProjects(c *gin.Context) {
	rows, err := database.DB.Query(c.Request.Context(),
		"SELECT id, name, description, enabled FROM projects ORDER BY name",
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var projects []gin.H
	for rows.Next() {
		var id, name, description string
		var enabled bool
		if err := rows.Scan(&id, &name, &description, &enabled); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		projects = append(projects, gin.H{
			"id":          id,
			"name":        name,
			"description": description,
			"enabled":     enabled,
			"domain_id":   "default",
		})
	}

	c.JSON(http.StatusOK, gin.H{"projects": projects})
}

// GetProject returns a single project
func (svc *Service) GetProject(c *gin.Context) {
	projectID := c.Param("id")

	var id, name, description string
	var enabled bool
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT id, name, description, enabled FROM projects WHERE id = $1",
		projectID,
	).Scan(&id, &name, &description, &enabled)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{
			"message": "project not found",
			"code":    404,
			"title":   "Not Found",
		}})
		return
	}

	c.JSON(http.StatusOK, gin.H{"project": gin.H{
		"id":          id,
		"name":        name,
		"description": description,
		"enabled":     enabled,
		"domain_id":   "default",
	}})
}

// ListRoles lists all roles
func (svc *Service) ListRoles(c *gin.Context) {
	rows, err := database.DB.Query(c.Request.Context(),
		"SELECT id, name FROM roles ORDER BY name",
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var roles []gin.H
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		roles = append(roles, gin.H{
			"id":   id,
			"name": name,
		})
	}

	c.JSON(http.StatusOK, gin.H{"roles": roles})
}
