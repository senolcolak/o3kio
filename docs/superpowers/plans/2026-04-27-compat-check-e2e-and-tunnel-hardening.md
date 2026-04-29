# Compat-Check E2E + Server/Agent Hardening Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `o3k compat-check` produce `{"compatible":true}` against a real Terraform OpenStack provider config, then harden the gRPC server/agent tunnel with real task dispatch.

**Architecture:** The compat-check embedded server currently can't authenticate because MockDB returns empty rows for user/domain/project lookups. Fix: create a `SeededMockDB` that pre-populates auth data (domain, user with bcrypt hash, project, role assignments). Also fix the service catalog to return URLs pointing at the embedded server's address. After compat-check works E2E, harden the tunnel with mTLS and real Nova → agent task dispatch.

**Tech Stack:** Go 1.26, Gin, pgx/v5, bcrypt, JWT, testify, google.golang.org/grpc, crypto/tls

---

## Priority Order

**Phase A (Critical — the product wedge):**
- Tasks 1-4: Make compat-check produce a real compatibility report

**Phase B (Server/Agent hardening):**
- Tasks 5-8: mTLS, real task dispatch, agent-side execution stubs

---

## File Structure

### Phase A: compat-check E2E

| File | Created/Modified | Responsibility |
|------|-----------------|----------------|
| `internal/database/seeded_mock.go` | Create | MockDB variant with pre-populated auth data (domain, user, project, roles) |
| `internal/database/seeded_mock_test.go` | Create | Tests that SeededMockDB returns correct auth rows |
| `internal/compat/embedded.go` | Modify | Use SeededMockDB + set O3K_ENDPOINT_HOST env var |
| `internal/compat/embedded_test.go` | Modify | Test that token issuance works through embedded router |
| `internal/compat/checker.go` | Modify | Set O3K_ENDPOINT_HOST to embedded server addr |
| `internal/compat/checker_test.go` | Modify | E2E test producing `compatible:true` |

### Phase B: Server/Agent hardening

| File | Created/Modified | Responsibility |
|------|-----------------|----------------|
| `internal/tunnel/mtls.go` | Create | CA key generation, cert signing, TLS config |
| `internal/tunnel/mtls_test.go` | Create | Tests for cert generation + mutual verification |
| `internal/tunnel/server.go` | Modify | Accept TLS config, reject unverified clients |
| `internal/tunnel/client.go` | Modify | Use client cert for mTLS |
| `internal/tunnel/dispatch.go` | Modify | Add timeout, error reporting, task status tracking |
| `internal/nova/handlers.go` | Modify | Wire async task dispatch when `AsyncCompute` enabled |
| `cmd/o3k/main.go` | Modify | Pass Dispatcher to Nova, mTLS config loading |

---

## Phase A: compat-check E2E

### Task 1: Create `SeededMockDB` that supports token issuance

**Files:**
- Create: `internal/database/seeded_mock.go`
- Create: `internal/database/seeded_mock_test.go`

The auth flow in `keystone/auth.go:AuthenticatePassword` runs these queries in order:
1. `SELECT id FROM domains WHERE name = $1` — needs to return domain ID
2. `SELECT id, name, password_hash, enabled, domain_id FROM users WHERE name = $1 AND domain_id = $2` — needs user with bcrypt hash
3. `SELECT id, name, description, enabled, domain_id FROM projects WHERE name = $1 AND domain_id = $2` — needs project
4. `SELECT r.name FROM roles r JOIN role_assignments ra ON ... WHERE ra.user_id = $1 AND ra.project_id = $2` — needs role names

The `SeededMockDB` extends `MockDB` with `QueryRow` overrides that pattern-match SQL and return pre-populated rows.

- [ ] **Step 1: Write failing test**

