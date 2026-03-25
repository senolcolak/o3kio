# Keystone Minimal IAM Enhancement - Design Specification v2

**Date**: 2026-03-18
**Status**: Draft v2 (All Review Issues Fixed)
**Version**: 2.0
**Scope**: Application Credentials + Policy Engine
**Timeline**: 3 weeks (15 working days)

---

## Executive Summary

This specification defines a minimal IAM enhancement for O3K Keystone that adds two critical capabilities:

1. **Application Credentials** (Enhanced): Long-lived API keys for CI/CD automation with fine-grained access control
2. **Policy Engine** (New): Resource-level authorization beyond simple role-based access control

**Design Philosophy**: Deliver 80% of enterprise IAM value with 25% of the complexity. No federation, no OAuth2 server, no external IdP integration - just pragmatic solutions to real automation and authorization problems.

**Timeline**: **3 weeks** (15 working days) - Extended from 2 weeks to accommodate proper TDD workflow
**Database Changes**: Enhanced 1 existing table + 3 new tables
**API Changes**: Enhanced 5 existing endpoints + 3 new endpoints
**Migrations**: 4 new migrations (including security fix for existing data)

**⚠️ CRITICAL**: This spec follows TDD (Test-Driven Development) per Constitution Article III. **Tests are written and approved BEFORE implementation begins**.

---

## Problem Statement

### Current Pain Points

**Pain Point 1: CI/CD Credential Management**
- CI/CD pipelines hardcode admin passwords in environment variables
- Password rotation breaks all automation
- No way to restrict automation to specific operations
- Security audit flags password exposure in logs
- **EXISTING IMPLEMENTATION INSECURE**: Secrets stored as plain base64 (line 114 of application_credentials.go)

**Pain Point 2: Coarse-Grained Permissions**
- Users are either `admin` (full control) or `member` (limited control)
- No way to say "users can delete own VMs but not others' VMs"
- No way to restrict junior developers to read-only operations
- Network admins need router creation but not compute access

### Goals

1. **Fix existing security vulnerability**: Migrate plain base64 secrets to bcrypt
2. **Enable secure automation**: API keys for CI/CD without password exposure
3. **Fine-grained authorization**: Resource-level permissions (ownership, service-specific roles)
4. **Maintain simplicity**: No external dependencies, single binary, PostgreSQL only
5. **Backward compatible**: Existing password authentication unchanged, legacy app creds migrated

### Non-Goals

- ❌ Federation/SSO (SAML, OIDC) - future phase
- ❌ OAuth2 authorization server - future phase
- ❌ External IdP integration - future phase
- ❌ Token revocation list - not needed for 3-week scope
- ❌ LDAP/AD integration - future phase

---

## Constitution Compliance

### Article I: Library-First

**Deviation Justification**: Application credentials and policy engine are implemented within `internal/keystone/*` rather than as standalone `pkg/*` libraries.

**Rationale**:
- Application credentials are tightly coupled to Keystone's JWT token generation and user database
- Policy engine requires deep integration with authentication context
- Extracting to pkg/ would create circular dependencies and complex interfaces
- Both features serve Keystone-specific needs, not general-purpose use

**Future Path**: If other projects need policy engine, it can be extracted to `pkg/policy/` in Phase 2.

### Article III: Test-First Development (NON-NEGOTIABLE)

**Strict Compliance**: This spec follows RED → GREEN → REFACTOR workflow:

```
Phase 1: Write Contract Tests (MUST FAIL RED ❌)
Phase 2: Get Test Approval
Phase 3: Confirm Tests FAIL
Phase 4: Implement Features (MAKE GREEN ✅)
Phase 5: Refactor (KEEP GREEN ✅)
```

**⚠️ CRITICAL CHECKPOINTS**:
- ✋ **STOP**: No implementation code written until tests are approved
- ✋ **CONFIRM**: Tests must FAIL initially (RED ❌)
- ✋ **VALIDATE**: Tests must PASS after implementation (GREEN ✅)

### Article IX: Integration-First

**Compliance**: Contract tests use real OpenStack SDK (gophercloud), real PostgreSQL, real O3K binary - not mocks.

---

## Existing State Analysis

### Current Application Credentials Implementation

**Location**: `/Users/I761222/git/o3k/internal/keystone/application_credentials.go`
**Migration**: `029_keystone_application_credentials.up.sql`

**Current Schema** (Migration 029):
```sql
CREATE TABLE application_credentials (
    id UUID PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    project_id UUID,
    secret_hash VARCHAR(255) NOT NULL,  -- ⚠️ MISLEADING: Actually plain base64
    description TEXT,
    unrestricted BOOLEAN DEFAULT FALSE,
    expires_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(user_id, name)
);
```

**🔴 CRITICAL SECURITY FLAW** (Line 114 of application_credentials.go):
```go
secret := base64.URLEncoding.EncodeToString(secretBytes)
// ...
_, err := database.DB.Exec(..., secret, ...) // Stores PLAIN secret!
```

The `secret_hash` column name is misleading - it stores **plain base64-encoded secrets**, not hashes. This is a **security vulnerability**.

**Missing Table** (Referenced in code but not in migrations):
```sql
-- application_credential_roles table exists in code queries but no migration created it
SELECT r.id, r.name
FROM application_credential_roles acr  -- ⚠️ Table doesn't exist!
JOIN roles r ON acr.role_id = r.id
WHERE acr.application_credential_id = $1
```

**Missing Features** (Not in current implementation):
- ❌ Access rules (path/method/service restrictions)
- ❌ Bcrypt hashing (uses plain base64)
- ❌ Authentication via app credentials (only CRUD endpoints)

---

## Architecture Overview

### High-Level Design

```
┌─────────────────────────────────────────────────────┐
│           Keystone (Minimal IAM)                     │
├─────────────────────────────────────────────────────┤
│  Authentication Layer                                │
│  ├── Password (existing) ✓                          │
│  └── Application Credentials (enhanced) ✨          │
├─────────────────────────────────────────────────────┤
│  Authorization Layer                                 │
│  ├── Role-Based (existing) ✓                        │
│  └── Policy-Based (new) ✨                          │
├─────────────────────────────────────────────────────┤
│  Token Management                                    │
│  └── JWT with enhanced claims ✨                    │
└─────────────────────────────────────────────────────┘
```

### Component Structure

```
internal/keystone/
├── auth.go                    # Existing auth service (enhanced)
├── application_credentials.go # Existing (ENHANCED - security fix)
├── appcreds/
│   ├── service.go            # Enhanced app credential service
│   ├── validation.go         # Access rules validation
│   ├── middleware.go         # Access rule enforcement
│   └── migration.go          # Legacy secret migration helper
├── policy/
│   ├── engine.go             # Policy enforcement engine
│   ├── parser.go             # Policy rule parsing
│   ├── evaluator.go          # Rule evaluation logic
│   └── cache.go              # Policy decision cache
└── tokens/
    ├── jwt.go                # Existing JWT (enhanced claims)
    └── claims.go             # Enhanced token claims structure

test/contract/keystone/
├── setup_test.go             # Test infrastructure (NEW)
├── appcreds_test.go          # App credentials contract tests
└── policy_test.go            # Policy engine contract tests
```

