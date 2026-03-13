package keystone

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/common"
	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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
		v3.GET("/auth/projects", svc.ListAuthProjects)

		// Users
		v3.GET("/users", svc.ListUsers)
		v3.POST("/users", svc.CreateUser)
		v3.GET("/users/:id", svc.GetUser)
		v3.PATCH("/users/:id", svc.UpdateUser)
		v3.DELETE("/users/:id", svc.DeleteUser)
		v3.POST("/users/:id/password", svc.ChangePassword)
		v3.GET("/users/:id/projects", svc.GetUserProjects)
		v3.GET("/users/:id/groups", svc.GetUserGroups)
		v3.POST("/users/:id/application_credentials", svc.CreateApplicationCredential)
		v3.GET("/users/:id/application_credentials", svc.ListApplicationCredentials)
		v3.GET("/users/:id/application_credentials/:cred_id", svc.GetApplicationCredential)
		v3.DELETE("/users/:id/application_credentials/:cred_id", svc.DeleteApplicationCredential)

		// Projects (role assignments must come before /projects/:id to avoid conflicts)
		v3.GET("/projects", svc.ListProjects)
		v3.POST("/projects", svc.CreateProject)
		v3.PUT("/projects/:id/users/:user_id/roles/:role_id", svc.AssignRole)
		v3.DELETE("/projects/:id/users/:user_id/roles/:role_id", svc.UnassignRole)
		v3.GET("/projects/:id/users/:user_id/roles", svc.ListUserProjectRoles)
		v3.GET("/projects/:id/users/:user_id/roles/:role_id", svc.CheckRoleAssignment)
		v3.GET("/projects/:id", svc.GetProject)
		v3.PATCH("/projects/:id", svc.UpdateProject)
		v3.DELETE("/projects/:id", svc.DeleteProject)

		// Groups
		v3.GET("/groups", svc.ListGroups)
		v3.POST("/groups", svc.CreateGroup)
		v3.GET("/groups/:id", svc.GetGroup)
		v3.PATCH("/groups/:id", svc.UpdateGroup)
		v3.DELETE("/groups/:id", svc.DeleteGroup)
		v3.GET("/groups/:id/users", svc.ListGroupUsers)
		v3.PUT("/groups/:id/users/:user_id", svc.AddUserToGroup)
		v3.DELETE("/groups/:id/users/:user_id", svc.RemoveUserFromGroup)

		// Roles
		v3.GET("/roles", svc.ListRoles)
		v3.POST("/roles", svc.CreateRole)
		v3.GET("/roles/:id", svc.GetRole)
		v3.PATCH("/roles/:id", svc.UpdateRole)
		v3.DELETE("/roles/:id", svc.DeleteRole)

		// Role Assignments (additional endpoint)
		v3.GET("/role_assignments", svc.ListRoleAssignments)

		// Domains
		v3.GET("/domains", svc.ListDomains)
		v3.POST("/domains", svc.CreateDomain)
		v3.GET("/domains/:id", svc.GetDomain)
		v3.PATCH("/domains/:id", svc.UpdateDomain)
		v3.DELETE("/domains/:id", svc.DeleteDomain)
		v3.GET("/domains/:id/config", svc.GetDomainConfig)

		// Services (catalog management)
		v3.GET("/services", svc.ListServices)
		v3.POST("/services", svc.CreateService)
		v3.GET("/services/:id", svc.GetService)
		v3.PATCH("/services/:id", svc.UpdateService)
		v3.DELETE("/services/:id", svc.DeleteService)

		// Endpoints
		v3.GET("/endpoints", svc.ListEndpoints)
		v3.POST("/endpoints", svc.CreateEndpoint)
		v3.DELETE("/endpoints/:id", svc.DeleteEndpoint)

		// Credentials
		v3.GET("/credentials", svc.ListCredentials)
		v3.POST("/credentials", svc.CreateCredential)
		v3.GET("/credentials/:id", svc.GetCredential)
		v3.PATCH("/credentials/:id", svc.UpdateCredential)
		v3.DELETE("/credentials/:id", svc.DeleteCredential)

		// Application Credentials (lookup by ID without user_id)
		v3.GET("/application_credentials/:id", svc.GetApplicationCredentialByID)
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
					"href": "http://localhost:35357/v3",
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
	// Read request body for debug logging
	bodyBytes, _ := io.ReadAll(c.Request.Body)
	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	var req AuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Log parse error with request body
		c.Error(fmt.Errorf("auth parse failed: %v, body: %s", err, string(bodyBytes)))
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"message": "invalid request body: " + err.Error(),
			"code":    400,
			"title":   "Bad Request",
		}})
		return
	}

	// Determine authentication method
	var resp *AuthResponse
	var tokenString string
	var err error

	if req.Auth.Identity.Token != nil && req.Auth.Identity.Token.ID != "" {
		// Token-based authentication (re-scoping)
		resp, tokenString, err = svc.authService.AuthenticateToken(c.Request.Context(), &req)
	} else if req.Auth.Identity.Password != nil {
		// Password-based authentication
		resp, tokenString, err = svc.authService.AuthenticatePassword(c.Request.Context(), &req)
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"message": "password or token authentication required",
			"code":    400,
			"title":   "Bad Request",
		}})
		return
	}

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