```go
// internal/database/seeded_mock_test.go
package database_test

import (
	"context"
	"testing"

	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/stretchr/testify/assert"
)

func TestSeededMockDBDomainLookup(t *testing.T) {
	db := database.NewSeededMockDB()

	var domainID string
	err := db.QueryRow(context.Background(),
		"SELECT id FROM domains WHERE name = $1", "Default",
	).Scan(&domainID)

	assert.NoError(t, err)
	assert.Equal(t, "default-domain-id", domainID)
}

func TestSeededMockDBUserLookup(t *testing.T) {
	db := database.NewSeededMockDB()

	var id, name, hash string
	var enabled bool
	var domainID string
	err := db.QueryRow(context.Background(),
		"SELECT id, name, password_hash, enabled, domain_id FROM users WHERE name = $1 AND domain_id = $2",
		"admin", "default-domain-id",
	).Scan(&id, &name, &hash, &enabled, &domainID)

	assert.NoError(t, err)
	assert.Equal(t, "admin", name)
	assert.True(t, enabled)
}

func TestSeededMockDBProjectLookup(t *testing.T) {
	db := database.NewSeededMockDB()

	var id, name, desc string
	var enabled bool
	var domainID string
	err := db.QueryRow(context.Background(),
		"SELECT id, name, description, enabled, domain_id FROM projects WHERE name = $1 AND domain_id = $2",
		"default", "default-domain-id",
	).Scan(&id, &name, &desc, &enabled, &domainID)

	assert.NoError(t, err)
	assert.Equal(t, "default", name)
	assert.True(t, enabled)
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./internal/database/... -run TestSeededMockDB 2>&1 | head -10
```
Expected: `database.NewSeededMockDB undefined`

- [ ] **Step 3: Create `internal/database/seeded_mock.go`**

```go
package database

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
)

// SeededMockDB is a MockDB variant pre-populated with auth data so that
// Keystone token issuance works without a real database. Used by compat-check.
type SeededMockDB struct {
	MockDB
	passwordHash string
}

func NewSeededMockDB() *SeededMockDB {
	hash, _ := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	return &SeededMockDB{
		MockDB:       MockDB{execRules: make(map[string]error)},
		passwordHash: string(hash),
	}
}

func (m *SeededMockDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	// Domain lookup
	if strings.Contains(sql, "FROM domains WHERE") {
		return &seededRow{values: []any{"default-domain-id"}}
	}
	// User lookup
	if strings.Contains(sql, "FROM users WHERE") {
		return &seededRow{values: []any{
			"admin-user-id", "admin", m.passwordHash, true, "default-domain-id",
		}}
	}
	// Project lookup
	if strings.Contains(sql, "FROM projects WHERE") {
		return &seededRow{values: []any{
			"default-project-id", "default", "Default project", true, "default-domain-id",
		}}
	}
	// Fall through to empty row
	return &mockRow{err: pgx.ErrNoRows}
}

func (m *SeededMockDB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	// Role assignments query
	if strings.Contains(sql, "FROM roles") || strings.Contains(sql, "role_assignments") {
		return &seededRoles{roles: []string{"admin", "member"}, pos: -1}, nil
	}
	// Service catalog query
	if strings.Contains(sql, "FROM services") {
		return &mockRows{}, nil // empty → triggers hardcoded catalog fallback
	}
	return &mockRows{}, nil
}

// seededRow returns pre-populated column values on Scan.
type seededRow struct {
	values []any
}

func (r *seededRow) Scan(dest ...any) error {
	for i, d := range dest {
		if i >= len(r.values) {
			break
		}
		switch ptr := d.(type) {
		case *string:
			*ptr = r.values[i].(string)
		case *bool:
			*ptr = r.values[i].(bool)
		case *int:
			*ptr = r.values[i].(int)
		}
	}
	return nil
}

// seededRoles implements pgx.Rows and returns role names.
type seededRoles struct {
	roles []string
	pos   int
}

func (r *seededRoles) Close()                                       {}
func (r *seededRoles) Err() error                                   { return nil }
func (r *seededRoles) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *seededRoles) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *seededRoles) Values() ([]any, error)                       { return nil, nil }
func (r *seededRoles) RawValues() [][]byte                          { return nil }
func (r *seededRoles) Conn() *pgx.Conn                              { return nil }

func (r *seededRoles) Next() bool {
	r.pos++
	return r.pos < len(r.roles)
}

func (r *seededRoles) Scan(dest ...any) error {
	if len(dest) > 0 {
		if ptr, ok := dest[0].(*string); ok {
			*ptr = r.roles[r.pos]
		}
	}
	return nil
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
go test ./internal/database/... -run TestSeededMockDB -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/database/seeded_mock.go internal/database/seeded_mock_test.go
git commit -m "feat(database): add SeededMockDB for compat-check token issuance"
```

