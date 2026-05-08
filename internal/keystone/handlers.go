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
	"github.com/cobaltcore-dev/o3k/pkg/cache"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

// Service handles Keystone API endpoints
type Service struct {
	authService *AuthService
	cache       *cache.Cache
	db          database.DBIF
}

// NewService creates a new Keystone service
func NewService(authService *AuthService, cacheInstance *cache.Cache) *Service {
	return &Service{
		authService: authService,
		cache:       cacheInstance,
	}
}

// NewServiceWithDB creates a Keystone service with an injected DB for testing.
func NewServiceWithDB(db database.DBIF, authService *AuthService, cacheInstance *cache.Cache) *Service {
	svc := NewService(authService, cacheInstance)
	svc.db = db
	return svc
}

// activeDB returns the injected DB or falls back to the global.
func (svc *Service) activeDB() database.DBIF {
	if svc.db != nil {
		return svc.db
	}
	return database.DB
}

// RegisterRoutes registers Keystone routes.
// adminMiddleware is applied to all destructive/admin write operations; pass nil to skip.
func (svc *Service) RegisterRoutes(r *gin.RouterGroup, adminMiddleware ...gin.HandlerFunc) {
	v3 := r.Group("/v3")
	{
		// Version discovery (unauthenticated — AuthMiddleware skips /v3)
		v3.GET("", svc.GetVersion)

		// Authentication (unauthenticated — AuthMiddleware skips POST /v3/auth/tokens)
		v3.POST("/auth/tokens", svc.AuthenticateToken)
		v3.GET("/auth/tokens", svc.ValidateToken)
		v3.DELETE("/auth/tokens", svc.RevokeToken)
		v3.GET("/auth/projects", svc.ListAuthProjects)

		// Users — read (any authenticated user)
		v3.GET("/users", svc.ListUsers)
		v3.GET("/users/:id", svc.GetUser)
		v3.POST("/users/:id/password", svc.ChangePassword)
		v3.GET("/users/:id/projects", svc.GetUserProjects)
		v3.GET("/users/:id/groups", svc.GetUserGroups)
		v3.GET("/users/:id/application_credentials", svc.ListApplicationCredentials)
		v3.GET("/users/:id/application_credentials/:cred_id", svc.GetApplicationCredential)
		v3.POST("/users/:id/application_credentials", svc.CreateApplicationCredential)
		v3.DELETE("/users/:id/application_credentials/:cred_id", svc.DeleteApplicationCredential)

		// Projects — read (any authenticated user)
		v3.GET("/projects", svc.ListProjects)
		v3.GET("/projects/:id", svc.GetProject)
		v3.GET("/projects/:id/users/:user_id/roles", svc.ListUserProjectRoles)
		v3.GET("/projects/:id/users/:user_id/roles/:role_id", svc.CheckRoleAssignment)

		// Groups — read (any authenticated user)
		v3.GET("/groups", svc.ListGroups)
		v3.GET("/groups/:id", svc.GetGroup)
		v3.GET("/groups/:id/users", svc.ListGroupUsers)

		// Roles — read (any authenticated user)
		v3.GET("/roles", svc.ListRoles)
		v3.GET("/roles/:id", svc.GetRole)

		// Role Assignments — read (any authenticated user)
		v3.GET("/role_assignments", svc.ListRoleAssignments)

		// Domains — read (any authenticated user)
		v3.GET("/domains", svc.ListDomains)
		v3.GET("/domains/:id", svc.GetDomain)
		v3.GET("/domains/:id/config", svc.GetDomainConfig)

		// Services (catalog) — read (any authenticated user)
		v3.GET("/services", svc.ListServices)
		v3.GET("/services/:id", svc.GetService)

		// Endpoints — read (any authenticated user)
		v3.GET("/endpoints", svc.ListEndpoints)

		// Credentials — read (any authenticated user)
		v3.GET("/credentials", svc.ListCredentials)
		v3.GET("/credentials/:id", svc.GetCredential)

		// Application Credentials (lookup by ID without user_id) — read
		v3.GET("/application_credentials/:id", svc.GetApplicationCredentialByID)
	}

	// Admin-only write operations
	admin := r.Group("/v3", adminMiddleware...)
	{
		// Users — write
		admin.POST("/users", svc.CreateUser)
		admin.PATCH("/users/:id", svc.UpdateUser)
		admin.DELETE("/users/:id", svc.DeleteUser)

		// Projects — write
		admin.POST("/projects", svc.CreateProject)
		admin.PATCH("/projects/:id", svc.UpdateProject)
		admin.DELETE("/projects/:id", svc.DeleteProject)
		admin.PUT("/projects/:id/users/:user_id/roles/:role_id", svc.AssignRole)
		admin.DELETE("/projects/:id/users/:user_id/roles/:role_id", svc.UnassignRole)

		// Groups — write
		admin.POST("/groups", svc.CreateGroup)
		admin.PATCH("/groups/:id", svc.UpdateGroup)
		admin.DELETE("/groups/:id", svc.DeleteGroup)
		admin.PUT("/groups/:id/users/:user_id", svc.AddUserToGroup)
		admin.DELETE("/groups/:id/users/:user_id", svc.RemoveUserFromGroup)

		// Roles — write
		admin.POST("/roles", svc.CreateRole)
		admin.PATCH("/roles/:id", svc.UpdateRole)
		admin.DELETE("/roles/:id", svc.DeleteRole)

		// Domains — write
		admin.POST("/domains", svc.CreateDomain)
		admin.PATCH("/domains/:id", svc.UpdateDomain)
		admin.DELETE("/domains/:id", svc.DeleteDomain)

		// Services (catalog) — write
		admin.POST("/services", svc.CreateService)
		admin.PATCH("/services/:id", svc.UpdateService)
		admin.DELETE("/services/:id", svc.DeleteService)

		// Endpoints — write
		admin.POST("/endpoints", svc.CreateEndpoint)
		admin.DELETE("/endpoints/:id", svc.DeleteEndpoint)

		// Credentials — write
		admin.POST("/credentials", svc.CreateCredential)
		admin.PATCH("/credentials/:id", svc.UpdateCredential)
		admin.DELETE("/credentials/:id", svc.DeleteCredential)

		// Policies — CRUD
		admin.GET("/policies", svc.ListPolicies)
		admin.POST("/policies", svc.CreatePolicy)
		admin.GET("/policies/:policy_id", svc.GetPolicy)
		admin.PATCH("/policies/:policy_id", svc.UpdatePolicy)
		admin.DELETE("/policies/:policy_id", svc.DeletePolicy)
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
					"href": fmt.Sprintf("%s/v3", common.BaseURL(c, 35357)),
				},
			},
			"media-types": []gin.H{
				{
					"base": "application/json",
					"type": "application/vnd.openstack.identity-v3+json",
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
		common.SendError(c, common.NewBadRequestError("invalid request body: "+err.Error()))
		return
	}

	// Determine authentication method
	var resp *AuthResponse
	var tokenString string
	var err error

	if req.Auth.Identity.Token != nil && req.Auth.Identity.Token.ID != "" {
		// Token-based authentication (re-scoping)
		resp, tokenString, err = svc.authService.AuthenticateToken(c.Request.Context(), &req)
	} else if req.Auth.Identity.ApplicationCredential != nil {
		// Application credential authentication
		var unrestricted bool
		resp, tokenString, unrestricted, err = svc.authService.AuthenticateApplicationCredential(c.Request.Context(), &req)
		if err == nil {
			c.Set("auth_method", "application_credential")
			c.Set("app_credential_unrestricted", unrestricted)
		}
	} else if req.Auth.Identity.Password != nil {
		// Password-based authentication
		resp, tokenString, err = svc.authService.AuthenticatePassword(c.Request.Context(), &req)
	} else {
		common.SendError(c, common.NewBadRequestError("password, token, or application_credential authentication required"))
		return
	}

	if err != nil {
		if osErr, ok := err.(*common.OpenStackError); ok {
			common.SendError(c, osErr)
			return
		}
		log.Error().Err(err).Str("operation", "authenticate").Msg("Authentication failed")
		common.SendError(c, common.NewInternalServerError("authentication failed"))
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
		common.SendError(c, common.NewBadRequestError("X-Subject-Token header required"))
		return
	}

	claims, err := svc.authService.ValidateToken(tokenString)
	if err != nil {
		if osErr, ok := err.(*common.OpenStackError); ok {
			common.SendError(c, osErr)
			return
		}
		common.SendError(c, common.NewUnauthorizedError("invalid token"))
		return
	}

	// Build full token response (same structure as POST /v3/auth/tokens)
	tokenResp := gin.H{
		"methods":    []string{"token"},
		"expires_at": claims.ExpiresAt.Time.Format(time.RFC3339),
		"issued_at":  claims.IssuedAt.Time.Format(time.RFC3339),
	}

	// Query user's domain
	var userDomainName, userDomainID string
	err = svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT d.id, d.name FROM users u JOIN domains d ON u.domain_id = d.id WHERE u.id = $1",
		claims.UserID,
	).Scan(&userDomainID, &userDomainName)
	if err != nil {
		userDomainID = "default"
		userDomainName = "Default"
	}

	tokenResp["user"] = gin.H{
		"id":   claims.UserID,
		"name": claims.UserName,
		"domain": gin.H{
			"id":   userDomainID,
			"name": userDomainName,
		},
	}

	// Add project, roles, and catalog if scoped
	if claims.ProjectID != "" {
		var projectName, projectDomainID, projectDomainName string
		err = svc.activeDB().QueryRow(c.Request.Context(),
			"SELECT p.name, p.domain_id, d.name FROM projects p JOIN domains d ON p.domain_id = d.id WHERE p.id = $1",
			claims.ProjectID,
		).Scan(&projectName, &projectDomainID, &projectDomainName)
		if err != nil {
			projectName = claims.ProjectID
			projectDomainID = "default"
			projectDomainName = "Default"
		}

		tokenResp["project"] = gin.H{
			"id":        claims.ProjectID,
			"name":      projectName,
			"is_domain": false,
			"domain": gin.H{
				"id":   projectDomainID,
				"name": projectDomainName,
			},
		}

		// Fetch role IDs from database
		var roles []gin.H
		for _, roleName := range claims.Roles {
			var roleID string
			err = svc.activeDB().QueryRow(c.Request.Context(),
				"SELECT id FROM roles WHERE name = $1",
				roleName,
			).Scan(&roleID)
			if err != nil {
				roleID = roleName // fallback to name as ID
			}
			roles = append(roles, gin.H{
				"id":   roleID,
				"name": roleName,
			})
		}
		if roles == nil {
			roles = []gin.H{}
		}
		tokenResp["roles"] = roles

		// Add service catalog
		tokenResp["catalog"] = svc.authService.BuildServiceCatalog(claims.ProjectID, svc.cache)
	} else {
		tokenResp["roles"] = []gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{"token": tokenResp})
}