// ListAuthProjects lists projects available to the authenticated user (GET /v3/auth/projects)
func (svc *Service) ListAuthProjects(c *gin.Context) {
	// Extract user ID from token
	tokenString := c.GetHeader("X-Auth-Token")
	if tokenString == "" {
		tokenString = c.GetHeader("X-Subject-Token")
	}
	if tokenString == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{
			"message": "authentication required",
			"code":    401,
			"title":   "Unauthorized",
		}})
		return
	}

	claims, err := svc.authService.ValidateToken(tokenString)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{
			"message": "invalid token",
			"code":    401,
			"title":   "Unauthorized",
		}})
		return
	}

	// Query projects the user has access to
	rows, err := database.DB.Query(c.Request.Context(), `
		SELECT DISTINCT p.id, p.name, p.description, p.enabled, p.domain_id, d.name as domain_name
		FROM projects p
		JOIN role_assignments ra ON ra.project_id = p.id
		JOIN domains d ON p.domain_id = d.id
		WHERE ra.user_id = $1 AND p.enabled = true
		ORDER BY p.name
	`, claims.UserID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"message": err.Error(),
			"code":    500,
			"title":   "Internal Server Error",
		}})
		return
	}
	defer rows.Close()

	var projects []gin.H
	for rows.Next() {
		var id, name, description, domainID, domainName string
		var enabled bool
		if err := rows.Scan(&id, &name, &description, &enabled, &domainID, &domainName); err != nil {
			continue
		}

		projects = append(projects, gin.H{
			"id":          id,
			"name":        name,
			"description": description,
			"enabled":     enabled,
			"domain_id":   domainID,
			"domain": gin.H{
				"id":   domainID,
				"name": domainName,
			},
		})
	}

	if projects == nil {
		projects = []gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{"projects": projects})
}

// ListUsers lists all users
func (svc *Service) ListUsers(c *gin.Context) {
	rows, err := database.DB.Query(c.Request.Context(),
		"SELECT id, name, enabled, domain_id FROM users ORDER BY name",
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var users []gin.H
	for rows.Next() {
		var id, name, domainID string
		var enabled bool
		if err := rows.Scan(&id, &name, &enabled, &domainID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		users = append(users, gin.H{
			"id":        id,
			"name":      name,
			"enabled":   enabled,
			"domain_id": domainID,
		})
	}

	c.JSON(http.StatusOK, gin.H{"users": users})
}

// GetUser returns a single user
func (svc *Service) GetUser(c *gin.Context) {
	userID := c.Param("id")

	var id, name, domainID string
	var enabled bool
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT id, name, enabled, domain_id FROM users WHERE id = $1",
		userID,
	).Scan(&id, &name, &enabled, &domainID)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{
			"message": "user not found",
			"code":    404,
			"title":   "Not Found",
		}})
		return
	}

	c.JSON(http.StatusOK, gin.H{"user": gin.H{
		"id":        id,
		"name":      name,
		"enabled":   enabled,
		"domain_id": domainID,
	}})
}