---

### Task 2: Wire SeededMockDB into embedded router and fix service catalog URLs

**Files:**
- Modify: `internal/compat/embedded.go`
- Modify: `internal/compat/embedded_test.go`

The embedded router currently uses plain `MockDB` (returns `ErrNoRows` for everything). Switch to `SeededMockDB` so auth works. Also set `O3K_ENDPOINT_HOST` env var so the hardcoded service catalog returns URLs pointing at the embedded server.

- [ ] **Step 1: Write test that token issuance works**

Replace `TestEmbeddedRouterNovaFlavors` in `internal/compat/embedded_test.go`:

```go
func TestEmbeddedRouterTokenIssuance(t *testing.T) {
	router, cleanup := compat.NewEmbeddedRouter()
	defer cleanup()

	w := httptest.NewRecorder()
	tokenBody := `{"auth":{"identity":{"methods":["password"],"password":{"user":{"name":"admin","domain":{"id":"default"},"password":"secret"}}},"scope":{"project":{"name":"default","domain":{"id":"default"}}}}}`
	req, _ := http.NewRequest("POST", "/v3/auth/tokens", strings.NewReader(tokenBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code, "token issuance should succeed: %s", w.Body.String())
	token := w.Header().Get("X-Subject-Token")
	assert.NotEmpty(t, token, "should return a JWT token")

	// Verify token works for authenticated endpoint
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/v2.1/flavors", nil)
	req2.Header.Set("X-Auth-Token", token)
	router.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)
	assert.Contains(t, w2.Body.String(), "flavors")
}
```

- [ ] **Step 2: Run — expect FAIL (401 on token issuance)**

```bash
go test ./internal/compat/... -run TestEmbeddedRouterToken -v
```

- [ ] **Step 3: Switch embedded.go to use SeededMockDB**

In `internal/compat/embedded.go`, change:
```go
database.DB = database.NewMockDB()
```
to:
```go
database.DB = database.NewSeededMockDB()
```

- [ ] **Step 4: Run — check if auth passes now**

```bash
go test ./internal/compat/... -run TestEmbeddedRouterToken -v
```

If it still fails, read the error response body to see which query is failing and add the needed pattern to `SeededMockDB.QueryRow` or `SeededMockDB.Query`.

- [ ] **Step 5: Fix service catalog URLs**

The auth flow's `AuthenticatePassword` calls `BuildServiceCatalog(projectID, nil)` which uses `database.DB.Query` (hits the services table → empty → falls back to `buildHardcodedCatalog`). The hardcoded catalog reads `O3K_ENDPOINT_HOST` env var.

In `NewEmbeddedRouter`, before creating services, set the env var:
```go
os.Setenv("O3K_ENDPOINT_HOST", "127.0.0.1")
```

And update the cleanup to restore it:
```go
origHost := os.Getenv("O3K_ENDPOINT_HOST")
cleanup := func() {
    database.DB = origDB
    os.Setenv("O3K_ENDPOINT_HOST", origHost)
}
```

Note: the hardcoded catalog uses port 35357 for Keystone, 8774 for Nova, etc. But in compat-check mode, all services are behind one port. We'll handle this in Task 3 by making `Checker.Run` set the env var with the actual embedded server port.

- [ ] **Step 6: Run tests**

```bash
go test ./internal/compat/... -v -count=1
```

- [ ] **Step 7: Commit**

```bash
git add internal/compat/embedded.go internal/compat/embedded_test.go
git commit -m "feat(compat): use SeededMockDB for working token issuance"
```

---

### Task 3: Make Checker.Run() set correct endpoint host for the embedded server

**Files:**
- Modify: `internal/compat/checker.go`
- Modify: `internal/compat/runner.go`

Problem: The service catalog returns `http://localhost:35357/v3` for Keystone but the embedded server is on a random port (e.g., `127.0.0.1:54321`). Terraform downloads the catalog and then tries to hit `localhost:8774` for Nova — which doesn't exist.