// RevokeToken revokes a token (DELETE /v3/auth/tokens)
func (svc *Service) RevokeToken(c *gin.Context) {
	tokenString := c.GetHeader("X-Subject-Token")
	if tokenString == "" {
		tokenString = c.GetHeader("X-Auth-Token")
	}
	if tokenString == "" {
		common.SendError(c, common.NewBadRequestError("missing token to revoke"))
		return
	}

	// Parse the token to get its expiry (so revocation entry can auto-expire)
	claims, err := svc.authService.ValidateToken(tokenString)
	if err != nil {
		// Token already invalid, still return 204 per OpenStack behavior
		c.Status(http.StatusNoContent)
		return
	}

	// Authorization check: only the token owner or an admin can revoke
	requestingUserID := c.GetString("user_id")
	isAdmin, _ := c.Get("is_admin")
	if claims.UserID != requestingUserID && isAdmin != true {
		common.SendError(c, common.NewForbiddenError("only the token owner or an admin can revoke this token"))
		return
	}

	expiresAt := claims.ExpiresAt.Time
	svc.authService.RevokeToken(tokenString, expiresAt)
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
		common.SendError(c, common.NewUnauthorizedError("authentication required"))
		return
	}

	claims, err := svc.authService.ValidateToken(tokenString)
	if err != nil {
		common.SendError(c, common.NewUnauthorizedError("invalid token"))
		return
	}

	// Query projects the user has access to
	rows, err := svc.activeDB().Query(c.Request.Context(), `
		SELECT DISTINCT p.id, p.name, p.description, p.enabled, p.domain_id, d.name as domain_name
		FROM projects p
		JOIN role_assignments ra ON ra.project_id = p.id
		JOIN domains d ON p.domain_id = d.id
		WHERE ra.user_id = $1 AND p.enabled = true
		ORDER BY p.name
	`, claims.UserID)

	if err != nil {
		log.Error().Err(err).Str("operation", "list_auth_projects").Str("user_id", claims.UserID).Msg("Failed to query auth projects")
		common.SendError(c, common.NewInternalServerError("failed to query projects"))
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

	if err := rows.Err(); err != nil {
		log.Error().Err(err).Str("operation", "list_auth_projects").Msg("row iteration error")
		common.SendError(c, common.NewInternalServerError("failed to read projects"))
		return
	}

	if projects == nil {
		projects = []gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{"projects": projects})
}

