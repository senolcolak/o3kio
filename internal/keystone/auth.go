package keystone

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/common"
	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/cobaltcore-dev/o3k/pkg/cache"
	"github.com/golang-jwt/jwt/v5"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/bcrypt"
)

// AccessRule defines a single access restriction for an application credential
type AccessRule struct {
	Path    string `json:"path"`
	Method  string `json:"method"`
	Service string `json:"service"`
}

// TokenClaims represents JWT token claims
type TokenClaims struct {
	UserID          string       `json:"user_id"`
	UserName        string       `json:"user_name"`
	ProjectID       string       `json:"project_id,omitempty"`
	Roles           []string     `json:"roles,omitempty"`
	AccessRules     []AccessRule `json:"access_rules,omitempty"`
	IsAppCredential bool         `json:"is_app_credential,omitempty"`
	Unrestricted    bool         `json:"unrestricted,omitempty"`
	jwt.RegisteredClaims
}

// AuthService handles authentication operations
type AuthService struct {
	jwtSecret     []byte
	tokenTTL      time.Duration
	cache         *cache.Cache
	db            database.DBIF
	revokedTokens sync.Map // token hash -> expiry time
}

// NewAuthService creates a new auth service
func NewAuthService(jwtSecret string, tokenTTL time.Duration, cacheInstance *cache.Cache) *AuthService {
	svc := &AuthService{
		jwtSecret: []byte(jwtSecret),
		tokenTTL:  tokenTTL,
		cache:     cacheInstance,
	}
	svc.loadRevokedTokens()
	return svc
}

// NewAuthServiceWithDB creates an AuthService with an injected DB for testing.
func NewAuthServiceWithDB(db database.DBIF, jwtSecret string, tokenTTL time.Duration, cacheInstance *cache.Cache) *AuthService {
	svc := NewAuthService(jwtSecret, tokenTTL, cacheInstance)
	svc.db = db
	return svc
}

// loadRevokedTokens preloads non-expired revoked tokens from the DB into the in-memory map.
// Called at startup. Logs a warning and returns silently if the DB is unavailable.
func (s *AuthService) loadRevokedTokens() {
	db := s.activeDB()
	if db == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rows, err := db.Query(ctx,
		`SELECT token_hash, expires_at FROM revoked_tokens WHERE expires_at > NOW()`)
	if err != nil {
		log.Warn().Err(err).Msg("keystone: failed to preload revoked tokens; in-memory map starts empty")
		return
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var hash string
		var expiresAt time.Time
		if err := rows.Scan(&hash, &expiresAt); err != nil {
			continue
		}
		s.revokedTokens.Store(hash, expiresAt)
		count++
	}
	log.Info().Int("count", count).Msg("keystone: preloaded revoked tokens into memory")
}

// activeDB returns the injected DB or falls back to the global.
func (s *AuthService) activeDB() database.DBIF {
	if s.db != nil {
		return s.db
	}
	return database.DB
}

// AuthRequest represents an authentication request
// ScopeField handles both string ("unscoped") and object scope formats
type ScopeField struct {
	IsUnscoped bool
	Project    *struct {
		Name   string `json:"name"`
		ID     string `json:"id"`
		Domain *struct {
			Name string `json:"name"`
			ID   string `json:"id"`
		} `json:"domain,omitempty"`
	}
}

// UnmarshalJSON implements custom JSON unmarshaling for ScopeField
func (s *ScopeField) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as string first
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		s.IsUnscoped = (str == "unscoped")
		return nil
	}

	// Otherwise unmarshal as object
	var temp struct {
		Project *struct {
			Name   string `json:"name"`
			ID     string `json:"id"`
			Domain *struct {
				Name string `json:"name"`
				ID   string `json:"id"`
			} `json:"domain,omitempty"`
		} `json:"project,omitempty"`
	}
	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}
	s.Project = temp.Project
	s.IsUnscoped = false
	return nil
}

type AuthRequest struct {
	Auth struct {
		Identity struct {
			Methods  []string `json:"methods"`
			Password *struct {
				User struct {
					Name     string `json:"name"`
					Password string `json:"password"`
					Domain   *struct {
						Name string `json:"name"`
					} `json:"domain"`
				} `json:"user"`
			} `json:"password,omitempty"`
			Token *struct {
				ID string `json:"id"`
			} `json:"token,omitempty"`
			ApplicationCredential *struct {
				ID     string `json:"id"`
				Secret string `json:"secret"`
			} `json:"application_credential,omitempty"`
		} `json:"identity"`
		Scope *ScopeField `json:"scope,omitempty"`
	} `json:"auth"`
}