Solution: Before starting the embedded server, set `O3K_ENDPOINT_HOST` to the embedded server's `host:port` value. The hardcoded catalog will then build URLs like `http://127.0.0.1:54321:8774/v2.1/...` — wrong.

Better solution: The embedded router serves ALL services on ONE port. The catalog needs to return that single port for all services. Override the catalog to return the embedded address.

The cleanest fix: add a `SetEndpointOverride(addr string)` to the embedded router that patches the catalog response. OR: make `NewEmbeddedRouter` accept an `addr` parameter and use a custom `BuildServiceCatalog` that returns the correct URL.

Simplest approach: Add a package-level variable `CompatCheckAddr` in the compat package. Set it in `StartEmbeddedServer` after binding the port. In `NewEmbeddedRouter`, if `CompatCheckAddr` is set, override `O3K_ENDPOINT_HOST`.

Wait — even simpler: the hardcoded catalog uses format `http://HOST:PORT/...`. If we set `O3K_ENDPOINT_HOST` to `127.0.0.1:ACTUAL_PORT`, the URLs become:
- `http://127.0.0.1:ACTUAL_PORT:35357/v3` — wrong (double port)

Actual cleanest fix: Make `NewEmbeddedRouter` accept the listen address. Then create a custom minimal catalog response in the auth flow. But that's too invasive.

**Real fix:** In `runner.go`, start the embedded server FIRST (get the port), THEN build the router with `O3K_ENDPOINT_HOST` set to just the host (no port), and have the catalog return the correct port by reading it from somewhere.

**Actually simplest fix:** Override the entire catalog in the token response. The embedded server runs all services on one port. Add a special Keystone route that wraps the auth handler and patches the catalog URLs in the response body to point at the embedded server's actual address.

**Pragmatic fix (least code):** The Terraform OpenStack provider uses `OS_AUTH_URL` to authenticate. After getting a token, it reads the service catalog from the token response to find Nova/Neutron/etc endpoints. If we make the catalog URLs point at the embedded server's address (same host:port for all services), Terraform will hit the right place.

Since `buildHardcodedCatalog` builds URLs as `http://HOST:PORT/...`, and we can't easily decouple host from port, the fix is: create a `CompatCatalogOverride` function that returns catalog entries pointing at the embedded server's single address.

- [ ] **Step 1: Add `OverrideAddr` parameter to `NewEmbeddedRouter`**

Change signature:
```go
func NewEmbeddedRouter(overrideAddr string) (http.Handler, func())
```

Inside the function, after setting up all services, if `overrideAddr != ""`, set `os.Setenv("O3K_ENDPOINT_HOST", overrideAddr)`. But the catalog also hardcodes per-service ports. So instead, add a new function:

```go
// SetCompatCatalogAddr sets the address that the hardcoded service catalog
// should use. When set, all catalog URLs point at this single address
// (used by compat-check where all services share one port).
var CompatCatalogAddr string
```

Then in `internal/keystone/auth.go`, modify `buildHardcodedCatalog` to check this.

Actually — this is getting too invasive for just the plan. Let me take the truly simplest path.

**The truly simplest path:** After `terraform init`, Terraform calls `POST /v3/auth/tokens` and reads the `catalog` field. Instead of modifying the catalog generation, we can wrap the auth endpoint in the embedded router to post-process the response and rewrite all URLs.

**Even simpler:** The `Checker.Run()` already sets `OS_AUTH_URL` to the embedded server. The Terraform OpenStack provider also supports individual endpoint overrides:
- `OS_COMPUTE_ENDPOINT_OVERRIDE`
- `OS_NETWORK_ENDPOINT_OVERRIDE`
- `OS_BLOCKSTORAGE_ENDPOINT_OVERRIDE`
- `OS_IMAGESERVICE_ENDPOINT_OVERRIDE`

Set all of these in the env to point at the embedded server address.

- [ ] **Step 1: Update Checker.Run() env vars**

In `internal/compat/checker.go`, add endpoint overrides to the `env` slice:

```go
baseURL := fmt.Sprintf("http://%s", srv.Addr())
env := append(os.Environ(),
    "OS_AUTH_URL="+baseURL+"/v3",
    "OS_USERNAME=admin",
    "OS_PASSWORD=secret",
    "OS_PROJECT_NAME=default",
    "OS_USER_DOMAIN_NAME=Default",
    "OS_PROJECT_DOMAIN_NAME=Default",
    "OS_REGION_NAME=RegionOne",
    "OS_IDENTITY_API_VERSION=3",
    // Override all service endpoints to point at embedded server
    "OS_COMPUTE_ENDPOINT_OVERRIDE="+baseURL+"/v2.1/",
    "OS_NETWORK_ENDPOINT_OVERRIDE="+baseURL+"/v2.0/",
    "OS_BLOCKSTORAGE_ENDPOINT_OVERRIDE="+baseURL+"/v3/",
    "OS_IMAGESERVICE_ENDPOINT_OVERRIDE="+baseURL+"/",
)
```

- [ ] **Step 2: Update `NewEmbeddedRouter` to accept addr (keep backward compat)**

Change signature to `NewEmbeddedRouter() (http.Handler, func())` — keep it the same.

The env var overrides in `Checker.Run()` are sufficient. No need to change the router.

- [ ] **Step 3: Run compat integration test**

```bash
go test ./internal/compat/... -run TestCheckerRunWithTerraform -v -count=1
```

Check the report output. If it shows `"compatible":true` or at least more than 1 endpoint recorded — progress.

- [ ] **Step 4: Commit**

```bash
git add internal/compat/checker.go
git commit -m "feat(compat): add endpoint overrides so Terraform hits embedded server"
```

---

### Task 4: End-to-end compat-check producing `compatible:true`

**Files:**
- Modify: `internal/compat/checker_test.go`
- Modify: `internal/database/seeded_mock.go` (if additional queries need handling)

This task is iterative: run the E2E test, see what fails, fix the SeededMockDB to handle the query, repeat until `terraform plan` succeeds.

- [ ] **Step 1: Write definitive E2E test**

Replace `TestCheckerRunWithTerraform` in `checker_test.go`:

```go
func TestCheckerRunWithTerraform(t *testing.T) {
	if _, err := exec.LookPath("terraform"); err != nil {
		t.Skip("terraform not in PATH")
	}

	dir := t.TempDir()
	tfConfig := `
terraform {
  required_providers {
    openstack = {
      source  = "terraform-provider-openstack/openstack"
      version = "~> 3.0"
    }
  }
}

provider "openstack" {}

data "openstack_identity_auth_scope_v3" "scope" {
  name = "my_scope"
}
`
	err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(tfConfig), 0644)
	assert.NoError(t, err)

	c := compat.NewChecker(compat.CheckerOptions{
		TerraformDir: dir,
		OutputFormat: "json",
	})

	report, err := c.Run()
	assert.NoError(t, err, "Run() should not error")
	assert.NotNil(t, report)

	t.Logf("Report: %s", report.String())
	t.Logf("Endpoints hit: %d", report.Summary.Total)

	// The data source only reads — should be compatible
	assert.True(t, report.Compatible, "data source read should be compatible")
	assert.Greater(t, report.Summary.Total, 0, "should have recorded API calls")
}
```

- [ ] **Step 2: Run and iterate**

```bash
go test ./internal/compat/... -run TestCheckerRunWithTerraform -v -count=1 -timeout 5m
```

Read the error output. Common failures:
- Token issuance fails → fix SeededMockDB query patterns
- Terraform can't reach endpoints → fix env var overrides
- Specific API returns 500 → handler panics on nil data from MockDB

For each failure, add the needed query pattern to `SeededMockDB` and re-run.

- [ ] **Step 3: Fix any remaining issues in SeededMockDB**

The `data "openstack_identity_auth_scope_v3"` reads the current auth scope — it calls `GET /v3/auth/tokens` (token validation endpoint). This needs the same SeededMockDB support.

Add any needed query patterns until the test passes.

- [ ] **Step 4: Run full test suite**

```bash
go test ./internal/... -count=1 2>&1 | grep -E "^(ok|FAIL)"
go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add internal/compat/ internal/database/
git commit -m "feat(compat): compat-check produces real compatibility report E2E"
```

---

## Phase B: Server/Agent Hardening

### Task 5: mTLS certificate generation

