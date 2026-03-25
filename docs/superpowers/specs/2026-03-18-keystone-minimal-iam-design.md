# Keystone Minimal IAM Enhancement - Design Specification

**Date**: 2026-03-18
**Status**: Draft
**Version**: 1.0
**Scope**: Application Credentials + Policy Engine
**Timeline**: 2 weeks (10 working days)

---

## Executive Summary

This specification defines a minimal IAM enhancement for O3K Keystone that adds two critical capabilities:

1. **Application Credentials**: Long-lived API keys for CI/CD automation with fine-grained access control
2. **Policy Engine**: Resource-level authorization beyond simple role-based access control

**Design Philosophy**: Deliver 80% of enterprise IAM value with 25% of the complexity. No federation, no OAuth2 server, no external IdP integration - just pragmatic solutions to real automation and authorization problems.

**Timeline**: 3 weeks (15 working days)
**Database Tables**: Enhanced 1 existing + 3 new = 4 total
**API Endpoints**: Enhanced 5 existing + 3 new = 8 total
**Migrations**: +4 (enhance existing app creds + new tables)

---

## Problem Statement

### Current Pain Points

**Pain Point 1: CI/CD Credential Management**
- CI/CD pipelines hardcode admin passwords in environment variables
- Password rotation breaks all automation
- No way to restrict automation to specific operations
- Security audit flags password exposure in logs

**Pain Point 2: Coarse-Grained Permissions**
- Users are either `admin` (full control) or `member` (limited control)
- No way to say "users can delete own VMs but not others' VMs"
- No way to restrict junior developers to read-only operations
- Network admins need router creation but not compute access

### Goals

1. **Enable secure automation**: API keys for CI/CD without password exposure
2. **Fine-grained authorization**: Resource-level permissions (ownership, service-specific roles)
3. **Maintain simplicity**: No external dependencies, single binary, PostgreSQL only
4. **Backward compatible**: Existing password authentication unchanged

### Non-Goals

- ❌ Federation/SSO (SAML, OIDC) - future phase
- ❌ OAuth2 authorization server - future phase
- ❌ External IdP integration - future phase
- ❌ Token revocation list - not needed for 2-week scope
- ❌ LDAP/AD integration - future phase

---

## Architecture Overview

### High-Level Design

```
┌─────────────────────────────────────────────────────┐
│           Keystone (Minimal IAM)                     │
├─────────────────────────────────────────────────────┤
│  Authentication Layer                                │
│  ├── Password (existing) ✓                          │
│  └── Application Credentials ✨ NEW                 │
├─────────────────────────────────────────────────────┤
│  Authorization Layer                                 │
│  ├── Role-Based (existing) ✓                        │
│  └── Policy-Based ✨ NEW                            │
├─────────────────────────────────────────────────────┤
│  Token Management                                    │
│  └── JWT with enhanced claims ✨                    │
└─────────────────────────────────────────────────────┘
```

### Authentication Flow Matrix

| Method | Use Case | Token Type | TTL | Security |
|--------|----------|-----------|-----|----------|
| Password (existing) | Interactive CLI/Horizon | JWT | 24h | bcrypt |
| Application Credentials ✨ | CI/CD, automation | JWT | 90-365d | bcrypt cost 12 |

### Component Structure

```
internal/keystone/
├── auth.go                    # Existing auth service (enhanced)
├── appcreds/
│   ├── service.go            # App credential service
│   ├── validation.go         # Access rules validation
│   └── middleware.go         # Access rule enforcement
├── policy/
│   ├── engine.go             # Policy enforcement engine
│   ├── parser.go             # Policy rule parsing
│   └── evaluator.go          # Rule evaluation logic
└── tokens/
    ├── jwt.go                # Existing JWT (enhanced claims)
    └── claims.go             # Enhanced token claims structure

test/contract/keystone/
├── appcreds_test.go          # App credentials contract tests
└── policy_test.go            # Policy engine contract tests
```

---

## Database Schema

### Migration Strategy

**3 new migrations**:
- `048_add_application_credentials.up.sql` / `.down.sql`
- `049_add_policy_engine.up.sql` / `.down.sql`
- `050_add_auth_events.up.sql` / `.down.sql`

### Table 1: Application Credentials

```sql
CREATE TABLE keystone_application_credentials (
    id UUID PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    secret_hash VARCHAR(255) NOT NULL,       -- bcrypt hash (cost 12)
    user_id UUID REFERENCES keystone_users(id) ON DELETE CASCADE,
    project_id UUID REFERENCES keystone_projects(id),
    description TEXT,
    expires_at TIMESTAMP,

    -- Restrictions
    unrestricted BOOLEAN DEFAULT false,      -- Can create other app creds?
    roles TEXT[],                            -- Subset of user's roles
    access_rules JSONB,                      -- Fine-grained restrictions

    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),

    INDEX idx_user_app_creds (user_id),
    UNIQUE(user_id, name)
);

-- Access rules structure (JSONB):
-- [
--   {
--     "path": "/v2.1/servers",
--     "method": "POST",
--     "service": "compute"
--   },
--   {
--     "path": "/v2.1/servers/*",
--     "method": "GET",
--     "service": "compute"
--   }
-- ]
```

**Design Decisions**:
- `secret_hash`: bcrypt cost 12 (2^12 rounds, ~250ms verification, secure for 2026)
- `roles`: Subset of user's roles (cannot escalate privileges)
- `access_rules`: Optional JSONB for path/method/service restrictions
- `unrestricted`: Default false (cannot create other app creds, prevents delegation attacks)
- `UNIQUE(user_id, name)`: Same user cannot create duplicate names

### Table 2: Policy Engine

```sql
CREATE TABLE keystone_policies (
    id UUID PRIMARY KEY,
    type VARCHAR(50) NOT NULL DEFAULT 'application/json',
    blob TEXT NOT NULL,                      -- Policy rules JSON
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Blob structure (JSON):
-- {
--   "admin_required": "role:admin",
--   "owner": "user_id:%(target.user_id)s",
--   "compute:delete": "rule:admin_required or rule:owner",
--   "network:create_router": "role:network_admin"
-- }
```

**Design Decisions**:
- Single policy table (not per-service) - simpler maintenance
- `blob` is TEXT (not JSONB) - treated as opaque policy document
- Type field allows future YAML support
- Policies loaded at startup, cached in memory

### Table 3: Audit Events

```sql
CREATE TABLE keystone_auth_events (
    id UUID PRIMARY KEY,
    user_id UUID,
    method VARCHAR(50) NOT NULL,             -- "password", "app_cred"
    success BOOLEAN NOT NULL,
    failure_reason TEXT,
    ip_address INET,
    user_agent TEXT,
    created_at TIMESTAMP DEFAULT NOW(),

    INDEX idx_user_events (user_id, created_at DESC),
    INDEX idx_method_events (method, created_at DESC)
);
```

