// Package keystone — federation_test exercises the SCS-0300-v1 federation
// surface added in Slice 7. Tests live in the keystone package (not
// keystone_test) so they can use the package-internal
// newFederationRegistryWithProviders test seam and the unexported config()
// lookup. This avoids the need to spin up a real OIDC issuer for unit tests.
package keystone

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cobaltcore-dev/o3k/internal/database"
)

// stubProvider is a minimal FederationProvider used in unit tests. It returns
// a canned identity (or canned error) without going near OIDC discovery or
// JWKS fetches.
type stubProvider struct {
	name     string
	identity *FederatedIdentity
	err      error
}

func (s *stubProvider) Name() string { return s.name }

func (s *stubProvider) Verify(_ context.Context, _ string) (*FederatedIdentity, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.identity, nil
}

// TestFederationConfigFromYAMLDefaults asserts that the YAML-to-keystone
// config bridge fills in the documented defaults (preferred_username, groups)
// when the operator leaves them blank.
func TestFederationConfigFromYAMLDefaults(t *testing.T) {
	in := []FederationProviderConfig{
		{Name: "p1"},                                 // both blank
		{Name: "p2", UsernameClaim: "email"},         // username overridden
		{Name: "p3", GroupsClaim: "roles"},           // groups overridden
		{Name: "p4", UsernameClaim: "x", GroupsClaim: "y"}, // both overridden
	}
	out := FederationConfigFromYAML(in)
	require.Len(t, out, 4)

	assert.Equal(t, "preferred_username", out[0].UsernameClaim)
	assert.Equal(t, "groups", out[0].GroupsClaim)

	assert.Equal(t, "email", out[1].UsernameClaim)
	assert.Equal(t, "groups", out[1].GroupsClaim)

	assert.Equal(t, "preferred_username", out[2].UsernameClaim)
	assert.Equal(t, "roles", out[2].GroupsClaim)

	assert.Equal(t, "x", out[3].UsernameClaim)
	assert.Equal(t, "y", out[3].GroupsClaim)
}

// TestFederationRegistryLookup verifies Provider() returns the registered
// provider on hit and ErrUnknownProvider on miss, and that config() round-trips.
func TestFederationRegistryLookup(t *testing.T) {
	stub := &stubProvider{name: "kc"}
	cfg := FederationProviderConfig{Name: "kc", Protocol: "openid", AutoProvision: true, DefaultProject: "default", DefaultRole: "member"}
	reg := newFederationRegistryWithProviders(
		map[string]FederationProvider{"kc": stub},
		map[string]FederationProviderConfig{"kc": cfg},
	)

	got, err := reg.Provider("kc")
	require.NoError(t, err)
	assert.Same(t, stub, got)

	_, err = reg.Provider("missing")
	require.ErrorIs(t, err, ErrUnknownProvider)

	gotCfg, ok := reg.config("kc")
	require.True(t, ok)
	assert.Equal(t, "default", gotCfg.DefaultProject)

	_, ok = reg.config("missing")
	assert.False(t, ok)

	names := reg.Names()
	require.Len(t, names, 1)
	assert.Equal(t, "kc", names[0])
}

