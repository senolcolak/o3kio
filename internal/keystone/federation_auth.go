// Package keystone — federated authentication path for SCS-0300-v1.
//
// AuthenticateFederated mirrors AuthenticatePassword's structure but swaps
// the credential check from bcrypt to OIDC ID-token verification. JIT user
// provisioning kicks in on first login when the configured provider has
// AutoProvision=true, deriving a stable O3K user UUID from the IdP's
// (issuer, subject) tuple via deterministicUUID.
package keystone

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/common"
	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/golang-jwt/jwt/v5"
)

// SetFederationRegistry attaches a registry to the AuthService. Called once
// at startup from main.go after NewFederationRegistry succeeds. Passing nil
// disables federated auth at runtime.
func (s *AuthService) SetFederationRegistry(r *FederationRegistry) {
	s.federation = r
}

// AuthenticateFederated authenticates a caller via a federated IdP. The
// credential carried in the request body (an OIDC ID token for v1) is
// verified against the named provider, then mapped to an O3K user via JIT
// provisioning. Project scoping and JWT issuance follow the same logic as
// AuthenticatePassword.
func (s *AuthService) AuthenticateFederated(ctx context.Context, req *AuthRequest) (*AuthResponse, string, error) {
	if s.federation == nil {
		return nil, "", common.NewBadRequestError("federation is not enabled on this server")
	}
	if req.Auth.Identity.Federated == nil {
		return nil, "", common.NewBadRequestError("federated identity block required")
	}
	fed := req.Auth.Identity.Federated
	if fed.Provider == "" {
		return nil, "", common.NewBadRequestError("federated.provider is required")
	}
	if fed.Credential == "" {
		return nil, "", common.NewBadRequestError("federated.credential is required")
	}

	provider, err := s.federation.Provider(fed.Provider)
	if errors.Is(err, ErrUnknownProvider) {
		return nil, "", common.NewBadRequestError(fmt.Sprintf("unknown federation provider %q", fed.Provider))
	}
	if err != nil {
		return nil, "", fmt.Errorf("federation provider lookup: %w", err)
	}

	identity, err := provider.Verify(ctx, fed.Credential)
	if err != nil {
		// Verification failures are 401, not 500 — the IdP says the
		// credential is bad, the server is fine.
		return nil, "", common.NewUnauthorizedError("federated credential verification failed")
	}

	// Look up the per-provider config to drive JIT provisioning behavior.
	cfg, ok := s.federation.config(fed.Provider)
	if !ok {
		// Provider exists in the registry but config missing — registry
		// invariant violation, not a user-facing error.
		return nil, "", fmt.Errorf("federation provider %q configured but config missing", fed.Provider)
	}

	user, err := s.resolveFederatedUser(ctx, identity, cfg)
	if err != nil {
		return nil, "", err
	}

	// Reject domain-scoped token requests — same constraint as password auth.
	if req.Auth.Scope != nil && req.Auth.Scope.IsDomainScoped {
		return nil, "", common.NewNotImplementedError("domain-scoped tokens are not supported")
	}

	// Determine project scope. Federated requests typically include an
	// explicit scope; if absent, fall back to the provider's
	// default_project (which is required when AutoProvision is true).
	projectID, project, roles, err := s.resolveFederatedScope(ctx, req, user, cfg)
	if err != nil {
		return nil, "", err
	}

	// Issue a normal O3K JWT. Methods=["openid"] tells downstream
	// observers (audit, AuthMiddleware) the auth path; structurally the
	// token is identical to a password-issued one.
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

	resp := s.buildFederatedResponse(ctx, user, project, projectID, roles, now, expiresAt)
	return resp, tokenString, nil
}