**Design Decisions**:
- Separate table (not mixed with other logs) - query performance
- Indexes on user_id and method for audit queries
- IP address and user agent for security analysis
- Retention policy: 90 days default (configurable)

---

## API Endpoints

### Application Credentials API

**Base Path**: `/v3/users/{user_id}/application_credentials`

#### 1. Create Application Credential

```http
POST /v3/users/{user_id}/application_credentials
```

**Request**:
```json
{
  "application_credential": {
    "name": "ci-pipeline",
    "description": "CI/CD pipeline credential",
    "expires_at": "2027-12-31T23:59:59Z",
    "roles": ["member"],
    "unrestricted": false,
    "access_rules": [
      {
        "path": "/v2.1/servers",
        "method": "POST",
        "service": "compute"
      },
      {
        "path": "/v2.1/servers/*",
        "method": "GET",
        "service": "compute"
      }
    ]
  }
}
```

**Response** (201 Created):
```json
{
  "application_credential": {
    "id": "abc123-uuid",
    "name": "ci-pipeline",
    "description": "CI/CD pipeline credential",
    "secret": "def456-secret-only-shown-once",
    "project_id": "project-uuid",
    "roles": [{"name": "member", "id": "role-uuid"}],
    "unrestricted": false,
    "expires_at": "2027-12-31T23:59:59Z",
    "links": {
      "self": "http://localhost:35357/v3/users/user-uuid/application_credentials/abc123-uuid"
    }
  }
}
```

**⚠️ Important**: Secret is only returned on creation, cannot be retrieved later.

#### 2. List Application Credentials

```http
GET /v3/users/{user_id}/application_credentials
```

**Response** (200 OK):
```json
{
  "application_credentials": [
    {
      "id": "abc123-uuid",
      "name": "ci-pipeline",
      "description": "CI/CD pipeline credential",
      "project_id": "project-uuid",
      "roles": [{"name": "member"}],
      "expires_at": "2027-12-31T23:59:59Z",
      "links": {
        "self": "http://localhost:35357/v3/users/user-uuid/application_credentials/abc123-uuid"
      }
    }
  ],
  "links": {
    "self": "http://localhost:35357/v3/users/user-uuid/application_credentials",
    "next": null,
    "previous": null
  }
}
```

**Note**: Secret is NOT included in list response.

#### 3. Get Application Credential

```http
GET /v3/users/{user_id}/application_credentials/{id}
```

**Response** (200 OK):
```json
{
  "application_credential": {
    "id": "abc123-uuid",
    "name": "ci-pipeline",
    "description": "CI/CD pipeline credential",
    "project_id": "project-uuid",
    "roles": [{"name": "member"}],
    "unrestricted": false,
    "access_rules": [
      {
        "path": "/v2.1/servers",
        "method": "POST",
        "service": "compute"
      }
    ],
    "expires_at": "2027-12-31T23:59:59Z"
  }
}
```

#### 4. Delete Application Credential

```http
DELETE /v3/users/{user_id}/application_credentials/{id}
```

**Response**: 204 No Content

#### 5. Authenticate with Application Credential

**Enhanced existing endpoint**:

```http
POST /v3/auth/tokens
```

**Request**:
```json
{
  "auth": {
    "identity": {
      "methods": ["application_credential"],
      "application_credential": {
        "id": "abc123-uuid",
        "secret": "def456-secret"
      }
    }
  }
}
```

**Response** (201 Created) + `X-Subject-Token` header:
```json
{
  "token": {
    "methods": ["application_credential"],
    "user": {
      "id": "user-uuid",
      "name": "ci-user",
      "domain": {"id": "default-uuid", "name": "Default"}
    },
    "project": {
      "id": "project-uuid",
      "name": "my-project",
      "domain": {"id": "default-uuid", "name": "Default"}
    },
    "roles": [
      {"id": "role-uuid", "name": "member"}
    ],
    "expires_at": "2026-03-19T12:00:00Z",
    "issued_at": "2026-03-18T12:00:00Z",
    "catalog": [...],
    "auth_context": {
      "method": "application_credential",
      "app_cred_id": "abc123-uuid",
      "access_rules": [...]
    }
  }
}
```

**Enhanced Claims**: Token includes `access_rules` for middleware enforcement.

### Policy Engine API

#### 1. List Policies (Admin Only)

```http
GET /v3/policies
```

**Response** (200 OK):
```json
{
  "policies": [
    {
      "id": "default-policy-uuid",
      "type": "application/json",
      "blob": "{\"compute:create\": \"role:member or role:admin\"}",
      "links": {
        "self": "http://localhost:35357/v3/policies/default-policy-uuid"
      }
    }
  ]
}
```

#### 2. Create Policy (Admin Only)

```http
POST /v3/policies
```

**Request**:
```json
{
  "policy": {
    "type": "application/json",
    "blob": "{\"compute:delete\": \"role:admin or user_id:%(target.user_id)s\"}"
  }
}
```

**Response** (201 Created):
```json
{
  "policy": {
    "id": "policy-uuid",
    "type": "application/json",
    "blob": "{\"compute:delete\": \"role:admin or user_id:%(target.user_id)s\"}",
    "links": {
      "self": "http://localhost:35357/v3/policies/policy-uuid"
    }
  }
}
```

#### 3. Enforce Policy (Internal API)

**Used by services (Nova, Neutron, etc.) to check permissions**:

```http
POST /v3/policy/enforce
```

**Request**:
```json
{
  "rule": "compute:delete",
  "target": {
    "user_id": "server-owner-uuid",
    "project_id": "project-uuid"
  },
  "credentials": {
    "user_id": "current-user-uuid",
    "project_id": "project-uuid",
    "roles": ["member"]
  }
}
```

**Response** (200 OK):
```json
{
  "allowed": true
}
```

or

```json
{
  "allowed": false,
  "reason": "Policy denied: user does not own resource"
}
```

---

## Implementation Details

### Application Credentials Implementation

#### Secret Generation

```go
// internal/keystone/appcreds/service.go
package appcreds

import (
    "crypto/rand"
    "encoding/base64"
    "golang.org/x/crypto/bcrypt"
)

func generateSecret(length int) (string, error) {
    bytes := make([]byte, length)
    if _, err := rand.Read(bytes); err != nil {
        return "", err
    }
    return base64.URLEncoding.EncodeToString(bytes), nil
}

func hashSecret(secret string) (string, error) {
    // bcrypt cost 12 = 2^12 rounds (~250ms verification)
    hash, err := bcrypt.GenerateFromPassword([]byte(secret), 12)
    if err != nil {
        return "", err
    }
    return string(hash), nil
}

func verifySecret(secret, hash string) error {
    return bcrypt.CompareHashAndPassword([]byte(hash), []byte(secret))
}
```