// ListProjects lists all projects
func (svc *Service) ListProjects(c *gin.Context) {
	rows, err := database.DB.Query(c.Request.Context(),
		"SELECT id, name, description, enabled, domain_id FROM projects ORDER BY name",
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var projects []gin.H
	for rows.Next() {
		var id, name, description, domainID string
		var enabled bool
		if err := rows.Scan(&id, &name, &description, &enabled, &domainID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		projects = append(projects, gin.H{
			"id":          id,
			"name":        name,
			"description": description,
			"enabled":     enabled,
			"domain_id":   domainID,
		})
	}

	c.JSON(http.StatusOK, gin.H{"projects": projects})
}

// GetProject returns a single project
func (svc *Service) GetProject(c *gin.Context) {
	projectID := c.Param("id")

	var id, name, description, domainID string
	var enabled bool
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT id, name, description, enabled, domain_id FROM projects WHERE id = $1",
		projectID,
	).Scan(&id, &name, &description, &enabled, &domainID)

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
		"domain_id":   domainID,
	}})
}

// CreateProject handles POST /v3/projects
func (svc *Service) CreateProject(c *gin.Context) {
	var req struct {
		Project struct {
			Name        string `json:"name" binding:"required"`
			Description string `json:"description"`
			DomainID    string `json:"domain_id"`
			Enabled     *bool  `json:"enabled"`
		} `json:"project" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"message": "invalid request body",
			"code":    400,
			"title":   "Bad Request",
		}})
		return
	}

	// Default domain_id if not provided
	domainID := req.Project.DomainID
	if domainID == "" {
		domainID = "00000000-0000-0000-0000-000000000100" // Default domain UUID
	}

	// Default enabled to true if not specified
	enabled := true
	if req.Project.Enabled != nil {
		enabled = *req.Project.Enabled
	}

	// Generate new project ID
	projectID := uuid.New().String()
	now := time.Now()

	// Insert into database
	_, err := database.DB.Exec(c.Request.Context(),
		`INSERT INTO projects (id, name, description, domain_id, enabled, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		projectID, req.Project.Name, req.Project.Description, domainID, enabled, now, now,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"message": err.Error(),
			"code":    500,
			"title":   "Internal Server Error",
		}})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"project": gin.H{
		"id":          projectID,
		"name":        req.Project.Name,
		"description": req.Project.Description,
		"domain_id":   domainID,
		"enabled":     enabled,
		"links": gin.H{
			"self": c.Request.Host + "/v3/projects/" + projectID,
		},
	}})
}

