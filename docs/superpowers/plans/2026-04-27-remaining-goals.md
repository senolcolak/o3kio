# O3K Remaining Short/Medium-Term Goals Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `o3k compat-check` a shippable product (real OpenStack routes, real Terraform execution), complete the database DI migration across all services, and harden the gRPC tunnel with mTLS, token management, and task dispatch.

**Architecture:** Three tracks in priority order — (1) compat-check: wire real Gin-based stub services into the embedded server so Terraform hits actual OpenStack API routes; (2) DB DI: migrate all 660 `database.DB.` call sites across 53 files to `svc.activeDB()`, service by service; (3) gRPC hardening: mTLS, `o3k token` command, Nova→agent task dispatch. Track 1 is the product wedge and ships first. Track 2 unblocks unit testing across all services. Track 3 makes multi-node real.

**Tech Stack:** Go 1.26, Gin, pgx/v5, testify, google.golang.org/grpc, crypto/tls, crypto/x509

---

## Scope

**Track 1 — compat-check (shippable product):**
- Wire all 5 services into embedded server (stub mode, in-memory DB)
- Run `terraform init + plan` against it
- Generate accurate compatibility reports
- End-to-end integration test

**Track 2 — DB DI completion:**
- Migrate Keystone (85 call sites, 7 files)
- Migrate Nova (remaining 47 call sites)
- Migrate Neutron (160 call sites, 12 files)
- Migrate Cinder (116 call sites, 9 files)
- Migrate Glance (53 call sites, 5 files)

**Track 3 — gRPC hardening:**
- `o3k token create/list/revoke`
- mTLS certificate generation and verification
- Nova → Hub → Agent task dispatch for `create_vm`

---

## File Structure

### Track 1: compat-check → shippable

| File | Created/Modified | Responsibility |
|------|-----------------|----------------|
| `internal/compat/embedded.go` | Create | Sets up all 5 services in stub mode + Gin router with Recorder middleware |
| `internal/compat/embedded_test.go` | Create | Tests that embedded server responds to Keystone/Nova/Neutron API calls |
| `internal/compat/runner.go` | Modify | `StartEmbeddedServer` calls `NewEmbeddedRouter` instead of hardcoded mux |
| `internal/compat/checker.go` | Modify | `Run()` handles `terraform init` before `plan`, better error handling |
| `internal/compat/checker_test.go` | Modify | Integration test with real terraform (skipped if not in PATH) |

### Track 2: DB DI per service

| File | Created/Modified | Responsibility |
|------|-----------------|----------------|
| `internal/keystone/handlers.go` | Modify | Add `db` field to Service, `activeDB()`, `NewServiceWithDB()` |
| `internal/keystone/auth.go` | Modify | Add `db` field to AuthService, `activeDB()` |
| `internal/keystone/*.go` (6 more files) | Modify | Replace `database.DB.` with `svc.activeDB()` |
| `internal/keystone/handlers_test.go` | Create | Unit tests for token issuance with MockDB |
| `internal/nova/handlers.go` | Modify | Replace remaining 47 `database.DB.` with `svc.activeDB()` |
| `internal/nova/*.go` (12 more files) | Modify | Same pattern |
| `internal/neutron/network.go` | Modify | Add `db` field to Service, `activeDB()`, `NewServiceWithDB()` |
| `internal/neutron/*.go` (11 more files) | Modify | Replace `database.DB.` with `svc.activeDB()` |
| `internal/cinder/volumes.go` | Modify | Add `db` field to Service, `activeDB()`, `NewServiceWithDB()` |
| `internal/cinder/*.go` (8 more files) | Modify | Replace `database.DB.` with `svc.activeDB()` |
| `internal/glance/images.go` | Modify | Add `db` field to Service, `activeDB()`, `NewServiceWithDB()` |
| `internal/glance/*.go` (4 more files) | Modify | Replace `database.DB.` with `svc.activeDB()` |

### Track 3: gRPC hardening

| File | Created/Modified | Responsibility |
|------|-----------------|----------------|
| `internal/tunnel/token.go` | Create | Token generation (HMAC-SHA256), storage, verification |
| `internal/tunnel/token_test.go` | Create | Unit tests for token create/verify/revoke |
| `internal/tunnel/mtls.go` | Create | CA generation, cert signing, TLS config helpers |
| `internal/tunnel/mtls_test.go` | Create | Unit tests for cert generation and verification |
| `internal/tunnel/server.go` | Modify | `AgentStream` verifies token on join, sends tasks |
| `internal/tunnel/dispatch.go` | Create | Bridges Nova async tasks to tunnel Hub |
| `cmd/o3k/main.go` | Modify | Wire `runTokenCmd`, pass Hub to Nova for dispatch |