// AuthResponse represents an authentication response
type AuthResponse struct {
	Token struct {
		ExpiresAt string                   `json:"expires_at"`
		IssuedAt  string                   `json:"issued_at"`
		Methods   []string                 `json:"methods"`
		AuditIDs  []string                 `json:"audit_ids"`
		IsDomain  bool                     `json:"is_domain"`
		User      map[string]interface{}   `json:"user"`
		Catalog   []CatalogEntry           `json:"catalog,omitempty"`
		Project   *map[string]interface{}  `json:"project,omitempty"`
		Roles     []map[string]interface{} `json:"roles,omitempty"`
	} `json:"token"`
}

// CatalogEntry represents a service in the catalog
type CatalogEntry struct {
	Type      string     `json:"type"`
	Name      string     `json:"name"`
	Endpoints []Endpoint `json:"endpoints"`
}

// Endpoint represents a service endpoint
type Endpoint struct {
	Interface string `json:"interface"`
	RegionID  string `json:"region_id,omitempty"`
	Region    string `json:"region,omitempty"` // backwards compat
	URL       string `json:"url"`
}

// AuthenticatePassword authenticates user with password
func (s *AuthService) AuthenticatePassword(ctx context.Context, req *AuthRequest) (*AuthResponse, string, error) {
	if req.Auth.Identity.Password == nil {
		return nil, "", common.NewBadRequestError("password authentication required")
	}

	username := req.Auth.Identity.Password.User.Name
	password := req.Auth.Identity.Password.User.Password

	// Get domain name (default to "Default" if not specified)
	domainName := "Default"
	if req.Auth.Identity.Password.User.Domain != nil && req.Auth.Identity.Password.User.Domain.Name != "" {
		domainName = req.Auth.Identity.Password.User.Domain.Name
	}

	// Look up domain ID
	var domainID string
	err := s.activeDB().QueryRow(ctx,
		"SELECT id FROM domains WHERE name = $1",
		domainName,
	).Scan(&domainID)

	if errors.Is(err, database.ErrNoRows) {
		return nil, "", common.NewUnauthorizedError("invalid domain")
	}
	if err != nil {
		return nil, "", fmt.Errorf("database error looking up domain: %w", err)
	}

	// Fetch user from database (with domain filter)
	var user database.User
	err = s.activeDB().QueryRow(ctx,
		"SELECT id, name, password_hash, enabled, domain_id FROM users WHERE name = $1 AND domain_id = $2",
		username, domainID,
	).Scan(&user.ID, &user.Name, &user.PasswordHash, &user.Enabled, &user.DomainID)

	if errors.Is(err, database.ErrNoRows) {
		return nil, "", common.NewUnauthorizedError("invalid credentials")
	}
	if err != nil {
		return nil, "", fmt.Errorf("database error: %w", err)
	}

	if !user.Enabled {
		return nil, "", common.NewUnauthorizedError("user is disabled")
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, "", common.NewUnauthorizedError("invalid credentials")
	}

	// Handle scoped vs unscoped
	var projectID string
	var roles []string
	var project *database.Project

	// Check if scope is explicitly requested
	if req.Auth.Scope != nil && !req.Auth.Scope.IsUnscoped && req.Auth.Scope.Project != nil {
		// Scoped authentication
		projectName := req.Auth.Scope.Project.Name
		projectIDParam := req.Auth.Scope.Project.ID

		// Fetch project (with domain filter)
		var proj database.Project
		var query string
		var params []interface{}
		if projectIDParam != "" {
			query = "SELECT id, name, description, enabled, domain_id FROM projects WHERE id = $1 AND domain_id = $2"
			params = []interface{}{projectIDParam, domainID}
		} else {
			query = "SELECT id, name, description, enabled, domain_id FROM projects WHERE name = $1 AND domain_id = $2"
			params = []interface{}{projectName, domainID}
		}

		err := s.activeDB().QueryRow(ctx, query, params...).Scan(
			&proj.ID, &proj.Name, &proj.Description, &proj.Enabled, &proj.DomainID,
		)
		if errors.Is(err, database.ErrNoRows) {
			return nil, "", common.NewUnauthorizedError("project not found")
		}
		if err != nil {
			return nil, "", fmt.Errorf("database error: %w", err)
		}

		if !proj.Enabled {
			return nil, "", common.NewUnauthorizedError("project is disabled")
		}

		projectID = proj.ID
		project = &proj

		// Fetch roles
		rows, err := s.activeDB().Query(ctx, `
			SELECT r.name
			FROM role_assignments ra
			JOIN roles r ON ra.role_id = r.id
			WHERE ra.user_id = $1 AND ra.project_id = $2
		`, user.ID, projectID)
		if err != nil {
			return nil, "", fmt.Errorf("failed to fetch roles: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var roleName string
			if err := rows.Scan(&roleName); err != nil {
				return nil, "", fmt.Errorf("failed to scan role: %w", err)
			}
			roles = append(roles, roleName)
		}
		if err := rows.Err(); err != nil {
			return nil, "", fmt.Errorf("failed to iterate roles: %w", err)
		}

		if len(roles) == 0 {
			return nil, "", common.NewForbiddenError("user has no roles on this project")
		}
	}
	// Otherwise: unscoped token (no project, no roles)

	// Generate JWT token
	now := time.Now()
	expiresAt := now.Add(s.tokenTTL)
	claims := &TokenClaims{
		UserID:    user.ID,
		UserName:  user.Name,
		ProjectID: projectID,
		Roles:     roles,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			Subject:   user.ID,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return nil, "", fmt.Errorf("failed to sign token: %w", err)
	}

	// Build response
	resp := &AuthResponse{}
	resp.Token.ExpiresAt = expiresAt.Format(time.RFC3339)
	resp.Token.IssuedAt = now.Format(time.RFC3339)
	resp.Token.Methods = []string{"password"}
	resp.Token.AuditIDs = []string{generateAuditID()}
	resp.Token.IsDomain = false

	// Query user's domain name
	var userDomainName string
	err = s.activeDB().QueryRow(ctx,
		"SELECT name FROM domains WHERE id = $1",
		user.DomainID,
	).Scan(&userDomainName)
	if err != nil {
		userDomainName = "Default" // fallback
	}

	resp.Token.User = map[string]interface{}{
		"id":   user.ID,
		"name": user.Name,
		"domain": map[string]interface{}{
			"id":   user.DomainID,
			"name": userDomainName,
		},
	}

	// Add project and catalog if scoped
	if projectID != "" {
		// Query project's domain name
		var projectDomainName string
		err = s.activeDB().QueryRow(ctx,
			"SELECT name FROM domains WHERE id = $1",
			project.DomainID,
		).Scan(&projectDomainName)
		if err != nil {
			projectDomainName = "Default" // fallback
		}

		resp.Token.Project = &map[string]interface{}{
			"id":        project.ID,
			"name":      project.Name,
			"is_domain": false,
			"domain": map[string]interface{}{
				"id":   project.DomainID,
				"name": projectDomainName,
			},
		}

		// Add roles with proper IDs
		for _, roleName := range roles {
			var roleID string
			_ = s.activeDB().QueryRow(ctx,
				"SELECT id FROM roles WHERE name = $1", roleName,
			).Scan(&roleID)
			if roleID == "" {
				roleID = roleName // fallback
			}
			resp.Token.Roles = append(resp.Token.Roles, map[string]interface{}{
				"id":   roleID,
				"name": roleName,
			})
		}

		// Add service catalog
		resp.Token.Catalog = s.BuildServiceCatalog(projectID, s.cache)
	}

	return resp, tokenString, nil
}

// AuthenticateToken handles token-based authentication (re-scoping)
func (s *AuthService) AuthenticateToken(ctx context.Context, req *AuthRequest) (*AuthResponse, string, error) {
	if req.Auth.Identity.Token == nil || req.Auth.Identity.Token.ID == "" {
		return nil, "", common.NewBadRequestError("token authentication required")
	}

	// Validate the provided token
	claims, err := s.ValidateToken(req.Auth.Identity.Token.ID)
	if err != nil {
		return nil, "", err
	}

	// Fetch user from database
	var user database.User
	err = s.activeDB().QueryRow(ctx,
		"SELECT id, name, password_hash, enabled, domain_id FROM users WHERE id = $1",
		claims.UserID,
	).Scan(&user.ID, &user.Name, &user.PasswordHash, &user.Enabled, &user.DomainID)

	if errors.Is(err, database.ErrNoRows) {
		return nil, "", common.NewUnauthorizedError("user not found")
	}
	if err != nil {
		return nil, "", fmt.Errorf("database error: %w", err)
	}

	if !user.Enabled {
		return nil, "", common.NewUnauthorizedError("user is disabled")
	}

	// Handle scoping (same logic as password auth)
	var projectID string
	var roles []string
	var project *database.Project

	if req.Auth.Scope == nil || (req.Auth.Scope != nil && req.Auth.Scope.IsUnscoped) {
		// Unscoped token - no project/roles
		projectID = ""
	} else if req.Auth.Scope != nil && req.Auth.Scope.Project != nil {
		// Scoped authentication
		projectName := req.Auth.Scope.Project.Name
		projectIDParam := req.Auth.Scope.Project.ID

		// Fetch project
		var proj database.Project
		var query string
		var params []interface{}
		if projectIDParam != "" {
			query = "SELECT id, name, description, enabled, domain_id FROM projects WHERE id = $1 AND domain_id = $2"
			params = []interface{}{projectIDParam, user.DomainID}
		} else {
			query = "SELECT id, name, description, enabled, domain_id FROM projects WHERE name = $1 AND domain_id = $2"
			params = []interface{}{projectName, user.DomainID}
		}

		err := s.activeDB().QueryRow(ctx, query, params...).Scan(
			&proj.ID, &proj.Name, &proj.Description, &proj.Enabled, &proj.DomainID,
		)
		if errors.Is(err, database.ErrNoRows) {
			return nil, "", common.NewUnauthorizedError("project not found")
		}
		if err != nil {
			return nil, "", fmt.Errorf("database error: %w", err)
		}

		if !proj.Enabled {
			return nil, "", common.NewUnauthorizedError("project is disabled")
		}

		projectID = proj.ID
		project = &proj

		// Fetch roles
		rows, err := s.activeDB().Query(ctx, `
			SELECT r.name
			FROM role_assignments ra
			JOIN roles r ON ra.role_id = r.id
			WHERE ra.user_id = $1 AND ra.project_id = $2
		`, user.ID, projectID)
		if err != nil {
			return nil, "", fmt.Errorf("failed to fetch roles: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var roleName string
			if err := rows.Scan(&roleName); err != nil {
				return nil, "", fmt.Errorf("failed to scan role: %w", err)
			}
			roles = append(roles, roleName)
		}
		if err := rows.Err(); err != nil {
			return nil, "", fmt.Errorf("failed to iterate roles: %w", err)
		}

		if len(roles) == 0 {
			return nil, "", common.NewForbiddenError("user has no roles on this project")
		}
	}

	// Generate new JWT token
	now := time.Now()
	expiresAt := now.Add(s.tokenTTL)
	tokenClaims := &TokenClaims{
		UserID:    user.ID,
		UserName:  user.Name,
		ProjectID: projectID,
		Roles:     roles,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			Subject:   user.ID,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, tokenClaims)
	tokenString, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return nil, "", fmt.Errorf("failed to sign token: %w", err)
	}

	// Build response
	resp := &AuthResponse{}
	resp.Token.ExpiresAt = expiresAt.Format(time.RFC3339)
	resp.Token.IssuedAt = now.Format(time.RFC3339)
	resp.Token.Methods = []string{"token"}
	resp.Token.AuditIDs = []string{generateAuditID()}
	resp.Token.IsDomain = false

	// Query user's domain name
	var userDomainName string
	err = s.activeDB().QueryRow(ctx,
		"SELECT name FROM domains WHERE id = $1",
		user.DomainID,
	).Scan(&userDomainName)
	if err != nil {
		userDomainName = "Default" // fallback
	}

	resp.Token.User = map[string]interface{}{
		"id":   user.ID,
		"name": user.Name,
		"domain": map[string]interface{}{
			"id":   user.DomainID,
			"name": userDomainName,
		},
	}

	// Add project and catalog if scoped
	if projectID != "" {
		// Query project's domain name
		var projectDomainName string
		err = s.activeDB().QueryRow(ctx,
			"SELECT name FROM domains WHERE id = $1",
			project.DomainID,
		).Scan(&projectDomainName)
		if err != nil {
			projectDomainName = "Default" // fallback
		}

		resp.Token.Project = &map[string]interface{}{
			"id":        project.ID,
			"name":      project.Name,
			"is_domain": false,
			"domain": map[string]interface{}{
				"id":   project.DomainID,
				"name": projectDomainName,
			},
		}

		// Add roles with proper IDs
		for _, roleName := range roles {
			var roleID string
			_ = s.activeDB().QueryRow(ctx,
				"SELECT id FROM roles WHERE name = $1", roleName,
			).Scan(&roleID)
			if roleID == "" {
				roleID = roleName // fallback
			}
			resp.Token.Roles = append(resp.Token.Roles, map[string]interface{}{
				"id":   roleID,
				"name": roleName,
			})
		}

		// Add service catalog
		resp.Token.Catalog = s.BuildServiceCatalog(projectID, s.cache)
	}

	return resp, tokenString, nil
}

// AuthenticateApplicationCredential authenticates using an application credential.
// The returned bool is the unrestricted flag of the credential used for authentication.
func (s *AuthService) AuthenticateApplicationCredential(ctx context.Context, req *AuthRequest) (*AuthResponse, string, bool, error) {
	if req.Auth.Identity.ApplicationCredential == nil {
		return nil, "", false, common.NewBadRequestError("application_credential authentication required")
	}

	credID := req.Auth.Identity.ApplicationCredential.ID
	secret := req.Auth.Identity.ApplicationCredential.Secret

	if credID == "" || secret == "" {
		return nil, "", false, common.NewBadRequestError("application credential id and secret are required")
	}

	// Look up the application credential by ID, including the unrestricted flag
	var userID, secretHash, name string
	var projectID *string
	var expiresAt *time.Time
	var unrestricted bool
	var legacyAuth bool
	var accessRulesJSON []byte
	err := s.activeDB().QueryRow(ctx, `
		SELECT user_id, project_id, secret_hash, name, expires_at, unrestricted, COALESCE(legacy_auth, false), access_rules
		FROM application_credentials
		WHERE id = $1
	`, credID).Scan(&userID, &projectID, &secretHash, &name, &expiresAt, &unrestricted, &legacyAuth, &accessRulesJSON)

	if errors.Is(err, database.ErrNoRows) {
		return nil, "", false, common.NewUnauthorizedError("invalid application credential")
	}
	if err != nil {
		return nil, "", false, fmt.Errorf("failed to look up application credential: %w", err)
	}

	// Check expiration before bcrypt to save CPU on expired credentials
	if expiresAt != nil && time.Now().After(*expiresAt) {
		return nil, "", false, common.NewUnauthorizedError("application credential has expired")
	}

	// Verify the secret
	if legacyAuth {
		// Legacy: direct string comparison (pre-bcrypt base64 secrets)
		if subtle.ConstantTimeCompare([]byte(secret), []byte(secretHash)) != 1 {
			return nil, "", false, common.NewUnauthorizedError("invalid application credential")
		}
		// Transparent upgrade: re-hash with bcrypt and clear legacy flag
		newHash, hashErr := bcrypt.GenerateFromPassword([]byte(secret), 12)
		if hashErr == nil {
			_, _ = s.activeDB().Exec(ctx,
				"UPDATE application_credentials SET secret_hash = $1, legacy_auth = false, updated_at = NOW() WHERE id = $2",
				string(newHash), credID)
		}
		log.Warn().Str("credential_id", credID).Str("credential_name", name).Msg("legacy application credential used; please rotate to bcrypt")
	} else {
		// Modern: bcrypt verification
		if err := bcrypt.CompareHashAndPassword([]byte(secretHash), []byte(secret)); err != nil {
			return nil, "", false, common.NewUnauthorizedError("invalid application credential")
		}
	}

	// Parse access rules from JSONB column (nil if the credential has no restrictions)
	var accessRules []AccessRule
	if accessRulesJSON != nil {
		if err := json.Unmarshal(accessRulesJSON, &accessRules); err != nil {
			log.Warn().Err(err).Str("credential_id", credID).Msg("failed to parse access_rules, treating as unrestricted")
			accessRules = nil
		}
	}

	// Fetch the associated user
	var user database.User
	err = s.activeDB().QueryRow(ctx,
		"SELECT id, name, password_hash, enabled, domain_id FROM users WHERE id = $1",
		userID,
	).Scan(&user.ID, &user.Name, &user.PasswordHash, &user.Enabled, &user.DomainID)
	if errors.Is(err, database.ErrNoRows) {
		return nil, "", false, common.NewUnauthorizedError("user not found for application credential")
	}
	if err != nil {
		return nil, "", false, fmt.Errorf("failed to look up user for application credential: %w", err)
	}

	if !user.Enabled {
		return nil, "", false, common.NewUnauthorizedError("user is disabled")
	}

	// Determine project scope from the app credential
	var scopeProjectID string
	if projectID != nil {
		scopeProjectID = *projectID
	}

	// Get roles for this application credential
	var roles []string
	rows, err := s.activeDB().Query(ctx, `
		SELECT r.name
		FROM application_credential_roles acr
		JOIN roles r ON acr.role_id = r.id
		WHERE acr.application_credential_id = $1
	`, credID)
	if err != nil {
		return nil, "", false, fmt.Errorf("failed to fetch application credential roles: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var roleName string
		if err := rows.Scan(&roleName); err != nil {
			return nil, "", false, fmt.Errorf("failed to scan role: %w", err)
		}
		roles = append(roles, roleName)
	}
	if err := rows.Err(); err != nil {
		return nil, "", false, fmt.Errorf("failed to iterate application credential roles: %w", err)
	}

	// Generate JWT token
	now := time.Now()
	expiresAtTime := now.Add(s.tokenTTL)
	claims := &TokenClaims{
		UserID:          user.ID,
		UserName:        user.Name,
		ProjectID:       scopeProjectID,
		Roles:           roles,
		AccessRules:     accessRules,
		IsAppCredential: true,
		Unrestricted:    unrestricted,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAtTime),
			Subject:   user.ID,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return nil, "", false, fmt.Errorf("failed to sign token: %w", err)
	}

	// Build response
	resp := &AuthResponse{}
	resp.Token.ExpiresAt = expiresAtTime.Format(time.RFC3339)
	resp.Token.IssuedAt = now.Format(time.RFC3339)
	resp.Token.Methods = []string{"application_credential"}
	resp.Token.AuditIDs = []string{generateAuditID()}
	resp.Token.IsDomain = false

	// Query user's domain name
	var userDomainName string
	err = s.activeDB().QueryRow(ctx,
		"SELECT name FROM domains WHERE id = $1",
		user.DomainID,
	).Scan(&userDomainName)
	if err != nil {
		userDomainName = "Default"
	}

	resp.Token.User = map[string]interface{}{
		"id":   user.ID,
		"name": user.Name,
		"domain": map[string]interface{}{
			"id":   user.DomainID,
			"name": userDomainName,
		},
	}

	// Add project and catalog if scoped
	if scopeProjectID != "" {
		var proj database.Project
		err = s.activeDB().QueryRow(ctx,
			"SELECT id, name, description, enabled, domain_id FROM projects WHERE id = $1",
			scopeProjectID,
		).Scan(&proj.ID, &proj.Name, &proj.Description, &proj.Enabled, &proj.DomainID)
		if err == nil {
			var projectDomainName string
			err = s.activeDB().QueryRow(ctx,
				"SELECT name FROM domains WHERE id = $1",
				proj.DomainID,
			).Scan(&projectDomainName)
			if err != nil {
				projectDomainName = "Default"
			}

			resp.Token.Project = &map[string]interface{}{
				"id":        proj.ID,
				"name":      proj.Name,
				"is_domain": false,
				"domain": map[string]interface{}{
					"id":   proj.DomainID,
					"name": projectDomainName,
				},
			}
		}

		// Add roles with proper IDs
		for _, roleName := range roles {
			var roleID string
			_ = s.activeDB().QueryRow(ctx,
				"SELECT id FROM roles WHERE name = $1", roleName,
			).Scan(&roleID)
			if roleID == "" {
				roleID = roleName // fallback
			}
			resp.Token.Roles = append(resp.Token.Roles, map[string]interface{}{
				"id":   roleID,
				"name": roleName,
			})
		}

		// Add service catalog
		resp.Token.Catalog = s.BuildServiceCatalog(scopeProjectID, s.cache)
	}

	return resp, tokenString, unrestricted, nil
}

// ValidateToken validates and parses a JWT token
func (s *AuthService) ValidateToken(tokenString string) (*TokenClaims, error) {
	// Check revocation first
	if s.IsTokenRevoked(tokenString) {
		return nil, common.NewUnauthorizedError("token has been revoked")
	}

	token, err := jwt.ParseWithClaims(tokenString, &TokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.jwtSecret, nil
	})

	if err != nil {
		return nil, common.NewUnauthorizedError("invalid token")
	}

	if claims, ok := token.Claims.(*TokenClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, common.NewUnauthorizedError("invalid token claims")
}

// RevokeToken adds a token to the denylist
func (s *AuthService) RevokeToken(tokenString string, expiresAt time.Time) {
	hash := tokenHash(tokenString)
	s.revokedTokens.Store(hash, expiresAt)

	// Persist to DB synchronously for multi-instance awareness
	if db := s.activeDB(); db != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := db.Exec(ctx,
			`INSERT INTO revoked_tokens (token_hash, expires_at) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			hash, expiresAt,
		); err != nil {
			log.Error().Err(err).Str("token_hash", hash).Msg("failed to persist token revocation to DB")
		}
	}
}

// IsTokenRevoked checks if a token has been revoked
func (s *AuthService) IsTokenRevoked(tokenString string) bool {
	hash := tokenHash(tokenString)

	// Fast path: check in-memory map
	if val, ok := s.revokedTokens.Load(hash); ok {
		expiry, ok := val.(time.Time)
		if !ok {
			s.revokedTokens.Delete(hash)
			return false
		}
		if time.Now().After(expiry) {
			s.revokedTokens.Delete(hash)
			return false
		}
		return true
	}

	// Slow path: check the database
	db := s.activeDB()
	if db == nil {
		return false
	}

	var expiresAt time.Time
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := db.QueryRow(ctx,
		"SELECT expires_at FROM revoked_tokens WHERE token_hash = $1",
		hash,
	).Scan(&expiresAt)

	if err != nil {
		// Not found or DB error — treat as not revoked
		return false
	}

	// Found in DB — check if already expired
	if time.Now().After(expiresAt) {
		// Token expired anyway, clean it up from DB asynchronously
		go func() {
			_, _ = db.Exec(context.Background(),
				"DELETE FROM revoked_tokens WHERE token_hash = $1", hash)
		}()
		return false
	}

	// Cache in sync.Map for future fast lookups
	s.revokedTokens.Store(hash, expiresAt)
	return true
}

// CleanExpiredRevocations removes expired entries from the denylist (in-memory and DB)
func (s *AuthService) CleanExpiredRevocations() {
	now := time.Now()
	s.revokedTokens.Range(func(key, value any) bool {
		expiry, ok := value.(time.Time)
		if !ok || now.After(expiry) {
			s.revokedTokens.Delete(key)
		}
		return true
	})

	// Also clean expired entries from the database
	if db := s.activeDB(); db != nil {
		_, _ = db.Exec(context.Background(),
			"DELETE FROM revoked_tokens WHERE expires_at < $1", now)
	}
}

func tokenHash(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// HashPassword hashes a password using bcrypt with cost 12
func (s *AuthService) HashPassword(password string) ([]byte, error) {
	return bcrypt.GenerateFromPassword([]byte(password), 12)
}

// CheckPassword verifies a password against a bcrypt hash
func (s *AuthService) CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// generateAuditID returns a random base64url-encoded 16-byte audit ID.
func generateAuditID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Extremely unlikely; fall back to a fixed string rather than panic.
		return "audit-id-unavailable"
	}
	return base64.URLEncoding.EncodeToString(b)
}

// BuildServiceCatalog builds the OpenStack service catalog from database
func (s *AuthService) BuildServiceCatalog(projectID string, cacheInstance *cache.Cache) []CatalogEntry {
	ctx := context.Background()

	// Try cache first (service catalog is immutable, 24h TTL)
	if cacheInstance != nil {
		cacheKey := "service_catalog:" + projectID
		var cached []CatalogEntry
		if err := cacheInstance.Get(ctx, cacheKey, &cached); err == nil {
			return cached
		}
	}

	catalog := []CatalogEntry{}

	// Query services and their endpoints from database
	rows, err := s.activeDB().Query(ctx, `
		SELECT s.id, s.type, s.name, e.id, e.interface, e.url, e.region
		FROM services s
		LEFT JOIN endpoints e ON s.id = e.service_id
		WHERE s.enabled = true AND (e.enabled = true OR e.enabled IS NULL)
		ORDER BY s.type, e.interface
	`)
	if err != nil {
		// Fall back to hardcoded catalog on error
		return buildHardcodedCatalog(projectID)
	}
	defer rows.Close()

	// Group endpoints by service
	serviceMap := make(map[string]*CatalogEntry)
	for rows.Next() {
		var (
			serviceID  string
			svcType    string
			svcName    string
			endpointID *string
			iface      *string
			url        *string
			region     *string
		)

		if err := rows.Scan(&serviceID, &svcType, &svcName, &endpointID, &iface, &url, &region); err != nil {
			continue
		}

		// Create service entry if not exists
		if _, exists := serviceMap[serviceID]; !exists {
			serviceMap[serviceID] = &CatalogEntry{
				Type:      svcType,
				Name:      svcName,
				Endpoints: []Endpoint{},
			}
		}

		// Add endpoint if present
		if endpointID != nil && iface != nil && url != nil {
			// Substitute URL templates
			substitutedURL := substituteURLTemplates(*url, projectID)

			endpoint := Endpoint{
				Interface: *iface,
				URL:       substitutedURL,
			}
			if region != nil {
				endpoint.RegionID = *region
				endpoint.Region = *region
			}
			serviceMap[serviceID].Endpoints = append(serviceMap[serviceID].Endpoints, endpoint)
		}
	}

	if err := rows.Err(); err != nil {
		return buildHardcodedCatalog(projectID)
	}

	// Convert map to slice
	for _, entry := range serviceMap {
		catalog = append(catalog, *entry)
	}

	// Fall back to hardcoded if query returned nothing
	if len(catalog) == 0 {
		return buildHardcodedCatalog(projectID)
	}

	// Store in cache (24h TTL per config)
	if cacheInstance != nil {
		_ = cacheInstance.Set(ctx, "service_catalog:"+projectID, catalog, 24*time.Hour)
	}

	return catalog
}

// substituteURLTemplates replaces template placeholders in endpoint URLs
func substituteURLTemplates(url, projectID string) string {
	// Replace {project_id} placeholder
	url = strings.Replace(url, "{project_id}", projectID, -1)
	// Also handle $(project_id)s format (OpenStack convention)
	url = strings.Replace(url, "$(project_id)s", projectID, -1)
	// Also handle %(project_id)s format (Python string formatting)
	url = strings.Replace(url, "%(project_id)s", projectID, -1)
	// Also handle plain %s format (legacy)
	url = strings.Replace(url, "%s", projectID, -1)

	// Replace hostname based on O3K_ENDPOINT_HOST environment variable
	// This allows using 'localhost' in CI and 'o3k' in docker-compose
	baseHost := os.Getenv("O3K_ENDPOINT_HOST")
	if baseHost == "" {
		baseHost = "localhost"
	}
	// Replace http://o3k: with the configured host
	url = strings.Replace(url, "http://o3k:", "http://"+baseHost+":", -1)
	// Also handle https://o3k: for future SSL support
	url = strings.Replace(url, "https://o3k:", "https://"+baseHost+":", -1)

	return url
}

// buildHardcodedCatalog provides fallback catalog (previous implementation)
func buildHardcodedCatalog(projectID string) []CatalogEntry {
	// Use O3K_ENDPOINT_HOST env var, default to "localhost" for CI compatibility
	baseHost := os.Getenv("O3K_ENDPOINT_HOST")
	if baseHost == "" {
		baseHost = "localhost"
	}
	baseURL := "http://" + baseHost

	// allInterfaces returns public/internal/admin endpoints for a URL.
	// In O3K all interfaces point to the same binary, so all URLs are identical.
	allInterfaces := func(url string) []Endpoint {
		return []Endpoint{
			{Interface: "public", RegionID: "RegionOne", Region: "RegionOne", URL: url},
			{Interface: "internal", RegionID: "RegionOne", Region: "RegionOne", URL: url},
			{Interface: "admin", RegionID: "RegionOne", Region: "RegionOne", URL: url},
		}
	}

	return []CatalogEntry{
		{
			Type:      "identity",
			Name:      "keystone",
			Endpoints: allInterfaces(fmt.Sprintf("%s:35357/v3", baseURL)),
		},
		{
			Type:      "compute",
			Name:      "nova",
			Endpoints: allInterfaces(fmt.Sprintf("%s:8774/v2.1/%s", baseURL, projectID)),
		},
		{
			Type:      "placement",
			Name:      "placement",
			Endpoints: allInterfaces(fmt.Sprintf("%s:8778", baseURL)),
		},
		{
			Type:      "network",
			Name:      "neutron",
			Endpoints: allInterfaces(fmt.Sprintf("%s:9696", baseURL)),
		},
		{
			Type:      "volumev3",
			Name:      "cinderv3",
			Endpoints: allInterfaces(fmt.Sprintf("%s:8776/v3/%s", baseURL, projectID)),
		},
		{
			Type:      "image",
			Name:      "glance",
			Endpoints: allInterfaces(fmt.Sprintf("%s:9292", baseURL)),
		},
	}
}