// UpdateProject handles PATCH /v3/projects/:id
func (svc *Service) UpdateProject(c *gin.Context) {
	projectID := c.Param("id")

	var req struct {
		Project struct {
			Name        *string `json:"name"`
			Description *string `json:"description"`
			Enabled     *bool   `json:"enabled"`
		} `json:"project" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"message": "invalid request body",
			"code":    400,
			"title":   "Bad Request",
		}})
		return
	}

	// Build dynamic update query
	updates := []string{}
	args := []interface{}{}
	argIdx := 1

	if req.Project.Name != nil {
		updates = append(updates, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *req.Project.Name)
		argIdx++
	}
	if req.Project.Description != nil {
		updates = append(updates, fmt.Sprintf("description = $%d", argIdx))
		args = append(args, *req.Project.Description)
		argIdx++
	}
	if req.Project.Enabled != nil {
		updates = append(updates, fmt.Sprintf("enabled = $%d", argIdx))
		args = append(args, *req.Project.Enabled)
		argIdx++
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"message": "No fields to update",
			"code":    400,
		}})
		return
	}

	updates = append(updates, "updated_at = NOW()")
	args = append(args, projectID)

	query := fmt.Sprintf("UPDATE projects SET %s WHERE id = $%d", joinUpdates(updates), argIdx)
	_, err := database.DB.Exec(c.Request.Context(), query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"message": err.Error(),
			"code":    500,
		}})
		return
	}

	// Fetch updated project
	var name, description, domainID string
	var enabled bool
	err = database.DB.QueryRow(c.Request.Context(),
		"SELECT name, description, domain_id, enabled FROM projects WHERE id = $1",
		projectID,
	).Scan(&name, &description, &domainID, &enabled)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{
			"message": "project not found",
			"code":    404,
		}})
		return
	}

	c.JSON(http.StatusOK, gin.H{"project": gin.H{
		"id":          projectID,
		"name":        name,
		"description": description,
		"domain_id":   domainID,
		"enabled":     enabled,
		"links": gin.H{
			"self": c.Request.Host + "/v3/projects/" + projectID,
		},
	}})
}

// DeleteProject handles DELETE /v3/projects/:id
func (svc *Service) DeleteProject(c *gin.Context) {
	projectID := c.Param("id")

	result, err := database.DB.Exec(c.Request.Context(),
		"DELETE FROM projects WHERE id = $1",
		projectID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"message": err.Error(),
			"code":    500,
		}})
		return
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{
			"message": "project not found",
			"code":    404,
			"title":   "Not Found",
		}})
		return
	}

	c.Status(http.StatusNoContent)
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

// GetRole handles GET /v3/roles/:id
func (svc *Service) GetRole(c *gin.Context) {
	roleID := c.Param("id")

	var id, name string
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT id, name FROM roles WHERE id = $1",
		roleID,
	).Scan(&id, &name)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{
			"message": "role not found",
			"code":    404,
			"title":   "Not Found",
		}})
		return
	}

	c.JSON(http.StatusOK, gin.H{"role": gin.H{
		"id":   id,
		"name": name,
		"links": gin.H{
			"self": c.Request.Host + "/v3/roles/" + id,
		},
	}})
}

// CreateRole handles POST /v3/roles
func (svc *Service) CreateRole(c *gin.Context) {
	var req struct {
		Role struct {
			Name     string `json:"name" binding:"required"`
			DomainID string `json:"domain_id"`
		} `json:"role" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"message": "invalid request body",
			"code":    400,
			"title":   "Bad Request",
		}})
		return
	}

	// Generate new role ID
	roleID := uuid.New().String()
	now := time.Now()

	// Insert into database (domain_id support can be added later via migration)
	_, err := database.DB.Exec(c.Request.Context(),
		`INSERT INTO roles (id, name, created_at)
		 VALUES ($1, $2, $3)`,
		roleID, req.Role.Name, now,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"message": err.Error(),
			"code":    500,
			"title":   "Internal Server Error",
		}})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"role": gin.H{
		"id":   roleID,
		"name": req.Role.Name,
		"links": gin.H{
			"self": c.Request.Host + "/v3/roles/" + roleID,
		},
	}})
}

