# SPEC-002: Enhanced Authentication System

**Status**: Draft
**Version**: 1.0
**Created**: 2026-03-09
**Priority**: Critical
**Depends On**: SPEC-001 (Modular Architecture)

## Overview

Enhance Keystone authentication to support OAuth2, OpenID Connect (OIDC), SAML, LDAP/Active Directory, application credentials, and token revocation lists. Maintain 100% backwards compatibility with existing JWT token system and openstack-client CLI.

## Goals

1. **OAuth2/OIDC Support**: Industry-standard authentication for APIs
2. **SAML Integration**: Enterprise SSO integration
3. **LDAP/AD Support**: Corporate directory integration
4. **Application Credentials**: Long-lived credentials for automation/CI/CD
5. **Token Revocation**: Active blacklist for compromised tokens
6. **Multi-Factor Authentication**: Optional 2FA/MFA support
7. **Backward Compatibility**: Existing JWT tokens continue to work

## Non-Goals

- Custom SSO provider (use standard protocols)
- Biometric authentication (future phase)
- Hardware token support (future phase)
- Federation v3.0 (keep existing federation)

## Current Authentication Flow

```
POST /v3/auth/tokens
{
  "auth": {
    "identity": {
      "methods": ["password"],
      "password": {
        "user": {"name": "admin", "password": "secret", "domain": {"id": "default"}}
      }
    },
    "scope": {"project": {"name": "default", "domain": {"id": "default"}}}
  }
}

Response:
X-Subject-Token: JWT (HMAC-SHA256 signed)
{
  "token": {
    "user": {...},
    "project": {...},
    "roles": [...],
    "catalog": [...]
  }
}
```

**Implementation**:
- Password hashed with bcrypt (cost 10)
- Tokens are stateless JWT (no database lookup)
- 24-hour TTL (configurable)
- No revocation list

## Enhanced Authentication Flow

### OAuth2 / OIDC Flow

```
1. Client redirects to: GET /v3/auth/oauth2/authorize
2. User authenticates with OAuth provider (Google, GitHub, Azure AD)
3. Provider redirects back with authorization code
4. Client exchanges code: POST /v3/auth/oauth2/token
5. Keystone validates with provider, creates OpenStack token
6. Return JWT token + service catalog
```

**Providers to Support**:
- Google (OIDC)
- GitHub (OAuth2)
- Azure AD (OIDC)
- Keycloak (OIDC)
- Generic OIDC provider

**Configuration**:
```yaml
keystone:
  oauth2:
    enabled: true
    providers:
      - name: google
        client_id: xxx
        client_secret: yyy
        issuer: https://accounts.google.com
        redirect_uri: http://localhost:35357/v3/auth/oauth2/callback
      - name: github
        client_id: xxx
        client_secret: yyy
        authorize_url: https://github.com/login/oauth/authorize
        token_url: https://github.com/login/oauth/access_token
```

### SAML Integration

```
1. Client redirects to: GET /v3/auth/saml/login
2. Keystone redirects to IdP (SAML AuthnRequest)
3. User authenticates with IdP
4. IdP posts SAML assertion to: POST /v3/auth/saml/acs
5. Keystone validates assertion, creates OpenStack token
6. Return JWT token + service catalog
```

**IdP Support**:
- Okta
- OneLogin
- Azure AD SAML
- ADFS
- Generic SAML 2.0

**Configuration**:
```yaml
keystone:
  saml:
    enabled: true
    idp_metadata_url: https://idp.example.com/metadata
    sp_entity_id: http://localhost:35357/v3/auth/saml/metadata
    assertion_consumer_service: http://localhost:35357/v3/auth/saml/acs
    attribute_mapping:
      email: http://schemas.xmlsoap.org/ws/2005/05/identity/claims/emailaddress
      name: http://schemas.xmlsoap.org/ws/2005/05/identity/claims/name
```

### LDAP / Active Directory

```
1. Client authenticates: POST /v3/auth/tokens (method: ldap)
2. Keystone binds to LDAP with user credentials
3. If bind succeeds, search for user attributes
4. Create or update user in local database
5. Return JWT token + service catalog
```

**Configuration**:
```yaml
keystone:
  ldap:
    enabled: true
    url: ldap://ldap.example.com:389
    bind_dn: cn=admin,dc=example,dc=com
    bind_password: secret
    user_tree_dn: ou=Users,dc=example,dc=com
    user_objectclass: inetOrgPerson
    user_id_attribute: uid
    user_name_attribute: cn
    user_mail_attribute: mail
    group_tree_dn: ou=Groups,dc=example,dc=com
    group_objectclass: groupOfNames
    group_member_attribute: member
```

### Application Credentials

Long-lived credentials for automation:

```
POST /v3/users/{user_id}/application_credentials
{
  "application_credential": {
    "name": "ci-pipeline",
    "description": "CI/CD automation",
    "expires_at": "2027-03-09T00:00:00Z",
    "roles": [{"name": "member"}],
    "unrestricted": false
  }
}

Response:
{
  "application_credential": {
    "id": "abc123",
    "name": "ci-pipeline",
    "secret": "generated-secret",  # Only returned once
    "expires_at": "2027-03-09T00:00:00Z"
  }
}

# Use in auth:
POST /v3/auth/tokens
{
  "auth": {
    "identity": {
      "methods": ["application_credential"],
      "application_credential": {
        "id": "abc123",
        "secret": "generated-secret"
      }
    }
  }
}
```

### Token Revocation List

Active blacklist for compromised tokens:

```
# Revoke token
DELETE /v3/auth/tokens
X-Auth-Token: <token-to-revoke>
X-Subject-Token: <token-to-revoke>

# Check revocation
GET /v3/auth/tokens/OS-REVOKE/events

# Internal validation
1. Validate JWT signature (fast)
2. Check revocation list (Redis cache, 1ms lookup)
3. If in revocation list → 401 Unauthorized
```

**Storage**:
- Redis for fast lookups (in-memory)
- PostgreSQL for persistence
- Automatic cleanup of expired entries

### Multi-Factor Authentication (Optional)

```
1. User authenticates with password: POST /v3/auth/tokens
2. If MFA required for user:
   Response: 401 + {"error": "mfa_required", "challenge_id": "xxx"}
3. Client prompts for MFA code
4. Client submits: POST /v3/auth/tokens/mfa
   {"challenge_id": "xxx", "code": "123456"}
5. If valid, return token
```

**MFA Methods**:
- TOTP (Time-based One-Time Password)
- SMS (via Twilio integration)
- Email codes
- Hardware tokens (WebAuthn - future)

## Database Schema Changes

### New Tables

```sql
-- OAuth2 providers
CREATE TABLE oauth2_providers (
    id UUID PRIMARY KEY,
    name VARCHAR(255) UNIQUE NOT NULL,
    client_id VARCHAR(255) NOT NULL,
    client_secret_encrypted TEXT NOT NULL,
    issuer VARCHAR(255),
    authorize_url VARCHAR(255),
    token_url VARCHAR(255),
    userinfo_url VARCHAR(255),
    enabled BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT NOW()
);

-- Application credentials
CREATE TABLE application_credentials (
    id UUID PRIMARY KEY,
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    secret_hash TEXT NOT NULL,  -- bcrypt
    description TEXT,
    expires_at TIMESTAMP,
    unrestricted BOOLEAN DEFAULT false,
    project_id UUID REFERENCES projects(id),
    created_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(user_id, name)
);

CREATE TABLE application_credential_roles (
    credential_id UUID REFERENCES application_credentials(id) ON DELETE CASCADE,
    role_id UUID REFERENCES roles(id) ON DELETE CASCADE,
    PRIMARY KEY (credential_id, role_id)
);

-- Token revocation
CREATE TABLE revoked_tokens (
    token_id VARCHAR(255) PRIMARY KEY,  -- JWT 'jti' claim
    revoked_at TIMESTAMP DEFAULT NOW(),
    expires_at TIMESTAMP NOT NULL,  -- For auto-cleanup
    reason VARCHAR(255)
);

CREATE INDEX idx_revoked_tokens_expires ON revoked_tokens(expires_at);

-- LDAP user mapping
CREATE TABLE ldap_users (
    id UUID PRIMARY KEY,
    user_id UUID REFERENCES users(id) ON DELETE CASCADE UNIQUE,
    ldap_dn TEXT NOT NULL UNIQUE,
    last_sync TIMESTAMP DEFAULT NOW()
);

-- MFA secrets
CREATE TABLE mfa_secrets (
    id UUID PRIMARY KEY,
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    method VARCHAR(50) NOT NULL,  -- totp, sms, email
    secret_encrypted TEXT NOT NULL,
    enabled BOOLEAN DEFAULT false,
    created_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(user_id, method)
);
```

## API Endpoints

### OAuth2
- `GET /v3/auth/oauth2/providers` - List configured providers
- `GET /v3/auth/oauth2/authorize?provider=google` - Start OAuth flow
- `GET /v3/auth/oauth2/callback` - OAuth callback handler
- `POST /v3/auth/oauth2/token` - Exchange authorization code

### SAML
- `GET /v3/auth/saml/metadata` - Service provider metadata
- `GET /v3/auth/saml/login?idp=okta` - Start SAML flow
- `POST /v3/auth/saml/acs` - Assertion consumer service
- `GET /v3/auth/saml/logout` - SAML logout

### Application Credentials
- `POST /v3/users/{user_id}/application_credentials` - Create
- `GET /v3/users/{user_id}/application_credentials` - List
- `GET /v3/users/{user_id}/application_credentials/{id}` - Get
- `DELETE /v3/users/{user_id}/application_credentials/{id}` - Delete