#### Create Application Credential

```go
func (s *Service) Create(req CreateRequest) (*ApplicationCredential, string, error) {
    // 1. Validate user has requested roles
    user := s.db.GetUser(req.UserID)
    if !hasRoles(user, req.Roles) {
        return nil, "", fmt.Errorf("user lacks requested roles")
    }

    // 2. Validate access rules (if provided)
    if err := s.validateAccessRules(req.AccessRules); err != nil {
        return nil, "", fmt.Errorf("invalid access rules: %w", err)
    }

    // 3. Generate secret (32 bytes = 256 bits entropy)
    secret, err := generateSecret(32)
    if err != nil {
        return nil, "", err
    }

    // 4. Hash secret
    secretHash, err := hashSecret(secret)
    if err != nil {
        return nil, "", err
    }

    // 5. Create credential
    appCred := &ApplicationCredential{
        ID:           uuid.New(),
        Name:         req.Name,
        SecretHash:   secretHash,
        UserID:       req.UserID,
        ProjectID:    req.ProjectID,
        Roles:        req.Roles,
        AccessRules:  req.AccessRules,
        Unrestricted: req.Unrestricted,
        ExpiresAt:    req.ExpiresAt,
    }

    if err := s.db.InsertApplicationCredential(appCred); err != nil {
        return nil, "", err
    }

    // 6. Return credential (with plain secret - only time it's visible)
    return appCred, secret, nil
}
```

#### Authenticate with Application Credential

```go
func (s *Service) Authenticate(id, secret string) (*Token, error) {
    // 1. Fetch credential
    appCred := s.db.GetApplicationCredential(id)
    if appCred == nil {
        return nil, fmt.Errorf("credential not found")
    }

    // 2. Check expiration
    if appCred.ExpiresAt != nil && time.Now().After(*appCred.ExpiresAt) {
        return nil, fmt.Errorf("credential expired")
    }

    // 3. Verify secret (bcrypt)
    if err := verifySecret(secret, appCred.SecretHash); err != nil {
        // Audit failed attempt
        s.db.InsertAuthEvent(AuthEvent{
            UserID:  appCred.UserID,
            Method:  "application_credential",
            Success: false,
            Reason:  "invalid secret",
        })
        return nil, fmt.Errorf("invalid secret")
    }

    // 4. Load user and generate token
    user := s.db.GetUser(appCred.UserID)
    token := s.generateToken(user, appCred.ProjectID, appCred.Roles, appCred.AccessRules)

    // 5. Audit successful authentication
    s.db.InsertAuthEvent(AuthEvent{
        UserID:  user.ID,
        Method:  "application_credential",
        Success: true,
    })

    return token, nil
}
```

#### Access Rule Validation

```go
func (s *Service) validateAccessRules(rules []AccessRule) error {
    for _, rule := range rules {
        // Validate path
        if rule.Path == "" {
            return fmt.Errorf("path cannot be empty")
        }

        // Validate method
        validMethods := []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD"}
        if !contains(validMethods, rule.Method) {
            return fmt.Errorf("invalid method: %s", rule.Method)
        }

        // Validate service
        validServices := []string{"compute", "network", "volume", "image", "identity"}
        if !contains(validServices, rule.Service) {
            return fmt.Errorf("invalid service: %s", rule.Service)
        }
    }
    return nil
}
```

#### Access Rule Enforcement (Middleware)

```go
// internal/keystone/appcreds/middleware.go
package appcreds

func EnforceAccessRules() gin.HandlerFunc {
    return func(c *gin.Context) {
        token := c.MustGet("token").(*Token)

        // Skip if not app credential auth
        if !contains(token.Methods, "application_credential") {
            c.Next()
            return
        }

        // No access rules = full access
        if len(token.AccessRules) == 0 {
            c.Next()
            return
        }

        // Check if request matches any access rule
        requestPath := c.Request.URL.Path
        requestMethod := c.Request.Method
        requestService := getServiceFromPath(requestPath) // e.g., "/v2.1/servers" → "compute"

        allowed := false
        for _, rule := range token.AccessRules {
            if matchesAccessRule(requestPath, requestMethod, requestService, rule) {
                allowed = true
                break
            }
        }

        if !allowed {
            c.JSON(403, gin.H{"error": "Access denied by application credential access rules"})
            c.Abort()
            return
        }

        c.Next()
    }
}

func matchesAccessRule(path, method, service string, rule AccessRule) bool {
    // Method must match exactly
    if rule.Method != method {
        return false
    }

    // Service must match
    if rule.Service != service {
        return false
    }

    // Path can use wildcards
    // "/v2.1/servers/*" matches "/v2.1/servers/abc-123"
    return matchPath(path, rule.Path)
}

func matchPath(actual, pattern string) bool {
    // Exact match
    if actual == pattern {
        return true
    }

    // Wildcard match
    if strings.HasSuffix(pattern, "/*") {
        prefix := strings.TrimSuffix(pattern, "/*")
        return strings.HasPrefix(actual, prefix+"/")
    }

    return false
}
```

### Policy Engine Implementation

#### Policy Rule Grammar

```
rule       := expression
expression := term (("or" | "and") term)*
term       := "role:" identifier
            | "user_id:" attribute
            | "project_id:" attribute
            | "rule:" identifier
            | "(" expression ")"

attribute  := "%(target." identifier ")s"
            | "%(credentials." identifier ")s"

identifier := [a-zA-Z_][a-zA-Z0-9_]*
```

**Examples**:
- `role:admin`
- `user_id:%(target.user_id)s`
- `role:admin or user_id:%(target.user_id)s`
- `rule:admin_required`

#### Parser Implementation