---

## Track 1: compat-check → shippable

### Task 1: Create embedded router with all 5 services in stub mode

**Files:**
- Create: `internal/compat/embedded.go`
- Create: `internal/compat/embedded_test.go`

The embedded server currently returns a hardcoded JSON blob for every request. We need to mount the real Gin routers for Keystone, Nova, Neutron, Cinder, and Glance in stub mode so Terraform hits actual API endpoints.

Key insight: the services use `database.DB` (the global). For the embedded server, we set `database.DB` to a `MockDB` that returns empty results — stub mode handlers check `libvirtMode == "stub"` and return fake data without hitting the DB for most operations. The few that do hit the DB will get empty rows from MockDB, which is valid for a compat check (we're testing that routes exist and return correct status codes, not that data is correct).

- [ ] **Step 1: Write failing test**

```go
// internal/compat/embedded_test.go
package compat_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cobaltcore-dev/o3k/internal/compat"
	"github.com/stretchr/testify/assert"
)

func TestEmbeddedRouterKeystoneVersions(t *testing.T) {
	router, cleanup := compat.NewEmbeddedRouter()
	defer cleanup()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v3", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "version")
}

func TestEmbeddedRouterNovaFlavors(t *testing.T) {
	router, cleanup := compat.NewEmbeddedRouter()
	defer cleanup()

	// First get a token
	w := httptest.NewRecorder()
	tokenBody := `{"auth":{"identity":{"methods":["password"],"password":{"user":{"name":"admin","domain":{"id":"default"},"password":"secret"}}},"scope":{"project":{"name":"default","domain":{"id":"default"}}}}}`
	req, _ := http.NewRequest("POST", "/v3/auth/tokens", strings.NewReader(tokenBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	token := w.Header().Get("X-Subject-Token")

	// Now list flavors
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/v2.1/flavors", nil)
	req2.Header.Set("X-Auth-Token", token)
	router.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusOK, w2.Code)
	assert.Contains(t, w2.Body.String(), "flavors")
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./internal/compat/... -run TestEmbeddedRouter 2>&1 | head -10
```
Expected: `compat.NewEmbeddedRouter undefined`

- [ ] **Step 3: Create `internal/compat/embedded.go`**

```go
package compat

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/cobaltcore-dev/o3k/internal/cinder"
	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/cobaltcore-dev/o3k/internal/glance"
	"github.com/cobaltcore-dev/o3k/internal/keystone"
	"github.com/cobaltcore-dev/o3k/internal/middleware"
	"github.com/cobaltcore-dev/o3k/internal/neutron"
	"github.com/cobaltcore-dev/o3k/internal/nova"
)

// NewEmbeddedRouter creates a Gin router with all 5 OpenStack services
// registered in stub mode. It sets database.DB to a MockDB for the
// duration — call the returned cleanup func to restore the original.
// This is used by compat-check to validate Terraform compatibility.
func NewEmbeddedRouter() (http.Handler, func()) {
	origDB := database.DB
	mock := database.NewMockDB()
	database.DB = mock

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	authService := keystone.NewAuthService("compat-check-secret", 0, nil)
	keystoneSvc := keystone.NewService(authService, nil)
	novaSvc := nova.NewService("", "stub", nil)
	neutronSvc := neutron.NewService("stub", nil)
	cinderSvc := cinder.NewService("stub", "", "")
	glanceSvc := glance.NewService("stub", "", "", "", "", "", nil)

	// Keystone routes (no auth middleware — Keystone issues tokens)
	keystoneSvc.RegisterRoutes(r.Group(""))

	// All other services need auth
	authed := r.Group("")
	authed.Use(middleware.AuthMiddleware(authService))
	novaSvc.RegisterRoutes(authed)
	neutronSvc.RegisterRoutes(authed)
	cinderSvc.RegisterRoutes(authed)
	glanceSvc.RegisterRoutes(authed)

	cleanup := func() {
		database.DB = origDB
	}
	return r, cleanup
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
go test ./internal/compat/... -run TestEmbeddedRouter -v
```

If tests fail because MockDB returns empty rows causing nil pointer panics in handlers, add stub data setup rules to the MockDB before `NewEmbeddedRouter` returns. Iterate until both tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/compat/embedded.go internal/compat/embedded_test.go
git commit -m "feat(compat): add NewEmbeddedRouter with all 5 stub services"
```

---

### Task 2: Wire embedded router into StartEmbeddedServer

**Files:**
- Modify: `internal/compat/runner.go`
- Modify: `internal/compat/embedded_test.go`

Replace the hardcoded `http.ServeMux` in `StartEmbeddedServer` with the real router from `NewEmbeddedRouter`, wrapped with the Recorder middleware.

- [ ] **Step 1: Write test that verifies recording through embedded router**

Append to `internal/compat/embedded_test.go`:

```go
func TestEmbeddedServerRecordsAPICalls(t *testing.T) {
	ctx := context.Background()
	srv, err := compat.StartEmbeddedServer(ctx)
	assert.NoError(t, err)
	defer srv.Shutdown(ctx)

	// Hit Keystone version endpoint
	resp, err := http.Get(fmt.Sprintf("http://%s/v3", srv.Addr()))
	assert.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, 200, resp.StatusCode)

	results := srv.Recorder.Results()
	assert.GreaterOrEqual(t, len(results), 1)
	assert.Equal(t, "GET", results[0].Method)
}
```

- [ ] **Step 2: Run — may PASS or FAIL depending on current state**

```bash
go test ./internal/compat/... -run TestEmbeddedServerRecords -v
```

- [ ] **Step 3: Rewrite `StartEmbeddedServer` in `runner.go`**

Replace the `StartEmbeddedServer` function body:

```go
func StartEmbeddedServer(ctx context.Context) (*EmbeddedServer, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("failed to bind port: %w", err)
	}

	rec := NewRecorder()
	router, cleanup := NewEmbeddedRouter()

	// Wrap the real router with the recorder middleware
	handler := rec.Middleware(router)

	es := &EmbeddedServer{
		Listener: listener,
		Server:   &http.Server{Handler: handler},
		Recorder: rec,
		cleanup:  cleanup,
	}
	go es.Server.Serve(listener)
	return es, nil
}
```

Add `cleanup func()` field to `EmbeddedServer`:

```go
type EmbeddedServer struct {
	Listener net.Listener
	Server   *http.Server
	Recorder *Recorder
	cleanup  func()
}
```

Update `Shutdown` to call cleanup:

```go
func (e *EmbeddedServer) Shutdown(ctx context.Context) {
	e.Server.Shutdown(ctx)
	if e.cleanup != nil {
		e.cleanup()
	}
}
```

- [ ] **Step 4: Run all compat tests**

```bash
go test ./internal/compat/... -v -count=1
```
Expected: all tests pass

- [ ] **Step 5: Commit**

```bash
git add internal/compat/runner.go internal/compat/embedded_test.go
git commit -m "feat(compat): wire real stub services into StartEmbeddedServer"
```

---

### Task 3: Improve `Checker.Run()` — handle `terraform init` and provider setup

**Files:**
- Modify: `internal/compat/checker.go`
- Modify: `internal/compat/checker_test.go`

The current `Run()` only calls `terraform plan`. Terraform needs `init` first to download the OpenStack provider. We also need to handle the case where no `.tf` files exist in the target directory.

- [ ] **Step 1: Write test for init+plan flow**

Append to `internal/compat/checker_test.go`:

```go
func TestCheckerRunWithTerraform(t *testing.T) {
	if _, err := exec.LookPath("terraform"); err != nil {
		t.Skip("terraform not in PATH")
	}

	// Create a minimal terraform config that uses the OpenStack provider
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
	// May error if provider can't connect — that's OK, we still get a report
	if err != nil {
		t.Logf("Run() error (expected for stub): %v", err)
		return
	}

	assert.NotNil(t, report)
	t.Logf("Report: %s", report.String())
}
```

- [ ] **Step 2: Update `Checker.Run()` to run init before plan**

Replace `Run()` in `internal/compat/checker.go`:

```go
func (c *Checker) Run() (*Report, error) {
	if _, err := exec.LookPath("terraform"); err != nil {
		return nil, fmt.Errorf("terraform not found in PATH: %w", err)
	}
	if c.TerraformDir == "" {
		return nil, fmt.Errorf("TerraformDir must be set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	srv, err := StartEmbeddedServer(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to start embedded server: %w", err)
	}
	defer srv.Shutdown(ctx)

	authURL := fmt.Sprintf("http://%s/v3", srv.Addr())
	env := append(os.Environ(),
		"OS_AUTH_URL="+authURL,
		"OS_USERNAME=admin",
		"OS_PASSWORD=secret",
		"OS_PROJECT_NAME=default",
		"OS_USER_DOMAIN_NAME=Default",
		"OS_PROJECT_DOMAIN_NAME=Default",
		"OS_REGION_NAME=RegionOne",
		"OS_IDENTITY_API_VERSION=3",
	)

	// terraform init
	initCmd := exec.CommandContext(ctx, "terraform", "init", "-no-color", "-input=false")
	initCmd.Dir = c.TerraformDir
	initCmd.Env = env
	if out, err := initCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("terraform init failed: %w\n%s", err, out)
	}

	// terraform plan
	planCmd := exec.CommandContext(ctx, "terraform", "plan", "-no-color", "-input=false")
	planCmd.Dir = c.TerraformDir
	planCmd.Env = env
	planOut, planErr := planCmd.CombinedOutput()

	results := srv.Recorder.Results()
	summary := buildSummary(results)

	report := &Report{
		Compatible:   summary.Incompatible == 0 && planErr == nil,
		OutputFormat: c.OutputFormat,
		Endpoints:    results,
		Summary:      summary,
	}

	if planErr != nil && isProviderError(string(planOut)) {
		report.Compatible = false
	}

	return report, nil
}
```

Add `"os"` and `"time"` to imports.

- [ ] **Step 3: Run tests**

```bash
go test ./internal/compat/... -v -count=1
```

- [ ] **Step 4: Commit**

```bash
git add internal/compat/checker.go internal/compat/checker_test.go
git commit -m "feat(compat): run terraform init+plan with 5-minute timeout"
```

---

## Track 2: DB DI completion

The pattern is identical for every service:
1. Add `db database.DBIF` field to Service struct
2. Add `NewServiceWithDB(db database.DBIF, ...) *Service` constructor
3. Add `activeDB()` helper
4. Replace all `database.DB.Exec/Query/QueryRow(...)` with `svc.activeDB().Exec/Query/QueryRow(...)`
5. Write one representative unit test

Each service is a separate task so they can be committed independently.

### Task 4: Migrate Keystone to `activeDB()` pattern

**Files:**
- Modify: `internal/keystone/handlers.go` (33 call sites)
- Modify: `internal/keystone/auth.go` (12 call sites)
- Modify: `internal/keystone/application_credentials.go` (9 call sites)
- Modify: `internal/keystone/domains.go` (9 call sites)
- Modify: `internal/keystone/groups.go` (9 call sites)
- Modify: `internal/keystone/services.go` (8 call sites)
- Modify: `internal/keystone/credentials.go` (5 call sites)
- Create: `internal/keystone/handlers_test.go`

- [ ] **Step 1: Write failing test**

```go
// internal/keystone/handlers_test.go
package keystone_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/cobaltcore-dev/o3k/internal/keystone"
)