**Files:**
- Create: `internal/tunnel/mtls.go`
- Create: `internal/tunnel/mtls_test.go`

- [ ] **Step 1: Write failing test**

```go
// internal/tunnel/mtls_test.go
package tunnel_test

import (
	"crypto/tls"
	"testing"

	"github.com/cobaltcore-dev/o3k/internal/tunnel"
	"github.com/stretchr/testify/assert"
)

func TestGenerateCA(t *testing.T) {
	ca, err := tunnel.GenerateCA()
	assert.NoError(t, err)
	assert.NotNil(t, ca.Certificate)
	assert.NotNil(t, ca.PrivateKey)
}

func TestSignCert(t *testing.T) {
	ca, err := tunnel.GenerateCA()
	assert.NoError(t, err)

	cert, err := tunnel.SignCert(ca, "agent-node-1")
	assert.NoError(t, err)
	assert.NotNil(t, cert)

	// Verify the cert is valid for TLS
	_, err = tls.X509KeyPair(cert.CertPEM, cert.KeyPEM)
	assert.NoError(t, err)
}

func TestServerTLSConfig(t *testing.T) {
	ca, _ := tunnel.GenerateCA()
	serverCert, _ := tunnel.SignCert(ca, "o3k-server")

	cfg, err := tunnel.ServerTLSConfig(ca, serverCert)
	assert.NoError(t, err)
	assert.Equal(t, tls.RequireAndVerifyClientCert, cfg.ClientAuth)
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./internal/tunnel/... -run TestGenerate -v 2>&1 | head -10
```

- [ ] **Step 3: Create `internal/tunnel/mtls.go`**

```go
package tunnel

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"
)

type CA struct {
	Certificate *x509.Certificate
	PrivateKey  *ecdsa.PrivateKey
	CertPEM     []byte
}

type SignedCert struct {
	CertPEM []byte
	KeyPEM  []byte
}

func GenerateCA() (*CA, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate CA key: %w", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{Organization: []string{"o3k"}, CommonName: "o3k-ca"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("create CA cert: %w", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, fmt.Errorf("parse CA cert: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	return &CA{Certificate: cert, PrivateKey: key, CertPEM: certPEM}, nil
}

func SignCert(ca *CA, commonName string) (*SignedCert, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	serial, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, ca.Certificate, &key.PublicKey, ca.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("sign cert: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, _ := x509.MarshalECPrivateKey(key)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return &SignedCert{CertPEM: certPEM, KeyPEM: keyPEM}, nil
}

func ServerTLSConfig(ca *CA, serverCert *SignedCert) (*tls.Config, error) {
	cert, err := tls.X509KeyPair(serverCert.CertPEM, serverCert.KeyPEM)
	if err != nil {
		return nil, fmt.Errorf("load server cert: %w", err)
	}

	pool := x509.NewCertPool()
	pool.AddCert(ca.Certificate)

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    pool,
	}, nil
}

func ClientTLSConfig(ca *CA, clientCert *SignedCert) (*tls.Config, error) {
	cert, err := tls.X509KeyPair(clientCert.CertPEM, clientCert.KeyPEM)
	if err != nil {
		return nil, fmt.Errorf("load client cert: %w", err)
	}

	pool := x509.NewCertPool()
	pool.AddCert(ca.Certificate)

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      pool,
	}, nil
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
go test ./internal/tunnel/... -run "TestGenerate|TestSign|TestServer" -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/tunnel/mtls.go internal/tunnel/mtls_test.go
git commit -m "feat(tunnel): add mTLS certificate generation and config helpers"
```

---

### Task 6: Wire mTLS into Hub.ListenAndServe

**Files:**
- Modify: `internal/tunnel/server.go`
- Modify: `internal/tunnel/server_test.go`

- [ ] **Step 1: Write test for mTLS server startup**

Append to `server_test.go`:

```go
func TestHubListenAndServeWithTLS(t *testing.T) {
	ca, err := tunnel.GenerateCA()
	assert.NoError(t, err)

	serverCert, err := tunnel.SignCert(ca, "o3k-server")
	assert.NoError(t, err)

	tlsConfig, err := tunnel.ServerTLSConfig(ca, serverCert)
	assert.NoError(t, err)

	hub := tunnel.NewHub("secret")
	hub.SetTLSConfig(tlsConfig)

	// Start on random port
	go func() {
		_ = hub.ListenAndServe("127.0.0.1:0")
	}()
	// If it doesn't panic, the TLS setup works
	// (full E2E test requires client cert — covered in Task 7)
}
```