```go
// internal/keystone/policy/parser.go
package policy

type TokenType int

const (
    TOKEN_ROLE TokenType = iota
    TOKEN_USER_ID
    TOKEN_PROJECT_ID
    TOKEN_RULE
    TOKEN_OR
    TOKEN_AND
    TOKEN_LPAREN
    TOKEN_RPAREN
    TOKEN_IDENTIFIER
    TOKEN_ATTRIBUTE
    TOKEN_EOF
)

type Token struct {
    Type  TokenType
    Value string
}

type Parser struct {
    tokens  []Token
    current int
}

type ASTNode struct {
    Type  string // "role", "user_id", "or", "and", "rule"
    Value string
    Left  *ASTNode
    Right *ASTNode
}

func (p *Parser) Parse(rule string) (*ASTNode, error) {
    p.tokens = p.tokenize(rule)
    p.current = 0
    return p.parseExpression()
}

func (p *Parser) parseExpression() (*ASTNode, error) {
    left, err := p.parseTerm()
    if err != nil {
        return nil, err
    }

    for p.current < len(p.tokens) {
        token := p.tokens[p.current]

        if token.Type == TOKEN_OR {
            p.current++
            right, err := p.parseTerm()
            if err != nil {
                return nil, err
            }
            left = &ASTNode{Type: "or", Left: left, Right: right}
        } else if token.Type == TOKEN_AND {
            p.current++
            right, err := p.parseTerm()
            if err != nil {
                return nil, err
            }
            left = &ASTNode{Type: "and", Left: left, Right: right}
        } else {
            break
        }
    }

    return left, nil
}

func (p *Parser) parseTerm() (*ASTNode, error) {
    token := p.tokens[p.current]
    p.current++

    switch token.Type {
    case TOKEN_ROLE:
        return &ASTNode{Type: "role", Value: token.Value}, nil
    case TOKEN_USER_ID:
        return &ASTNode{Type: "user_id", Value: token.Value}, nil
    case TOKEN_PROJECT_ID:
        return &ASTNode{Type: "project_id", Value: token.Value}, nil
    case TOKEN_RULE:
        return &ASTNode{Type: "rule", Value: token.Value}, nil
    case TOKEN_LPAREN:
        expr, err := p.parseExpression()
        if err != nil {
            return nil, err
        }
        // Expect closing paren
        if p.tokens[p.current].Type != TOKEN_RPAREN {
            return nil, fmt.Errorf("expected )")
        }
        p.current++
        return expr, nil
    default:
        return nil, fmt.Errorf("unexpected token: %v", token)
    }
}
```

#### Evaluator Implementation

```go
// internal/keystone/policy/evaluator.go
package policy

type Engine struct {
    policies map[string]string // rule name → rule expression
    cache    *Cache
}

func (e *Engine) Enforce(rule string, target, credentials map[string]interface{}) bool {
    // Check cache first
    if cached, ok := e.cache.Get(rule, target, credentials); ok {
        return cached
    }

    // Parse rule
    ruleExpr := e.policies[rule]
    if ruleExpr == "" {
        // Rule not found = deny by default
        return false
    }

    parser := NewParser()
    ast, err := parser.Parse(ruleExpr)
    if err != nil {
        log.Errorf("Failed to parse rule %s: %v", rule, err)
        return false
    }

    // Evaluate AST
    result := e.evaluate(ast, target, credentials)

    // Cache result
    e.cache.Set(rule, target, credentials, result)

    return result
}

func (e *Engine) evaluate(node *ASTNode, target, credentials map[string]interface{}) bool {
    switch node.Type {
    case "role":
        // Check if user has role
        roles, ok := credentials["roles"].([]string)
        if !ok {
            return false
        }
        return contains(roles, node.Value)

    case "user_id":
        // Check if user ID matches
        // node.Value = "%(target.user_id)s"
        targetUserID := e.interpolate(node.Value, target, credentials)
        credUserID, _ := credentials["user_id"].(string)
        return credUserID == targetUserID

    case "project_id":
        // Check if project ID matches
        targetProjectID := e.interpolate(node.Value, target, credentials)
        credProjectID, _ := credentials["project_id"].(string)
        return credProjectID == targetProjectID

    case "rule":
        // Recursive rule reference
        return e.Enforce(node.Value, target, credentials)

    case "or":
        return e.evaluate(node.Left, target, credentials) || e.evaluate(node.Right, target, credentials)

    case "and":
        return e.evaluate(node.Left, target, credentials) && e.evaluate(node.Right, target, credentials)
    }

    return false
}

func (e *Engine) interpolate(template string, target, credentials map[string]interface{}) string {
    // Replace %(target.user_id)s with actual value
    // "%(target.user_id)s" → target["user_id"]

    re := regexp.MustCompile(`%\((target|credentials)\.([a-zA-Z_]+)\)s`)
    return re.ReplaceAllStringFunc(template, func(match string) string {
        parts := re.FindStringSubmatch(match)
        if len(parts) != 3 {
            return match
        }

        scope := parts[1] // "target" or "credentials"
        key := parts[2]   // "user_id", "project_id", etc.

        var data map[string]interface{}
        if scope == "target" {
            data = target
        } else {
            data = credentials
        }

        value, ok := data[key]
        if !ok {
            return ""
        }

        return fmt.Sprintf("%v", value)
    })
}
```

#### Policy Cache Implementation

```go
// internal/keystone/policy/cache.go
package policy

import (
    "crypto/sha256"
    "encoding/json"
    "fmt"
    "sync"
    "time"
)

type CacheEntry struct {
    Allowed   bool
    ExpiresAt time.Time
}

type Cache struct {
    entries map[string]*CacheEntry
    mu      sync.RWMutex
    ttl     time.Duration
}

func NewCache(ttl time.Duration) *Cache {
    cache := &Cache{
        entries: make(map[string]*CacheEntry),
        ttl:     ttl,
    }

    go cache.cleanupLoop()

    return cache
}

func (c *Cache) Get(rule string, target, credentials map[string]interface{}) (bool, bool) {
    key := c.generateKey(rule, target, credentials)

    c.mu.RLock()
    defer c.mu.RUnlock()

    entry, ok := c.entries[key]
    if !ok {
        return false, false
    }

    if time.Now().After(entry.ExpiresAt) {
        return false, false
    }

    return entry.Allowed, true
}

func (c *Cache) Set(rule string, target, credentials map[string]interface{}, allowed bool) {
    key := c.generateKey(rule, target, credentials)

    c.mu.Lock()
    defer c.mu.Unlock()

    c.entries[key] = &CacheEntry{
        Allowed:   allowed,
        ExpiresAt: time.Now().Add(c.ttl),
    }
}

func (c *Cache) generateKey(rule string, target, credentials map[string]interface{}) string {
    // Hash rule + sorted JSON of context
    targetJSON, _ := json.Marshal(target)
    credsJSON, _ := json.Marshal(credentials)

    h := sha256.New()
    h.Write([]byte(rule))
    h.Write(targetJSON)
    h.Write(credsJSON)

    return fmt.Sprintf("policy:%x", h.Sum(nil))
}

func (c *Cache) cleanupLoop() {
    ticker := time.NewTicker(1 * time.Minute)
    defer ticker.Stop()

    for range ticker.C {
        c.cleanup()
    }
}

func (c *Cache) cleanup() {
    c.mu.Lock()
    defer c.mu.Unlock()

    now := time.Now()
    for key, entry := range c.entries {
        if now.After(entry.ExpiresAt) {
            delete(c.entries, key)
        }
    }
}
```