// TestNewFederationRegistryUnsupportedProtocol verifies the registry rejects
// protocols outside the v1 set so a typo can't silently disable a provider.
func TestNewFederationRegistryUnsupportedProtocol(t *testing.T) {
	_, err := NewFederationRegistry(t.Context(), []FederationProviderConfig{
		{Name: "ldap-thing", Protocol: "ldap"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported protocol")
}

// TestAuthenticateFederatedNoRegistry asserts the federated entry point
// returns 400 when federation is not enabled on the server.
func TestAuthenticateFederatedNoRegistry(t *testing.T) {
	mock := database.NewSeededMockDB()
	svc := NewAuthServiceWithDB(mock, "test-secret", time.Hour, nil)

	req := &AuthRequest{}
	req.Auth.Identity.Methods = []string{"openid"}
	req.Auth.Identity.Federated = &FederationAuthRequest{
		Provider: "kc", Protocol: "openid", Credential: "x.y.z",
	}

	_, _, err := svc.AuthenticateFederated(t.Context(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "federation is not enabled")
}

// TestAuthenticateFederatedMissingFederatedBlock asserts a 400 when the
// caller sends methods=["openid"] but forgets the identity.federated block.
func TestAuthenticateFederatedMissingFederatedBlock(t *testing.T) {
	mock := database.NewSeededMockDB()
	svc := NewAuthServiceWithDB(mock, "test-secret", time.Hour, nil)
	svc.SetFederationRegistry(newFederationRegistryWithProviders(
		map[string]FederationProvider{"kc": &stubProvider{name: "kc"}},
		map[string]FederationProviderConfig{"kc": {Name: "kc"}},
	))

	req := &AuthRequest{}
	req.Auth.Identity.Methods = []string{"openid"}
	// No Federated field set.

	_, _, err := svc.AuthenticateFederated(t.Context(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "federated identity block required")
}

// TestAuthenticateFederatedMissingProvider asserts the empty-provider-name
// validation path returns a 400.
func TestAuthenticateFederatedMissingProvider(t *testing.T) {
	mock := database.NewSeededMockDB()
	svc := NewAuthServiceWithDB(mock, "test-secret", time.Hour, nil)
	svc.SetFederationRegistry(newFederationRegistryWithProviders(
		map[string]FederationProvider{},
		map[string]FederationProviderConfig{},
	))

	req := &AuthRequest{}
	req.Auth.Identity.Federated = &FederationAuthRequest{Credential: "x.y.z"}

	_, _, err := svc.AuthenticateFederated(t.Context(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider is required")
}

// TestAuthenticateFederatedMissingCredential asserts the empty-credential
// validation path returns a 400.
func TestAuthenticateFederatedMissingCredential(t *testing.T) {
	mock := database.NewSeededMockDB()
	svc := NewAuthServiceWithDB(mock, "test-secret", time.Hour, nil)
	svc.SetFederationRegistry(newFederationRegistryWithProviders(
		map[string]FederationProvider{"kc": &stubProvider{name: "kc"}},
		map[string]FederationProviderConfig{"kc": {Name: "kc"}},
	))

	req := &AuthRequest{}
	req.Auth.Identity.Federated = &FederationAuthRequest{Provider: "kc"}

	_, _, err := svc.AuthenticateFederated(t.Context(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "credential is required")
}

// TestAuthenticateFederatedUnknownProvider asserts a 400 when the named
// provider isn't configured (distinct from credential-verification failures).
func TestAuthenticateFederatedUnknownProvider(t *testing.T) {
	mock := database.NewSeededMockDB()
	svc := NewAuthServiceWithDB(mock, "test-secret", time.Hour, nil)
	svc.SetFederationRegistry(newFederationRegistryWithProviders(
		map[string]FederationProvider{},
		map[string]FederationProviderConfig{},
	))

	req := &AuthRequest{}
	req.Auth.Identity.Federated = &FederationAuthRequest{
		Provider: "ghost", Protocol: "openid", Credential: "x.y.z",
	}

	_, _, err := svc.AuthenticateFederated(t.Context(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown federation provider")
}

// TestAuthenticateFederatedVerifyFailure asserts that an IdP-side credential
// rejection surfaces as 401 — the server is fine, the IdP says the token is
// bad.
func TestAuthenticateFederatedVerifyFailure(t *testing.T) {
	mock := database.NewSeededMockDB()
	svc := NewAuthServiceWithDB(mock, "test-secret", time.Hour, nil)
	svc.SetFederationRegistry(newFederationRegistryWithProviders(
		map[string]FederationProvider{"kc": &stubProvider{
			name: "kc",
			err:  errors.New("expired id token"),
		}},
		map[string]FederationProviderConfig{"kc": {Name: "kc", Protocol: "openid"}},
	))

	req := &AuthRequest{}
	req.Auth.Identity.Federated = &FederationAuthRequest{
		Provider: "kc", Protocol: "openid", Credential: "x.y.z",
	}

	_, _, err := svc.AuthenticateFederated(t.Context(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "federated credential verification failed")
}

// TestAuthenticateFederatedHappyPath exercises the full federated flow with
// a stub provider returning a verified identity. The seeded mock DB resolves
// any user-by-id query to the admin record, so the test skips JIT-INSERT and
// instead validates token issuance, methods=["openid"], and project scoping.
func TestAuthenticateFederatedHappyPath(t *testing.T) {
	mock := database.NewSeededMockDB()
	svc := NewAuthServiceWithDB(mock, "test-secret", time.Hour, nil)
	svc.SetFederationRegistry(newFederationRegistryWithProviders(
		map[string]FederationProvider{"kc": &stubProvider{
			name: "kc",
			identity: &FederatedIdentity{
				Issuer:            "https://kc.example.com/realms/scs",
				Subject:           "user-sub-123",
				Email:             "alice@example.com",
				PreferredUsername: "alice",
				Groups:            []string{"developers"},
			},
		}},
		map[string]FederationProviderConfig{"kc": {
			Name:           "kc",
			Protocol:       "openid",
			AutoProvision:  true,
			DefaultProject: "default",
			// DefaultRole intentionally empty: the seeded mock DB doesn't
			// model the `roles` table, so the ensureFederatedDefaultRole
			// branch can't run cleanly here. The role-name list returned by
			// the seeded role_assignments query already gives us non-empty
			// roles for the response, which is what we want to assert.
			UsernameClaim: "preferred_username",
			GroupsClaim:   "groups",
		}},
	))

	req := &AuthRequest{}
	req.Auth.Identity.Methods = []string{"openid"}
	req.Auth.Identity.Federated = &FederationAuthRequest{
		Provider: "kc", Protocol: "openid", Credential: "x.y.z",
	}
	// Default-project scope handled by the provider config; explicit scope omitted.

	resp, token, err := svc.AuthenticateFederated(t.Context(), req)
	require.NoError(t, err)
	require.NotEmpty(t, token)
	require.NotNil(t, resp)

	assert.Equal(t, []string{"openid"}, resp.Token.Methods)
	assert.NotEmpty(t, resp.Token.ExpiresAt)
	assert.NotEmpty(t, resp.Token.IssuedAt)
	assert.NotEmpty(t, resp.Token.AuditIDs)

	// Token must round-trip through ValidateToken — federated tokens are
	// structurally identical to password tokens, so AuthMiddleware works
	// unchanged. This is the central design property of SCS-0300.
	claims, err := svc.ValidateToken(token)
	require.NoError(t, err)
	assert.NotEmpty(t, claims.UserID)
	assert.NotEmpty(t, claims.ProjectID)
}

// TestAuthenticateFederatedRejectsDomainScope mirrors the password-auth
// constraint: domain-scoped tokens are not implemented, federated callers
// must scope to a project (or unscoped).
func TestAuthenticateFederatedRejectsDomainScope(t *testing.T) {
	mock := database.NewSeededMockDB()
	svc := NewAuthServiceWithDB(mock, "test-secret", time.Hour, nil)
	svc.SetFederationRegistry(newFederationRegistryWithProviders(
		map[string]FederationProvider{"kc": &stubProvider{
			name: "kc",
			identity: &FederatedIdentity{
				Issuer:  "https://kc.example.com/realms/scs",
				Subject: "user-sub-123",
			},
		}},
		map[string]FederationProviderConfig{"kc": {
			Name: "kc", Protocol: "openid", AutoProvision: true,
			DefaultProject: "default", DefaultRole: "member",
		}},
	))

	req := &AuthRequest{}
	req.Auth.Identity.Federated = &FederationAuthRequest{
		Provider: "kc", Protocol: "openid", Credential: "x.y.z",
	}
	req.Auth.Scope = &ScopeField{IsDomainScoped: true}

	_, _, err := svc.AuthenticateFederated(t.Context(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "domain-scoped tokens are not supported")
}
