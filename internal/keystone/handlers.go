package keystone

import (
	"fmt"
	"net/http"
	"strings"

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
		v3.POST("/users", svc.CreateUser)
		v3.GET("/users/:id", svc.GetUser)
		v3.PATCH("/users/:id", svc.UpdateUser)
		v3.DELETE("/users/:id", svc.DeleteUser)
		v3.POST("/users/:id/password", svc.ChangePassword)
		v3.GET("/users/:id/projects", svc.GetUserProjects)
		v3.GET("/users/:id/groups", svc.GetUserGroups)

		// Projects
		v3.GET("/projects", svc.ListProjects)
		v3.GET("/projects/:id", svc.GetProject)

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

	// Always use "Default" domain for now (TODO: support multiple domains)
	var domainID string
	domainErr := database.DB.QueryRow(c.Request.Context(),
		"SELECT id FROM domains WHERE name = $1",
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
		`SELECT DISTINCT p.id, p.name, p.description, p.enabled
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
		var id, name, description string
		var enabled bool
		if err := rows.Scan(&id, &name, &description, &enabled); err != nil {
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
			"domain_id":   "default",
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
