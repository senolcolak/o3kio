// Package keystone — SCS-0300-v1 SSO identity federation.
//
// Federation lets a Keystone client authenticate against an external IdP
// (Keycloak, Zitadel, Auth0, …) and receive a normal O3K JWT scoped to a
// project. The token is structurally identical to a password-issued token —
// existing AuthMiddleware works unchanged — only the `methods` claim differs
// (`["openid"]` for browser flows, `["mapped"]` for CLI flows).
//
// The minimum surface for v1 is OIDC. LDAP and OAuth2-direct will follow as
// separate slices. SCS-0300-v1 itself does not mandate protocol coverage; the
// "Conformance Tests" section is OPTIONAL/empty in the published v1.
//
// API surface mirrored from the IAM-federation reference (Keystone+Keycloak):
//
//	POST  /v3/auth/OS-FEDERATION/identity_providers/<idp>/protocols/openid/websso
//	      browser flow — Authorization Code grant, ends with set-cookie + redirect
//	POST  /v3/OS-FEDERATION/identity_providers/<idp>/protocols/mapped/auth
//	      CLI flow — caller presents an OAuth2 bearer, server verifies via JWKS
//	POST  /v3/auth/tokens
//	      CLI flow — `identity.methods=["openid"]`, body contains the IdP token
//
// JIT user provisioning is on by default (provider config flag
// `auto_provision: false` to disable). On first federated login we
// deterministically derive the O3K user UUID from (issuer, subject) using
// deterministicUUID() from auth.go so that re-logins find the same user.
//
// This file is the Phase-2 scaffold — interface, types, and stubbed adapter.
// Phase 3 fills in the OIDC verification and provisioning logic.
package keystone

import (
	"context"
	"errors"
	"fmt"
)

// FederationProvider is the contract a federated IdP adapter satisfies. One
// concrete implementation (OIDCProvider) ships in v1; LDAP/OAuth2-direct
// providers can be added without changing this interface.
type FederationProvider interface {
	// Name returns the provider identifier used in URLs and config
	// (e.g. "keycloak-prod", "zitadel-staging").
	Name() string

	// Verify validates a credential presented by the caller (typically an
	// OIDC ID token or an OAuth2 access token) and returns the verified
	// identity claims. The credential format depends on the protocol:
	//   - openid: a signed JWT ID token
	//   - mapped: a bearer access token, verified against IdP JWKS
	// Returns an error if the credential is invalid, expired, or the
	// signature cannot be verified.
	Verify(ctx context.Context, credential string) (*FederatedIdentity, error)
}

// FederatedIdentity is the verified output of a successful credential check.
// All fields are populated from IdP claims; mapping to O3K users/projects/
// roles happens in the AuthService layer, not here.
type FederatedIdentity struct {
	// Issuer is the IdP issuer URL (`iss` claim). Combined with Subject it
	// forms the stable cross-session identity of the federated user.
	Issuer string

	// Subject is the IdP-side stable user identifier (`sub` claim). Never
	// reuse for display — it is opaque.
	Subject string

	// Email is the user's email address, used for display and as a
	// fallback username when the IdP doesn't supply `preferred_username`.
	Email string

	// PreferredUsername is the IdP-supplied username (`preferred_username`
	// claim in OIDC). Empty if the IdP doesn't set it.
	PreferredUsername string

	// Groups is the list of IdP group memberships. Used by the role-mapping
	// table to translate IdP groups into O3K role assignments.
	Groups []string

	// Raw is the verified claim set, retained for audit logging and for
	// custom claim mappings beyond the standard fields above.
	Raw map[string]any
}