### Service Integration

Each service (Nova, Neutron, Cinder, Glance) checks policies before operations:

```go
// Example: Nova DeleteServer
func (s *NovaService) DeleteServer(c *gin.Context, serverID string) error {
    token := c.MustGet("token").(*keystone.Token)

    // Get server (need owner info for policy check)
    server := s.db.GetServer(serverID)
    if server == nil {
        return fmt.Errorf("server not found")
    }

    // Check policy
    allowed := s.keystoneClient.CheckPolicy("compute:delete", map[string]interface{}{
        "user_id":    server.UserID,
        "project_id": server.ProjectID,
    }, map[string]interface{}{
        "user_id":    token.User.ID,
        "project_id": token.Project.ID,
        "roles":      token.RoleNames(),
    })

    if !allowed {
        return fmt.Errorf("policy denied: compute:delete")
    }

    // Proceed with deletion
    return s.deleteServer(server)
}
```

---

## Testing Strategy

### Test-Driven Development (TDD)

**Mandatory workflow** per Constitution Article III:

```
1. Write contract test (RED ❌)
2. Get approval
3. Confirm test fails
4. Implement feature (GREEN ✅)
5. Refactor (keep GREEN ✅)
```

### Contract Tests

#### Application Credentials Tests

```go
// test/contract/keystone/appcreds_test.go
package keystone_test

func TestApplicationCredentialLifecycle(t *testing.T) {
    client := setupKeystoneClient(t)
    user := createTestUser(t, client)
    token := authenticateUser(t, client, user)

    // Create
    appCred, secret, err := client.CreateApplicationCredential(token, CreateAppCredRequest{
        Name:      "ci-pipeline",
        Roles:     []string{"member"},
        ExpiresAt: time.Now().Add(90 * 24 * time.Hour),
    })
    require.NoError(t, err)
    assert.NotEmpty(t, secret)
    assert.Equal(t, "ci-pipeline", appCred.Name)

    // List
    creds, err := client.ListApplicationCredentials(token, user.ID)
    require.NoError(t, err)
    assert.Len(t, creds, 1)

    // Get
    retrieved, err := client.GetApplicationCredential(token, user.ID, appCred.ID)
    require.NoError(t, err)
    assert.Equal(t, appCred.ID, retrieved.ID)

    // Authenticate with app credential
    appToken, err := client.AuthenticateWithAppCredential(appCred.ID, secret)
    require.NoError(t, err)
    assert.Equal(t, user.ID, appToken.User.ID)
    assert.Contains(t, appToken.Methods, "application_credential")

    // Verify token works
    projects, err := client.ListProjects(appToken)
    require.NoError(t, err)
    assert.NotEmpty(t, projects)

    // Delete
    err = client.DeleteApplicationCredential(token, user.ID, appCred.ID)
    require.NoError(t, err)

    // Should fail to authenticate now
    _, err = client.AuthenticateWithAppCredential(appCred.ID, secret)
    assert.Error(t, err)
}

func TestApplicationCredentialAccessRules(t *testing.T) {
    client := setupKeystoneClient(t)
    user := createTestUser(t, client)
    token := authenticateUser(t, client, user)

    // Create credential with restricted access
    appCred, secret, err := client.CreateApplicationCredential(token, CreateAppCredRequest{
        Name:  "restricted",
        Roles: []string{"member"},
        AccessRules: []AccessRule{
            {Path: "/v2.1/servers", Method: "POST", Service: "compute"},
            {Path: "/v2.1/servers/*", Method: "GET", Service: "compute"},
        },
    })
    require.NoError(t, err)

    // Authenticate
    restrictedToken, err := client.AuthenticateWithAppCredential(appCred.ID, secret)
    require.NoError(t, err)

    novaClient := setupNovaClient(restrictedToken)

    // Should allow: Create server
    server, err := novaClient.CreateServer(CreateServerRequest{
        Name:     "test-vm",
        FlavorID: "m1.small",
        ImageID:  "cirros",
    })
    require.NoError(t, err)

    // Should allow: Get server
    _, err = novaClient.GetServer(server.ID)
    require.NoError(t, err)

    // Should deny: Delete server (not in access rules)
    err = novaClient.DeleteServer(server.ID)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "access denied")
}

func TestApplicationCredentialExpiration(t *testing.T) {
    client := setupKeystoneClient(t)
    user := createTestUser(t, client)
    token := authenticateUser(t, client, user)

    // Create credential expiring in 1 second
    appCred, secret, err := client.CreateApplicationCredential(token, CreateAppCredRequest{
        Name:      "short-lived",
        Roles:     []string{"member"},
        ExpiresAt: time.Now().Add(1 * time.Second),
    })
    require.NoError(t, err)

    // Wait for expiration
    time.Sleep(2 * time.Second)

    // Should fail to authenticate
    _, err = client.AuthenticateWithAppCredential(appCred.ID, secret)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "expired")
}

func TestApplicationCredentialUnrestricted(t *testing.T) {
    client := setupKeystoneClient(t)
    user := createTestUser(t, client)
    token := authenticateUser(t, client, user)

    // Create unrestricted credential
    unrestrictedCred, secret, err := client.CreateApplicationCredential(token, CreateAppCredRequest{
        Name:         "unrestricted",
        Roles:        []string{"member"},
        Unrestricted: true,
    })
    require.NoError(t, err)

    // Authenticate
    unrestrictedToken, err := client.AuthenticateWithAppCredential(unrestrictedCred.ID, secret)
    require.NoError(t, err)

    // Should allow: Create another app credential
    _, _, err = client.CreateApplicationCredential(unrestrictedToken, CreateAppCredRequest{
        Name:  "nested-cred",
        Roles: []string{"member"},
    })
    require.NoError(t, err)

    // Create restricted credential (default)
    restrictedCred, restrictedSecret, err := client.CreateApplicationCredential(token, CreateAppCredRequest{
        Name:  "restricted",
        Roles: []string{"member"},
    })
    require.NoError(t, err)

    restrictedToken, err := client.AuthenticateWithAppCredential(restrictedCred.ID, restrictedSecret)
    require.NoError(t, err)

    // Should deny: Create another app credential (unrestricted=false)
    _, _, err = client.CreateApplicationCredential(restrictedToken, CreateAppCredRequest{
        Name:  "nested-cred-2",
        Roles: []string{"member"},
    })
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "unrestricted")
}
```

#### Policy Engine Tests

