# Keystone Authentication Flow with Horizon

**Last Updated**: 2026-03-13
**O3K Version**: 1.0.0+

## Overview

This document explains how OpenStack Keystone identity service authenticates users and provides service discovery for Horizon dashboard when using O3K as the backend.

## Table of Contents

1. [Authentication Architecture](#authentication-architecture)
2. [JWT Token Structure](#jwt-token-structure)
3. [Service Catalog](#service-catalog)
4. [Multi-Tenancy](#multi-tenancy)
5. [Token Validation](#token-validation)
6. [Security Considerations](#security-considerations)

---

## Authentication Architecture

### Component Roles

```
┌────────────────┐
│    Horizon     │  ← Web UI (Django application)
│   Dashboard    │
└────────┬───────┘
         │
         │ 1. POST /v3/auth/tokens
         │    (username + password)
         ▼
┌────────────────┐
│   Keystone     │  ← Identity Service
│  (Port 35357)  │    - Validates credentials
│                │    - Generates JWT tokens
│                │    - Returns service catalog
└────────┬───────┘
         │
         │ Queries
         ▼
┌────────────────┐
│   PostgreSQL   │  ← User/Project Database
│    Database    │    - users table
│                │    - projects table
│                │    - role_assignments table
└────────────────┘
```

### Authentication Flow (Detailed)

```
Step 1: User Login
──────────────────
User enters credentials in Horizon:
  - Username: admin
  - Password: secret
  - Domain: Default
  - Project: default (optional for scoped token)

Step 2: Horizon → Keystone
───────────────────────────
POST http://o3k:35357/v3/auth/tokens
Content-Type: application/json

{
  "auth": {
    "identity": {
      "methods": ["password"],
      "password": {
        "user": {
          "name": "admin",
          "password": "secret",
          "domain": {"name": "Default"}
        }
      }
    },
    "scope": {
      "project": {
        "name": "default",
        "domain": {"name": "Default"}
      }
    }
  }
}

Step 3: Keystone Validation
────────────────────────────
1. Keystone queries database:
   SELECT * FROM users
   WHERE name = 'admin'
   AND domain_id = (SELECT id FROM domains WHERE name = 'Default')

2. Verifies password hash (bcrypt):
   bcrypt.CompareHashAndPassword(stored_hash, provided_password)

3. Validates project access:
   SELECT project_id FROM role_assignments
   WHERE user_id = <user_id> AND project_id = <project_id>

4. Retrieves user roles:
   SELECT roles.name FROM roles
   JOIN role_assignments ON roles.id = role_assignments.role_id
   WHERE role_assignments.user_id = <user_id>
   AND role_assignments.project_id = <project_id>

Step 4: JWT Token Generation
─────────────────────────────
Keystone generates JWT token with payload:
{
  "user_id": "uuid-of-user",
  "project_id": "uuid-of-project",
  "roles": ["admin", "member"],
  "exp": 1710417600,  // Expiration timestamp (4 hours from now)
  "iat": 1710403200   // Issued at timestamp
}

Signed with: HMAC-SHA256 using jwt_secret from config

Step 5: Service Catalog Construction
─────────────────────────────────────
Keystone queries database for services:

SELECT s.type, s.name, e.interface, e.url, e.region
FROM services s
JOIN endpoints e ON s.id = e.service_id
WHERE e.region = 'RegionOne'

Builds catalog structure:
{
  "catalog": [
    {
      "type": "compute",
      "name": "nova",
      "endpoints": [{
        "interface": "public",
        "region": "RegionOne",
        "url": "http://o3k-host:8774/v2.1"
      }]
    },
    ...repeat for neutron, cinder, glance...
  ]
}

Step 6: Response to Horizon
────────────────────────────
HTTP/1.1 201 Created
X-Subject-Token: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX2lkIjoi...
Content-Type: application/json

{
  "token": {
    "expires_at": "2026-03-13T18:00:00.000000Z",
    "issued_at": "2026-03-13T14:00:00.000000Z",
    "methods": ["password"],
    "user": {
      "id": "uuid-of-user",
      "name": "admin",
      "domain": {"id": "...", "name": "Default"}
    },
    "project": {
      "id": "uuid-of-project",
      "name": "default",
      "domain": {"id": "...", "name": "Default"}
    },
    "roles": [
      {"id": "...", "name": "admin"},
      {"id": "...", "name": "member"}
    ],
    "catalog": [...service endpoints...]
  }
}

Step 7: Horizon Uses Token
───────────────────────────
All subsequent API calls include token:

GET http://o3k:8774/v2.1/servers
X-Auth-Token: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...

Step 8: Token Validation (Every Request)
─────────────────────────────────────────
Nova/Neutron/Cinder/Glance middleware:
1. Extract token from X-Auth-Token header
2. Verify JWT signature using shared jwt_secret
3. Check expiration: time.Now() < token.exp
4. Extract user_id, project_id, roles from payload
5. Authorize operation based on roles
6. Apply project_id filter to database queries
```

---

## JWT Token Structure

### Token Format

O3K uses **JSON Web Tokens (JWT)** for stateless authentication.

**Token Structure**:
```
Header.Payload.Signature
eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX2lkIjoi...
```

### Header

```json
{
  "alg": "HS256",
  "typ": "JWT"
}
```

- **alg**: HMAC-SHA256 signature algorithm
- **typ**: Token type (JWT)

### Payload (Claims)

```json
{
  "user_id": "d7f4c9a1-3b5e-4f1a-8c2d-9e6f1b4a7c3d",
  "project_id": "a1b2c3d4-5e6f-7g8h-9i0j-k1l2m3n4o5p6",
  "roles": ["admin", "member"],
  "user_name": "admin",
  "project_name": "default",
  "domain_id": "default",
  "exp": 1710417600,
  "iat": 1710403200
}
```

**Claims**:
- `user_id`: UUID of authenticated user
- `project_id`: UUID of scoped project
- `roles`: Array of role names assigned to user in project
- `user_name`: Human-readable username (for debugging)
- `project_name`: Human-readable project name (for debugging)
- `domain_id`: Domain identifier
- `exp`: Expiration timestamp (Unix epoch)
- `iat`: Issued at timestamp (Unix epoch)

### Signature

```
HMACSHA256(
  base64UrlEncode(header) + "." + base64UrlEncode(payload),
  jwt_secret
)
```

- **jwt_secret**: Configured in `config/o3k.yaml` (keystone.jwt_secret)
- **⚠️ CRITICAL**: Must be kept secret and changed in production
- **Shared**: All O3K services use the same secret for validation

### Token Lifetime

- **Default TTL**: 4 hours (14400 seconds)
- **Configurable**: `keystone.token_ttl` in O3K config
- **Non-renewable**: Tokens cannot be refreshed (must re-authenticate)
- **Stateless**: No database lookups required for validation

### Token Decoding Example

```bash
# Extract payload from token
TOKEN="eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX2lkIjoi..."
PAYLOAD=$(echo $TOKEN | cut -d '.' -f 2)

# Base64 decode payload
echo $PAYLOAD | base64 --decode | jq '.'
```

Output:
```json
{
  "user_id": "d7f4c9a1-3b5e-4f1a-8c2d-9e6f1b4a7c3d",
  "project_id": "a1b2c3d4-5e6f-7g8h-9i0j-k1l2m3n4o5p6",
  "roles": ["admin", "member"],
  "exp": 1710417600,
  "iat": 1710403200
}
```

---

## Service Catalog

### Purpose

The service catalog tells Horizon where to find each OpenStack service (Nova, Neutron, Cinder, Glance).

### Catalog Structure

```json
{
  "catalog": [
    {
      "type": "identity",
      "name": "keystone",
      "id": "uuid-of-service",
      "endpoints": [
        {
          "id": "uuid-of-endpoint",
          "interface": "public",
          "region": "RegionOne",
          "region_id": "RegionOne",
          "url": "http://o3k-host:35357/v3"
        },
        {
          "id": "uuid-of-endpoint-2",
          "interface": "admin",
          "region": "RegionOne",
          "region_id": "RegionOne",
          "url": "http://o3k-internal:35357/v3"
        }
      ]
    },
    {
      "type": "compute",
      "name": "nova",
      "id": "uuid-of-service",
      "endpoints": [
        {
          "interface": "public",
          "region": "RegionOne",
          "url": "http://o3k-host:8774/v2.1"
        }
      ]
    },
    {
      "type": "network",
      "name": "neutron",
      "endpoints": [
        {
          "interface": "public",
          "region": "RegionOne",
          "url": "http://o3k-host:9696/v2.0"
        }
      ]
    },
    {
      "type": "volumev3",
      "name": "cinderv3",
      "endpoints": [
        {
          "interface": "public",
          "region": "RegionOne",
          "url": "http://o3k-host:8776/v3/%(project_id)s"
        }
      ]
    },
    {
      "type": "image",
      "name": "glance",
      "endpoints": [
        {
          "interface": "public",
          "region": "RegionOne",
          "url": "http://o3k-host:9292/v2"
        }
      ]
    }
  ]
}
```

### Endpoint Interfaces

- **public**: Exposed to end users (Horizon uses this)
- **internal**: Internal service-to-service communication
- **admin**: Administrative operations (not typically used by Horizon)

### URL Templates

Cinder endpoints use URL templates with project_id:
```
http://o3k-host:8776/v3/%(project_id)s
```

Horizon substitutes `%(project_id)s` with actual project UUID from token.

### Service Discovery

Horizon uses catalog to construct API URLs:

1. Parse token response, extract catalog
2. Find service by type (e.g., `type: "compute"`)
3. Filter endpoints by interface (`public`) and region (`RegionOne`)
4. Use endpoint URL for API calls

**Example** (Horizon code):
```python
# Get Nova endpoint from catalog
nova_endpoint = catalog.url_for(
    service_type='compute',
    interface='public',
    region_name='RegionOne'
)
# Result: http://o3k-host:8774/v2.1

# List instances
response = requests.get(
    f"{nova_endpoint}/servers",
    headers={'X-Auth-Token': token}
)
```

### Dynamic Configuration

Service catalog is **dynamic** - no hardcoded URLs in Horizon:
- Services can be added/removed without Horizon changes
- Endpoint URLs can change (load balancer, migration)
- Multi-region deployments supported

---

## Multi-Tenancy

### Project-Based Isolation

Every token is scoped to a **project** (tenant):

```
User: admin
Project: project-a
  ├─ Instances: server-1, server-2
  ├─ Networks: network-a
  └─ Volumes: volume-a

Same User: admin
Project: project-b
  ├─ Instances: server-3
  ├─ Networks: network-b
  └─ Volumes: volume-b

API calls use project_id from token to filter results
```

### Database Queries with project_id Filter

**Example** (Nova list servers):
```sql
SELECT * FROM instances
WHERE project_id = 'uuid-from-token'
ORDER BY created_at DESC;
```

**Result**: User only sees instances in their current project.

### Project Switching

Horizon allows users to switch projects via dropdown:

1. User selects different project in Horizon UI
2. Horizon re-authenticates with new project scope:
   ```json
   {
     "auth": {
       "identity": {"methods": ["token"], "token": {"id": "current-token"}},
       "scope": {"project": {"id": "new-project-id"}}
     }
   }
   ```
3. Keystone returns new token scoped to new project
4. Horizon uses new token for subsequent requests
5. User now sees resources in new project

### Role-Based Access Control (RBAC)

Roles determine what actions users can perform:

| Role | Permissions |
|------|-------------|
| **admin** | Full access: create, read, update, delete all resources; admin-only operations (os-resetState, os-evacuate) |
| **member** | Standard access: create, read, update, delete own resources; cannot perform admin operations |
| **reader** | Read-only: can view resources but not modify |

**Role Check Example** (Nova os-resetState):
```go
roles := c.GetStringSlice("roles")
isAdmin := false
for _, role := range roles {
    if role == "admin" {
        isAdmin = true
        break
    }
}
if !isAdmin {
    return 403 Forbidden
}
```

---

## Token Validation

### Middleware Flow

Every API request goes through authentication middleware:

```
HTTP Request
    │
    ├─ Extract X-Auth-Token header
    │
    ├─ Validate JWT signature
    │  ├─ Decode token
    │  ├─ Verify signature with jwt_secret
    │  └─ If invalid: return 401 Unauthorized
    │
    ├─ Check expiration
    │  ├─ Compare token.exp with time.Now()
    │  └─ If expired: return 401 Unauthorized
    │
    ├─ Extract claims
    │  ├─ user_id
    │  ├─ project_id
    │  └─ roles
    │
    ├─ Set context variables
    │  ├─ c.Set("user_id", token.UserID)
    │  ├─ c.Set("project_id", token.ProjectID)
    │  └─ c.Set("roles", token.Roles)
    │
    └─ Continue to handler
        └─ Handler uses context values for authorization
```

### Implementation (Go)

```go
// middleware/auth.go
func AuthMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        tokenString := c.GetHeader("X-Auth-Token")
        if tokenString == "" {
            c.JSON(401, gin.H{"error": "missing authentication token"})
            c.Abort()
            return
        }

        // Parse and validate JWT token
        token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
            return []byte(jwtSecret), nil
        })

        if err != nil || !token.Valid {
            c.JSON(401, gin.H{"error": "invalid token"})
            c.Abort()
            return
        }

        claims := token.Claims.(*Claims)

        // Check expiration
        if time.Now().Unix() > claims.ExpiresAt {
            c.JSON(401, gin.H{"error": "token expired"})
            c.Abort()
            return
        }

        // Set context variables for handlers
        c.Set("user_id", claims.UserID)
        c.Set("project_id", claims.ProjectID)
        c.Set("roles", claims.Roles)

        c.Next()
    }
}
```

### Validation Performance

- **No database queries**: JWT validation is stateless
- **Signature verification**: ~0.1ms per request
- **Scalable**: Handles thousands of requests per second
- **Cached**: `jwt_secret` loaded once at startup

---

## Security Considerations

### JWT Secret Management

**⚠️ CRITICAL SECURITY REQUIREMENT**:

1. **Never commit jwt_secret to version control**
   - Use environment variables or secret management systems
   - Rotate periodically (requires invalidating all existing tokens)

2. **Use strong random secret**
   ```bash
   # Generate secure secret
   openssl rand -base64 32
   ```

3. **Share secret across all O3K services**
   - All services must use the same secret for validation
   - Store in centralized configuration (HashiCorp Vault, AWS Secrets Manager)

### Token Storage

Horizon stores tokens in:
- **Session cookies** (encrypted, HTTP-only)
- **Browser session storage** (for SPA mode)

**Security Best Practices**:
- Enable `SESSION_COOKIE_SECURE = True` (requires HTTPS)
- Set `SESSION_COOKIE_HTTPONLY = True` (prevent XSS)
- Use `CSRF_COOKIE_SECURE = True` (HTTPS only)

### Token Expiration

**Why 4-hour TTL?**
- **Balance**: Security vs user convenience
- **Too short**: Frequent re-authentication annoys users
- **Too long**: Extended exposure window if token leaked

**Recommendations**:
- **Development**: 24 hours (86400s) for convenience
- **Production**: 4 hours (14400s) standard
- **High-security**: 1 hour (3600s) with MFA

### Token Revocation

JWT tokens are **stateless** - cannot be revoked before expiration.

**Mitigation Strategies**:
1. **Short TTL**: Limits exposure window
2. **Secret rotation**: Invalidates all tokens (disruptive)
3. **Blacklist** (future): Maintain revoked token IDs in Redis

### Password Security

- **Hashing**: bcrypt with cost 10+
- **Never logged**: Passwords never appear in logs
- **Transmission**: HTTPS required for production
- **Storage**: Only bcrypt hash stored, never plaintext

### HTTPS/TLS

**Production Requirement**: All traffic must use HTTPS:

- **Horizon**: TLS termination at load balancer/nginx
- **O3K API**: TLS termination at reverse proxy
- **Inter-service**: Can use HTTP if on trusted private network

**Certificate Management**:
- Use Let's Encrypt for free certificates
- Automated renewal (certbot)
- TLS 1.2+ only (disable TLS 1.0/1.1)

---

## Troubleshooting Authentication

### Common Issues

#### 1. "Invalid token" Error

**Symptoms**: API returns 401 with "invalid token" message

**Causes**:
- Token expired
- jwt_secret mismatch between services
- Token corrupted (copy/paste error)

**Debug**:
```bash
# Decode token to check expiration
TOKEN="eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
echo $TOKEN | cut -d '.' -f 2 | base64 --decode | jq '.exp'

# Compare with current time
date +%s
```

#### 2. "Forbidden" Error (403)

**Symptoms**: Operation denied despite valid token

**Causes**:
- User lacks required role (e.g., non-admin trying os-resetState)
- Project mismatch (trying to access resource in different project)

**Debug**:
```bash
# Check token roles
echo $TOKEN | cut -d '.' -f 2 | base64 --decode | jq '.roles'

# Verify project_id matches resource
openstack server show <instance-id> -c project_id
```

#### 3. "Token has expired"

**Symptoms**: Horizon forces re-login after short time

**Cause**: Token TTL too short

**Solution**: Increase token_ttl in O3K config and match Horizon SESSION_TIMEOUT

---

## Implementation Reference

### Database Schema

**users table**:
```sql
CREATE TABLE users (
    id UUID PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    password_hash TEXT NOT NULL,  -- bcrypt hash
    domain_id UUID REFERENCES domains(id),
    enabled BOOLEAN DEFAULT true,
    created_at TIMESTAMP
);
```

**role_assignments table**:
```sql
CREATE TABLE role_assignments (
    user_id UUID REFERENCES users(id),
    project_id UUID REFERENCES projects(id),
    role_id UUID REFERENCES roles(id),
    PRIMARY KEY (user_id, project_id, role_id)
);
```

### Token Generation Code

```go
// internal/keystone/auth.go
func generateToken(user User, project Project, roles []string) (string, error) {
    expiresAt := time.Now().Add(time.Duration(tokenTTL) * time.Second)

    claims := &Claims{
        UserID:      user.ID,
        ProjectID:   project.ID,
        Roles:       roles,
        UserName:    user.Name,
        ProjectName: project.Name,
        ExpiresAt:   expiresAt.Unix(),
        IssuedAt:    time.Now().Unix(),
    }

    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    return token.SignedString([]byte(jwtSecret))
}
```

---

## References

- **JWT Specification**: https://tools.ietf.org/html/rfc7519
- **OpenStack Keystone API**: https://docs.openstack.org/api-ref/identity/v3/
- **OpenStack Auth Tokens**: https://docs.openstack.org/keystone/latest/admin/tokens-overview.html
- **O3K Source Code**: `/internal/keystone/` and `/internal/middleware/`

---

**Version**: 1.0.0
**Last Updated**: 2026-03-13
**Authors**: O3K Development Team