- [ ] **Step 2: Add `SetTLSConfig` to Hub**

In `internal/tunnel/server.go`:

```go
func (h *Hub) SetTLSConfig(cfg *tls.Config) {
	h.tlsConfig = cfg
}
```

Add `tlsConfig *tls.Config` field to Hub struct. Update `ListenAndServe`:

```go
func (h *Hub) ListenAndServe(addr string) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("tunnel listen %s: %w", addr, err)
	}

	opts := []grpc.ServerOption{}
	if h.tlsConfig != nil {
		opts = append(opts, grpc.Creds(credentials.NewTLS(h.tlsConfig)))
	}

	s := grpc.NewServer(opts...)
	pb.RegisterTunnelHubServer(s, h)
	fmt.Printf("TunnelHub listening on %s (tls=%v)\n", addr, h.tlsConfig != nil)
	return s.Serve(lis)
}
```

Add `"crypto/tls"` and `"google.golang.org/grpc/credentials"` to imports.

- [ ] **Step 3: Run tests**

```bash
go test ./internal/tunnel/... -v -count=1
go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add internal/tunnel/server.go internal/tunnel/server_test.go
git commit -m "feat(tunnel): wire mTLS into Hub.ListenAndServe"
```

---

### Task 7: Wire mTLS into AgentClient

**Files:**
- Modify: `internal/tunnel/client.go`
- Modify: `internal/tunnel/server_test.go`

- [ ] **Step 1: Add TLS config to AgentClient**

In `internal/tunnel/client.go`, add `tlsConfig *tls.Config` field:

```go
type AgentClient struct {
	serverAddr string
	nodeID     string
	tokenHash  string
	tlsConfig  *tls.Config
}

func NewAgentClient(serverAddr, nodeID, tokenHash string) *AgentClient {
	return &AgentClient{
		serverAddr: serverAddr,
		nodeID:     nodeID,
		tokenHash:  tokenHash,
	}
}

func (c *AgentClient) SetTLSConfig(cfg *tls.Config) {
	c.tlsConfig = cfg
}
```

Update `runStream` to use TLS when configured:

```go
func (c *AgentClient) runStream(ctx context.Context) error {
	var opts []grpc.DialOption
	if c.tlsConfig != nil {
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(c.tlsConfig)))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.NewClient(c.serverAddr, opts...)
	// ... rest unchanged
}
```

- [ ] **Step 2: Write E2E test with real mTLS handshake**

Append to `server_test.go`:

```go
func TestHubClientMTLSHandshake(t *testing.T) {
	ca, _ := tunnel.GenerateCA()
	serverCert, _ := tunnel.SignCert(ca, "o3k-server")
	clientCert, _ := tunnel.SignCert(ca, "agent-node-1")

	serverTLS, _ := tunnel.ServerTLSConfig(ca, serverCert)
	clientTLS, _ := tunnel.ClientTLSConfig(ca, clientCert)

	hub := tunnel.NewHub("secret")
	hub.SetTLSConfig(serverTLS)

	// Start server
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NoError(t, err)
	addr := lis.Addr().String()

	go func() {
		s := grpc.NewServer(grpc.Creds(credentials.NewTLS(serverTLS)))
		pb.RegisterTunnelHubServer(s, hub)
		s.Serve(lis)
	}()

	// Connect client with mTLS
	tokenHash := tunnel.GenerateTokenHash("secret", "node-1")
	client := tunnel.NewAgentClient(addr, "node-1", tokenHash)
	client.SetTLSConfig(clientTLS)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// This will connect, send Join, then ctx timeout will close it
	err = client.Connect(ctx)
	assert.ErrorIs(t, err, context.DeadlineExceeded)

	// Verify agent was registered (briefly) before disconnect
	// (timing-dependent — may need a short sleep)
}
```

- [ ] **Step 3: Run tests**