```go
// test/contract/keystone/policy_test.go
package keystone_test

func TestBasicPolicyEvaluation(t *testing.T) {
    engine := policy.NewEngine()
    engine.LoadPolicy(map[string]string{
        "compute:create": "role:member or role:admin",
    })

    // Member can create
    result := engine.Enforce("compute:create", nil, map[string]interface{}{
        "roles": []string{"member"},
    })
    assert.True(t, result)

    // Admin can create
    result = engine.Enforce("compute:create", nil, map[string]interface{}{
        "roles": []string{"admin"},
    })
    assert.True(t, result)

    // Guest cannot create
    result = engine.Enforce("compute:create", nil, map[string]interface{}{
        "roles": []string{"guest"},
    })
    assert.False(t, result)
}

func TestOwnershipPolicyEvaluation(t *testing.T) {
    engine := policy.NewEngine()
    engine.LoadPolicy(map[string]string{
        "owner":          "user_id:%(target.user_id)s",
        "admin_required": "role:admin",
        "compute:delete": "rule:admin_required or rule:owner",
    })

    // Owner can delete
    result := engine.Enforce("compute:delete",
        map[string]interface{}{"user_id": "user-123"},
        map[string]interface{}{"user_id": "user-123", "roles": []string{"member"}},
    )
    assert.True(t, result)

    // Non-owner cannot delete
    result = engine.Enforce("compute:delete",
        map[string]interface{}{"user_id": "user-456"},
        map[string]interface{}{"user_id": "user-123", "roles": []string{"member"}},
    )
    assert.False(t, result)

    // Admin can delete anyone's resource
    result = engine.Enforce("compute:delete",
        map[string]interface{}{"user_id": "user-456"},
        map[string]interface{}{"user_id": "admin-123", "roles": []string{"admin"}},
    )
    assert.True(t, result)
}

func TestComplexPolicyRules(t *testing.T) {
    engine := policy.NewEngine()
    engine.LoadPolicy(map[string]string{
        "compute:migrate": "role:admin and project_id:%(target.project_id)s",
    })

    // Admin in same project can migrate
    result := engine.Enforce("compute:migrate",
        map[string]interface{}{"project_id": "proj-123"},
        map[string]interface{}{
            "roles":      []string{"admin"},
            "project_id": "proj-123",
        },
    )
    assert.True(t, result)

    // Admin in different project cannot migrate
    result = engine.Enforce("compute:migrate",
        map[string]interface{}{"project_id": "proj-456"},
        map[string]interface{}{
            "roles":      []string{"admin"},
            "project_id": "proj-123",
        },
    )
    assert.False(t, result)
}

func TestPolicyEnforceAPI(t *testing.T) {
    client := setupKeystoneClient(t)
    adminToken := authenticateAdmin(t, client)

    // Load policy
    policy, err := client.CreatePolicy(adminToken, CreatePolicyRequest{
        Type: "application/json",
        Blob: `{
            "compute:delete": "role:admin or user_id:%(target.user_id)s"
        }`,
    })
    require.NoError(t, err)

    // Test enforcement API
    result, err := client.EnforcePolicy(adminToken, EnforcePolicyRequest{
        Rule: "compute:delete",
        Target: map[string]interface{}{
            "user_id": "user-123",
        },
        Credentials: map[string]interface{}{
            "user_id": "user-123",
            "roles":   []string{"member"},
        },
    })
    require.NoError(t, err)
    assert.True(t, result.Allowed)
}
```

### Integration Tests

```bash
#!/bin/bash
# test/integration/iam_test.sh

set -e

echo "=== Keystone Minimal IAM Integration Tests ==="

# Start O3K
docker-compose up -d
sleep 5

# Source admin credentials
source test/fixtures/admin-openrc.sh

echo "Test 1: Application Credentials"
# Create app credential
APP_CRED=$(openstack application credential create ci-pipeline \
  --role member \
  --expiration 2027-12-31 \
  -f json)

APP_CRED_ID=$(echo $APP_CRED | jq -r '.id')
APP_CRED_SECRET=$(echo $APP_CRED | jq -r '.secret')

echo "  ✓ Created application credential"

# Authenticate with app credential
export OS_AUTH_TYPE=v3applicationcredential
export OS_APPLICATION_CREDENTIAL_ID=$APP_CRED_ID
export OS_APPLICATION_CREDENTIAL_SECRET=$APP_CRED_SECRET
unset OS_USERNAME OS_PASSWORD

# Verify works
TOKEN=$(openstack token issue -f json)
echo "  ✓ Authenticated with application credential"

# Create server
SERVER_ID=$(openstack server create \
  --flavor m1.small \
  --image cirros \
  test-vm -f value -c id)
echo "  ✓ Created server with app credential"

# Cleanup
openstack server delete $SERVER_ID --wait
unset OS_AUTH_TYPE OS_APPLICATION_CREDENTIAL_ID OS_APPLICATION_CREDENTIAL_SECRET
source test/fixtures/admin-openrc.sh

echo "Test 2: Policy Enforcement"
# Create policy
cat > /tmp/policy.json <<EOF
{
  "compute:create": "role:member",
  "compute:delete": "role:admin or user_id:%(target.user_id)s",
  "network:create_router": "role:network_admin"
}
EOF

openstack policy create default \
  --type application/json \
  --blob @/tmp/policy.json

echo "  ✓ Loaded policy"

# Create user1 and server
openstack user create user1 --password secret || true
openstack role add --user user1 --project default member
export OS_USERNAME=user1
export OS_PASSWORD=secret

SERVER_ID=$(openstack server create \
  --flavor m1.small \
  --image cirros \
  user1-vm -f value -c id)
echo "  ✓ User1 created server"

# Try to delete as user2 (should fail)
openstack user create user2 --password secret || true
openstack role add --user user2 --project default member
export OS_USERNAME=user2

openstack server delete $SERVER_ID 2>&1 | grep -q "Forbidden"
echo "  ✓ User2 cannot delete user1's server (policy denied)"

# Delete as user1 (should work)
export OS_USERNAME=user1
openstack server delete $SERVER_ID --wait
echo "  ✓ User1 deleted own server (policy allowed)"

# Admin can delete anyone's server
export OS_USERNAME=admin
export OS_PASSWORD=secret
SERVER_ID=$(openstack server create \
  --flavor m1.small \
  --image cirros \
  test-vm -f value -c id)
openstack server delete $SERVER_ID --wait
echo "  ✓ Admin deleted server (policy allowed)"

echo "=== All Tests Passed ==="
```

### Performance Tests

```bash
#!/bin/bash
# test/performance/iam_perf_test.sh

echo "=== Performance Testing ==="

# Test 1: App credential authentication throughput
echo "Test 1: Application credential auth (target: <30ms p95)"
ab -n 1000 -c 100 \
  -H "Content-Type: application/json" \
  -p appcred-auth.json \
  http://localhost:35357/v3/auth/tokens

# Test 2: Policy enforcement overhead
echo "Test 2: Policy enforcement (target: <5ms p95)"
for i in {1..1000}; do
  curl -s -X POST http://localhost:35357/v3/policy/enforce \
    -H "X-Auth-Token: $TOKEN" \
    -d @policy-check.json &
done
wait

echo "=== Performance Tests Complete ==="
```