// resolveFederatedUser returns the O3K user record for a verified federated
// identity. On first login (AutoProvision=true), creates the user record
// using a deterministic UUID derived from (issuer, subject). On subsequent
// logins, the same UUID resolves to the existing record.
func (s *AuthService) resolveFederatedUser(ctx context.Context, id *FederatedIdentity, cfg FederationProviderConfig) (*database.User, error) {
	uid := deterministicUUID(id.Issuer, id.Subject)

	var user database.User
	err := s.activeDB().QueryRow(ctx,
		"SELECT id, name, password_hash, enabled, domain_id FROM users WHERE id = $1",
		uid,
	).Scan(&user.ID, &user.Name, &user.PasswordHash, &user.Enabled, &user.DomainID)

	if err == nil {
		if !user.Enabled {
			return nil, common.NewUnauthorizedError("user is disabled")
		}
		return &user, nil
	}
	if !errors.Is(err, database.ErrNoRows) {
		return nil, fmt.Errorf("database error looking up federated user: %w", err)
	}

	if !cfg.AutoProvision {
		return nil, common.NewUnauthorizedError("federated user not found and auto-provisioning is disabled")
	}

	// JIT provisioning — username falls back to email then subject when
	// the IdP doesn't supply preferred_username.
	username := id.PreferredUsername
	if username == "" {
		username = id.Email
	}
	if username == "" {
		username = id.Subject
	}

	// Resolve domain — federated users land in "Default" for v1.
	var domainID string
	if err := s.activeDB().QueryRow(ctx,
		"SELECT id FROM domains WHERE name = $1", "Default",
	).Scan(&domainID); err != nil {
		return nil, fmt.Errorf("federated JIT: failed to resolve Default domain: %w", err)
	}

	// password_hash is set to a sentinel that can never match a bcrypt
	// check ("!" prefix is invalid bcrypt), so federated users cannot
	// fall back to password authentication.
	_, err = s.activeDB().Exec(ctx, `
		INSERT INTO users (id, name, password_hash, enabled, domain_id)
		VALUES ($1, $2, $3, $4, $5)
	`, uid, username, "!federated", true, domainID)
	if err != nil {
		return nil, fmt.Errorf("federated JIT: failed to create user: %w", err)
	}

	return &database.User{
		ID:           uid,
		Name:         username,
		PasswordHash: "!federated",
		Enabled:      true,
		DomainID:     domainID,
	}, nil
}

// resolveFederatedScope determines the target project and the user's roles
// on it. Explicit scope in the request wins; otherwise the provider's
// default_project is used. Returns (projectID, project, roleNames, error).
func (s *AuthService) resolveFederatedScope(
	ctx context.Context,
	req *AuthRequest,
	user *database.User,
	cfg FederationProviderConfig,
) (string, *database.Project, []string, error) {
	// Determine the project name/id to scope to.
	var projectName, projectIDParam string
	if req.Auth.Scope != nil && !req.Auth.Scope.IsUnscoped && req.Auth.Scope.Project != nil {
		projectName = req.Auth.Scope.Project.Name
		projectIDParam = req.Auth.Scope.Project.ID
	} else if cfg.DefaultProject != "" {
		projectName = cfg.DefaultProject
	} else {
		// Unscoped federated token — allowed, but produces a token with
		// no project_id and no roles. Useful for "list my projects"
		// flows.
		return "", nil, nil, nil
	}

	var proj database.Project
	var query string
	var params []interface{}
	if projectIDParam != "" {
		query = "SELECT id, name, description, enabled, domain_id FROM projects WHERE id = $1"
		params = []interface{}{projectIDParam}
	} else {
		query = "SELECT id, name, description, enabled, domain_id FROM projects WHERE name = $1"
		params = []interface{}{projectName}
	}
	err := s.activeDB().QueryRow(ctx, query, params...).Scan(
		&proj.ID, &proj.Name, &proj.Description, &proj.Enabled, &proj.DomainID,
	)
	if errors.Is(err, database.ErrNoRows) {
		return "", nil, nil, common.NewUnauthorizedError("project not found")
	}
	if err != nil {
		return "", nil, nil, fmt.Errorf("federated scope: project lookup failed: %w", err)
	}
	if !proj.Enabled {
		return "", nil, nil, common.NewUnauthorizedError("project is disabled")
	}

	// Ensure a default-role assignment exists when JIT provisioning a
	// federated user onto the default project for the first time. This
	// is the minimum role-mapping in v1; richer claim-to-role mapping
	// from federation_role_mappings is a follow-up.
	if cfg.DefaultRole != "" {
		if err := s.ensureFederatedDefaultRole(ctx, user.ID, proj.ID, cfg.DefaultRole); err != nil {
			return "", nil, nil, err
		}
	}

	rows, err := s.activeDB().Query(ctx, `
		SELECT r.name
		FROM role_assignments ra
		JOIN roles r ON ra.role_id = r.id
		WHERE ra.user_id = $1 AND ra.project_id = $2
	`, user.ID, proj.ID)
	if err != nil {
		return "", nil, nil, fmt.Errorf("federated scope: role fetch failed: %w", err)
	}
	defer rows.Close()

	var roles []string
	for rows.Next() {
		var roleName string
		if err := rows.Scan(&roleName); err != nil {
			return "", nil, nil, fmt.Errorf("federated scope: role scan failed: %w", err)
		}
		roles = append(roles, roleName)
	}
	if err := rows.Err(); err != nil {
		return "", nil, nil, fmt.Errorf("federated scope: role iter failed: %w", err)
	}

	if len(roles) == 0 {
		return "", nil, nil, common.NewForbiddenError("federated user has no roles on this project")
	}
	return proj.ID, &proj, roles, nil
}