// ListUsers lists all users (admin sees all, non-admin sees only themselves)
func (svc *Service) ListUsers(c *gin.Context) {
	isAdmin, _ := c.Get("is_admin")
	requestingUserID := c.GetString("user_id")

	ctx := c.Request.Context()
	var rows pgx.Rows
	var err error

	if isAdmin == true {
		query := "SELECT id, name, enabled, domain_id FROM users WHERE 1=1"
		args := []any{}
		argIdx := 1

		if name := c.Query("name"); name != "" {
			query += fmt.Sprintf(" AND name = $%d", argIdx)
			args = append(args, name)
			argIdx++
		}
		if enabledStr := c.Query("enabled"); enabledStr != "" {
			query += fmt.Sprintf(" AND enabled = $%d", argIdx)
			args = append(args, enabledStr == "true")
			argIdx++
		}
		if domainID := c.Query("domain_id"); domainID != "" {
			query += fmt.Sprintf(" AND domain_id = $%d", argIdx)
			args = append(args, domainID)
			argIdx++
		}
		_ = argIdx // suppress unused warning if no filters added

		query += " ORDER BY name"
		rows, err = svc.activeDB().Query(ctx, query, args...)
	} else {
		rows, err = svc.activeDB().Query(ctx,
			"SELECT id, name, enabled, domain_id FROM users WHERE id = $1",
			requestingUserID,
		)
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "list_users").Msg("Failed to query users")
		common.SendError(c, common.NewInternalServerError("failed to query users"))
		return
	}
	defer rows.Close()

	var users []gin.H
	for rows.Next() {
		var id, name, domainID string
		var enabled bool
		if err := rows.Scan(&id, &name, &enabled, &domainID); err != nil {
			log.Error().Err(err).Str("operation", "list_users_scan").Msg("Failed to scan user row")
			common.SendError(c, common.NewInternalServerError("failed to read users"))
			return
		}
		users = append(users, gin.H{
			"id":        id,
			"name":      name,
			"enabled":   enabled,
			"domain_id": domainID,
		})
	}

	if err := rows.Err(); err != nil {
		log.Error().Err(err).Str("operation", "list_users").Msg("row iteration error")
		common.SendError(c, common.NewInternalServerError("failed to read users"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"users": users})
}