// FederationProviderConfig is the per-provider configuration loaded from
// `keystone.federation.providers` in o3k.yaml. One config produces one
// FederationProvider instance at startup.
//
// Schema:
//
//	keystone:
//	  federation:
//	    enabled: true
//	    providers:
//	      - name: keycloak-prod
//	        protocol: openid
//	        issuer: https://keycloak.example.com/realms/scs
//	        client_id: o3k
//	        client_secret: ${KEYCLOAK_CLIENT_SECRET}
//	        auto_provision: true
//	        username_claim: preferred_username
//	        groups_claim: groups
//	        default_project: default
//	        default_role: member
type FederationProviderConfig struct {
	// Name is the provider identifier — used in URLs and as a foreign key
	// in federation_role_mappings. Must be unique. Lowercase letters,
	// digits, and hyphens only (validated at config load).
	Name string `yaml:"name"`

	// Protocol selects the adapter. v1 ships "openid"; "ldap" and "oauth2"
	// are reserved for future slices.
	Protocol string `yaml:"protocol"`

	// Issuer is the IdP issuer URL (used for OIDC discovery). Required
	// when Protocol == "openid".
	Issuer string `yaml:"issuer"`

	// ClientID and ClientSecret are the OAuth2 client credentials O3K uses
	// when redeeming Authorization Code grants. Browser/SSO flow only.
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`

	// AutoProvision controls JIT user creation. When true (the default),
	// a federated login for a previously-unknown subject creates an O3K
	// user record on the fly. When false, the login is rejected and an
	// admin must pre-provision the user.
	AutoProvision bool `yaml:"auto_provision"`

	// UsernameClaim names the OIDC claim used as the O3K username.
	// Defaults to "preferred_username"; "email" is a common alternative.
	UsernameClaim string `yaml:"username_claim"`

	// GroupsClaim names the OIDC claim that holds group memberships.
	// Defaults to "groups". The claim must be a JSON array of strings.
	GroupsClaim string `yaml:"groups_claim"`

	// DefaultProject is the project a JIT-provisioned user is scoped to
	// when no project mapping rule fires. Required when AutoProvision is
	// true.
	DefaultProject string `yaml:"default_project"`

	// DefaultRole is the role granted to a JIT-provisioned user on the
	// default project. "member" is the typical choice.
	DefaultRole string `yaml:"default_role"`
}

// FederationConfig is the top-level federation block on KeystoneConfig.
// Disabled by default — `enabled: false` and an empty providers list mean
// the federation routes are not even registered.
type FederationConfig struct {
	Enabled   bool                       `yaml:"enabled"`
	Providers []FederationProviderConfig `yaml:"providers"`
}

// FederationAuthRequest is the body shape for the federated leg of
// /v3/auth/tokens. Mirrors the password/token/application_credential pattern
// in AuthRequest.Auth.Identity.
//
//	{
//	  "auth": {
//	    "identity": {
//	      "methods": ["openid"],
//	      "federated": {
//	        "provider": "keycloak-prod",
//	        "protocol": "openid",
//	        "credential": "<id token or access token>"
//	      }
//	    },
//	    "scope": { "project": { "name": "default", "domain": {"name": "Default"} } }
//	  }
//	}
type FederationAuthRequest struct {
	Provider   string `json:"provider"`
	Protocol   string `json:"protocol"`
	Credential string `json:"credential"`
}

// ErrUnknownProvider is returned when a federated auth request names a
// provider that isn't configured. Distinct from credential failures so the
// API can return 400 vs 401 appropriately.
var ErrUnknownProvider = errors.New("unknown federation provider")

// ErrFederationDisabled is returned by federation entry points when the
// keystone.federation.enabled flag is false.
var ErrFederationDisabled = errors.New("federation disabled")

// FederationRegistry holds the set of configured providers, keyed by Name.
// Constructed once at startup from FederationConfig.Providers and passed
// into AuthService. Read-only after construction — adding a provider
// requires a restart, which matches the operational model for IdP trust.
//
// Configs are stored alongside providers so the auth path can read
// per-provider knobs (AutoProvision, DefaultProject, DefaultRole) without
// the adapter having to surface them.
type FederationRegistry struct {
	providers map[string]FederationProvider
	configs   map[string]FederationProviderConfig
}

// NewFederationRegistry builds a registry from a slice of provider configs.
// Each config triggers protocol-specific adapter construction (OIDC discovery
// for "openid"). Returns an error if any provider fails to construct so a
// half-configured deployment can't silently accept logins from a working
// subset of IdPs.
func NewFederationRegistry(ctx context.Context, configs []FederationProviderConfig) (*FederationRegistry, error) {
	r := &FederationRegistry{
		providers: make(map[string]FederationProvider, len(configs)),
		configs:   make(map[string]FederationProviderConfig, len(configs)),
	}
	for _, cfg := range configs {
		if _, dup := r.providers[cfg.Name]; dup {
			return nil, fmt.Errorf("federation provider %q: duplicate name", cfg.Name)
		}
		switch cfg.Protocol {
		case "openid", "":
			// Empty protocol defaults to openid for v1 — the only adapter
			// shipped today. ldap/oauth2 land in later slices.
			p, err := NewOIDCProvider(ctx, cfg)
			if err != nil {
				return nil, err
			}
			r.providers[cfg.Name] = p
			r.configs[cfg.Name] = cfg
		default:
			return nil, fmt.Errorf("federation provider %q: unsupported protocol %q (only \"openid\" is supported in v1)", cfg.Name, cfg.Protocol)
		}
	}
	return r, nil
}

// Provider looks up a provider by name. Returns ErrUnknownProvider if the
// name isn't configured.
func (r *FederationRegistry) Provider(name string) (FederationProvider, error) {
	p, ok := r.providers[name]
	if !ok {
		return nil, ErrUnknownProvider
	}
	return p, nil
}

// config returns the per-provider configuration. Used by the auth path to
// read AutoProvision, DefaultProject, DefaultRole knobs without exposing
// them through the FederationProvider interface (which is concerned with
// credential verification, not provisioning policy).
func (r *FederationRegistry) config(name string) (FederationProviderConfig, bool) {
	c, ok := r.configs[name]
	return c, ok
}

// Names returns the set of configured provider names. Used by the federation
// discovery endpoint (`GET /v3/OS-FEDERATION/identity_providers`).
func (r *FederationRegistry) Names() []string {
	out := make([]string, 0, len(r.providers))
	for name := range r.providers {
		out = append(out, name)
	}
	return out
}

// newFederationRegistryWithProviders is a test seam that wires pre-built
// providers into a registry without going through NewOIDCProvider's real
// OIDC discovery. Production callers always go through NewFederationRegistry.
func newFederationRegistryWithProviders(providers map[string]FederationProvider, configs map[string]FederationProviderConfig) *FederationRegistry {
	return &FederationRegistry{providers: providers, configs: configs}
}

// FederationConfigFromYAML converts the YAML-loaded provider list (defined in
// internal/common to avoid an import cycle) into the keystone-typed shape
// consumed by NewFederationRegistry. The two struct shapes are field-for-field
// identical; this helper exists only to bridge the package boundary.
//
// `providers` is the raw slice from common.KeystoneFederationYAML.Providers.
// We accept []any here so the caller in cmd/o3k/main.go can pass the slice
// without us importing common (which would re-introduce the cycle in the
// other direction once Phase 3 grows). The caller is responsible for mapping
// each entry's fields into a FederationProviderConfig.
func FederationConfigFromYAML(yaml []FederationProviderConfig) []FederationProviderConfig {
	// Pass-through today — Phase 3 may apply defaults (e.g. UsernameClaim
	// fallback to "preferred_username") here so they live in one place.
	out := make([]FederationProviderConfig, len(yaml))
	for i, p := range yaml {
		if p.UsernameClaim == "" {
			p.UsernameClaim = "preferred_username"
		}
		if p.GroupsClaim == "" {
			p.GroupsClaim = "groups"
		}
		out[i] = p
	}
	return out
}
