package keystone

import (
	"context"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/cobaltcore-dev/o3k/internal/common"
	"github.com/cobaltcore-dev/o3k/internal/database"
	"golang.org/x/crypto/bcrypt"
)

// TokenClaims represents JWT token claims
type TokenClaims struct {
	UserID    string   `json:"user_id"`
	UserName  string   `json:"user_name"`
	ProjectID string   `json:"project_id,omitempty"`
	Roles     []string `json:"roles,omitempty"`
	jwt.RegisteredClaims
}

// AuthService handles authentication operations
type AuthService struct {
	jwtSecret []byte
	tokenTTL  time.Duration
}

// NewAuthService creates a new auth service
func NewAuthService(jwtSecret string, tokenTTL time.Duration) *AuthService {
	return &AuthService{
		jwtSecret: []byte(jwtSecret),
		tokenTTL:  tokenTTL,
	}
}

// AuthRequest represents an authentication request
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
		} `json:"identity"`
		Scope *struct {
			Project *struct {
				Name   string `json:"name"`
				ID     string `json:"id"`
				Domain *struct {
					Name string `json:"name"`
					ID   string `json:"id"`
				} `json:"domain,omitempty"`
			} `json:"project,omitempty"`
		} `json:"scope,omitempty"`
	} `json:"auth"`
}

// AuthResponse represents an authentication response
type AuthResponse struct {
	Token struct {
		ExpiresAt string                 `json:"expires_at"`
		IssuedAt  string                 `json:"issued_at"`
		Methods   []string               `json:"methods"`
		User      map[string]interface{} `json:"user"`
		Catalog   []CatalogEntry         `json:"catalog,omitempty"`
		Project   *map[string]interface{} `json:"project,omitempty"`
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
	Region    string `json:"region"`
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
	err := database.DB.QueryRow(ctx,
		"SELECT id FROM domains WHERE name = $1",
		domainName,
	).Scan(&domainID)

	if err == pgx.ErrNoRows {
		return nil, "", common.NewUnauthorizedError("invalid domain")
	}
	if err != nil {
		return nil, "", fmt.Errorf("database error looking up domain: %w", err)
	}

	// Fetch user from database (with domain filter)
	var user database.User
	err = database.DB.QueryRow(ctx,
		"SELECT id, name, password_hash, enabled, domain_id FROM users WHERE name = $1 AND domain_id = $2",
		username, domainID,
	).Scan(&user.ID, &user.Name, &user.PasswordHash, &user.Enabled, &user.DomainID)

	if err == pgx.ErrNoRows {
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

	if req.Auth.Scope != nil && req.Auth.Scope.Project != nil {
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

		err := database.DB.QueryRow(ctx, query, params...).Scan(
			&proj.ID, &proj.Name, &proj.Description, &proj.Enabled, &proj.DomainID,
		)
		if err == pgx.ErrNoRows {
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
		rows, err := database.DB.Query(ctx, `
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

		if len(roles) == 0 {
			return nil, "", common.NewForbiddenError("user has no roles on this project")
		}
	}

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
	resp.Token.Methods = req.Auth.Identity.Methods
	resp.Token.User = map[string]interface{}{
		"id":   user.ID,
		"name": user.Name,
		"domain": map[string]interface{}{
			"id":   "default",
			"name": "Default",
		},
	}

	// Add project and catalog if scoped
	if projectID != "" {
		resp.Token.Project = &map[string]interface{}{
			"id":   project.ID,
			"name": project.Name,
			"domain": map[string]interface{}{
				"id":   "default",
				"name": "Default",
			},
		}

		// Add roles
		for _, roleName := range roles {
			resp.Token.Roles = append(resp.Token.Roles, map[string]interface{}{
				"id":   roleName,
				"name": roleName,
			})
		}

		// Add service catalog
		resp.Token.Catalog = BuildServiceCatalog(projectID)
	}

	return resp, tokenString, nil
}

// ValidateToken validates and parses a JWT token
func (s *AuthService) ValidateToken(tokenString string) (*TokenClaims, error) {
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

// HashPassword hashes a password using bcrypt
func (s *AuthService) HashPassword(password string) ([]byte, error) {
	return bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
}

// CheckPassword verifies a password against a bcrypt hash
func (s *AuthService) CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// BuildServiceCatalog builds the OpenStack service catalog from database
func BuildServiceCatalog(projectID string) []CatalogEntry {
	catalog := []CatalogEntry{}

	// Query services and their endpoints from database
	rows, err := database.DB.Query(context.Background(), `
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
			endpoint := Endpoint{
				Interface: *iface,
				URL:       *url,
			}
			if region != nil {
				endpoint.Region = *region
			}
			serviceMap[serviceID].Endpoints = append(serviceMap[serviceID].Endpoints, endpoint)
		}
	}

	// Convert map to slice
	for _, entry := range serviceMap {
		catalog = append(catalog, *entry)
	}

	// Fall back to hardcoded if query returned nothing
	if len(catalog) == 0 {
		return buildHardcodedCatalog(projectID)
	}

	return catalog
}

// buildHardcodedCatalog provides fallback catalog (previous implementation)
func buildHardcodedCatalog(projectID string) []CatalogEntry {
	baseURL := "http://localhost"

	return []CatalogEntry{
		{
			Type: "identity",
			Name: "keystone",
			Endpoints: []Endpoint{
				{Interface: "public", Region: "RegionOne", URL: fmt.Sprintf("%s:35357/v3", baseURL)},
			},
		},
		{
			Type: "compute",
			Name: "nova",
			Endpoints: []Endpoint{
				{Interface: "public", Region: "RegionOne", URL: fmt.Sprintf("%s:8774/v2.1", baseURL)},
			},
		},
		{
			Type: "network",
			Name: "neutron",
			Endpoints: []Endpoint{
				{Interface: "public", Region: "RegionOne", URL: fmt.Sprintf("%s:9696/v2.0", baseURL)},
			},
		},
		{
			Type: "volumev3",
			Name: "cinderv3",
			Endpoints: []Endpoint{
				{Interface: "public", Region: "RegionOne", URL: fmt.Sprintf("%s:8776/v3/%s", baseURL, projectID)},
			},
		},
		{
			Type: "image",
			Name: "glance",
			Endpoints: []Endpoint{
				{Interface: "public", Region: "RegionOne", URL: fmt.Sprintf("%s:9292", baseURL)},
			},
		},
	}
}