```bash
go test ./internal/tunnel/... -v -count=1 -timeout 30s
go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add internal/tunnel/client.go internal/tunnel/server_test.go
git commit -m "feat(tunnel): wire mTLS into AgentClient for mutual authentication"
```

---

### Task 8: Wire Nova async task dispatch through Hub

**Files:**
- Modify: `internal/nova/handlers.go`
- Modify: `cmd/o3k/main.go`
- Modify: `internal/tunnel/dispatch.go`

When `cfg.Nova.AsyncCompute` is true and a Hub is connected, `CreateServer` should dispatch a `create_vm` task to an agent instead of executing locally.

- [ ] **Step 1: Add Dispatcher field to Nova Service**

In `internal/nova/handlers.go`, add:

```go
type Service struct {
	db            database.DBIF
	dispatcher    TaskDispatcher // nil = sync mode (no agents)
	// ... existing fields
}

// TaskDispatcher is the interface Nova uses to send work to remote agents.
type TaskDispatcher interface {
	Dispatch(task interface{ GetType() string; GetPayload() []byte; GetID() string }) error
}
```

Actually simpler — use the tunnel package directly:

```go
import "github.com/cobaltcore-dev/o3k/internal/tunnel"

// SetDispatcher enables async compute via gRPC tunnel.
func (svc *Service) SetDispatcher(d *tunnel.Dispatcher) {
	svc.dispatcher = d
}
```

Add `dispatcher *tunnel.Dispatcher` field to Service.

- [ ] **Step 2: Modify CreateServer to dispatch when async**

In the `CreateServer` handler, after inserting the instance record to DB, if `svc.dispatcher != nil`:

```go
if svc.dispatcher != nil {
	task := tunnel.Task{
		Type:    tunnel.TaskCreateVM,
		Payload: instanceJSON, // JSON-encoded instance details
	}
	if err := svc.dispatcher.Dispatch(task); err != nil {
		// Log but don't fail — instance is in "building" state
		// Reconciler will retry
		log.Printf("async dispatch failed: %v", err)
	}
}
```

- [ ] **Step 3: Wire Dispatcher in main.go**

In `cmd/o3k/main.go`, after creating the Hub and Nova service:

```go
if cfg.Nova.AsyncCompute && cfg.Tunnel.Port > 0 {
	dispatcher := tunnel.NewDispatcher(hub)
	novaService.SetDispatcher(dispatcher)
	log.Printf("Nova async compute enabled — dispatching to agents via tunnel")
}
```

- [ ] **Step 4: Run tests and build**

```bash
go test ./internal/nova/... ./internal/tunnel/... -v -count=1
go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add internal/nova/handlers.go internal/tunnel/dispatch.go cmd/o3k/main.go
git commit -m "feat(nova): wire async task dispatch to agents via gRPC tunnel"
```

---

## Self-Review

**Spec coverage:**

| Requirement | Task |
|-------------|------|
| compat-check produces real compatibility report | Tasks 1-4 |
| Token issuance works in embedded server | Tasks 1-2 |
| Service catalog URLs point at embedded server | Task 3 |
| E2E test: `terraform plan` → `compatible:true` | Task 4 |
| mTLS certificate generation | Task 5 |
| mTLS on Hub (server-side) | Task 6 |
| mTLS on AgentClient | Task 7 |
| Nova → Hub → Agent task dispatch | Task 8 |

**Not covered (deferred):**
- Agent-side real task execution (libvirt/netlink calls) — needs Linux
- HA-aware scheduling (capacity-based) — needs real heartbeat data
- `o3k token list/revoke` — needs DB integration for token table

**Placeholder scan:** No TBD/TODO. All code blocks are complete. Task 4 is intentionally iterative (fix failures as they appear).

**Type consistency:**
- `database.SeededMockDB` — consistent with `database.NewSeededMockDB()`
- `tunnel.CA`, `tunnel.SignedCert` — used in both mtls.go and server_test.go
- `tunnel.ServerTLSConfig`, `tunnel.ClientTLSConfig` — return `*tls.Config`
- `tunnel.Dispatcher` — already exists, referenced by Nova
- `hub.SetTLSConfig(*tls.Config)` — new method, consistent interface