// UpdateRole handles PATCH /v3/roles/:id
func (svc *Service) UpdateRole(c *gin.Context) {
	roleID := c.Param("id")

	var req struct {
		Role struct {
			Name *string `json:"name"`
		} `json:"role" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"message": "invalid request body",
			"code":    400,
			"title":   "Bad Request",
		}})
		return
	}

	// Build dynamic update query
	updates := []string{}
	args := []interface{}{}
	argIdx := 1

	if req.Role.Name != nil {
		updates = append(updates, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *req.Role.Name)
		argIdx++
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"message": "No fields to update",
			"code":    400,
		}})
		return
	}

	args = append(args, roleID)

	query := fmt.Sprintf("UPDATE roles SET %s WHERE id = $%d", joinUpdates(updates), argIdx)
	_, err := database.DB.Exec(c.Request.Context(), query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"message": err.Error(),
			"code":    500,
		}})
		return
	}

	// Fetch updated role
	var name string
	err = database.DB.QueryRow(c.Request.Context(),
		"SELECT name FROM roles WHERE id = $1",
		roleID,
	).Scan(&name)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{
			"message": "role not found",
			"code":    404,
		}})
		return
	}

	c.JSON(http.StatusOK, gin.H{"role": gin.H{
		"id":   roleID,
		"name": name,
		"links": gin.H{
			"self": c.Request.Host + "/v3/roles/" + roleID,
		},
	}})
}

// DeleteRole handles DELETE /v3/roles/:id
func (svc *Service) DeleteRole(c *gin.Context) {
	roleID := c.Param("id")

	result, err := database.DB.Exec(c.Request.Context(),
		"DELETE FROM roles WHERE id = $1",
		roleID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"message": err.Error(),
			"code":    500,
		}})
		return
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{
			"message": "role not found",
			"code":    404,
			"title":   "Not Found",
		}})
		return
	}

	c.Status(http.StatusNoContent)
}

// AssignRole handles PUT /v3/projects/:id/users/:user_id/roles/:role_id
func (svc *Service) AssignRole(c *gin.Context) {
	projectID := c.Param("id")
	userID := c.Param("user_id")
	roleID := c.Param("role_id")

	// Insert role assignment (idempotent)
	_, err := database.DB.Exec(c.Request.Context(),
		`INSERT INTO role_assignments (id, user_id, project_id, role_id, created_at)
		 VALUES (gen_random_uuid(), $1, $2, $3, NOW())
		 ON CONFLICT (user_id, project_id, role_id) DO NOTHING`,
		userID, projectID, roleID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"message": err.Error(),
			"code":    500,
		}})
		return
	}

	c.Status(http.StatusNoContent)
}

// UnassignRole handles DELETE /v3/projects/:id/users/:user_id/roles/:role_id
func (svc *Service) UnassignRole(c *gin.Context) {
	projectID := c.Param("id")
	userID := c.Param("user_id")
	roleID := c.Param("role_id")

	result, err := database.DB.Exec(c.Request.Context(),
		"DELETE FROM role_assignments WHERE user_id = $1 AND project_id = $2 AND role_id = $3",
		userID, projectID, roleID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"message": err.Error(),
			"code":    500,
		}})
		return
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{
			"message": "role assignment not found",
			"code":    404,
		}})
		return
	}

	c.Status(http.StatusNoContent)
}

// ListUserProjectRoles handles GET /v3/projects/:id/users/:user_id/roles
func (svc *Service) ListUserProjectRoles(c *gin.Context) {
	projectID := c.Param("id")
	userID := c.Param("user_id")

	rows, err := database.DB.Query(c.Request.Context(),
		`SELECT r.id, r.name
		 FROM roles r
		 INNER JOIN role_assignments ra ON r.id = ra.role_id
		 WHERE ra.user_id = $1 AND ra.project_id = $2`,
		userID, projectID,
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
			continue
		}
		roles = append(roles, gin.H{
			"id":   id,
			"name": name,
		})
	}

	if roles == nil {
		roles = []gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{"roles": roles})
}

// CheckRoleAssignment handles GET /v3/projects/:id/users/:user_id/roles/:role_id
func (svc *Service) CheckRoleAssignment(c *gin.Context) {
	projectID := c.Param("id")
	userID := c.Param("user_id")
	roleID := c.Param("role_id")

	var exists bool
	err := database.DB.QueryRow(c.Request.Context(),
		`SELECT EXISTS(
			SELECT 1 FROM role_assignments
			WHERE user_id = $1 AND project_id = $2 AND role_id = $3
		)`,
		userID, projectID, roleID,
	).Scan(&exists)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{
			"message": "role assignment not found",
			"code":    404,
		}})
		return
	}

	c.Status(http.StatusNoContent)
}

// ListRoleAssignments handles GET /v3/role_assignments
func (svc *Service) ListRoleAssignments(c *gin.Context) {
	// Parse query parameters
	userID := c.Query("user.id")
	projectID := c.Query("scope.project.id")
	roleID := c.Query("role.id")

	// Build dynamic query
	query := `SELECT ra.user_id, ra.project_id, ra.role_id, r.name as role_name
	          FROM role_assignments ra
	          INNER JOIN roles r ON ra.role_id = r.id
	          WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if userID != "" {
		query += fmt.Sprintf(" AND ra.user_id = $%d", argIdx)
		args = append(args, userID)
		argIdx++
	}
	if projectID != "" {
		query += fmt.Sprintf(" AND ra.project_id = $%d", argIdx)
		args = append(args, projectID)
		argIdx++
	}
	if roleID != "" {
		query += fmt.Sprintf(" AND ra.role_id = $%d", argIdx)
		args = append(args, roleID)
		argIdx++
	}

	rows, err := database.DB.Query(c.Request.Context(), query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var assignments []gin.H
	for rows.Next() {
		var uid, pid, rid, rname string
		if err := rows.Scan(&uid, &pid, &rid, &rname); err != nil {
			continue
		}
		assignments = append(assignments, gin.H{
			"user": gin.H{
				"id": uid,
			},
			"scope": gin.H{
				"project": gin.H{
					"id": pid,
				},
			},
			"role": gin.H{
				"id":   rid,
				"name": rname,
			},
		})
	}

	if assignments == nil {
		assignments = []gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{"role_assignments": assignments})
}

// CreateUser creates a new user (POST /v3/users)
func (svc *Service) CreateUser(c *gin.Context) {
	var req struct {
		User struct {
			Name        string  `json:"name" binding:"required"`
			Password    string  `json:"password"`
			Email       string  `json:"email"`
			Description string  `json:"description"`
			Enabled     *bool   `json:"enabled"`
			DomainID    string  `json:"domain_id"`
		} `json:"user" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"message": "invalid request body: " + err.Error(),
			"code":    400,
			"title":   "Bad Request",
		}})
		return
	}

	// Default enabled to true
	enabled := true
	if req.User.Enabled != nil {
		enabled = *req.User.Enabled
	}

	// Get domain ID from request or default to "Default" domain
	var domainID string
	if req.User.DomainID != "" {
		// User specified domain by ID - verify it exists and is enabled
		domainErr := database.DB.QueryRow(c.Request.Context(),
			"SELECT id FROM domains WHERE id = $1 AND enabled = true",
			req.User.DomainID,
		).Scan(&domainID)
		if domainErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
				"message": "specified domain not found or disabled",
				"code":    400,
				"title":   "Bad Request",
			}})
			return
		}
	} else {
		// Default to "Default" domain
		domainErr := database.DB.QueryRow(c.Request.Context(),
			"SELECT id FROM domains WHERE name = $1 AND enabled = true",
			"Default",
		).Scan(&domainID)
		if domainErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
				"message": "failed to resolve default domain: " + domainErr.Error(),
				"code":    500,
				"title":   "Internal Server Error",
			}})
			return
		}
	}

	// Hash password if provided
	var passwordHash string
	if req.User.Password != "" {
		hashedBytes, err := svc.authService.HashPassword(req.User.Password)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
				"message": "failed to hash password",
				"code":    500,
				"title":   "Internal Server Error",
			}})
			return
		}
		passwordHash = string(hashedBytes)
	}

	// Insert user into database
	var userID, userName, userEmail, userDescription string
	var userEnabled bool
	var userDomainID *string

	err := database.DB.QueryRow(c.Request.Context(),
		`INSERT INTO users (name, password_hash, email, description, enabled, domain_id)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, name, email, description, enabled, domain_id`,
		req.User.Name, passwordHash, req.User.Email, req.User.Description, enabled, domainID,
	).Scan(&userID, &userName, &userEmail, &userDescription, &userEnabled, &userDomainID)

	if err != nil {
		if err.Error() == "ERROR: duplicate key value violates unique constraint \"users_name_key\" (SQLSTATE 23505)" {
			c.JSON(http.StatusConflict, gin.H{"error": gin.H{
				"message": "user with name '" + req.User.Name + "' already exists",
				"code":    409,
				"title":   "Conflict",
			}})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"message": "failed to create user: " + err.Error(),
			"code":    500,
			"title":   "Internal Server Error",
		}})
		return
	}

	// Return created user
	responseDomainID := "default"
	if userDomainID != nil {
		responseDomainID = *userDomainID
	}

	c.JSON(http.StatusCreated, gin.H{
		"user": gin.H{
			"id":          userID,
			"name":        userName,
			"email":       userEmail,
			"description": userDescription,
			"enabled":     userEnabled,
			"domain_id":   responseDomainID,
		},
	})
}

// UpdateUser updates an existing user (PATCH /v3/users/:id)
func (svc *Service) UpdateUser(c *gin.Context) {
	userID := c.Param("id")

	var req struct {
		User struct {
			Name        *string `json:"name"`
			Email       *string `json:"email"`
			Description *string `json:"description"`
			Enabled     *bool   `json:"enabled"`
			Password    *string `json:"password"`
		} `json:"user" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"message": "invalid request body: " + err.Error(),
			"code":    400,
			"title":   "Bad Request",
		}})
		return
	}

	// Check if user exists
	var exists bool
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)",
		userID,
	).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{
			"message": "user not found",
			"code":    404,
			"title":   "Not Found",
		}})
		return
	}

	// Build dynamic UPDATE query
	updates := []string{}
	args := []interface{}{userID}
	argIndex := 2

	if req.User.Name != nil {
		updates = append(updates, "name = $"+fmt.Sprint(argIndex))
		args = append(args, *req.User.Name)
		argIndex++
	}
	if req.User.Email != nil {
		updates = append(updates, "email = $"+fmt.Sprint(argIndex))
		args = append(args, *req.User.Email)
		argIndex++
	}
	if req.User.Description != nil {
		updates = append(updates, "description = $"+fmt.Sprint(argIndex))
		args = append(args, *req.User.Description)
		argIndex++
	}
	if req.User.Enabled != nil {
		updates = append(updates, "enabled = $"+fmt.Sprint(argIndex))
		args = append(args, *req.User.Enabled)
		argIndex++
	}
	if req.User.Password != nil {
		hashedBytes, err := svc.authService.HashPassword(*req.User.Password)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
				"message": "failed to hash password",
				"code":    500,
				"title":   "Internal Server Error",
			}})
			return
		}
		updates = append(updates, "password_hash = $"+fmt.Sprint(argIndex))
		args = append(args, string(hashedBytes))
		argIndex++
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"message": "no fields to update",
			"code":    400,
			"title":   "Bad Request",
		}})
		return
	}

	// Add updated_at timestamp
	updates = append(updates, "updated_at = CURRENT_TIMESTAMP")

	// Execute update
	query := "UPDATE users SET " + strings.Join(updates, ", ") + " WHERE id = $1"
	_, err = database.DB.Exec(c.Request.Context(), query, args...)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"message": "failed to update user: " + err.Error(),
			"code":    500,
			"title":   "Internal Server Error",
		}})
		return
	}

	// Fetch updated user
	var id, name, email, description string
	var enabled bool
	var domainID *string

	err = database.DB.QueryRow(c.Request.Context(),
		"SELECT id, name, email, description, enabled, domain_id FROM users WHERE id = $1",
		userID,
	).Scan(&id, &name, &email, &description, &enabled, &domainID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"message": "failed to fetch updated user",
			"code":    500,
			"title":   "Internal Server Error",
		}})
		return
	}

	responseDomainID := "default"
	if domainID != nil {
		responseDomainID = *domainID
	}

	c.JSON(http.StatusOK, gin.H{
		"user": gin.H{
			"id":          id,
			"name":        name,
			"email":       email,
			"description": description,
			"enabled":     enabled,
			"domain_id":   responseDomainID,
		},
	})
}