// GetUser returns a single user (non-admin can only view their own record)
func (svc *Service) GetUser(c *gin.Context) {
	isAdmin := c.GetBool("is_admin")
	callerID := c.GetString("user_id")
	requestedID := c.Param("id")

	if !isAdmin && callerID != requestedID {
		common.SendError(c, common.NewForbiddenError("insufficient privileges"))
		return
	}

	userID := requestedID

	var id, name, domainID string
	var enabled bool
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT id, name, enabled, domain_id FROM users WHERE id = $1",
		userID,
	).Scan(&id, &name, &enabled, &domainID)

	if err != nil {
		common.HandleDatabaseError(c, err, "user")
		return
	}

	c.JSON(http.StatusOK, gin.H{"user": gin.H{
		"id":        id,
		"name":      name,
		"enabled":   enabled,
		"domain_id": domainID,
	}})
}

// ListProjects lists projects (admin sees all, non-admin sees only assigned projects)
func (svc *Service) ListProjects(c *gin.Context) {
	isAdmin := c.GetBool("is_admin")
	userID := c.GetString("user_id")
	ctx := c.Request.Context()

	var rows pgx.Rows
	var err error

	if isAdmin {
		query := "SELECT id, name, description, enabled, domain_id FROM projects WHERE 1=1"
		args := []any{}
		argIdx := 1

		if name := c.Query("name"); name != "" {
			query += fmt.Sprintf(" AND name = $%d", argIdx)
			args = append(args, name)
			argIdx++
		}
		if enabledStr := c.Query("enabled"); enabledStr != "" {
			query += fmt.Sprintf(" AND enabled = $%d", argIdx)
			args = append(args, enabledStr == "true")
			argIdx++
		}
		if domainID := c.Query("domain_id"); domainID != "" {
			query += fmt.Sprintf(" AND domain_id = $%d", argIdx)
			args = append(args, domainID)
			argIdx++
		}
		_ = argIdx // suppress unused warning if no filters added

		query += " ORDER BY name"
		rows, err = svc.activeDB().Query(ctx, query, args...)
	} else {
		rows, err = svc.activeDB().Query(ctx,
			`SELECT DISTINCT p.id, p.name, p.description, p.enabled, p.domain_id
			 FROM projects p
			 JOIN role_assignments ra ON ra.project_id = p.id
			 WHERE ra.user_id = $1
			 ORDER BY p.name`, userID)
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "list_projects").Msg("Failed to query projects")
		common.SendError(c, common.NewInternalServerError("failed to query projects"))
		return
	}
	defer rows.Close()

	var projects []gin.H
	for rows.Next() {
		var id, name, description, domainID string
		var enabled bool
		if err := rows.Scan(&id, &name, &description, &enabled, &domainID); err != nil {
			log.Error().Err(err).Str("operation", "list_projects_scan").Msg("Failed to scan project row")
			common.SendError(c, common.NewInternalServerError("failed to read projects"))
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

	if err := rows.Err(); err != nil {
		log.Error().Err(err).Str("operation", "list_projects").Msg("row iteration error")
		common.SendError(c, common.NewInternalServerError("failed to read projects"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"projects": projects})
}

// GetProject returns a single project
func (svc *Service) GetProject(c *gin.Context) {
	projectID := c.Param("id")

	isAdmin := c.GetBool("is_admin")
	callerProjectID := c.GetString("project_id")

	// Non-admin users can only see the project scoped in their token.
	if !isAdmin && projectID != callerProjectID {
		common.SendError(c, common.NewNotFoundError("project"))
		return
	}

	var id, name, description, domainID string
	var enabled bool
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT id, name, description, enabled, domain_id FROM projects WHERE id = $1",
		projectID,
	).Scan(&id, &name, &description, &enabled, &domainID)

	if err != nil {
		common.HandleDatabaseError(c, err, "project")
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
		common.SendError(c, common.NewBadRequestError("invalid request body"))
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
	_, err := svc.activeDB().Exec(c.Request.Context(),
		`INSERT INTO projects (id, name, description, domain_id, enabled, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		projectID, req.Project.Name, req.Project.Description, domainID, enabled, now, now,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "create_project").Msg("Failed to create project")
		common.SendError(c, common.NewInternalServerError("failed to create project"))
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
		common.SendError(c, common.NewBadRequestError("invalid request body"))
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
		common.SendError(c, common.NewBadRequestError("no fields to update"))
		return
	}

	updates = append(updates, "updated_at = NOW()")
	args = append(args, projectID)

	query := fmt.Sprintf("UPDATE projects SET %s WHERE id = $%d", joinUpdates(updates), argIdx)
	_, err := svc.activeDB().Exec(c.Request.Context(), query, args...)
	if err != nil {
		log.Error().Err(err).Str("operation", "update_project").Str("project_id", projectID).Msg("Failed to update project")
		common.SendError(c, common.NewInternalServerError("failed to update project"))
		return
	}

	// Fetch updated project
	var name, description, domainID string
	var enabled bool
	err = svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT name, description, domain_id, enabled FROM projects WHERE id = $1",
		projectID,
	).Scan(&name, &description, &domainID, &enabled)

	if err != nil {
		common.HandleDatabaseError(c, err, "project")
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

	result, err := svc.activeDB().Exec(c.Request.Context(),
		"DELETE FROM projects WHERE id = $1",
		projectID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "delete_project").Str("project_id", projectID).Msg("Failed to delete project")
		common.SendError(c, common.NewInternalServerError("failed to delete project"))
		return
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		common.SendError(c, common.NewNotFoundError("project"))
		return
	}

	c.Status(http.StatusNoContent)
}

// ListRoles lists all roles
func (svc *Service) ListRoles(c *gin.Context) {
	rows, err := svc.activeDB().Query(c.Request.Context(),
		"SELECT id, name FROM roles ORDER BY name",
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "list_roles").Msg("Failed to query roles")
		common.SendError(c, common.NewInternalServerError("failed to query roles"))
		return
	}
	defer rows.Close()

	var roles []gin.H
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			log.Error().Err(err).Str("operation", "list_roles_scan").Msg("Failed to scan role row")
			common.SendError(c, common.NewInternalServerError("failed to read roles"))
			return
		}
		roles = append(roles, gin.H{
			"id":   id,
			"name": name,
		})
	}

	if err := rows.Err(); err != nil {
		log.Error().Err(err).Str("operation", "list_roles").Msg("row iteration error")
		common.SendError(c, common.NewInternalServerError("failed to read roles"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"roles": roles})
}

// GetRole handles GET /v3/roles/:id
func (svc *Service) GetRole(c *gin.Context) {
	roleID := c.Param("id")

	var id, name string
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT id, name FROM roles WHERE id = $1",
		roleID,
	).Scan(&id, &name)

	if err != nil {
		common.HandleDatabaseError(c, err, "role")
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
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Generate new role ID
	roleID := uuid.New().String()
	now := time.Now()

	// Insert into database (domain_id support can be added later via migration)
	_, err := svc.activeDB().Exec(c.Request.Context(),
		`INSERT INTO roles (id, name, created_at)
		 VALUES ($1, $2, $3)`,
		roleID, req.Role.Name, now,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "create_role").Msg("Failed to create role")
		common.SendError(c, common.NewInternalServerError("failed to create role"))
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
		common.SendError(c, common.NewBadRequestError("invalid request body"))
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
		common.SendError(c, common.NewBadRequestError("no fields to update"))
		return
	}

	args = append(args, roleID)

	query := fmt.Sprintf("UPDATE roles SET %s WHERE id = $%d", joinUpdates(updates), argIdx)
	_, err := svc.activeDB().Exec(c.Request.Context(), query, args...)
	if err != nil {
		log.Error().Err(err).Str("operation", "update_role").Str("role_id", roleID).Msg("Failed to update role")
		common.SendError(c, common.NewInternalServerError("failed to update role"))
		return
	}

	// Fetch updated role
	var name string
	err = svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT name FROM roles WHERE id = $1",
		roleID,
	).Scan(&name)

	if err != nil {
		common.HandleDatabaseError(c, err, "role")
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

	result, err := svc.activeDB().Exec(c.Request.Context(),
		"DELETE FROM roles WHERE id = $1",
		roleID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "delete_role").Str("role_id", roleID).Msg("Failed to delete role")
		common.SendError(c, common.NewInternalServerError("failed to delete role"))
		return
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		common.SendError(c, common.NewNotFoundError("role"))
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
	_, err := svc.activeDB().Exec(c.Request.Context(),
		`INSERT INTO role_assignments (id, user_id, project_id, role_id, created_at)
		 VALUES (gen_random_uuid(), $1, $2, $3, NOW())
		 ON CONFLICT (user_id, project_id, role_id) DO NOTHING`,
		userID, projectID, roleID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "assign_role").Str("project_id", projectID).Str("user_id", userID).Str("role_id", roleID).Msg("Failed to assign role")
		common.SendError(c, common.NewInternalServerError("failed to assign role"))
		return
	}

	c.Status(http.StatusNoContent)
}

// UnassignRole handles DELETE /v3/projects/:id/users/:user_id/roles/:role_id
func (svc *Service) UnassignRole(c *gin.Context) {
	projectID := c.Param("id")
	userID := c.Param("user_id")
	roleID := c.Param("role_id")

	result, err := svc.activeDB().Exec(c.Request.Context(),
		"DELETE FROM role_assignments WHERE user_id = $1 AND project_id = $2 AND role_id = $3",
		userID, projectID, roleID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "unassign_role").Msg("Failed to unassign role")
		common.SendError(c, common.NewInternalServerError("failed to unassign role"))
		return
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		common.SendError(c, common.NewNotFoundError("role assignment"))
		return
	}

	c.Status(http.StatusNoContent)
}

// ListUserProjectRoles handles GET /v3/projects/:id/users/:user_id/roles
func (svc *Service) ListUserProjectRoles(c *gin.Context) {
	projectID := c.Param("id")
	userID := c.Param("user_id")

	rows, err := svc.activeDB().Query(c.Request.Context(),
		`SELECT r.id, r.name
		 FROM roles r
		 INNER JOIN role_assignments ra ON r.id = ra.role_id
		 WHERE ra.user_id = $1 AND ra.project_id = $2`,
		userID, projectID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "list_user_project_roles").Msg("Failed to query user project roles")
		common.SendError(c, common.NewInternalServerError("failed to query roles"))
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

	if err := rows.Err(); err != nil {
		log.Error().Err(err).Str("operation", "list_user_project_roles").Msg("row iteration error")
		common.SendError(c, common.NewInternalServerError("failed to read roles"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"roles": roles})
}

// CheckRoleAssignment handles GET /v3/projects/:id/users/:user_id/roles/:role_id
func (svc *Service) CheckRoleAssignment(c *gin.Context) {
	projectID := c.Param("id")
	userID := c.Param("user_id")
	roleID := c.Param("role_id")

	var exists bool
	err := svc.activeDB().QueryRow(c.Request.Context(),
		`SELECT EXISTS(
			SELECT 1 FROM role_assignments
			WHERE user_id = $1 AND project_id = $2 AND role_id = $3
		)`,
		userID, projectID, roleID,
	).Scan(&exists)

	if err != nil {
		log.Error().Err(err).Str("operation", "check_role_assignment").Msg("Failed to check role assignment")
		common.SendError(c, common.NewInternalServerError("failed to check role assignment"))
		return
	}

	if !exists {
		common.SendError(c, common.NewNotFoundError("role assignment"))
		return
	}

	c.Status(http.StatusNoContent)
}

// ListRoleAssignments handles GET /v3/role_assignments
func (svc *Service) ListRoleAssignments(c *gin.Context) {
	isAdmin := c.GetBool("is_admin")
	callerID := c.GetString("user_id")

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

	// Non-admin users can only see their own role assignments
	if !isAdmin {
		query += fmt.Sprintf(" AND ra.user_id = $%d", argIdx)
		args = append(args, callerID)
		argIdx++
	}

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

	rows, err := svc.activeDB().Query(c.Request.Context(), query, args...)
	if err != nil {
		log.Error().Err(err).Str("operation", "list_role_assignments").Msg("Failed to query role assignments")
		common.SendError(c, common.NewInternalServerError("failed to query role assignments"))
		return
	}
	defer rows.Close()

	var assignments []gin.H
	for rows.Next() {
		var uid, pid, rid, rname string
		if err := rows.Scan(&uid, &pid, &rid, &rname); err != nil {
			log.Warn().Err(err).Msg("failed to scan role assignment row")
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

	if err := rows.Err(); err != nil {
		log.Error().Err(err).Str("operation", "list_role_assignments").Msg("row iteration error")
		common.SendError(c, common.NewInternalServerError("failed to read role assignments"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"role_assignments": assignments})
}

// CreateUser creates a new user (POST /v3/users)
func (svc *Service) CreateUser(c *gin.Context) {
	var req struct {
		User struct {
			Name        string `json:"name" binding:"required"`
			Password    string `json:"password"`
			Email       string `json:"email"`
			Description string `json:"description"`
			Enabled     *bool  `json:"enabled"`
			DomainID    string `json:"domain_id"`
		} `json:"user" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body: "+err.Error()))
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
		domainErr := svc.activeDB().QueryRow(c.Request.Context(),
			"SELECT id FROM domains WHERE id = $1 AND enabled = true",
			req.User.DomainID,
		).Scan(&domainID)
		if domainErr != nil {
			common.SendError(c, common.NewBadRequestError("specified domain not found or disabled"))
			return
		}
	} else {
		// Default to "Default" domain
		domainErr := svc.activeDB().QueryRow(c.Request.Context(),
			"SELECT id FROM domains WHERE name = $1 AND enabled = true",
			"Default",
		).Scan(&domainID)
		if domainErr != nil {
			log.Error().Err(domainErr).Str("operation", "create_user_resolve_domain").Msg("Failed to resolve default domain")
			common.SendError(c, common.NewInternalServerError("failed to resolve default domain"))
			return
		}
	}

	// Hash password if provided
	var passwordHash string
	if req.User.Password != "" {
		hashedBytes, err := svc.authService.HashPassword(req.User.Password)
		if err != nil {
			log.Error().Err(err).Str("operation", "create_user_hash_password").Msg("Failed to hash password")
			common.SendError(c, common.NewInternalServerError("failed to hash password"))
			return
		}
		passwordHash = string(hashedBytes)
	}

	// Insert user into database
	var userID, userName, userEmail, userDescription string
	var userEnabled bool
	var userDomainID *string

	err := svc.activeDB().QueryRow(c.Request.Context(),
		`INSERT INTO users (name, password_hash, email, description, enabled, domain_id)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, name, email, description, enabled, domain_id`,
		req.User.Name, passwordHash, req.User.Email, req.User.Description, enabled, domainID,
	).Scan(&userID, &userName, &userEmail, &userDescription, &userEnabled, &userDomainID)

	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") && strings.Contains(err.Error(), "users_name_key") {
			common.SendError(c, common.NewConflictError("user with name '"+req.User.Name+"' already exists"))
			return
		}
		log.Error().Err(err).Str("operation", "create_user").Msg("Failed to create user")
		common.SendError(c, common.NewInternalServerError("failed to create user"))
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
		common.SendError(c, common.NewBadRequestError("invalid request body: "+err.Error()))
		return
	}

	// Check if user exists
	var exists bool
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)",
		userID,
	).Scan(&exists)

	if err != nil || !exists {
		common.SendError(c, common.NewNotFoundError("user"))
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
			log.Error().Err(err).Str("operation", "update_user_hash_password").Msg("Failed to hash password")
			common.SendError(c, common.NewInternalServerError("failed to hash password"))
			return
		}
		updates = append(updates, "password_hash = $"+fmt.Sprint(argIndex))
		args = append(args, string(hashedBytes))
		argIndex++
	}

	if len(updates) == 0 {
		common.SendError(c, common.NewBadRequestError("no fields to update"))
		return
	}

	// Add updated_at timestamp
	updates = append(updates, "updated_at = CURRENT_TIMESTAMP")

	// Execute update
	query := "UPDATE users SET " + strings.Join(updates, ", ") + " WHERE id = $1"
	_, err = svc.activeDB().Exec(c.Request.Context(), query, args...)

	if err != nil {
		log.Error().Err(err).Str("operation", "update_user").Str("user_id", userID).Msg("Failed to update user")
		common.SendError(c, common.NewInternalServerError("failed to update user"))
		return
	}

	// Fetch updated user
	var id, name, email, description string
	var enabled bool
	var domainID *string

	err = svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT id, name, email, description, enabled, domain_id FROM users WHERE id = $1",
		userID,
	).Scan(&id, &name, &email, &description, &enabled, &domainID)

	if err != nil {
		log.Error().Err(err).Str("operation", "update_user_fetch").Str("user_id", userID).Msg("Failed to fetch updated user")
		common.SendError(c, common.NewInternalServerError("failed to fetch updated user"))
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
	result, err := svc.activeDB().Exec(c.Request.Context(),
		"DELETE FROM users WHERE id = $1",
		userID,
	)

	if err != nil {
		log.Error().Err(err).Str("operation", "delete_user").Str("user_id", userID).Msg("Failed to delete user")
		common.SendError(c, common.NewInternalServerError("failed to delete user"))
		return
	}

	// Check if user was actually deleted
	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		common.SendError(c, common.NewNotFoundError("user"))
		return
	}

	c.Status(http.StatusNoContent)
}