// ensureFederatedDefaultRole grants the configured default role to a
// federated user on the target project if no assignment exists yet.
// Idempotent: calling twice for the same (user, project) is a no-op.
func (s *AuthService) ensureFederatedDefaultRole(ctx context.Context, userID, projectID, roleName string) error {
	var roleID string
	err := s.activeDB().QueryRow(ctx,
		"SELECT id FROM roles WHERE name = $1", roleName,
	).Scan(&roleID)
	if errors.Is(err, database.ErrNoRows) {
		return fmt.Errorf("federated default role %q does not exist", roleName)
	}
	if err != nil {
		return fmt.Errorf("federated default role lookup: %w", err)
	}

	var existing string
	err = s.activeDB().QueryRow(ctx, `
		SELECT id FROM role_assignments
		WHERE user_id = $1 AND project_id = $2 AND role_id = $3
	`, userID, projectID, roleID).Scan(&existing)
	if err == nil {
		return nil // already assigned
	}
	if !errors.Is(err, database.ErrNoRows) {
		return fmt.Errorf("federated default role: existence check failed: %w", err)
	}

	assignmentID := deterministicUUID(userID, projectID, roleID)
	if _, err := s.activeDB().Exec(ctx, `
		INSERT INTO role_assignments (id, user_id, project_id, role_id)
		VALUES ($1, $2, $3, $4)
	`, assignmentID, userID, projectID, roleID); err != nil {
		return fmt.Errorf("federated default role assignment: %w", err)
	}
	return nil
}

// buildFederatedResponse assembles the AuthResponse body for a federated
// login. Methods is set to ["openid"] to distinguish from password tokens
// in audit logs and downstream consumers.
func (s *AuthService) buildFederatedResponse(
	ctx context.Context,
	user *database.User,
	project *database.Project,
	projectID string,
	roles []string,
	now, expiresAt time.Time,
) *AuthResponse {
	resp := &AuthResponse{}
	resp.Token.ExpiresAt = expiresAt.Format(time.RFC3339)
	resp.Token.IssuedAt = now.Format(time.RFC3339)
	resp.Token.Methods = []string{"openid"}
	resp.Token.AuditIDs = []string{generateAuditID()}
	resp.Token.IsDomain = false

	var userDomainName string
	if err := s.activeDB().QueryRow(ctx,
		"SELECT name FROM domains WHERE id = $1", user.DomainID,
	).Scan(&userDomainName); err != nil {
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

	if projectID != "" && project != nil {
		var projectDomainName string
		if err := s.activeDB().QueryRow(ctx,
			"SELECT name FROM domains WHERE id = $1", project.DomainID,
		).Scan(&projectDomainName); err != nil {
			projectDomainName = "Default"
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
		for _, roleName := range roles {
			var roleID string
			_ = s.activeDB().QueryRow(ctx,
				"SELECT id FROM roles WHERE name = $1", roleName,
			).Scan(&roleID)
			if roleID == "" {
				roleID = roleName
			}
			resp.Token.Roles = append(resp.Token.Roles, map[string]interface{}{
				"id":   roleID,
				"name": roleName,
			})
		}
		resp.Token.Catalog = s.BuildServiceCatalog(projectID, s.cache)
	}
	return resp
}