---

## Configuration

```yaml
# config/o3k.yaml (enhanced Keystone section)

keystone:
  # Existing configuration
  jwt_secret: "env:KEYSTONE_JWT_SECRET"  # Use environment variable
  token_ttl: 24h

  # Application Credentials (NEW)
  application_credentials:
    enabled: true
    max_ttl: 365d                    # Maximum credential lifetime
    default_ttl: 90d                  # Default if not specified
    allow_unrestricted: false         # Allow creating other app creds by default

  # Policy Engine (NEW)
  policy:
    enabled: true
    default_policy_file: "/etc/o3k/policy.json"
    cache_ttl: 5m                     # Cache policy decisions

  # Audit Logging (NEW)
  audit:
    enabled: true
    log_authentication_events: true
    log_policy_decisions: false       # Too verbose for production
    retention_days: 90
```

### Default Policy File

```json
{
  "admin_required": "role:admin",
  "owner": "user_id:%(target.user_id)s",
  "admin_or_owner": "rule:admin_required or rule:owner",

  "identity:get_user": "rule:admin_or_owner",
  "identity:list_users": "rule:admin_required",
  "identity:create_user": "rule:admin_required",
  "identity:update_user": "rule:admin_or_owner",
  "identity:delete_user": "rule:admin_required",

  "compute:create": "role:member or role:admin",
  "compute:get": "rule:admin_or_owner",
  "compute:list": "",
  "compute:update": "rule:admin_or_owner",
  "compute:delete": "rule:admin_or_owner",
  "compute:start": "rule:admin_or_owner",
  "compute:stop": "rule:admin_or_owner",
  "compute:reboot": "rule:admin_or_owner",
  "compute:migrate": "rule:admin_required",

  "network:create_network": "role:member or role:admin",
  "network:get_network": "",
  "network:update_network": "rule:admin_or_owner",
  "network:delete_network": "rule:admin_or_owner",
  "network:create_router": "role:network_admin or role:admin",

  "volume:create": "role:member or role:admin",
  "volume:get": "rule:admin_or_owner",
  "volume:update": "rule:admin_or_owner",
  "volume:delete": "rule:admin_or_owner",

  "image:get_image": "",
  "image:upload_image": "role:member or role:admin",
  "image:delete_image": "rule:admin_or_owner"
}
```

---

## Deployment

### Docker Compose

```yaml
# deployments/docker-compose.yml (updated)

version: '3.8'

services:
  postgres:
    image: postgres:18-alpine
    environment:
      POSTGRES_USER: lightstack
      POSTGRES_PASSWORD: secret
      POSTGRES_DB: lightstack
    volumes:
      - postgres-data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U lightstack"]
      interval: 10s
      timeout: 5s
      retries: 5

  o3k:
    build:
      context: ..
      dockerfile: deployments/Dockerfile
    image: o3k:iam-enhanced
    depends_on:
      postgres:
        condition: service_healthy
    environment:
      O3K_DATABASE_URL: "postgres://lightstack:secret@postgres/lightstack?sslmode=disable"
      KEYSTONE_JWT_SECRET: "${KEYSTONE_JWT_SECRET:-dev-secret-change-in-production}"
    volumes:
      - ./config/o3k.yaml:/etc/o3k/o3k.yaml
      - ./policy.json:/etc/o3k/policy.json
    ports:
      - "35357:35357"
      - "8774:8774"
      - "9696:9696"
      - "8776:8776"
      - "9292:9292"
    command: ["/o3k", "--config", "/etc/o3k/o3k.yaml"]

volumes:
  postgres-data:
```

### Migration Procedure

```bash
#!/bin/bash
# scripts/migrate-to-iam.sh

set -e

echo "=== O3K IAM Migration ==="

# 1. Backup database
echo "1. Backing up database..."
docker-compose exec postgres pg_dump -U lightstack lightstack > backup-$(date +%Y%m%d).sql

# 2. Stop O3K
echo "2. Stopping O3K..."
docker-compose stop o3k

# 3. Pull new image
echo "3. Pulling enhanced O3K image..."
docker-compose pull o3k

# 4. Run migrations
echo "4. Running database migrations..."
docker-compose run --rm o3k /o3k migrate --config /etc/o3k/o3k.yaml

# 5. Start O3K
echo "5. Starting enhanced O3K..."
docker-compose up -d o3k

# 6. Verify
echo "6. Verifying..."
sleep 5
curl -f http://localhost:35357/v3/ > /dev/null
echo "✓ Keystone API responding"

echo "=== Migration Complete ==="
echo "Existing users and tokens continue working"
echo "New IAM features now available"
```

---

## Performance Targets

| Operation | Target (p95) | Notes |
|-----------|--------------|-------|
| Password auth | < 50ms | Unchanged from current |
| App credential auth | < 30ms | bcrypt cost 12 ~250ms, but only once per session |
| Token validation (cached) | < 1ms | In-memory lookup |
| Token validation (uncached) | < 10ms | JWT verify |
| Policy check (cached) | < 1ms | Hash lookup |
| Policy check (uncached) | < 5ms | Parse + evaluate |
| Access rule enforcement | < 0.5ms | Path pattern matching |

**Caching Strategy**:
- Token cache TTL: 5 minutes (balances freshness vs performance)
- Policy cache TTL: 5 minutes (policy changes are infrequent)
- In-memory caches (no Redis dependency)

---

## Security Considerations

### Secrets Management

**Application Credential Secrets**:
- Generated with 32 bytes (256 bits) cryptographic randomness
- Hashed with bcrypt cost 12 (2^12 rounds, ~250ms verification)
- Only shown once on creation (never retrievable)
- Stored hashed in database

**JWT Secret**:
- Must be strong (32+ bytes entropy)
- Use environment variable: `KEYSTONE_JWT_SECRET`
- Rotate regularly (30-90 days)

**Configuration Example**:
```yaml
keystone:
  jwt_secret: "env:KEYSTONE_JWT_SECRET"  # From environment
```

### Rate Limiting

**Application Credential Authentication**:
- Max 10 failed attempts per credential per hour
- Account lockout after threshold
- IP-based rate limiting recommended (via reverse proxy)

**Policy Enforcement API**:
- Internal only (not exposed externally)
- Called by O3K services (Nova, Neutron, etc.)

### Audit Logging

**Logged Events**:
- All authentication attempts (success/failure)
- Application credential creation/deletion
- Policy enforcement decisions (configurable, off by default for performance)