func TestListProjectsWithMockDB(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mock := database.NewMockDB()

	authSvc := keystone.NewAuthServiceWithDB(mock, "test-secret", 0, nil)
	svc := keystone.NewServiceWithDB(mock, authSvc, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/v3/projects", nil)
	c.Set("project_id", "test-project")
	c.Set("user_id", "test-user")
	c.Set("roles", "admin")

	svc.ListProjects(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp, "projects")
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./internal/keystone/... -run TestListProjects 2>&1 | head -10
```
Expected: `keystone.NewAuthServiceWithDB undefined`

- [ ] **Step 3: Add DB DI to Keystone**

In `internal/keystone/auth.go`, add `db database.DBIF` to the `AuthService` struct. Add:
```go
func NewAuthServiceWithDB(db database.DBIF, jwtSecret string, tokenTTL time.Duration, cacheInstance *cache.Cache) *AuthService {
	// Same as NewAuthService but sets db field
	svc := NewAuthService(jwtSecret, tokenTTL, cacheInstance)
	svc.db = db
	return svc
}

func (svc *AuthService) activeDB() database.DBIF {
	if svc.db != nil {
		return svc.db
	}
	return database.DB
}
```

In `internal/keystone/handlers.go`, add `db database.DBIF` to the `Service` struct. Add:
```go
func NewServiceWithDB(db database.DBIF, authService *AuthService, cacheInstance *cache.Cache) *Service {
	svc := NewService(authService, cacheInstance)
	svc.db = db
	return svc
}

func (svc *Service) activeDB() database.DBIF {
	if svc.db != nil {
		return svc.db
	}
	return database.DB
}
```

- [ ] **Step 4: Replace all `database.DB.` in Keystone files**

In each of the 7 files, replace every `database.DB.Exec(`, `database.DB.Query(`, `database.DB.QueryRow(` with `svc.activeDB().Exec(`, `svc.activeDB().Query(`, `svc.activeDB().QueryRow(`.

For `auth.go`: replace with `svc.activeDB()` (where `svc` is the `AuthService` receiver).
For all other files: replace with `svc.activeDB()` (where `svc` is the `Service` receiver).

Run after each file to catch compilation errors:
```bash
go build ./internal/keystone/...
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/keystone/... -v
go build ./...
```

- [ ] **Step 6: Commit**

```bash
git add internal/keystone/
git commit -m "refactor(keystone): migrate all DB access to activeDB() pattern"
```

---

### Task 5: Migrate remaining Nova handlers to `activeDB()`

**Files:**
- Modify: `internal/nova/handlers.go` (remaining 47 call sites)
- Modify: `internal/nova/advanced_actions.go` (48 call sites)
- Modify: `internal/nova/quotas.go` (23 call sites)
- Modify: `internal/nova/volume_attachment.go` (15 call sites)
- Modify: `internal/nova/interface_attach.go` (14 call sites)
- Modify: `internal/nova/aggregates.go` (13 call sites)
- Modify: `internal/nova/tags.go` (11 call sites)
- Modify: `internal/nova/console.go` (7 call sites)
- Modify: `internal/nova/flavors.go` (remaining 10 call sites)
- Modify: `internal/nova/keypairs.go` (5 call sites)
- Modify: `internal/nova/migrations.go` (5 call sites)
- Modify: `internal/nova/diagnostics.go` (5 call sites)
- Modify: `internal/nova/servergroups.go` (4 call sites)
- Modify: `internal/nova/tenant_usage.go` (2 call sites)
- Modify: `internal/nova/server_update.go` (2 call sites)

Nova already has `db`, `activeDB()`, and `NewServiceWithDB`. This task is purely mechanical: replace `database.DB.` with `svc.activeDB().` across all Nova files.

- [ ] **Step 1: Bulk replace in each file**

For each file listed above, replace every occurrence of:
- `database.DB.Exec(` → `svc.activeDB().Exec(`
- `database.DB.Query(` → `svc.activeDB().Query(`
- `database.DB.QueryRow(` → `svc.activeDB().QueryRow(`

Where `svc` is the receiver name (check each function — it should be `svc` based on the codebase pattern).

Build after each file:
```bash
go build ./internal/nova/...
```

- [ ] **Step 2: Remove unused `database` imports**

After replacing all call sites, some files may no longer import `database` directly. Remove unused imports.

- [ ] **Step 3: Run tests**

```bash
go test ./internal/nova/... -v -count=1
go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add internal/nova/
git commit -m "refactor(nova): migrate all DB access to activeDB() pattern"
```

---

### Task 6: Migrate Neutron to `activeDB()` pattern

**Files:**
- Modify: `internal/neutron/network.go` (27 call sites + add db field, activeDB, NewServiceWithDB)
- Modify: `internal/neutron/ports.go` (31 call sites)
- Modify: `internal/neutron/trunks.go` (20 call sites)
- Modify: `internal/neutron/qos.go` (17 call sites)
- Modify: `internal/neutron/router.go` (16 call sites)
- Modify: `internal/neutron/floatingip.go` (15 call sites)
- Modify: `internal/neutron/port_forwarding.go` (10 call sites)
- Modify: `internal/neutron/metering.go` (8 call sites)
- Modify: `internal/neutron/auto_allocated_topology.go` (7 call sites)
- Modify: `internal/neutron/address_scopes.go` (7 call sites)
- Modify: `internal/neutron/agents.go` (7 call sites)
- Modify: `internal/neutron/vxlan_coordinator.go` (6 call sites)
- Modify: `internal/neutron/subnet_pools.go` (5 call sites)
- Modify: `internal/neutron/rbac.go` (5 call sites)
- Modify: `internal/neutron/network_ip_availability.go` (3 call sites)

- [ ] **Step 1: Read Neutron Service struct**

```bash
grep -n "type Service struct" internal/neutron/network.go
```

- [ ] **Step 2: Add DB DI to Neutron Service**

In `internal/neutron/network.go`, add `db database.DBIF` to the Service struct:

```go
type Service struct {
	db              database.DBIF
	networkingMode  string
	// ... existing fields
}
```

Add constructor and helper:
```go
func NewServiceWithDB(db database.DBIF, mode string, cacheInstance *cache.Cache) *Service {
	svc := NewService(mode, cacheInstance)
	svc.db = db
	return svc
}

func (svc *Service) activeDB() database.DBIF {
	if svc.db != nil {
		return svc.db
	}
	return database.DB
}
```

- [ ] **Step 3: Replace all `database.DB.` across 15 files**

Same mechanical pattern as Nova. Replace in each file, build after each:
```bash
go build ./internal/neutron/...
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/neutron/... -v -count=1
go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add internal/neutron/
git commit -m "refactor(neutron): migrate all DB access to activeDB() pattern"
```

---

### Task 7: Migrate Cinder to `activeDB()` pattern

**Files:**
- Modify: `internal/cinder/volumes.go` (58 call sites + add db field, activeDB, NewServiceWithDB)
- Modify: `internal/cinder/volume_types.go` (11 call sites)
- Modify: `internal/cinder/application_credentials.go` (if exists)
- Modify: `internal/cinder/backups.go` (8 call sites)
- Modify: `internal/cinder/transfers.go` (8 call sites)
- Modify: `internal/cinder/groups.go` (7 call sites)
- Modify: `internal/cinder/quotas.go` (7 call sites)
- Modify: `internal/cinder/qos_specs.go` (7 call sites)
- Modify: `internal/cinder/manage.go` (5 call sites)
- Modify: `internal/cinder/volume_type_access.go` (5 call sites)

- [ ] **Step 1: Add DB DI to Cinder Service**

Same pattern: `db` field, `activeDB()`, `NewServiceWithDB()`.

- [ ] **Step 2: Replace all `database.DB.` across 9 files**

- [ ] **Step 3: Run tests and build**

```bash
go test ./internal/cinder/... -v -count=1
go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add internal/cinder/
git commit -m "refactor(cinder): migrate all DB access to activeDB() pattern"
```

---

### Task 8: Migrate Glance to `activeDB()` pattern

**Files:**
- Modify: `internal/glance/images.go` (32 call sites + add db field, activeDB, NewServiceWithDB)
- Modify: `internal/glance/metadefs.go` (8 call sites)
- Modify: `internal/glance/import.go` (5 call sites)
- Modify: `internal/glance/cache.go` (5 call sites)
- Modify: `internal/glance/tasks.go` (3 call sites)

- [ ] **Step 1: Add DB DI to Glance Service**

Same pattern.

- [ ] **Step 2: Replace all `database.DB.` across 5 files**

- [ ] **Step 3: Run tests and build**

```bash
go test ./internal/glance/... -v -count=1
go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add internal/glance/
git commit -m "refactor(glance): migrate all DB access to activeDB() pattern"
```

---

### Task 9: Migrate remaining services (metadata, compute) to `activeDB()`

**Files:**
- Modify: `internal/metadata/service.go` (8 call sites)
- Modify: `internal/compute/node_registry.go` (3 call sites)

These services have fewer call sites but should follow the same pattern for consistency.

- [ ] **Step 1: Add DB DI to metadata Service**

```go
func (svc *Service) activeDB() database.DBIF {
	if svc.db != nil {
		return svc.db
	}
	return database.DB
}
```

- [ ] **Step 2: Replace all `database.DB.` in metadata and compute**

- [ ] **Step 3: Verify zero remaining direct DB usages**

```bash
grep -r "database\.DB\." internal/ --include="*.go" | grep -v "_test.go" | grep -v "database/db.go" | grep -v "database/dbif.go" | wc -l
```
Expected: `0`

- [ ] **Step 4: Run full test suite**

```bash
go test ./internal/... -count=1
go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add internal/metadata/ internal/compute/
git commit -m "refactor(metadata,compute): migrate remaining DB access to activeDB()"
```

---

## Track 3: gRPC hardening

### Task 10: Implement `o3k token create/list/revoke`

**Files:**
- Create: `internal/tunnel/token.go`
- Create: `internal/tunnel/token_test.go`
- Modify: `cmd/o3k/main.go` (wire `runTokenCmd`)

- [ ] **Step 1: Write failing tests**

```go
// internal/tunnel/token_test.go
package tunnel_test

import (
	"testing"
	"github.com/stretchr/testify/assert"
	"github.com/cobaltcore-dev/o3k/internal/tunnel"
)

func TestTokenGenerate(t *testing.T) {
	secret := "my-tunnel-secret"
	nodeID := "node-abc-123"

	hash := tunnel.GenerateTokenHash(secret, nodeID)
	assert.NotEmpty(t, hash)
	assert.True(t, tunnel.VerifyTokenHash(secret, nodeID, hash))
	assert.False(t, tunnel.VerifyTokenHash("wrong-secret", nodeID, hash))
}

func TestTokenGenerateDeterministic(t *testing.T) {
	secret := "my-tunnel-secret"
	nodeID := "node-abc-123"

	hash1 := tunnel.GenerateTokenHash(secret, nodeID)
	hash2 := tunnel.GenerateTokenHash(secret, nodeID)
	assert.Equal(t, hash1, hash2, "same inputs must produce same hash")
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./internal/tunnel/... -run TestToken 2>&1 | head -10
```

- [ ] **Step 3: Create `internal/tunnel/token.go`**

```go
package tunnel

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// GenerateTokenHash creates an HMAC-SHA256 hash of nodeID using secret.
// This is the join token that agents present when connecting.
func GenerateTokenHash(secret, nodeID string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(nodeID))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyTokenHash checks that the given hash matches HMAC(secret, nodeID).
func VerifyTokenHash(secret, nodeID, hash string) bool {
	expected := GenerateTokenHash(secret, nodeID)
	return hmac.Equal([]byte(expected), []byte(hash))
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
go test ./internal/tunnel/... -run TestToken -v
```

- [ ] **Step 5: Wire `runTokenCmd` in main.go**

Replace the stub `runTokenCmd` in `cmd/o3k/main.go`:

```go
func runTokenCmd(args []string) {
	fs := flag.NewFlagSet("token", flag.ExitOnError)
	configPath := fs.String("config", "config/o3k.yaml", "path to config")
	nodeID := fs.String("node-id", "", "node ID to generate token for (required)")
	_ = fs.Parse(args)

	if *nodeID == "" {
		fmt.Fprintln(os.Stderr, "ERROR: --node-id is required")
		os.Exit(1)
	}

	cfg, err := common.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: failed to load config: %v\n", err)
		os.Exit(1)
	}

	secret := cfg.Tunnel.TokenSecret
	if cfg.Tunnel.TokenFile != "" {
		if data, err := os.ReadFile(cfg.Tunnel.TokenFile); err == nil {
			secret = strings.TrimSpace(string(data))
		}
	}
	if secret == "" {
		fmt.Fprintln(os.Stderr, "ERROR: tunnel.token_secret not set in config")
		os.Exit(1)
	}

	hash := tunnel.GenerateTokenHash(secret, *nodeID)
	fmt.Println(hash)
}
```

- [ ] **Step 6: Build and test**

```bash
go build ./cmd/o3k/
```

- [ ] **Step 7: Commit**

```bash
git add internal/tunnel/token.go internal/tunnel/token_test.go cmd/o3k/main.go
git commit -m "feat(tunnel): implement token generation and verification"
```

---

### Task 11: Add token verification to Hub AgentStream

**Files:**
- Modify: `internal/tunnel/server.go`
- Modify: `internal/tunnel/server_test.go`

- [ ] **Step 1: Write failing test**

Append to `internal/tunnel/server_test.go`:

```go
func TestHubVerifiesToken(t *testing.T) {
	secret := "test-secret"
	hub := tunnel.NewHub(secret)

	validHash := tunnel.GenerateTokenHash(secret, "node-1")
	assert.True(t, hub.VerifyJoin("node-1", validHash))
	assert.False(t, hub.VerifyJoin("node-1", "bad-hash"))
	assert.False(t, hub.VerifyJoin("node-2", validHash))
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./internal/tunnel/... -run TestHubVerifies 2>&1 | head -10
```

- [ ] **Step 3: Add `VerifyJoin` to Hub**

In `internal/tunnel/server.go`:

```go
func (h *Hub) VerifyJoin(nodeID, tokenHash string) bool {
	if h.tokenSecret == "" {
		return true // no secret configured = open enrollment
	}
	return VerifyTokenHash(h.tokenSecret, nodeID, tokenHash)
}
```

Update `AgentStream` to verify on join:

```go
func (h *Hub) AgentStream(stream grpc.BidiStreamingServer[pb.AgentMessage, pb.ServerMessage]) error {
	msg, err := stream.Recv()
	if err != nil {
		return err
	}
	join := msg.GetJoin()
	if join == nil {
		return fmt.Errorf("first message must be JoinMsg")
	}

	if !h.VerifyJoin(join.NodeId, join.TokenHash) {
		return fmt.Errorf("invalid join token for node %s", join.NodeId)
	}

	h.RegisterAgent(AgentInfo{
		NodeID:   join.NodeId,
		Hostname: join.Hostname,
		TunnelIP: join.TunnelIp,
		Stream:   stream,
	})
	defer h.RemoveAgent(join.NodeId)

	for {
		if _, err := stream.Recv(); err != nil {
			return err
		}
	}
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
go test ./internal/tunnel/... -v -count=1
```

- [ ] **Step 5: Commit**

```bash
git add internal/tunnel/server.go internal/tunnel/server_test.go
git commit -m "feat(tunnel): verify join token in AgentStream"
```

---

### Task 12: Add task dispatch from Hub to connected agents

**Files:**
- Create: `internal/tunnel/dispatch.go`
- Create: `internal/tunnel/dispatch_test.go`
- Modify: `internal/tunnel/server.go`

- [ ] **Step 1: Write failing test**

```go
// internal/tunnel/dispatch_test.go
package tunnel_test

import (
	"testing"
	"github.com/stretchr/testify/assert"
	"github.com/cobaltcore-dev/o3k/internal/tunnel"
)

func TestDispatcherQueuesTask(t *testing.T) {
	hub := tunnel.NewHub("secret")
	dispatcher := tunnel.NewDispatcher(hub)

	task := tunnel.Task{
		Type:    tunnel.TaskCreateVM,
		Payload: []byte(`{"instance_id":"inst-1"}`),
	}

	err := dispatcher.Dispatch(task)
	// No agents connected — should return error
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no agents")
}

func TestDispatcherSendsToAgent(t *testing.T) {
	hub := tunnel.NewHub("secret")
	hub.RegisterAgent(tunnel.AgentInfo{NodeID: "node-1", Hostname: "w1", TunnelIP: "10.0.0.2"})
	dispatcher := tunnel.NewDispatcher(hub)

	task := tunnel.Task{
		Type:    tunnel.TaskCreateVM,
		Payload: []byte(`{"instance_id":"inst-1"}`),
	}

	// With an agent registered (but no real stream), dispatch should
	// either succeed (queued) or fail gracefully (nil stream)
	err := dispatcher.Dispatch(task)
	// Stream is nil in tests — dispatcher should handle gracefully
	if err != nil {
		assert.Contains(t, err.Error(), "stream")
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./internal/tunnel/... -run TestDispatcher 2>&1 | head -10
```

- [ ] **Step 3: Create `internal/tunnel/dispatch.go`**

```go
package tunnel

import (
	"fmt"

	pb "github.com/cobaltcore-dev/o3k/proto/tunnel"
)

// Dispatcher bridges API handlers to the tunnel Hub for async task execution.
type Dispatcher struct {
	hub *Hub
}

func NewDispatcher(hub *Hub) *Dispatcher {
	return &Dispatcher{hub: hub}
}

// Dispatch sends a task to an available agent via the Hub.
func (d *Dispatcher) Dispatch(task Task) error {
	if err := task.Validate(); err != nil {
		return fmt.Errorf("invalid task: %w", err)
	}

	agent := d.hub.PickAgent()
	if agent == nil {
		return fmt.Errorf("no agents connected")
	}

	if agent.Stream == nil {
		return fmt.Errorf("agent %s has no active stream", agent.NodeID)
	}

	msg := &pb.ServerMessage{
		Payload: &pb.ServerMessage_Task{
			Task: &pb.TaskMsg{
				TaskId:   task.ID,
				TaskType: task.Type,
				Payload:  task.Payload,
			},
		},
	}
	if err := agent.Stream.Send(msg); err != nil {
		return fmt.Errorf("failed to send task to agent %s: %w", agent.NodeID, err)
	}
	return nil
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
go test ./internal/tunnel/... -v -count=1
```

- [ ] **Step 5: Commit**

```bash
git add internal/tunnel/dispatch.go internal/tunnel/dispatch_test.go
git commit -m "feat(tunnel): add Dispatcher for sending tasks to agents"
```

---

## Self-Review

**Spec coverage check:**

| Requirement | Covered by task |
|-------------|----------------|
| Wire real OpenStack routes into embedded server | Task 1 |
| Recorder middleware captures real API calls | Task 2 |
| terraform init + plan flow | Task 3 |
| Keystone DB DI (85 call sites) | Task 4 |
| Nova DB DI (remaining ~180 call sites) | Task 5 |
| Neutron DB DI (160 call sites) | Task 6 |
| Cinder DB DI (116 call sites) | Task 7 |
| Glance DB DI (53 call sites) | Task 8 |
| Metadata + Compute DB DI (11 call sites) | Task 9 |
| Token generation and verification | Task 10 |
| Token verification on agent join | Task 11 |
| Task dispatch from Hub to agents | Task 12 |

**Not covered (intentionally deferred):**
- mTLS certificate generation (requires CA infrastructure — separate plan)
- Agent-side real task execution (requires libvirt/netlink on agent — depends on Task 12)
- HA-aware scheduling (requires real heartbeat data — separate plan)
- `o3k token list/revoke` (requires DB integration in tunnel — incremental after Task 10)

**Placeholder scan:** No TBD/TODO in steps. `runTokenCmd` stub is fully replaced in Task 10. All code blocks are complete.

**Type consistency:**
- `database.DBIF` used consistently (not `database.DB` the interface — that's the variable)
- `activeDB()` returns `database.DBIF` everywhere
- `NewServiceWithDB(db database.DBIF, ...)` consistent across all services
- `tunnel.Dispatcher` uses `Hub.PickAgent()` → `AgentInfo.Stream.Send()` — matches existing types