### Token Revocation
- `DELETE /v3/auth/tokens` - Revoke token (existing, enhanced)
- `GET /v3/auth/revocations` - List revocation events

### MFA
- `POST /v3/users/{user_id}/mfa/totp` - Enable TOTP
- `POST /v3/users/{user_id}/mfa/verify` - Verify MFA setup
- `DELETE /v3/users/{user_id}/mfa/{method}` - Disable MFA
- `POST /v3/auth/tokens/mfa` - Submit MFA challenge response

## Implementation Strategy

### Library Dependencies

```go
// OAuth2/OIDC
"golang.org/x/oauth2"
"github.com/coreos/go-oidc/v3/oidc"

// SAML
"github.com/crewjam/saml"

// LDAP
"github.com/go-ldap/ldap/v3"

// TOTP
"github.com/pquerna/otp"

// Redis (revocation list)
"github.com/redis/go-redis/v9"
```

### Configuration Validation

```go
type AuthConfig struct {
    Password PasswordConfig `yaml:"password"`
    OAuth2   OAuth2Config   `yaml:"oauth2"`
    SAML     SAMLConfig     `yaml:"saml"`
    LDAP     LDAPConfig     `yaml:"ldap"`
    MFA      MFAConfig      `yaml:"mfa"`
    Revocation RevocationConfig `yaml:"revocation"`
}

func (c *AuthConfig) Validate() error {
    // At least one auth method must be enabled
    // OAuth2 requires client_id + client_secret
    // SAML requires IdP metadata
    // LDAP requires bind credentials
}
```

### Backward Compatibility

Existing JWT tokens continue to work:
1. JWT secret remains same
2. Token validation checks signature first
3. New tokens include `jti` claim for revocation
4. Old tokens without `jti` cannot be revoked (log warning)

## Testing Strategy

### Unit Tests
- OAuth2 token exchange
- SAML assertion parsing
- LDAP bind and search
- Application credential generation
- Revocation list operations
- MFA code validation

### Integration Tests
- Full OAuth2 flow with mock provider
- SAML flow with test IdP
- LDAP authentication with test server
- Token revocation end-to-end
- openstack-client compatibility

### Contract Tests
```bash
# Test password auth (existing)
openstack token issue

# Test application credential auth
export OS_AUTH_TYPE=v3applicationcredential
export OS_APPLICATION_CREDENTIAL_ID=xxx
export OS_APPLICATION_CREDENTIAL_SECRET=yyy
openstack token issue

# Test OAuth2 (via browser)
openstack token issue --os-auth-type v3oidc --os-identity-provider google
```

## Migration Path

### Phase 1: Application Credentials (Week 1)
- Simplest, no external dependencies
- Database schema + API endpoints
- CLI integration
- Contract tests

### Phase 2: Token Revocation (Week 2)
- Redis integration
- Revocation API
- Validation middleware update
- Load testing

### Phase 3: LDAP/AD (Week 3-4)
- LDAP bind authentication
- User/group synchronization
- Role mapping
- Integration tests

### Phase 4: OAuth2/OIDC (Week 5-6)
- Provider configuration
- Authorization flow
- Token exchange
- Multiple provider support

### Phase 5: SAML (Week 7-8)
- Service provider setup
- IdP integration
- Assertion parsing
- Metadata generation

### Phase 6: MFA (Week 9-10)
- TOTP implementation
- MFA enrollment flow
- Challenge-response API
- Recovery codes

## Success Criteria

- [ ] openstack-client works with all auth methods
- [ ] Horizon dashboard supports OAuth2/SAML login
- [ ] LDAP users can authenticate
- [ ] Application credentials work in CI/CD pipelines
- [ ] Token revocation is fast (< 5ms overhead)
- [ ] MFA enrollment and verification work
- [ ] Backward compatibility: existing JWT tokens work
- [ ] No breaking API changes
- [ ] Security audit passes
- [ ] Documentation complete

## Security Considerations

1. **Secret Storage**: Encrypt OAuth2 client secrets, MFA secrets
2. **Rate Limiting**: Auth endpoints must have rate limiting
3. **Audit Logging**: All auth attempts logged
4. **Token Rotation**: Recommend short-lived tokens with refresh
5. **SAML Signature Validation**: Always validate SAML assertions
6. **OAuth2 State Parameter**: CSRF protection required
7. **LDAP Injection**: Sanitize all LDAP queries
8. **Revocation List Sync**: Redis-PostgreSQL consistency

## Performance Targets

- Token validation: < 5ms (with revocation check)
- OAuth2 token exchange: < 500ms
- SAML assertion validation: < 100ms
- LDAP bind: < 200ms
- Application credential auth: < 10ms

## References

- OpenStack Keystone v3 API
- RFC 6749 (OAuth 2.0)
- RFC 6750 (OAuth 2.0 Bearer Tokens)
- OpenID Connect Core 1.0
- SAML 2.0 Web Browser SSO Profile
- RFC 4511 (LDAP)
- RFC 6238 (TOTP)