// DeleteUser deletes a user (DELETE /v3/users/:id)
func (svc *Service) DeleteUser(c *gin.Context) {
	userID := c.Param("id")

	// Execute delete
	result, err := database.DB.Exec(c.Request.Context(),
		"DELETE FROM users WHERE id = $1",
		userID,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"message": "failed to delete user: " + err.Error(),
			"code":    500,
			"title":   "Internal Server Error",
		}})
		return
	}

	// Check if user was actually deleted
	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{
			"message": "user not found",
			"code":    404,
			"title":   "Not Found",
		}})
		return
	}

	c.Status(http.StatusNoContent)
}

// ChangePassword changes a user's password (POST /v3/users/:id/password)
func (svc *Service) ChangePassword(c *gin.Context) {
	userID := c.Param("id")

	var req struct {
		User struct {
			OriginalPassword string `json:"original_password" binding:"required"`
			Password         string `json:"password" binding:"required"`
		} `json:"user" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"message": "invalid request body: " + err.Error(),
			"code":    400,
			"title":   "Bad Request",
		}})
		return
	}

	// Fetch current password hash
	var currentPasswordHash string
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT password_hash FROM users WHERE id = $1",
		userID,
	).Scan(&currentPasswordHash)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{
			"message": "user not found",
			"code":    404,
			"title":   "Not Found",
		}})
		return
	}

	// Verify original password
	if !svc.authService.CheckPassword(req.User.OriginalPassword, currentPasswordHash) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{
			"message": "original password is incorrect",
			"code":    401,
			"title":   "Unauthorized",
		}})
		return
	}

	// Hash new password
	hashedBytes, err := svc.authService.HashPassword(req.User.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"message": "failed to hash password",
			"code":    500,
			"title":   "Internal Server Error",
		}})
		return
	}

	// Update password
	_, err = database.DB.Exec(c.Request.Context(),
		"UPDATE users SET password_hash = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2",
		string(hashedBytes), userID,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"message": "failed to update password: " + err.Error(),
			"code":    500,
			"title":   "Internal Server Error",
		}})
		return
	}

	c.Status(http.StatusNoContent)
}

// GetUserProjects returns projects for a user (GET /v3/users/:id/projects)
func (svc *Service) GetUserProjects(c *gin.Context) {
	userID := c.Param("id")

	// Check if user exists
	var exists bool
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)",
		userID,
	).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{
			"message": "user not found",
			"code":    404,
			"title":   "Not Found",
		}})
		return
	}

	// Fetch projects via role_assignments
	rows, err := database.DB.Query(c.Request.Context(),
		`SELECT DISTINCT p.id, p.name, p.description, p.enabled, p.domain_id
		 FROM projects p
		 INNER JOIN role_assignments ra ON ra.project_id = p.id
		 WHERE ra.user_id = $1
		 ORDER BY p.name`,
		userID,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"message": "failed to fetch user projects: " + err.Error(),
			"code":    500,
			"title":   "Internal Server Error",
		}})
		return
	}
	defer rows.Close()

	var projects []gin.H
	for rows.Next() {
		var id, name, description, domainID string
		var enabled bool
		if err := rows.Scan(&id, &name, &description, &enabled, &domainID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
				"message": "failed to scan project: " + err.Error(),
				"code":    500,
				"title":   "Internal Server Error",
			}})
			return
		}
		projects = append(projects, gin.H{
			"id":          id,
			"name":        name,
			"description": description,
			"enabled":     enabled,
			"domain_id":   domainID,
		})
	}

	c.JSON(http.StatusOK, gin.H{"projects": projects})
}

// GetUserGroups returns groups for a user (GET /v3/users/:id/groups)
func (svc *Service) GetUserGroups(c *gin.Context) {
	userID := c.Param("id")

	// Check if user exists
	var exists bool
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)",
		userID,
	).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{
			"message": "user not found",
			"code":    404,
			"title":   "Not Found",
		}})
		return
	}

	// Fetch groups via group_members
	rows, err := database.DB.Query(c.Request.Context(),
		`SELECT g.id, g.name, g.description, g.domain_id
		 FROM groups g
		 INNER JOIN group_members gm ON gm.group_id = g.id
		 WHERE gm.user_id = $1
		 ORDER BY g.name`,
		userID,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"message": "failed to fetch user groups: " + err.Error(),
			"code":    500,
			"title":   "Internal Server Error",
		}})
		return
	}
	defer rows.Close()

	var groups []gin.H
	for rows.Next() {
		var id, name, description, domainID string
		if err := rows.Scan(&id, &name, &description, &domainID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
				"message": "failed to scan group: " + err.Error(),
				"code":    500,
				"title":   "Internal Server Error",
			}})
			return
		}
		groups = append(groups, gin.H{
			"id":          id,
			"name":        name,
			"description": description,
			"domain_id":   domainID,
		})
	}

	c.JSON(http.StatusOK, gin.H{"groups": groups})
}