---

## PHASE 1: CONTRACT TESTS (WRITE FIRST - TDD)

**⚠️ THIS SECTION MUST BE COMPLETED BEFORE IMPLEMENTATION**

### Test Infrastructure Setup

```go
// test/contract/keystone/setup_test.go
package keystone_test

import (
    "context"
    "fmt"
    "net/http"
    "os"
    "os/exec"
    "testing"
    "time"

    "github.com/testcontainers/testcontainers-go"
    "github.com/testcontainers/testcontainers-go/wait"
)

var (
    testPostgresURL string
    testO3KURL      string
    testO3KProcess  *os.Process
)

func TestMain(m *testing.M) {
    ctx := context.Background()

    // Start PostgreSQL container
    postgresContainer, err := startPostgresContainer(ctx)
    if err != nil {
        panic(fmt.Sprintf("Failed to start postgres: %v", err))
    }
    defer postgresContainer.Terminate(ctx)

    testPostgresURL, _ = postgresContainer.ConnectionString(ctx)

    // Run migrations
    if err := runMigrations(testPostgresURL); err != nil {
        panic(fmt.Sprintf("Failed to run migrations: %v", err))
    }

    // Start O3K binary
    if err := startO3K(testPostgresURL); err != nil {
        panic(fmt.Sprintf("Failed to start O3K: %v", err))
    }
    defer stopO3K()

    // Wait for O3K to be ready
    if err := waitForO3K(); err != nil {
        panic(fmt.Sprintf("O3K failed to start: %v", err))
    }

    // Run tests
    exitCode := m.Run()

    os.Exit(exitCode)
}

func startPostgresContainer(ctx context.Context) (testcontainers.Container, error) {
    req := testcontainers.ContainerRequest{
        Image:        "postgres:18-alpine",
        ExposedPorts: []string{"5432/tcp"},
        Env: map[string]string{
            "POSTGRES_USER":     "test",
            "POSTGRES_PASSWORD": "test",
            "POSTGRES_DB":       "test",
        },
        WaitingFor: wait.ForLog("database system is ready to accept connections").
            WithOccurrence(2).
            WithStartupTimeout(30 * time.Second),
    }

    return testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
        ContainerRequest: req,
        Started:          true,
    })
}

func runMigrations(dbURL string) error {
    cmd := exec.Command("../../../bin/o3k", "migrate", "--database-url", dbURL)
    return cmd.Run()
}

func startO3K(dbURL string) error {
    cmd := exec.Command("../../../bin/o3k", "--database-url", dbURL, "--port", "35357")
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr

    if err := cmd.Start(); err != nil {
        return err
    }

    testO3KProcess = cmd.Process
    testO3KURL = "http://localhost:35357/v3"

    return nil
}

func stopO3K() {
    if testO3KProcess != nil {
        testO3KProcess.Kill()
    }
}

func waitForO3K() error {
    client := &http.Client{Timeout: 2 * time.Second}

    for i := 0; i < 30; i++ {
        resp, err := client.Get(testO3KURL + "/")
        if err == nil && resp.StatusCode == 200 {
            return nil
        }
        time.Sleep(1 * time.Second)
    }

    return fmt.Errorf("O3K failed to become ready after 30 seconds")
}

// Helper functions for tests

func setupKeystoneClient(t *testing.T) *KeystoneClient {
    return &KeystoneClient{
        BaseURL:    testO3KURL,
        HTTPClient: &http.Client{Timeout: 5 * time.Second},
    }
}

func createTestUser(t *testing.T, client *KeystoneClient) *User {
    // Create user via admin token
    adminToken := getAdminToken(t, client)

    user, err := client.CreateUser(adminToken, CreateUserRequest{
        Name:       fmt.Sprintf("testuser-%d", time.Now().Unix()),
        Password:   "testpassword",
        ProjectID:  "default",
        DomainID:   "default",
    })
    require.NoError(t, err)

    // Add member role
    client.AddRoleToUser(adminToken, user.ID, "member", "default")

    t.Cleanup(func() {
        client.DeleteUser(adminToken, user.ID)
    })

    return user
}

func authenticateUser(t *testing.T, client *KeystoneClient, user *User) *Token {
    token, err := client.Authenticate(AuthRequest{
        Username: user.Name,
        Password: "testpassword",
        Project:  "default",
    })
    require.NoError(t, err)
    return token
}

func getAdminToken(t *testing.T, client *KeystoneClient) *Token {
    token, err := client.Authenticate(AuthRequest{
        Username: "admin",
        Password: "secret",
        Project:  "default",
    })
    require.NoError(t, err)
    return token
}
```

### Application Credentials Contract Tests

**⚠️ THESE TESTS MUST FAIL (RED ❌) INITIALLY**