// ChangePassword changes a user's password (POST /v3/users/:id/password)
func (svc *Service) ChangePassword(c *gin.Context) {
	userID := c.Param("id")

	callerID := c.GetString("user_id")
	if callerID != userID {
		roles, _ := c.Get("roles")
		roleList, _ := roles.([]string)
		isAdmin := false
		for _, r := range roleList {
			if r == "admin" {
				isAdmin = true
				break
			}
		}
		if !isAdmin {
			common.SendError(c, common.NewForbiddenError("cannot change another user's password"))
			return
		}
	}

	var req struct {
		User struct {
			OriginalPassword string `json:"original_password"`
			Password         string `json:"password" binding:"required"`
		} `json:"user" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body: "+err.Error()))
		return
	}

	// Fetch current password hash
	var currentPasswordHash string
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT password_hash FROM users WHERE id = $1",
		userID,
	).Scan(&currentPasswordHash)

	if err != nil {
		common.HandleDatabaseError(c, err, "user")
		return
	}

	// Verify original password only for self-service changes
	if callerID == userID {
		if !svc.authService.CheckPassword(req.User.OriginalPassword, currentPasswordHash) {
			common.SendError(c, common.NewUnauthorizedError("original password is incorrect"))
			return
		}
	}

	// Hash new password
	hashedBytes, err := svc.authService.HashPassword(req.User.Password)
	if err != nil {
		log.Error().Err(err).Str("operation", "change_password_hash").Msg("Failed to hash password")
		common.SendError(c, common.NewInternalServerError("failed to hash password"))
		return
	}

	// Update password
	_, err = svc.activeDB().Exec(c.Request.Context(),
		"UPDATE users SET password_hash = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2",
		string(hashedBytes), userID,
	)

	if err != nil {
		log.Error().Err(err).Str("operation", "change_password_update").Str("user_id", userID).Msg("Failed to update password")
		common.SendError(c, common.NewInternalServerError("failed to update password"))
		return
	}

	c.Status(http.StatusNoContent)
}

// GetUserProjects returns projects for a user (GET /v3/users/:id/projects)
func (svc *Service) GetUserProjects(c *gin.Context) {
	userID := c.Param("id")

	// Check if user exists
	var exists bool
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)",
		userID,
	).Scan(&exists)

	if err != nil || !exists {
		common.SendError(c, common.NewNotFoundError("user"))
		return
	}

	// Fetch projects via role_assignments
	rows, err := svc.activeDB().Query(c.Request.Context(),
		`SELECT DISTINCT p.id, p.name, p.description, p.enabled, p.domain_id
		 FROM projects p
		 INNER JOIN role_assignments ra ON ra.project_id = p.id
		 WHERE ra.user_id = $1
		 ORDER BY p.name`,
		userID,
	)

	if err != nil {
		log.Error().Err(err).Str("operation", "get_user_projects").Str("user_id", userID).Msg("Failed to fetch user projects")
		common.SendError(c, common.NewInternalServerError("failed to fetch user projects"))
		return
	}
	defer rows.Close()

	var projects []gin.H
	for rows.Next() {
		var id, name, description, domainID string
		var enabled bool
		if err := rows.Scan(&id, &name, &description, &enabled, &domainID); err != nil {
			log.Error().Err(err).Str("operation", "get_user_projects_scan").Msg("Failed to scan project row")
			common.SendError(c, common.NewInternalServerError("failed to read project data"))
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

	if err := rows.Err(); err != nil {
		log.Error().Err(err).Str("operation", "get_user_projects").Msg("row iteration error")
		common.SendError(c, common.NewInternalServerError("failed to read projects"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"projects": projects})
}

// GetUserGroups returns groups for a user (GET /v3/users/:id/groups)
func (svc *Service) GetUserGroups(c *gin.Context) {
	userID := c.Param("id")

	// Check if user exists
	var exists bool
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)",
		userID,
	).Scan(&exists)

	if err != nil || !exists {
		common.SendError(c, common.NewNotFoundError("user"))
		return
	}

	// Fetch groups via group_members
	rows, err := svc.activeDB().Query(c.Request.Context(),
		`SELECT g.id, g.name, g.description, g.domain_id
		 FROM groups g
		 INNER JOIN group_members gm ON gm.group_id = g.id
		 WHERE gm.user_id = $1
		 ORDER BY g.name`,
		userID,
	)

	if err != nil {
		log.Error().Err(err).Str("operation", "get_user_groups").Str("user_id", userID).Msg("Failed to fetch user groups")
		common.SendError(c, common.NewInternalServerError("failed to fetch user groups"))
		return
	}
	defer rows.Close()

	var groups []gin.H
	for rows.Next() {
		var id, name, description, domainID string
		if err := rows.Scan(&id, &name, &description, &domainID); err != nil {
			log.Error().Err(err).Str("operation", "get_user_groups_scan").Msg("Failed to scan group row")
			common.SendError(c, common.NewInternalServerError("failed to read group data"))
			return
		}
		groups = append(groups, gin.H{
			"id":          id,
			"name":        name,
			"description": description,
			"domain_id":   domainID,
		})
	}

	if err := rows.Err(); err != nil {
		log.Error().Err(err).Str("operation", "get_user_groups").Msg("row iteration error")
		common.SendError(c, common.NewInternalServerError("failed to read groups"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"groups": groups})
}