**Log Retention**:
- Default: 90 days
- Configurable via `keystone.audit.retention_days`
- Periodic cleanup job

### Access Control

**Application Credential Endpoints**:
- Users can only manage own credentials
- Admin can view all credentials (but not secrets)

**Policy Management**:
- Admin-only endpoints
- Policies affect all users immediately (cached for 5 minutes)

---

## Documentation

### User Guide

```markdown
# Application Credentials - User Guide

## Creating Application Credentials

```bash
# Basic creation
openstack application credential create ci-pipeline \
  --role member \
  --expiration 2027-12-31

# Output includes secret (save it!)
# +-------------+---------------------------+
# | Field       | Value                     |
# +-------------+---------------------------+
# | id          | abc123...                 |
# | secret      | def456... ← SAVE THIS!    |
# +-------------+---------------------------+
```

## Using Application Credentials

```bash
# Set authentication type
export OS_AUTH_TYPE=v3applicationcredential
export OS_APPLICATION_CREDENTIAL_ID=abc123...
export OS_APPLICATION_CREDENTIAL_SECRET=def456...
unset OS_USERNAME OS_PASSWORD

# Use OpenStack CLI normally
openstack server list
openstack volume create --size 10 my-volume
```

## Access Rules (Restricting Permissions)

```bash
# Create credential that can ONLY create and list servers
cat > access-rules.json <<EOF
[
  {
    "path": "/v2.1/servers",
    "method": "POST",
    "service": "compute"
  },
  {
    "path": "/v2.1/servers",
    "method": "GET",
    "service": "compute"
  }
]
EOF

openstack application credential create restricted \
  --role member \
  --access-rules @access-rules.json
```

## Best Practices

1. **One credential per use case**: Separate CI/CD, monitoring, backups
2. **Use shortest TTL possible**: 90 days for CI/CD, 30 days for testing
3. **Apply access rules**: Restrict to minimum required operations
4. **Rotate regularly**: Delete old, create new
5. **Store securely**: Use secrets manager (Vault, AWS Secrets Manager)
```

### Policy Guide

```markdown
# Policy Engine - Admin Guide

## Loading Policies

```bash
# Create policy file
cat > /etc/o3k/policy.json <<EOF
{
  "compute:delete": "role:admin or user_id:%(target.user_id)s"
}
EOF

# Load policy
openstack policy create default \
  --type application/json \
  --blob @/etc/o3k/policy.json
```

## Policy Rule Syntax

### Role Checks
```json
"compute:create": "role:member"
```

### Ownership Checks
```json
"compute:delete": "user_id:%(target.user_id)s"
```

### Combined Rules
```json
"compute:delete": "role:admin or user_id:%(target.user_id)s"
```

### Rule References
```json
{
  "admin_required": "role:admin",
  "owner": "user_id:%(target.user_id)s",
  "compute:delete": "rule:admin_required or rule:owner"
}
```

## Testing Policies

```bash
# Test as different users
OS_USERNAME=user1 openstack server delete vm1  # Should fail if not owner
OS_USERNAME=admin openstack server delete vm1  # Should succeed (admin)
```
```

---

## Implementation Timeline

### Week 1: Application Credentials

**Day 1-2**: Database schema and migrations
- Create migration files
- Add application_credentials table
- Add auth_events table
- Test migrations

**Day 3-4**: CRUD operations
- Implement Create (with secret generation/hashing)
- Implement List/Get/Delete
- Add API endpoints and handlers
- Contract tests GREEN

**Day 5**: Authentication integration
- Add app credential auth method to token endpoint
- Implement access rule validation
- Add access rule enforcement middleware
- Integration tests GREEN

### Week 2: Policy Engine

**Day 6-7**: Policy parser and evaluator
- Implement tokenizer and parser
- Build AST evaluator
- Support role/user_id/project_id checks
- Support OR/AND operators
- Contract tests GREEN

**Day 8-9**: Service integration
- Add policy checks to Nova (before create/delete/update)
- Add policy checks to Neutron
- Add policy checks to Cinder
- Add policy checks to Glance
- Create default policy file
- Integration tests GREEN

**Day 10**: Testing and documentation
- Performance testing
- Security review
- Update CLAUDE.md
- Write user guides
- Create examples

---

## Success Criteria

### Week 1 Exit Criteria
- ✅ Application credentials CRUD working
- ✅ Authentication with app credentials passes tests
- ✅ Access rules enforced correctly
- ✅ Expiration handling works
- ✅ OpenStack CLI integration works
- ✅ Contract tests GREEN (10 new tests)

### Week 2 Exit Criteria
- ✅ Policy engine parses and evaluates rules
- ✅ All 5 services enforce policies
- ✅ Ownership checks work
- ✅ Admin override works
- ✅ Performance targets met
- ✅ Contract tests GREEN (5 new tests)

### Final Gate
- ✅ All contract tests GREEN (81 total: 71 existing + 10 new)
- ✅ Integration tests pass
- ✅ Performance benchmarks met
- ✅ Documentation complete
- ✅ Backward compatible
- ✅ Migration tested

---

## Risk Mitigation

| Risk | Impact | Mitigation | Owner |
|------|--------|------------|-------|
| bcrypt performance issue | Medium | Cost 12 is standard, acceptable 250ms | Dev |
| Policy cache stale data | Low | 5min TTL, manual invalidation available | Dev |
| Access rule complexity | Medium | Start simple, add features incrementally | Product |
| Migration breaks existing auth | High | Extensive backward compat testing | QA |
| Performance regression | Medium | Benchmark before/after, cache aggressively | Dev |

---

## Future Enhancements

**Not included in 2-week scope, potential future phases**:

### Phase 2: Federation (3-4 weeks)
- SAML 2.0 Service Provider
- OpenID Connect Provider
- External IdP integration (Google, Azure AD)
- Federated user shadow accounts

### Phase 3: OAuth2 Server (1-2 weeks)
- OAuth2 authorization server
- Third-party app integration
- OIDC discovery endpoints

### Phase 4: Advanced Features (2-3 weeks)
- Token revocation list (Redis-backed)
- LDAP/AD integration
- Policy templates and inheritance
- Fine-grained project policies

**Decision Point**: Implement future phases based on user demand and feedback after 2-week minimal IAM deployment.

---

## References

- OpenStack Keystone API v3: https://docs.openstack.org/api-ref/identity/v3/
- Application Credentials: https://docs.openstack.org/keystone/latest/user/application_credentials.html
- Policy Enforcement: https://docs.openstack.org/oslo.policy/latest/
- O3K Constitution: memory/constitution.md (Article III: Test-First Development)
- O3K Roadmap: ROADMAP.md (Phase 1: Modular Transformation)

---

**End of Specification**