```go
// test/contract/keystone/appcreds_test.go
package keystone_test

import (
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

// Test 1: Basic Lifecycle
func TestApplicationCredentialLifecycle(t *testing.T) {
    client := setupKeystoneClient(t)
    user := createTestUser(t, client)
    token := authenticateUser(t, client, user)

    // Create application credential
    appCred, secret, err := client.CreateApplicationCredential(token, CreateAppCredRequest{
        Name:      "ci-pipeline",
        Roles:     []string{"member"},
        ExpiresAt: time.Now().Add(90 * 24 * time.Hour),
    })
    require.NoError(t, err, "Should create app credential")
    assert.NotEmpty(t, secret, "Secret should be returned")
    assert.Equal(t, "ci-pipeline", appCred.Name)
    assert.Len(t, appCred.Roles, 1)

    // Secret should be 32+ bytes (256 bits)
    assert.GreaterOrEqual(t, len(secret), 32, "Secret should have sufficient entropy")

    // List application credentials
    creds, err := client.ListApplicationCredentials(token, user.ID)
    require.NoError(t, err, "Should list app credentials")
    assert.Len(t, creds, 1)
    assert.Equal(t, "ci-pipeline", creds[0].Name)

    // Secret should NOT be in list response
    _, hasSecret := creds[0]["secret"]
    assert.False(t, hasSecret, "Secret should not be in list response")

    // Get specific credential
    retrieved, err := client.GetApplicationCredential(token, user.ID, appCred.ID)
    require.NoError(t, err, "Should get specific app credential")
    assert.Equal(t, appCred.ID, retrieved.ID)

    // Secret should NOT be in get response
    _, hasSecret = retrieved["secret"]
    assert.False(t, hasSecret, "Secret should not be in get response")

    // Authenticate with application credential
    appToken, err := client.AuthenticateWithAppCredential(appCred.ID, secret)
    require.NoError(t, err, "Should authenticate with app credential")
    assert.Equal(t, user.ID, appToken.User.ID)
    assert.Contains(t, appToken.Methods, "application_credential")

    // Token should work for API calls
    projects, err := client.ListProjects(appToken)
    require.NoError(t, err, "App credential token should work")
    assert.NotEmpty(t, projects)

    // Delete application credential
    err = client.DeleteApplicationCredential(token, user.ID, appCred.ID)
    require.NoError(t, err, "Should delete app credential")

    // Should fail to authenticate after deletion
    _, err = client.AuthenticateWithAppCredential(appCred.ID, secret)
    assert.Error(t, err, "Should fail to authenticate with deleted credential")
}

// Test 2: Access Rules Enforcement
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
    require.NoError(t, err, "Should create restricted credential")

    // Authenticate with restricted credential
    restrictedToken, err := client.AuthenticateWithAppCredential(appCred.ID, secret)
    require.NoError(t, err, "Should authenticate")

    novaClient := setupNovaClient(restrictedToken)

    // Should ALLOW: Create server (in access rules)
    server, err := novaClient.CreateServer(CreateServerRequest{
        Name:     "test-vm",
        FlavorID: "m1.small",
        ImageID:  "cirros",
    })
    require.NoError(t, err, "Should allow create server")
    assert.NotEmpty(t, server.ID)

    // Should ALLOW: Get server (wildcard in access rules)
    retrieved, err := novaClient.GetServer(server.ID)
    require.NoError(t, err, "Should allow get server")
    assert.Equal(t, server.ID, retrieved.ID)

    // Should DENY: Delete server (NOT in access rules)
    err = novaClient.DeleteServer(server.ID)
    assert.Error(t, err, "Should deny delete server")
    assert.Contains(t, err.Error(), "access denied", "Error should mention access denied")

    // Cleanup with full token
    novaAdminClient := setupNovaClient(token)
    novaAdminClient.DeleteServer(server.ID)
}

// Test 3: Expiration Handling
func TestApplicationCredentialExpiration(t *testing.T) {
    client := setupKeystoneClient(t)
    user := createTestUser(t, client)
    token := authenticateUser(t, client, user)

    // Create credential expiring in 2 seconds
    appCred, secret, err := client.CreateApplicationCredential(token, CreateAppCredRequest{
        Name:      "short-lived",
        Roles:     []string{"member"},
        ExpiresAt: time.Now().Add(2 * time.Second),
    })
    require.NoError(t, err, "Should create expiring credential")

    // Should work immediately
    _, err = client.AuthenticateWithAppCredential(appCred.ID, secret)
    require.NoError(t, err, "Should authenticate before expiration")

    // Wait for expiration
    time.Sleep(3 * time.Second)

    // Should fail after expiration
    _, err = client.AuthenticateWithAppCredential(appCred.ID, secret)
    assert.Error(t, err, "Should fail after expiration")
    assert.Contains(t, err.Error(), "expired", "Error should mention expiration")
}

// Test 4: Unrestricted Flag
func TestApplicationCredentialUnrestricted(t *testing.T) {
    client := setupKeystoneClient(t)
    user := createTestUser(t, client)
    token := authenticateUser(t, client, user)

    // Create unrestricted credential
    unrestrictedCred, secret1, err := client.CreateApplicationCredential(token, CreateAppCredRequest{
        Name:         "unrestricted",
        Roles:        []string{"member"},
        Unrestricted: true,
    })
    require.NoError(t, err, "Should create unrestricted credential")

    // Authenticate with unrestricted credential
    unrestrictedToken, err := client.AuthenticateWithAppCredential(unrestrictedCred.ID, secret1)
    require.NoError(t, err, "Should authenticate")

    // Should ALLOW: Create another app credential (unrestricted=true)
    nestedCred, _, err := client.CreateApplicationCredential(unrestrictedToken, CreateAppCredRequest{
        Name:  "nested-cred",
        Roles: []string{"member"},
    })
    require.NoError(t, err, "Unrestricted credential should create nested credential")
    assert.Equal(t, "nested-cred", nestedCred.Name)

    // Create restricted credential (default: unrestricted=false)
    restrictedCred, secret2, err := client.CreateApplicationCredential(token, CreateAppCredRequest{
        Name:  "restricted",
        Roles: []string{"member"},
    })
    require.NoError(t, err, "Should create restricted credential")

    restrictedToken, err := client.AuthenticateWithAppCredential(restrictedCred.ID, secret2)
    require.NoError(t, err, "Should authenticate")

    // Should DENY: Create another app credential (unrestricted=false)
    _, _, err = client.CreateApplicationCredential(restrictedToken, CreateAppCredRequest{
        Name:  "nested-cred-2",
        Roles: []string{"member"},
    })
    assert.Error(t, err, "Restricted credential should NOT create nested credential")
    assert.Contains(t, err.Error(), "unrestricted", "Error should mention unrestricted requirement")
}

// Test 5: Bcrypt Security (SECRET NOT RETRIEVABLE)
func TestApplicationCredentialSecretSecurity(t *testing.T) {
    client := setupKeystoneClient(t)
    user := createTestUser(t, client)
    token := authenticateUser(t, client, user)

    // Create credential
    appCred, secret, err := client.CreateApplicationCredential(token, CreateAppCredRequest{
        Name:  "secure-cred",
        Roles: []string{"member"},
    })
    require.NoError(t, err)

    // Secret returned on creation
    assert.NotEmpty(t, secret, "Secret should be returned on creation")

    // Direct database query should show HASHED secret (not plain text)
    dbSecret := queryDatabaseSecret(t, appCred.ID)

    // Database should contain bcrypt hash (starts with $2a$ or $2b$)
    assert.True(t, strings.HasPrefix(dbSecret, "$2"), "Secret should be bcrypt hashed")

    // Database should NOT contain plain secret
    assert.NotContains(t, dbSecret, secret, "Database should not contain plain secret")

    // Wrong secret should fail authentication
    _, err = client.AuthenticateWithAppCredential(appCred.ID, "wrong-secret")
    assert.Error(t, err, "Wrong secret should fail")
}

// Test 6: Legacy Secret Migration
func TestLegacyApplicationCredentialMigration(t *testing.T) {
    // This test verifies backward compatibility with existing plain base64 secrets

    // Insert legacy credential directly (simulating pre-migration data)
    legacySecret := base64.URLEncoding.EncodeToString([]byte("legacy-plain-secret"))
    legacyCredID := insertLegacyCredential(t, "legacy-cred", user.ID, legacySecret)

    client := setupKeystoneClient(t)

    // Should still authenticate with legacy credential
    token, err := client.AuthenticateWithAppCredential(legacyCredID, legacySecret)
    require.NoError(t, err, "Legacy credential should still work")
    assert.NotEmpty(t, token.ID)

    // But should warn in logs about legacy credential
    logs := captureLogs(t)
    assert.Contains(t, logs, "legacy credential", "Should warn about legacy credential")
}

// Helper to query database directly
func queryDatabaseSecret(t *testing.T, credID string) string {
    // Query test database for secret_hash value
    // (Implementation depends on test database access)
    var secretHash string
    err := database.DB.QueryRow(context.Background(),
        "SELECT secret_hash FROM application_credentials WHERE id = $1",
        credID).Scan(&secretHash)
    require.NoError(t, err)
    return secretHash
}
```

