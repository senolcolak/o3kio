// Package keystone — OIDC adapter for SCS-0300-v1 federated identity.
//
// OIDCProvider satisfies FederationProvider for IdPs that speak OpenID
// Connect (Keycloak, Zitadel, Auth0, Okta, etc.). It uses OIDC discovery
// at construction time to fetch the JWKS endpoint and supported algorithms,
// then verifies presented ID tokens against that JWKS. JWKS rotation is
// handled by go-oidc internally.
package keystone

import (
	"context"
	"errors"
	"fmt"

	"github.com/coreos/go-oidc/v3/oidc"
)

// OIDCProvider verifies OIDC ID tokens for one configured IdP.
type OIDCProvider struct {
	name     string
	cfg      FederationProviderConfig
	verifier *oidc.IDTokenVerifier
}

// NewOIDCProvider performs OIDC discovery against cfg.Issuer and builds a
// verifier bound to cfg.ClientID (audience). Returns an error if discovery
// fails or required fields are missing.
func NewOIDCProvider(ctx context.Context, cfg FederationProviderConfig) (*OIDCProvider, error) {
	if cfg.Name == "" {
		return nil, errors.New("federation provider: name is required")
	}
	if cfg.Issuer == "" {
		return nil, fmt.Errorf("federation provider %q: issuer is required for openid protocol", cfg.Name)
	}
	if cfg.ClientID == "" {
		return nil, fmt.Errorf("federation provider %q: client_id is required for openid protocol", cfg.Name)
	}

	provider, err := oidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("federation provider %q: OIDC discovery failed: %w", cfg.Name, err)
	}

	verifier := provider.Verifier(&oidc.Config{ClientID: cfg.ClientID})

	return &OIDCProvider{
		name:     cfg.Name,
		cfg:      cfg,
		verifier: verifier,
	}, nil
}

// Name returns the configured provider identifier.
func (p *OIDCProvider) Name() string { return p.name }

// Verify validates an OIDC ID token against the IdP's JWKS and returns the
// extracted identity claims. The credential is the raw compact-serialized
// JWT presented by the caller.
func (p *OIDCProvider) Verify(ctx context.Context, credential string) (*FederatedIdentity, error) {
	if credential == "" {
		return nil, errors.New("federation: empty credential")
	}

	idToken, err := p.verifier.Verify(ctx, credential)
	if err != nil {
		return nil, fmt.Errorf("federation provider %q: ID token verification failed: %w", p.name, err)
	}

	var raw map[string]any
	if err := idToken.Claims(&raw); err != nil {
		return nil, fmt.Errorf("federation provider %q: failed to decode claims: %w", p.name, err)
	}

	id := &FederatedIdentity{
		Issuer:  idToken.Issuer,
		Subject: idToken.Subject,
		Raw:     raw,
	}

	// Email is a standard OIDC claim.
	if v, ok := raw["email"].(string); ok {
		id.Email = v
	}

	// Username comes from the configured claim, defaulting to
	// preferred_username (handled by FederationConfigFromYAML).
	usernameClaim := p.cfg.UsernameClaim
	if usernameClaim == "" {
		usernameClaim = "preferred_username"
	}
	if v, ok := raw[usernameClaim].(string); ok {
		id.PreferredUsername = v
	}

	// Groups come from the configured claim, defaulting to "groups". The
	// claim must be a JSON array of strings; mixed-type arrays are skipped
	// element-by-element so a malformed claim doesn't abort the whole login.
	groupsClaim := p.cfg.GroupsClaim
	if groupsClaim == "" {
		groupsClaim = "groups"
	}
	if v, ok := raw[groupsClaim].([]any); ok {
		for _, g := range v {
			if s, ok := g.(string); ok {
				id.Groups = append(id.Groups, s)
			}
		}
	}

	return id, nil
}