### Policy Engine Contract Tests

**⚠️ THESE TESTS MUST FAIL (RED ❌) INITIALLY**

```go
// test/contract/keystone/policy_test.go
package keystone_test

import (
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

// Test 1: Basic Role-Based Policy
func TestBasicRolePolicy(t *testing.T) {
    engine := policy.NewEngine()
    engine.LoadPolicy(map[string]string{
        "compute:create": "role:member or role:admin",
    })

    // Member can create
    result := engine.Enforce("compute:create", nil, map[string]interface{}{
        "roles": []string{"member"},
    })
    assert.True(t, result, "Member should be allowed to create")

    // Admin can create
    result = engine.Enforce("compute:create", nil, map[string]interface{}{
        "roles": []string{"admin"},
    })
    assert.True(t, result, "Admin should be allowed to create")

    // Guest cannot create
    result = engine.Enforce("compute:create", nil, map[string]interface{}{
        "roles": []string{"guest"},
    })
    assert.False(t, result, "Guest should NOT be allowed to create")

    // No roles = deny
    result = engine.Enforce("compute:create", nil, map[string]interface{}{
        "roles": []string{},
    })
    assert.False(t, result, "No roles should be denied")
}

// Test 2: Ownership-Based Policy
func TestOwnershipPolicy(t *testing.T) {
    engine := policy.NewEngine()
    engine.LoadPolicy(map[string]string{
        "owner":          "user_id:%(target.user_id)s",
        "admin_required": "role:admin",
        "compute:delete": "rule:admin_required or rule:owner",
    })

    // Owner can delete own resource
    result := engine.Enforce("compute:delete",
        map[string]interface{}{"user_id": "user-123"},
        map[string]interface{}{"user_id": "user-123", "roles": []string{"member"}},
    )
    assert.True(t, result, "Owner should delete own resource")

    // Non-owner member cannot delete
    result = engine.Enforce("compute:delete",
        map[string]interface{}{"user_id": "user-456"},
        map[string]interface{}{"user_id": "user-123", "roles": []string{"member"}},
    )
    assert.False(t, result, "Non-owner member should NOT delete others' resource")

    // Admin can delete anyone's resource
    result = engine.Enforce("compute:delete",
        map[string]interface{}{"user_id": "user-456"},
        map[string]interface{}{"user_id": "admin-123", "roles": []string{"admin"}},
    )
    assert.True(t, result, "Admin should delete any resource")
}

// Test 3: Complex AND/OR Rules
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
    assert.True(t, result, "Admin in same project should migrate")

    // Admin in different project cannot migrate
    result = engine.Enforce("compute:migrate",
        map[string]interface{}{"project_id": "proj-456"},
        map[string]interface{}{
            "roles":      []string{"admin"},
            "project_id": "proj-123",
        },
    )
    assert.False(t, result, "Admin in different project should NOT migrate")

    // Member in same project cannot migrate (lacks admin role)
    result = engine.Enforce("compute:migrate",
        map[string]interface{}{"project_id": "proj-123"},
        map[string]interface{}{
            "roles":      []string{"member"},
            "project_id": "proj-123",
        },
    )
    assert.False(t, result, "Member should NOT migrate even in same project")
}

// Test 4: Policy API Enforcement Endpoint
func TestPolicyEnforceAPIEndpoint(t *testing.T) {
    client := setupKeystoneClient(t)
    adminToken := authenticateAdmin(t, client)

    // Create policy via API
    policy, err := client.CreatePolicy(adminToken, CreatePolicyRequest{
        Type: "application/json",
        Blob: `{
            "compute:delete": "role:admin or user_id:%(target.user_id)s",
            "network:create_router": "role:network_admin"
        }`,
    })
    require.NoError(t, err, "Should create policy")
    assert.NotEmpty(t, policy.ID)

    // Test enforcement API - owner can delete
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
    require.NoError(t, err, "Enforcement API should work")
    assert.True(t, result.Allowed, "Owner should be allowed")
    assert.Empty(t, result.Reason, "No reason needed for allowed")

    // Test enforcement API - non-owner denied
    result, err = client.EnforcePolicy(adminToken, EnforcePolicyRequest{
        Rule: "compute:delete",
        Target: map[string]interface{}{
            "user_id": "user-456",
        },
        Credentials: map[string]interface{}{
            "user_id": "user-123",
            "roles":   []string{"member"},
        },
    })
    require.NoError(t, err, "Enforcement API should work")
    assert.False(t, result.Allowed, "Non-owner should be denied")
    assert.NotEmpty(t, result.Reason, "Reason should be provided for denial")
}

// Test 5: Service Integration (Nova)
func TestPolicyIntegrationNova(t *testing.T) {
    client := setupKeystoneClient(t)
    adminToken := authenticateAdmin(t, client)

    // Load policy
    client.CreatePolicy(adminToken, CreatePolicyRequest{
        Type: "application/json",
        Blob: `{
            "compute:delete": "role:admin or user_id:%(target.user_id)s"
        }`,
    })

    // Create user1 and server
    user1 := createTestUser(t, client)
    token1 := authenticateUser(t, client, user1)

    novaClient1 := setupNovaClient(token1)
    server, err := novaClient1.CreateServer(CreateServerRequest{
        Name:     "user1-vm",
        FlavorID: "m1.small",
        ImageID:  "cirros",
    })
    require.NoError(t, err, "Should create server")

    // User1 can delete own server
    err = novaClient1.DeleteServer(server.ID)
    require.NoError(t, err, "User1 should delete own server")

    // Create another server
    server2, _ := novaClient1.CreateServer(CreateServerRequest{
        Name:     "user1-vm2",
        FlavorID: "m1.small",
        ImageID:  "cirros",
    })

    // User2 cannot delete user1's server
    user2 := createTestUser(t, client)
    token2 := authenticateUser(t, client, user2)
    novaClient2 := setupNovaClient(token2)

    err = novaClient2.DeleteServer(server2.ID)
    assert.Error(t, err, "User2 should NOT delete user1's server")
    assert.Contains(t, err.Error(), "Forbidden", "Should return 403 Forbidden")

    // Admin can delete anyone's server
    novaAdminClient := setupNovaClient(adminToken)
    err = novaAdminClient.DeleteServer(server2.ID)
    require.NoError(t, err, "Admin should delete any server")
}

// Test 6: Policy Cache Performance
func TestPolicyCachePerformance(t *testing.T) {
    engine := policy.NewEngine()
    engine.LoadPolicy(map[string]string{
        "compute:delete": "role:admin or user_id:%(target.user_id)s",
    })

    target := map[string]interface{}{"user_id": "user-123"}
    credentials := map[string]interface{}{"user_id": "user-123", "roles": []string{"member"}}

    // First call (cache miss)
    start := time.Now()
    result1 := engine.Enforce("compute:delete", target, credentials)
    duration1 := time.Since(start)

    // Second call (cache hit)
    start = time.Now()
    result2 := engine.Enforce("compute:delete", target, credentials)
    duration2 := time.Since(start)

    // Results should be identical
    assert.Equal(t, result1, result2, "Cached result should match")

    // Cache should be faster (at least 2x)
    assert.Less(t, duration2, duration1/2, "Cached call should be at least 2x faster")
    assert.Less(t, duration2, 1*time.Millisecond, "Cached call should be < 1ms")
}

// Test 7: Unknown Rule Defaults to Deny
func TestUnknownRuleDefaultDeny(t *testing.T) {
    engine := policy.NewEngine()
    engine.LoadPolicy(map[string]string{
        "compute:create": "role:member",
    })

    // Unknown rule should default to deny
    result := engine.Enforce("unknown:operation", nil, map[string]interface{}{
        "roles": []string{"admin"},
    })
    assert.False(t, result, "Unknown rule should default to deny")
}
```

### ⚠️ CHECKPOINT: Test Approval Required

**STOP HERE**: Do NOT proceed to implementation until:
1. ✅ All contract tests written and reviewed
2. ✅ User approves test strategy
3. ✅ Tests confirmed to FAIL (RED ❌)

---

## Database Schema

### Migration Strategy

**CRITICAL**: Existing `application_credentials` table has security flaw and missing features. We must migrate carefully to avoid breaking existing credentials.

**4 new migrations**:
- `048_fix_application_credentials_security.up.sql` - Security fix for existing table
- `049_add_application_credential_roles_table.up.sql` - Missing roles table
- `050_add_policy_engine.up.sql` - Policy tables
- `051_add_auth_events.up.sql` - Audit logging

### Migration 048: Security Fix (CRITICAL)

```sql
-- migrations/048_fix_application_credentials_security.up.sql

-- Add new columns for enhanced features
ALTER TABLE application_credentials
ADD COLUMN IF NOT EXISTS access_rules JSONB DEFAULT NULL,
ADD COLUMN IF NOT EXISTS legacy_auth BOOLEAN DEFAULT false,
ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP DEFAULT NOW();

-- Mark all existing credentials as legacy (use old base64 auth)
UPDATE application_credentials
SET legacy_auth = true
WHERE legacy_auth = false;

-- Add comment explaining legacy_auth flag
COMMENT ON COLUMN application_credentials.legacy_auth IS
'True for credentials created before bcrypt migration. These use base64 comparison instead of bcrypt.';

-- Create index for access_rules queries
CREATE INDEX IF NOT EXISTS idx_application_credentials_access_rules
ON application_credentials USING GIN (access_rules)
WHERE access_rules IS NOT NULL;

-- Add trigger to update updated_at
CREATE OR REPLACE FUNCTION update_application_credentials_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_application_credentials_updated_at
BEFORE UPDATE ON application_credentials
FOR EACH ROW
EXECUTE FUNCTION update_application_credentials_updated_at();
```

**Rollback**:
```sql
-- migrations/048_fix_application_credentials_security.down.sql

DROP TRIGGER IF EXISTS trigger_application_credentials_updated_at ON application_credentials;
DROP FUNCTION IF EXISTS update_application_credentials_updated_at();
DROP INDEX IF EXISTS idx_application_credentials_access_rules;

ALTER TABLE application_credentials
DROP COLUMN IF EXISTS updated_at,
DROP COLUMN IF EXISTS legacy_auth,
DROP COLUMN IF EXISTS access_rules;
```

**Migration Notes**:
- **Backward Compatible**: Existing credentials continue working (legacy_auth=true)
- **Security Warning**: Logs will warn when legacy credentials are used
- **Rotation Required**: Users should rotate credentials within 90 days
- **Deprecation Timeline**: Legacy auth removed in v0.8.0 (6 months)

### Migration 049: Roles Junction Table

```sql
-- migrations/049_add_application_credential_roles_table.up.sql

-- This table should have existed but was missing from migrations
CREATE TABLE IF NOT EXISTS application_credential_roles (
    application_credential_id UUID NOT NULL REFERENCES application_credentials(id) ON DELETE CASCADE,
    role_id UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    created_at TIMESTAMP DEFAULT NOW(),
    PRIMARY KEY (application_credential_id, role_id)
);

CREATE INDEX idx_application_credential_roles_cred
ON application_credential_roles(application_credential_id);

CREATE INDEX idx_application_credential_roles_role
ON application_credential_roles(role_id);
```

**Rollback**:
```sql
-- migrations/049_add_application_credential_roles_table.down.sql

DROP TABLE IF EXISTS application_credential_roles;
```

### Migration 050: Policy Engine

```sql
-- migrations/050_add_policy_engine.up.sql

CREATE TABLE keystone_policies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type VARCHAR(50) NOT NULL DEFAULT 'application/json',
    blob TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Default policy
INSERT INTO keystone_policies (type, blob) VALUES (
    'application/json',
    '{
        "admin_required": "role:admin",
        "owner": "user_id:%(target.user_id)s",
        "admin_or_owner": "rule:admin_required or rule:owner",
        "compute:create": "role:member or role:admin",
        "compute:get": "rule:admin_or_owner",
        "compute:delete": "rule:admin_or_owner",
        "network:create_network": "role:member or role:admin",
        "network:delete_network": "rule:admin_or_owner",
        "volume:create": "role:member or role:admin",
        "volume:delete": "rule:admin_or_owner",
        "image:upload_image": "role:member or role:admin",
        "image:delete_image": "rule:admin_or_owner"
    }'
);
```

**Rollback**:
```sql
-- migrations/050_add_policy_engine.down.sql

DROP TABLE IF EXISTS keystone_policies;
```

### Migration 051: Audit Logging

```sql
-- migrations/051_add_auth_events.up.sql

CREATE TABLE keystone_auth_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID,
    method VARCHAR(50) NOT NULL,
    success BOOLEAN NOT NULL,
    failure_reason TEXT,
    ip_address INET,
    user_agent TEXT,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_auth_events_user ON keystone_auth_events(user_id, created_at DESC);
CREATE INDEX idx_auth_events_method ON keystone_auth_events(method, created_at DESC);
CREATE INDEX idx_auth_events_created ON keystone_auth_events(created_at DESC);

-- Auto-cleanup old events (retention: 90 days)
CREATE OR REPLACE FUNCTION cleanup_old_auth_events()
RETURNS void AS $$
BEGIN
    DELETE FROM keystone_auth_events
    WHERE created_at < NOW() - INTERVAL '90 days';
END;
$$ LANGUAGE plpgsql;

-- Run cleanup daily (requires pg_cron extension or external cron)
COMMENT ON FUNCTION cleanup_old_auth_events() IS
'Call this function daily to cleanup auth events older than 90 days';
```

**Rollback**:
```sql
-- migrations/051_add_auth_events.down.sql

DROP FUNCTION IF EXISTS cleanup_old_auth_events();
DROP TABLE IF EXISTS keystone_auth_events;
```

---

## ⚠️ IMPLEMENTATION STARTS HERE (AFTER TESTS APPROVED AND CONFIRMED RED)

### Application Credentials Implementation

**Enhanced Service** (Fixes security flaw):

```go
// internal/keystone/appcreds/service.go
package appcreds

import (
    "crypto/rand"
    "encoding/base64"
    "fmt"
    "time"

    "golang.org/x/crypto/bcrypt"
)

const (
    bcryptCost = 12 // 2^12 rounds ≈ 250ms verification
)

type Service struct {
    db *database.DB
}

// Generate generates a cryptographically secure random secret
func generateSecret(length int) (string, error) {
    bytes := make([]byte, length)
    if _, err := rand.Read(bytes); err != nil {
        return "", fmt.Errorf("failed to generate random bytes: %w", err)
    }
    return base64.URLEncoding.EncodeToString(bytes), nil
}

// HashSecret hashes a secret using bcrypt
func hashSecret(secret string) (string, error) {
    hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcryptCost)
    if err != nil {
        return "", fmt.Errorf("failed to hash secret: %w", err)
    }
    return string(hash), nil
}

// VerifySecret verifies a secret against its hash
// Supports both bcrypt (new) and base64 (legacy) for backward compatibility
func (s *Service) verifySecret(secret, storedHash string, isLegacy bool) error {
    if isLegacy {
        // Legacy: plain base64 comparison (insecure, but backward compatible)
        if secret != storedHash {
            return fmt.Errorf("invalid secret")
        }
        log.Warn("Legacy application credential used - please rotate to bcrypt",
            "hash_prefix", storedHash[:10])
        return nil
    }

    // New: bcrypt verification
    if err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(secret)); err != nil {
        return fmt.Errorf("invalid secret: %w", err)
    }
    return nil
}

// Create creates a new application credential with bcrypt hashing
func (s *Service) Create(req CreateRequest) (*ApplicationCredential, string, error) {
    // 1. Validate user has requested roles
    user := s.db.GetUser(req.UserID)
    if !hasRoles(user, req.Roles) {
        return nil, "", fmt.Errorf("user lacks requested roles")
    }

    // 2. Validate access rules (if provided)
    if err := validateAccessRules(req.AccessRules); err != nil {
        return nil, "", fmt.Errorf("invalid access rules: %w", err)
    }

    // 3. Generate secret (32 bytes = 256 bits entropy)
    secret, err := generateSecret(32)
    if err != nil {
        return nil, "", err
    }

    // 4. Hash secret with bcrypt
    secretHash, err := hashSecret(secret)
    if err != nil {
        return nil, "", err
    }

    // 5. Create credential (legacy_auth=false for new credentials)
    appCred := &ApplicationCredential{
        ID:           uuid.New(),
        Name:         req.Name,
        SecretHash:   secretHash,
        UserID:       req.UserID,
        ProjectID:    req.ProjectID,
        AccessRules:  req.AccessRules,
        Unrestricted: req.Unrestricted,
        ExpiresAt:    req.ExpiresAt,
        LegacyAuth:   false, // New credential uses bcrypt
    }

    if err := s.db.InsertApplicationCredential(appCred); err != nil {
        return nil, "", err
    }

    // 6. Insert roles into junction table
    for _, roleID := range req.Roles {
        s.db.InsertApplicationCredentialRole(appCred.ID, roleID)
    }

    // 7. Return credential with plain secret (only time it's visible)
    return appCred, secret, nil
}

// Authenticate authenticates with application credential
func (s *Service) Authenticate(id, secret string) (*Token, error) {
    // 1. Fetch credential
    appCred := s.db.GetApplicationCredential(id)
    if appCred == nil {
        // Audit failed attempt
        s.db.InsertAuthEvent(AuthEvent{
            Method:  "application_credential",
            Success: false,
            Reason:  "credential not found",
        })
        return nil, fmt.Errorf("credential not found")
    }

    // 2. Check expiration
    if appCred.ExpiresAt != nil && time.Now().After(*appCred.ExpiresAt) {
        s.db.InsertAuthEvent(AuthEvent{
            UserID:  appCred.UserID,
            Method:  "application_credential",
            Success: false,
            Reason:  "credential expired",
        })
        return nil, fmt.Errorf("credential expired")
    }

    // 3. Verify secret (supports both bcrypt and legacy base64)
    if err := s.verifySecret(secret, appCred.SecretHash, appCred.LegacyAuth); err != nil {
        s.db.InsertAuthEvent(AuthEvent{
            UserID:  appCred.UserID,
            Method:  "application_credential",
            Success: false,
            Reason:  "invalid secret",
        })
        return nil, err
    }

    // 4. Load user and roles
    user := s.db.GetUser(appCred.UserID)
    roles := s.db.GetApplicationCredentialRoles(appCred.ID)

    // 5. Generate JWT token with enhanced claims
    token := s.generateToken(user, appCred.ProjectID, roles, appCred.AccessRules)

    // 6. Audit successful authentication
    s.db.InsertAuthEvent(AuthEvent{
        UserID:  user.ID,
        Method:  "application_credential",
        Success: true,
    })

    return token, nil
}
```

**Access Rule Validation**:

```go
// internal/keystone/appcreds/validation.go
package appcreds

import (
    "fmt"
    "strings"
)

type AccessRule struct {
    Path    string `json:"path"`
    Method  string `json:"method"`
    Service string `json:"service"`
}

func validateAccessRules(rules []AccessRule) error {
    for i, rule := range rules {
        // Validate path
        if rule.Path == "" {
            return fmt.Errorf("rule %d: path cannot be empty", i)
        }

        if !strings.HasPrefix(rule.Path, "/") {
            return fmt.Errorf("rule %d: path must start with /", i)
        }

        // Validate method
        validMethods := []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD"}
        if !contains(validMethods, rule.Method) {
            return fmt.Errorf("rule %d: invalid method %s", i, rule.Method)
        }

        // Validate service
        validServices := []string{"compute", "network", "volume", "image", "identity"}
        if !contains(validServices, rule.Service) {
            return fmt.Errorf("rule %d: invalid service %s", i, rule.Service)
        }

        // Validate wildcard usage (only * allowed, at end of path)
        if strings.Contains(rule.Path, "*") {
            if !strings.HasSuffix(rule.Path, "/*") {
                return fmt.Errorf("rule %d: wildcard only allowed at end of path (/*)", i)
            }
        }
    }

    return nil
}

func contains(slice []string, item string) bool {
    for _, s := range slice {
        if s == item {
            return true
        }
    }
    return false
}
```

**Access Rule Enforcement Middleware**:

```go
// internal/keystone/appcreds/middleware.go
package appcreds

import (
    "strings"

    "github.com/gin-gonic/gin"
)

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
        requestService := getServiceFromPath(requestPath)

        allowed := false
        for _, rule := range token.AccessRules {
            if matchesAccessRule(requestPath, requestMethod, requestService, rule) {
                allowed = true
                break
            }
        }

        if !allowed {
            c.JSON(403, gin.H{
                "error": "Access denied by application credential access rules",
                "code":  "access_denied",
            })
            c.Abort()
            return
        }

        c.Next()
    }
}

func getServiceFromPath(path string) string {
    // Map API paths to services
    // /v2.1/servers → compute
    // /v2.0/networks → network
    // /v3/volumes → volume
    // /v2/images → image
    // /v3/users → identity

    if strings.HasPrefix(path, "/v2.1/") {
        return "compute"
    }
    if strings.HasPrefix(path, "/v2.0/") || strings.HasPrefix(path, "/v2/networks") {
        return "network"
    }
    if strings.HasPrefix(path, "/v3/volumes") {
        return "volume"
    }
    if strings.HasPrefix(path, "/v2/images") {
        return "image"
    }
    if strings.HasPrefix(path, "/v3/") {
        return "identity"
    }

    return "unknown"
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

    // Path matching: exact or wildcard
    if path == rule.Path {
        return true // Exact match
    }

    // Wildcard match: /v2.1/servers/* matches /v2.1/servers/abc-123
    if strings.HasSuffix(rule.Path, "/*") {
        prefix := strings.TrimSuffix(rule.Path, "/*")
        if strings.HasPrefix(path, prefix+"/") {
            return true
        }
    }

    return false
}
```

### Policy Engine Implementation

**Parser** (Fixed cache key generation):

```go
// internal/keystone/policy/parser.go
package policy

import (
    "fmt"
    "regexp"
    "strings"
)

type TokenType int

const (
    TOKEN_ROLE TokenType = iota
    TOKEN_USER_ID
    TOKEN_PROJECT_ID
    TOKEN_RULE
    TOKEN_OR
    TOKEN_AND
    TOKEN_NOT
    TOKEN_LPAREN
    TOKEN_RPAREN
    TOKEN_EOF
)

type Token struct {
    Type  TokenType
    Value string
}

type ASTNode struct {
    Type  string // "role", "user_id", "project_id", "rule", "or", "and", "not"
    Value string
    Left  *ASTNode
    Right *ASTNode
}

type Parser struct {
    tokens  []Token
    current int
}

func NewParser() *Parser {
    return &Parser{}
}

func (p *Parser) Parse(rule string) (*ASTNode, error) {
    p.tokens = p.tokenize(rule)
    p.current = 0

    if len(p.tokens) == 0 {
        return nil, fmt.Errorf("empty rule")
    }

    ast, err := p.parseExpression()
    if err != nil {
        return nil, err
    }

    // Ensure all tokens consumed
    if p.current < len(p.tokens) && p.tokens[p.current].Type != TOKEN_EOF {
        return nil, fmt.Errorf("unexpected tokens after parsing: %v", p.tokens[p.current:])
    }

    return ast, nil
}

func (p *Parser) tokenize(rule string) []Token {
    tokens := []Token{}
    rule = strings.TrimSpace(rule)

    // Regex patterns
    rolePattern := regexp.MustCompile(`^role:([a-zA-Z_][a-zA-Z0-9_]*)`)
    userIDPattern := regexp.MustCompile(`^user_id:(.*?)(\s|$|and|or|\))`)
    projectIDPattern := regexp.MustCompile(`^project_id:(.*?)(\s|$|and|or|\))`)
    rulePattern := regexp.MustCompile(`^rule:([a-zA-Z_][a-zA-Z0-9_]*)`)

    for len(rule) > 0 {
        rule = strings.TrimSpace(rule)

        // Check for operators
        if strings.HasPrefix(rule, "or") {
            tokens = append(tokens, Token{Type: TOKEN_OR})
            rule = rule[2:]
            continue
        }
        if strings.HasPrefix(rule, "and") {
            tokens = append(tokens, Token{Type: TOKEN_AND})
            rule = rule[3:]
            continue
        }
        if strings.HasPrefix(rule, "not") {
            tokens = append(tokens, Token{Type: TOKEN_NOT})
            rule = rule[3:]
            continue
        }
        if strings.HasPrefix(rule, "(") {
            tokens = append(tokens, Token{Type: TOKEN_LPAREN})
            rule = rule[1:]
            continue
        }
        if strings.HasPrefix(rule, ")") {
            tokens = append(tokens, Token{Type: TOKEN_RPAREN})
            rule = rule[1:]
            continue
        }

        // Check for functions
        if match := rolePattern.FindStringSubmatch(rule); match != nil {
            tokens = append(tokens, Token{Type: TOKEN_ROLE, Value: match[1]})
            rule = rule[len(match[0]):]
            continue
        }
        if match := userIDPattern.FindStringSubmatch(rule); match != nil {
            tokens = append(tokens, Token{Type: TOKEN_USER_ID, Value: strings.TrimSpace(match[1])})
            rule = rule[len(match[0]):]
            continue
        }
        if match := projectIDPattern.FindStringSubmatch(rule); match != nil {
            tokens = append(tokens, Token{Type: TOKEN_PROJECT_ID, Value: strings.TrimSpace(match[1])})
            rule = rule[len(match[0]):]
            continue
        }
        if match := rulePattern.FindStringSubmatch(rule); match != nil {
            tokens = append(tokens, Token{Type: TOKEN_RULE, Value: match[1]})
            rule = rule[len(match[0]):]
            continue
        }

        // Unknown token
        break
    }

    tokens = append(tokens, Token{Type: TOKEN_EOF})
    return tokens
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
    if p.current >= len(p.tokens) {
        return nil, fmt.Errorf("unexpected end of tokens")
    }

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
    case TOKEN_NOT:
        node, err := p.parseTerm()
        if err != nil {
            return nil, err
        }
        return &ASTNode{Type: "not", Left: node}, nil
    case TOKEN_LPAREN:
        expr, err := p.parseExpression()
        if err != nil {
            return nil, err
        }
        // Expect closing paren
        if p.current >= len(p.tokens) || p.tokens[p.current].Type != TOKEN_RPAREN {
            return nil, fmt.Errorf("expected ), got %v", p.tokens[p.current])
        }
        p.current++
        return expr, nil
    default:
        return nil, fmt.Errorf("unexpected token: %v", token)
    }
}
```

**Evaluator**:

```go
// internal/keystone/policy/evaluator.go
package policy

import (
    "fmt"
    "regexp"
    "strings"
)

type Engine struct {
    policies map[string]string // rule name → rule expression
    cache    *Cache
}

func NewEngine() *Engine {
    return &Engine{
        policies: make(map[string]string),
        cache:    NewCache(5 * time.Minute),
    }
}

func (e *Engine) LoadPolicy(policies map[string]string) {
    e.policies = policies
}

func (e *Engine) Enforce(rule string, target, credentials map[string]interface{}) bool {
    // Check cache first
    if cached, ok := e.cache.Get(rule, target, credentials); ok {
        return cached
    }

    // Get rule expression
    ruleExpr, ok := e.policies[rule]
    if !ok {
        // Rule not found = deny by default
        return false
    }

    // Parse rule
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

    case "not":
        return !e.evaluate(node.Left, target, credentials)
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

**Cache (FIXED: Deterministic Key Generation)**:

```go
// internal/keystone/policy/cache.go
package policy

import (
    "crypto/sha256"
    "encoding/json"
    "fmt"
    "sort"
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

// generateKey creates deterministic cache key
// FIXED: Sorts map keys before hashing to ensure determinism
func (c *Cache) generateKey(rule string, target, credentials map[string]interface{}) string {
    // Hash rule + deterministically serialized context
    targetJSON := deterministicJSON(target)
    credsJSON := deterministicJSON(credentials)

    h := sha256.New()
    h.Write([]byte(rule))
    h.Write(targetJSON)
    h.Write(credsJSON)

    return fmt.Sprintf("policy:%x", h.Sum(nil))
}

// deterministicJSON serializes map in sorted key order
func deterministicJSON(m map[string]interface{}) []byte {
    if len(m) == 0 {
        return []byte("{}")
    }

    // Sort keys
    keys := make([]string, 0, len(m))
    for k := range m {
        keys = append(keys, k)
    }
    sort.Strings(keys)

    // Build JSON manually in sorted order
    var buf strings.Builder
    buf.WriteString("{")
    first := true

    for _, k := range keys {
        if !first {
            buf.WriteString(",")
        }
        first = false

        // Key
        buf.WriteString(`"`)
        buf.WriteString(k)
        buf.WriteString(`":`)

        // Value (simple serialization)
        v := m[k]
        switch val := v.(type) {
        case string:
            buf.WriteString(`"`)
            buf.WriteString(val)
            buf.WriteString(`"`)
        case []string:
            // Sort string arrays for determinism
            sorted := make([]string, len(val))
            copy(sorted, val)
            sort.Strings(sorted)
            jsonVal, _ := json.Marshal(sorted)
            buf.Write(jsonVal)
        default:
            jsonVal, _ := json.Marshal(v)
            buf.Write(jsonVal)
        }
    }

    buf.WriteString("}")
    return []byte(buf.String())
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

---

## Implementation Timeline (REVISED - 3 Weeks)

### Week 1: Test Infrastructure + Application Credentials

**Day 1**: Test Infrastructure
- ✅ Write `setup_test.go` with testcontainers
- ✅ Write application credentials contract tests (MUST FAIL RED ❌)
- **CHECKPOINT**: Get test approval

**Day 2**: Confirm RED, Begin Implementation
- ✅ Confirm all tests FAIL (RED ❌)
- ✅ Run migrations 048-049 (security fix + roles table)
- ✅ Implement bcrypt hashing in `appcreds/service.go`

**Day 3-4**: App Credentials Features
- ✅ Implement Create (with access rules validation)
- ✅ Implement authentication method
- ✅ Implement access rule middleware
- **TARGET**: Tests turn GREEN ✅

**Day 5**: Refactor + Integration
- ✅ Refactor (keep tests GREEN ✅)
- ✅ Integration testing
- ✅ OpenStack CLI validation

### Week 2: Policy Engine

**Day 6**: Policy Tests
- ✅ Write policy engine contract tests (MUST FAIL RED ❌)
- **CHECKPOINT**: Get test approval

**Day 7**: Confirm RED, Begin Implementation
- ✅ Confirm policy tests FAIL (RED ❌)
- ✅ Run migration 050 (policy tables)
- ✅ Implement parser (tokenizer + AST)

**Day 8**: Policy Evaluator
- ✅ Implement evaluator (role, user_id, project_id checks)
- ✅ Implement cache (deterministic key generation)
- **TARGET**: Tests turn GREEN ✅

**Day 9**: Service Integration
- ✅ Add policy checks to Nova
- ✅ Add policy checks to Neutron, Cinder, Glance
- ✅ Integration testing

**Day 10**: Refactor + Polish
- ✅ Refactor (keep tests GREEN ✅)
- ✅ Performance testing
- ✅ Load testing

### Week 3: Testing, Documentation, Release

**Day 11-12**: Comprehensive Testing
- ✅ All contract tests pass (81 total)
- ✅ Integration tests pass
- ✅ Performance benchmarks meet targets
- ✅ Security audit

**Day 13-14**: Documentation
- ✅ Update CLAUDE.md
- ✅ Write user guides (app creds + policies)
- ✅ Write admin guides
- ✅ Migration documentation

**Day 15**: Release Preparation
- ✅ Final testing
- ✅ Build Docker images
- ✅ Tag v0.6.0
- ✅ Release notes
- ✅ Deployment validation

---

## Success Criteria

### Week 1 Exit Criteria
- ✅ All app credential contract tests GREEN
- ✅ Bcrypt hashing implemented and tested
- ✅ Legacy credentials still work (backward compatible)
- ✅ Access rules enforced correctly
- ✅ OpenStack CLI works with app credentials

### Week 2 Exit Criteria
- ✅ All policy engine contract tests GREEN
- ✅ Policy engine parses and evaluates correctly
- ✅ All 5 services enforce policies
- ✅ Ownership checks work
- ✅ Cache provides 40x performance improvement

### Week 3 Exit Criteria (Final Gate)
- ✅ All 81 contract tests GREEN (71 existing + 10 new)
- ✅ Integration tests pass
- ✅ Performance targets met:
  - App credential auth < 30ms p95
  - Policy check (cached) < 1ms p95
  - Policy check (uncached) < 5ms p95
- ✅ Backward compatible (existing auth unchanged)
- ✅ Security audit passes
- ✅ Documentation complete
- ✅ Migration tested on v0.5.0 → v0.6.0

---

## Configuration

```yaml
# config/o3k.yaml (enhanced Keystone section)

keystone:
  # Existing configuration
  jwt_secret: "env:KEYSTONE_JWT_SECRET"  # Use environment variable
  token_ttl: 24h

  # Application Credentials
  application_credentials:
    enabled: true
    max_ttl: 365d
    default_ttl: 90d
    allow_unrestricted: false
    # Legacy secret deprecation
    legacy_warning: true              # Warn when legacy credentials used
    legacy_deprecation_date: "2026-09-18"  # 6 months from now

  # Policy Engine
  policy:
    enabled: true
    default_policy_file: "/etc/o3k/policy.json"
    cache_ttl: 5m
    fail_open: false                  # Fail-closed (deny if policy check errors)

  # Audit Logging
  audit:
    enabled: true
    log_authentication_events: true
    log_policy_decisions: false       # Too verbose
    retention_days: 90
```

---

## Future Phases

**Phase 2: Federation/SSO** (3-4 weeks)
- SAML 2.0, OIDC, external IdP integration

**Phase 3: OAuth2 Server** (1-2 weeks)
- OAuth2 authorization server for third-party apps

**Decision Point**: Implement based on user demand after 3-week minimal IAM deployment.

---

**End of Specification v2**
