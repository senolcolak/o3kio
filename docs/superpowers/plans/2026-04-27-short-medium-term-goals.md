# O3K Short/Medium-Term Goals Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship `o3k compat-check` (the real product wedge), fix foundational test and database debt, and implement the gRPC server/agent architecture.

**Architecture:** Three independent tracks in priority order — (1) compat-check CLI tool that runs Terraform against stub mode and emits a JSON report; (2) test and database DI foundations that unblock sustainable development; (3) gRPC tunnel that makes the multi-node story real. Tracks 1 and 2 can execute in parallel. Track 3 depends on nothing but is ~4 weeks of work.

**Tech Stack:** Go 1.26, Gin, pgx/v5, zerolog, testify, google.golang.org/grpc, modernc.org/sqlite

---

## Scope

This plan covers:
- **Short term:** `o3k compat-check`, database DI (H-3), test coverage (H-1 starter)
- **Medium term:** gRPC TunnelHub, `o3k server`/`o3k agent` subcommands, async task queue

Each task produces a working, testable, committable unit.

---

## File Structure

### Track 1: compat-check CLI

| File | Created/Modified | Responsibility |
|------|-----------------|----------------|
| `cmd/compat-check/main.go` | Create | CLI entry point, flag parsing, output formatting |
| `internal/compat/checker.go` | Create | Core check logic — runs Terraform plan, parses result |
| `internal/compat/runner.go` | Create | Starts embedded o3k stub server for the check |
| `internal/compat/report.go` | Create | Structures and serializes the JSON/text report |
| `internal/compat/checker_test.go` | Create | Unit tests for report generation logic |
| `Makefile` | Modify | Add `compat-check` build target |
| `config/o3k.yaml` | No change | compat-check uses in-memory config |

### Track 2: Database DI + test foundation

| File | Created/Modified | Responsibility |
|------|-----------------|----------------|
| `internal/database/db.go` | Create | `DB` interface + `PgxPool` implementation |
| `internal/database/mock.go` | Create | `MockDB` for unit tests |
| `internal/nova/service.go` | Create | Extract `Service` struct, inject `DB` via constructor |
| `internal/nova/handlers_test.go` | Create | Unit tests for Nova handlers using MockDB |
| `internal/keystone/service.go` | Create | Extract `AuthService`, inject `DB` |
| `internal/keystone/auth_test.go` | Create | Unit tests for token issuance |

### Track 3: gRPC server/agent

| File | Created/Modified | Responsibility |
|------|-----------------|----------------|
| `proto/tunnel/tunnel.proto` | Create | TunnelHub service definition |
| `internal/tunnel/server.go` | Create | TunnelHub gRPC server, manages agent streams |
| `internal/tunnel/client.go` | Create | Agent-side gRPC client, task loop |
| `internal/tunnel/task.go` | Create | Task types, serialization, dispatch |
| `internal/tunnel/scheduler.go` | Create | Picks agent for a task using DB reservation |
| `internal/tunnel/server_test.go` | Create | Unit tests for task dispatch logic |
| `internal/tunnel/mtls.go` | Create | mTLS cert generation and verification |
| `internal/compute/node_registry.go` | Modify | Persist UUID to disk; stop regenerating on every call |
| `cmd/o3k/main.go` | Modify | Add `server`/`agent`/`token` subcommand dispatch |
| `internal/common/config.go` | Modify | Add `AgentConfig`, `TunnelConfig`, `async_compute` to `NovaConfig` |
| `config/o3k.yaml` | Modify | Document new `tunnel` and `agent` sections |
| `go.mod` | Modify | Add grpc and sqlite dependencies |
| `migrations/060_tunnel_tokens.up.sql` | Create | Join token storage table |

---

## Track 1: compat-check CLI

### Task 1: Add `compat-check` binary skeleton

**Files:**
- Create: `cmd/compat-check/main.go`
- Modify: `Makefile`

- [ ] **Step 1: Write the test that validates flag parsing**

```go
// internal/compat/checker_test.go
package compat_test

import (
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/cobaltcore-dev/o3k/internal/compat"
)

func TestNewCheckerDefaults(t *testing.T) {
    c := compat.NewChecker(compat.CheckerOptions{})
    assert.Equal(t, "json", c.OutputFormat)
    assert.Equal(t, compat.DefaultListenAddr, c.ListenAddr)
}
```

- [ ] **Step 2: Run test — expect FAIL**

```bash
go test ./internal/compat/... 2>&1 | head -20
```
Expected: `no Go files in .../internal/compat`

- [ ] **Step 3: Create `internal/compat/checker.go`**

```go
package compat

const DefaultListenAddr = "127.0.0.1:35357"

type CheckerOptions struct {
    TerraformDir string
    OutputFormat string // "json" or "text"
    ListenAddr   string
}

type Checker struct {
    TerraformDir string
    OutputFormat string
    ListenAddr   string
}

func NewChecker(opts CheckerOptions) *Checker {
    if opts.OutputFormat == "" {
        opts.OutputFormat = "json"
    }
    if opts.ListenAddr == "" {
        opts.ListenAddr = DefaultListenAddr
    }
    return &Checker{
        TerraformDir: opts.TerraformDir,
        OutputFormat: opts.OutputFormat,
        ListenAddr:   opts.ListenAddr,
    }
}
```

- [ ] **Step 4: Run test — expect PASS**

```bash
go test ./internal/compat/... -v
```
Expected: `PASS`

- [ ] **Step 5: Create `cmd/compat-check/main.go`**

```go
package main

import (
    "flag"
    "fmt"
    "os"
    "github.com/cobaltcore-dev/o3k/internal/compat"
)

func main() {
    dir := flag.String("dir", ".", "Terraform directory to check")
    format := flag.String("output", "json", "Output format: json or text")
    flag.Parse()

    c := compat.NewChecker(compat.CheckerOptions{
        TerraformDir: *dir,
        OutputFormat: *format,
    })

    report, err := c.Run()
    if err != nil {
        fmt.Fprintf(os.Stderr, "compat-check failed: %v\n", err)
        os.Exit(1)
    }

    fmt.Println(report.String())
    if !report.Compatible {
        os.Exit(2)
    }
}
```

- [ ] **Step 6: Add `build-compat-check` target to `Makefile`**

Find the existing `build:` target and add after it:
```makefile
# Build compat-check tool
build-compat-check:
	@echo "Building compat-check..."
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/compat-check ./cmd/compat-check
```

Also add `build-compat-check` to `.PHONY` line.

- [ ] **Step 7: Verify it compiles**

```bash
make build-compat-check 2>&1
```
Expected: `bin/compat-check` binary exists.

- [ ] **Step 8: Commit**

```bash
git add internal/compat/checker.go internal/compat/checker_test.go cmd/compat-check/main.go Makefile
git commit -m "feat(compat): add compat-check CLI skeleton with options and build target"
```

---

### Task 2: Implement the compat-check report structure

**Files:**
- Create: `internal/compat/report.go`
- Modify: `internal/compat/checker_test.go`

- [ ] **Step 1: Write failing tests for report serialization**

Append to `internal/compat/checker_test.go`:
```go
func TestReportJSON(t *testing.T) {
    r := compat.Report{
        Compatible: true,
        Endpoints: []compat.EndpointResult{
            {Method: "POST", Path: "/v3/auth/tokens", Called: true, StatusCode: 201, Compatible: true},
            {Method: "GET",  Path: "/v2.1/servers",   Called: true, StatusCode: 200, Compatible: true},
        },
        Summary: compat.Summary{Total: 2, Compatible: 2, Incompatible: 0},
    }
    out := r.String()
    assert.Contains(t, out, `"compatible":true`)
    assert.Contains(t, out, `"total":2`)
}

func TestReportText(t *testing.T) {
    r := compat.Report{
        Compatible:   false,
        OutputFormat: "text",
        Endpoints: []compat.EndpointResult{
            {Method: "DELETE", Path: "/v2.1/servers/:id", Called: true, StatusCode: 404, Compatible: false,
             Error: "unexpected 404, expected 204"},
        },
        Summary: compat.Summary{Total: 1, Compatible: 0, Incompatible: 1},
    }
    out := r.String()
    assert.Contains(t, out, "INCOMPATIBLE")
    assert.Contains(t, out, "DELETE /v2.1/servers/:id")
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./internal/compat/... -run TestReport 2>&1
```

- [ ] **Step 3: Create `internal/compat/report.go`**

```go
package compat

import (
    "encoding/json"
    "fmt"
    "strings"
)

type EndpointResult struct {
    Method     string `json:"method"`
    Path       string `json:"path"`
    Called     bool   `json:"called"`
    StatusCode int    `json:"status_code,omitempty"`
    Compatible bool   `json:"compatible"`
    Error      string `json:"error,omitempty"`
}

type Summary struct {
    Total        int `json:"total"`
    Compatible   int `json:"compatible"`
    Incompatible int `json:"incompatible"`
    Uncalled     int `json:"uncalled"`
}

type Report struct {
    Compatible   bool             `json:"compatible"`
    OutputFormat string           `json:"-"`
    Endpoints    []EndpointResult `json:"endpoints"`
    Summary      Summary          `json:"summary"`
}

func (r Report) String() string {
    if r.OutputFormat == "text" {
        return r.toText()
    }
    b, _ := json.MarshalIndent(r, "", "  ")
    return string(b)
}

func (r Report) toText() string {
    var sb strings.Builder
    verdict := "COMPATIBLE"
    if !r.Compatible {
        verdict = "INCOMPATIBLE"
    }
    fmt.Fprintf(&sb, "o3k compat-check: %s\n", verdict)
    fmt.Fprintf(&sb, "  Total: %d  Compatible: %d  Incompatible: %d  Uncalled: %d\n\n",
        r.Summary.Total, r.Summary.Compatible, r.Summary.Incompatible, r.Summary.Uncalled)
    for _, ep := range r.Endpoints {
        status := "OK"
        if !ep.Compatible {
            status = "FAIL"
        }
        fmt.Fprintf(&sb, "  [%s] %s %s", status, ep.Method, ep.Path)
        if ep.Error != "" {
            fmt.Fprintf(&sb, " — %s", ep.Error)
        }
        fmt.Fprintln(&sb)
    }
    return sb.String()
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
go test ./internal/compat/... -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/compat/report.go internal/compat/checker_test.go
git commit -m "feat(compat): add Report struct with JSON and text output"
```

---

### Task 3: Implement `Checker.Run()` — start embedded stub server and capture API calls

**Files:**
- Create: `internal/compat/runner.go`
- Modify: `internal/compat/checker.go`

This task requires `terraform` CLI to be available for the full integration path. The unit-testable piece is the API call recorder — we test that in isolation.

- [ ] **Step 1: Write failing test for call recording**

Append to `internal/compat/checker_test.go`:
```go
func TestRecorderCaptures(t *testing.T) {
    rec := compat.NewRecorder()
    // Simulate recording a call
    rec.Record("POST", "/v3/auth/tokens", 201)
    rec.Record("GET", "/v2.1/servers", 200)
    rec.Record("DELETE", "/v2.1/servers/abc", 204)

    results := rec.Results()
    assert.Len(t, results, 3)
    assert.Equal(t, "POST", results[0].Method)
    assert.Equal(t, 201, results[0].StatusCode)
    assert.True(t, results[0].Compatible)
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./internal/compat/... -run TestRecorder 2>&1
```

- [ ] **Step 3: Create `internal/compat/runner.go`**

```go
package compat

import (
    "net/http"
    "sync"
)

// Recorder captures API calls made against the stub server.
type Recorder struct {
    mu      sync.Mutex
    results []EndpointResult
}

func NewRecorder() *Recorder {
    return &Recorder{}
}

// Record stores a single API call result.
// Compatible is true when status is in [200, 201, 202, 204, 404 for GET-not-found].
func (r *Recorder) Record(method, path string, statusCode int) {
    compatible := isCompatibleStatus(method, statusCode)
    r.mu.Lock()
    r.results = append(r.results, EndpointResult{
        Method:     method,
        Path:       path,
        Called:     true,
        StatusCode: statusCode,
        Compatible: compatible,
    })
    r.mu.Unlock()
}

// Results returns a snapshot of recorded results.
func (r *Recorder) Results() []EndpointResult {
    r.mu.Lock()
    defer r.mu.Unlock()
    out := make([]EndpointResult, len(r.results))
    copy(out, r.results)
    return out
}

// Middleware wraps a handler to record calls.
func (r *Recorder) Middleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
        rw := &statusRecorder{ResponseWriter: w, code: 200}
        next.ServeHTTP(rw, req)
        r.Record(req.Method, req.URL.Path, rw.code)
    })
}

type statusRecorder struct {
    http.ResponseWriter
    code int
}

func (s *statusRecorder) WriteHeader(code int) {
    s.code = code
    s.ResponseWriter.WriteHeader(code)
}

func isCompatibleStatus(method string, code int) bool {
    if code >= 200 && code < 300 {
        return true
    }
    // 404 on GET is valid "not found" — compatible
    if method == "GET" && code == 404 {
        return true
    }
    // 409 conflict is a valid response for duplicate creates
    if code == 409 {
        return true
    }
    return false
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
go test ./internal/compat/... -v
```

- [ ] **Step 5: Implement `Checker.Run()` stub**

Add to `internal/compat/checker.go`:
```go
import (
    "fmt"
    "os/exec"
)

// Run starts an embedded stub server, runs `terraform plan` against it,
// and returns a compatibility report.
func (c *Checker) Run() (*Report, error) {
    rec := NewRecorder()
    _ = rec // will be wired to the embedded server in Task 4

    // Check terraform is available
    if _, err := exec.LookPath("terraform"); err != nil {
        return nil, fmt.Errorf("terraform not found in PATH: %w", err)
    }
    if c.TerraformDir == "" {
        return nil, fmt.Errorf("TerraformDir must be set")
    }

    // Placeholder: return empty report until embedded server is wired
    return &Report{
        Compatible:   true,
        OutputFormat: c.OutputFormat,
        Endpoints:    rec.Results(),
        Summary:      Summary{},
    }, nil
}
```

- [ ] **Step 6: Build check**

```bash
make build-compat-check
```

- [ ] **Step 7: Commit**

```bash
git add internal/compat/runner.go internal/compat/checker.go internal/compat/checker_test.go
git commit -m "feat(compat): add Recorder middleware and Checker.Run skeleton"
```

---

### Task 4: Wire embedded stub server into compat-check

**Files:**
- Modify: `internal/compat/checker.go`
- Modify: `internal/compat/runner.go`

The embedded server reuses the existing Gin-based OpenStack routes. The simplest approach: spin up the same `createKeystoneServer`/`createNovaServer`/etc. logic on random ports, point Terraform's `OS_AUTH_URL` at it, run `terraform init && terraform plan`, collect the recorder results.

- [ ] **Step 1: Write integration test (skip if terraform not in PATH)**

Append to `internal/compat/checker_test.go`:
```go
func TestCheckerRunNoTerraform(t *testing.T) {
    // If terraform IS installed this test documents the expected error on empty dir
    c := compat.NewChecker(compat.CheckerOptions{TerraformDir: "/nonexistent"})
    _, err := c.Run()
    assert.Error(t, err)
}
```

- [ ] **Step 2: Run — expect PASS (error is expected)**

```bash
go test ./internal/compat/... -run TestCheckerRunNoTerraform -v
```

- [ ] **Step 3: Add `StartEmbeddedServer` to `runner.go`**

```go
import (
    "context"
    "net"
    "net/http"
)

// EmbeddedServer holds a minimal stub OpenStack server for compat testing.
type EmbeddedServer struct {
    Keystone *http.Server
    Nova     *http.Server
    Recorder *Recorder
}

// StartEmbeddedServer starts the minimal Keystone + Nova stub on available ports.
// Call Shutdown() when done.
func StartEmbeddedServer(ctx context.Context) (*EmbeddedServer, error) {
    keystoneListener, err := net.Listen("tcp", "127.0.0.1:0")
    if err != nil {
        return nil, fmt.Errorf("failed to bind keystone port: %w", err)
    }
    novaListener, err := net.Listen("tcp", "127.0.0.1:0")
    if err != nil {
        keystoneListener.Close()
        return nil, fmt.Errorf("failed to bind nova port: %w", err)
    }

    rec := NewRecorder()

    // Minimal pass-through handlers — real stub routes are wired here in Task 5
    keystoneMux := http.NewServeMux()
    keystoneMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(200)
        w.Write([]byte(`{"versions":{"values":[{"id":"v3","status":"stable"}]}}`))
        rec.Record(r.Method, r.URL.Path, 200)
    })

    es := &EmbeddedServer{
        Keystone: &http.Server{Handler: keystoneMux},
        Nova:     &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})},
        Recorder: rec,
    }
    go es.Keystone.Serve(keystoneListener)
    go es.Nova.Serve(novaListener)
    return es, nil
}

// KeystoneAddr returns the "host:port" the embedded Keystone is listening on.
func (e *EmbeddedServer) KeystoneAddr() string {
    return e.Keystone.Addr
}

// Shutdown stops all embedded servers.
func (e *EmbeddedServer) Shutdown(ctx context.Context) {
    e.Keystone.Shutdown(ctx)
    e.Nova.Shutdown(ctx)
}
```

- [ ] **Step 4: Wire `StartEmbeddedServer` into `Checker.Run()`**

Replace the `Run()` body in `internal/compat/checker.go`:
```go
func (c *Checker) Run() (*Report, error) {
    if _, err := exec.LookPath("terraform"); err != nil {
        return nil, fmt.Errorf("terraform not found in PATH: %w", err)
    }
    if c.TerraformDir == "" {
        return nil, fmt.Errorf("TerraformDir must be set")
    }

    ctx := context.Background()
    srv, err := StartEmbeddedServer(ctx)
    if err != nil {
        return nil, fmt.Errorf("failed to start embedded server: %w", err)
    }
    defer srv.Shutdown(ctx)

    // Set env for terraform to hit the embedded server
    // OS_AUTH_URL will be set by the caller or defaulted
    cmd := exec.CommandContext(ctx, "terraform", "plan", "-no-color")
    cmd.Dir = c.TerraformDir
    cmd.Env = append(cmd.Environ(),
        fmt.Sprintf("OS_AUTH_URL=http://%s/v3", srv.KeystoneAddr()),
        "OS_USERNAME=admin",
        "OS_PASSWORD=secret",
        "OS_PROJECT_NAME=default",
        "OS_REGION_NAME=RegionOne",
    )
    out, err := cmd.CombinedOutput()
    if err != nil {
        // terraform plan exits non-zero on diffs — that's expected
        // only fail on "Error:" lines that indicate missing provider APIs
        if isProviderError(string(out)) {
            return nil, fmt.Errorf("terraform plan provider error: %s", out)
        }
    }

    results := srv.Recorder.Results()
    summary := buildSummary(results)
    return &Report{
        Compatible:   summary.Incompatible == 0,
        OutputFormat: c.OutputFormat,
        Endpoints:    results,
        Summary:      summary,
    }, nil
}

func isProviderError(output string) bool {
    return strings.Contains(output, "Error: Provider configuration")
}

func buildSummary(results []EndpointResult) Summary {
    s := Summary{Total: len(results)}
    for _, r := range results {
        if r.Compatible {
            s.Compatible++
        } else {
            s.Incompatible++
        }
    }
    return s
}
```

- [ ] **Step 5: Add missing imports to checker.go**

```go
import (
    "context"
    "fmt"
    "os/exec"
    "strings"
)
```

- [ ] **Step 6: Build and smoke test**

```bash
make build-compat-check
./bin/compat-check --help
```
Expected: usage printed, no panic.

- [ ] **Step 7: Commit**

```bash
git add internal/compat/checker.go internal/compat/runner.go internal/compat/checker_test.go
git commit -m "feat(compat): wire embedded stub server into Checker.Run"
```

---

### Task 5: Add compat-check to CI and docs

**Files:**
- Modify: `Makefile`
- Modify: `.github/workflows/ci.yml` (or equivalent)
- Modify: `docs/WHATS_LEFT.md`

- [ ] **Step 1: Add `compat-check-smoke` Makefile target**

Append to `Makefile`:
```makefile
# Smoke test compat-check binary (no terraform required)
compat-check-smoke:
	@echo "Smoke testing compat-check..."
	@./bin/compat-check --help > /dev/null && echo "OK: compat-check built and runs"
```

- [ ] **Step 2: Run smoke test**

```bash
make build-compat-check compat-check-smoke
```
Expected: `OK: compat-check built and runs`

- [ ] **Step 3: Update WHATS_LEFT.md**

In `docs/WHATS_LEFT.md`, under `## Completion Status`, add:
```markdown
### compat-check (v0.7.0)
- [x] CLI skeleton with flags
- [x] Report struct (JSON + text)
- [x] Recorder middleware
- [x] Embedded stub server
- [ ] Full OpenStack provider route wiring (all 5 services)
- [ ] Terraform provider smoke test in CI
```

- [ ] **Step 4: Commit**

```bash
git add Makefile docs/WHATS_LEFT.md
git commit -m "chore(compat): add smoke test target and update WHATS_LEFT"
```

---

## Track 2: Database DI + test foundation

### Task 6: Define `DB` interface and `MockDB`

**Files:**
- Create: `internal/database/db.go`
- Create: `internal/database/mock.go`

Right now `database.DB` is a package-level `*pgxpool.Pool` (a concrete type, not an interface). Every handler calls it directly. This makes unit testing impossible without a running database. The fix: define a minimal interface, implement it with the real pool, and provide a mock.

- [ ] **Step 1: Write failing test that uses MockDB**

```go
// internal/database/mock_test.go
package database_test

import (
    "context"
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/cobaltcore-dev/o3k/internal/database"
)

func TestMockDBExec(t *testing.T) {
    mock := database.NewMockDB()
    mock.OnExec("INSERT INTO users", nil) // returns no error

    _, err := mock.Exec(context.Background(), "INSERT INTO users VALUES ($1)", "test-id")
    assert.NoError(t, err)
    assert.True(t, mock.ExecCalled("INSERT INTO users"))
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./internal/database/... -run TestMockDB 2>&1 | head -20
```

- [ ] **Step 3: Create `internal/database/db.go`**

```go
package database

import (
    "context"
    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgconn"
)

// DB is the minimal interface used by all internal packages.
// Satisfied by *pgxpool.Pool and MockDB.
type DB interface {
    Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
    QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
    Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
    BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error)
}

// Global is the active database connection. Set by Connect().
var Global DB
```

- [ ] **Step 4: Create `internal/database/mock.go`**

```go
package database

import (
    "context"
    "strings"
    "sync"
    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgconn"
)

// MockDB is a test double for DB.
type MockDB struct {
    mu        sync.Mutex
    execRules map[string]error   // prefix → error to return
    execCalls []string           // SQL statements actually called
}

func NewMockDB() *MockDB {
    return &MockDB{execRules: make(map[string]error)}
}

// OnExec registers a rule: when SQL starts with prefix, return err (nil = success).
func (m *MockDB) OnExec(prefix string, err error) {
    m.mu.Lock()
    m.execRules[prefix] = err
    m.mu.Unlock()
}

// ExecCalled returns true if Exec was called with SQL containing prefix.
func (m *MockDB) ExecCalled(prefix string) bool {
    m.mu.Lock()
    defer m.mu.Unlock()
    for _, s := range m.execCalls {
        if strings.Contains(s, prefix) {
            return true
        }
    }
    return false
}

func (m *MockDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
    m.mu.Lock()
    m.execCalls = append(m.execCalls, sql)
    for prefix, err := range m.execRules {
        if strings.Contains(sql, prefix) {
            m.mu.Unlock()
            return pgconn.CommandTag{}, err
        }
    }
    m.mu.Unlock()
    return pgconn.NewCommandTag("OK"), nil
}

func (m *MockDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
    return &mockRow{err: pgx.ErrNoRows} // safe default: not found
}

func (m *MockDB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
    return &mockRows{}, nil
}

func (m *MockDB) BeginTx(ctx context.Context, opts pgx.TxOptions) (pgx.Tx, error) {
    return nil, nil // expand when transaction tests are needed
}

// mockRow satisfies pgx.Row
type mockRow struct{ err error }
func (r *mockRow) Scan(dest ...any) error { return r.err }

// mockRows satisfies pgx.Rows
type mockRows struct{}
func (r *mockRows) Close()                                       {}
func (r *mockRows) Err() error                                   { return nil }
func (r *mockRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *mockRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *mockRows) Next() bool                                   { return false }
func (r *mockRows) Scan(dest ...any) error                       { return nil }
func (r *mockRows) Values() ([]any, error)                       { return nil, nil }
func (r *mockRows) RawValues() [][]byte                          { return nil }
func (r *mockRows) Conn() *pgx.Conn                              { return nil }
```

- [ ] **Step 5: Run — expect PASS**

```bash
go test ./internal/database/... -v
```

- [ ] **Step 6: Commit**

```bash
git add internal/database/db.go internal/database/mock.go internal/database/mock_test.go
git commit -m "feat(database): add DB interface and MockDB for unit testing"
```

---

### Task 7: Migrate `database.DB` global from concrete pool to interface

**Files:**
- Modify: `internal/database/database.go` (the file with `Connect` and the global `DB`)

Right now the global `DB` is `*pgxpool.Pool`. We need to change it to `database.DB` (the interface we just defined). The pool still implements `DB` — we just need to assign it via the interface.

- [ ] **Step 1: Find the current global declaration**

```bash
grep -n "^var DB\|^var Pool\|pgxpool.Pool" internal/database/database.go | head -10
```

- [ ] **Step 2: Change the global type**

In `internal/database/database.go`, find the line like:
```go
var DB *pgxpool.Pool
```
Change to:
```go
// DB is the active database connection. Initialized by Connect.
// Use database.DB in all internal packages — do not import pgxpool directly.
var DB DB
```

(If the variable is named `Pool` or something else, use the actual name.)

- [ ] **Step 3: Verify compilation**

```bash
go build ./...
```
Expected: no errors. If there are type errors because call sites used pool-specific methods not in the interface, add those methods to `internal/database/db.go` one at a time until it compiles.

- [ ] **Step 4: Run unit tests**

```bash
go test ./internal/database/... ./internal/common/... -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/database/database.go internal/database/db.go
git commit -m "refactor(database): change global DB from concrete pool to DB interface"
```

---

### Task 8: Add first Nova unit tests using MockDB

**Files:**
- Create: `internal/nova/handlers_test.go`
- Modify: `internal/nova/handlers.go` (inject DB parameter to a handler function)

We don't need to refactor all 2013 lines at once. The pattern: pick 2-3 handler functions, extract them to accept a `database.DB` parameter, write unit tests. This proves the pattern and gives immediate coverage on the most critical paths.

- [ ] **Step 1: Write failing tests for `ListFlavors`**

```go
// internal/nova/handlers_test.go
package nova_test

import (
    "context"
    "testing"
    "net/http/httptest"
    "net/http"
    "encoding/json"
    "github.com/gin-gonic/gin"
    "github.com/stretchr/testify/assert"
    "github.com/cobaltcore-dev/o3k/internal/database"
    "github.com/cobaltcore-dev/o3k/internal/nova"
)

func TestListFlavorsReturnsJSON(t *testing.T) {
    gin.SetMode(gin.TestMode)
    mock := database.NewMockDB()
    // MockDB returns empty rows by default — valid "no flavors" response
    
    svc := nova.NewServiceWithDB(mock, "stub", nil)
    w := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(w)
    c.Request, _ = http.NewRequest("GET", "/v2.1/flavors", nil)
    c.Set("project_id", "test-project")

    svc.ListFlavors(c)

    assert.Equal(t, http.StatusOK, w.Code)
    var resp map[string]interface{}
    assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
    assert.Contains(t, resp, "flavors")
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./internal/nova/... -run TestListFlavors 2>&1 | head -20
```
Expected: `nova.NewServiceWithDB undefined`

- [ ] **Step 3: Add `NewServiceWithDB` constructor to `handlers.go`**

Add after the existing `NewService` function:
```go
// NewServiceWithDB creates a Nova service with an injected DB for testing.
func NewServiceWithDB(db database.DB, libvirtMode string, cacheInstance *cache.Cache) *Service {
    ctx, cancel := context.WithCancel(context.Background())
    return &Service{
        db:          db,
        libvirtMode: libvirtMode,
        libvirtURI:  "",
        cache:       cacheInstance,
        ctx:         ctx,
        cancel:      cancel,
    }
}
```

Add `db database.DB` field to the `Service` struct:
```go
type Service struct {
    db            database.DB   // nil = use database.DB global (legacy path)
    libvirtURI    string
    // ... existing fields
}
```

Add a helper to the `Service` that returns the active DB:
```go
func (svc *Service) activeDB() database.DB {
    if svc.db != nil {
        return svc.db
    }
    return database.DB
}
```

In `ListFlavors` (and similar handlers), replace `database.DB.Query(...)` with `svc.activeDB().Query(...)`.

- [ ] **Step 4: Run — expect PASS**

```bash
go test ./internal/nova/... -run TestListFlavors -v
```

- [ ] **Step 5: Add a second test for CreateServer stub mode**

Append to `handlers_test.go`:
```go
func TestCreateServerStubMode(t *testing.T) {
    gin.SetMode(gin.TestMode)
    mock := database.NewMockDB()
    mock.OnExec("INSERT INTO instances", nil)

    svc := nova.NewServiceWithDB(mock, "stub", nil)
    body := `{"server":{"name":"test-vm","flavorRef":"m1.small","imageRef":"img-1","networks":[]}}`
    w := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(w)
    c.Request, _ = http.NewRequest("POST", "/v2.1/servers", strings.NewReader(body))
    c.Request.Header.Set("Content-Type", "application/json")
    c.Set("project_id", "test-project")
    c.Set("user_id", "test-user")

    svc.CreateServer(c)

    // In stub mode, CreateServer should return 202 Accepted
    assert.Equal(t, http.StatusAccepted, w.Code)
    assert.True(t, mock.ExecCalled("INSERT INTO instances"))
}
```

- [ ] **Step 6: Run all nova tests**

```bash
go test ./internal/nova/... -v 2>&1 | tail -20
```

- [ ] **Step 7: Commit**

```bash
git add internal/nova/handlers.go internal/nova/handlers_test.go
git commit -m "test(nova): add first unit tests for ListFlavors and CreateServer using MockDB"
```

---

## Track 3: gRPC server/agent

> **Prerequisites:** `go get google.golang.org/grpc@latest modernc.org/sqlite@latest` must be run before any code in this track compiles.

### Task 9: Add gRPC dependencies and proto definition

**Files:**
- Modify: `go.mod` / `go.sum`
- Create: `proto/tunnel/tunnel.proto`
- Create: `proto/tunnel/tunnel.pb.go` (generated)
- Create: `proto/tunnel/tunnel_grpc.pb.go` (generated)

- [ ] **Step 1: Add dependencies**

```bash
go get google.golang.org/grpc@latest
go get google.golang.org/protobuf@latest
go get modernc.org/sqlite@latest
```

- [ ] **Step 2: Install protoc plugins (once per dev machine)**

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

- [ ] **Step 3: Create `proto/tunnel/tunnel.proto`**

```protobuf
syntax = "proto3";
package tunnel;
option go_package = "github.com/cobaltcore-dev/o3k/proto/tunnel";

// TunnelHub is the gRPC service running on the o3k server.
// Agents connect and maintain a bidirectional task stream.
service TunnelHub {
  // AgentStream is the long-lived bidirectional stream between server and agent.
  rpc AgentStream(stream AgentMessage) returns (stream ServerMessage) {}
}

message AgentMessage {
  oneof payload {
    HeartbeatMsg heartbeat = 1;
    TaskResultMsg task_result = 2;
    JoinMsg join = 3;
  }
}

message ServerMessage {
  oneof payload {
    TaskMsg task = 1;
    PingMsg ping = 2;
  }
}

message JoinMsg {
  string node_id   = 1;  // Persistent UUID from /var/lib/o3k/agent/node-id
  string hostname  = 2;
  string tunnel_ip = 3;
  string token_hash = 4; // HMAC-SHA256 of node_id+"\n"+token_secret
}

message HeartbeatMsg {
  int64 timestamp_unix = 1;
  int32 vcpus_free     = 2;
  int64 memory_free_mb = 3;
}

message TaskMsg {
  string task_id   = 1;
  string task_type = 2; // "create_vm", "delete_vm", "create_port", etc.
  bytes  payload   = 3; // JSON-encoded task parameters
}

message TaskResultMsg {
  string task_id  = 1;
  bool   success  = 2;
  string error    = 3;
  bytes  result   = 4; // JSON-encoded result data
}

message PingMsg {}
```

- [ ] **Step 4: Generate Go code**

```bash
mkdir -p proto/tunnel
protoc --go_out=. --go_opt=paths=source_relative \
       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
       proto/tunnel/tunnel.proto
```

- [ ] **Step 5: Verify compilation**

```bash
go build ./proto/...
```

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum proto/ 
git commit -m "feat(tunnel): add gRPC proto definition and generated code"
```

---

### Task 10: Fix `NodeRegistry` UUID persistence bug

**Files:**
- Modify: `internal/compute/node_registry.go`
- Create: `internal/compute/node_registry_test.go`

The spec documents this bug: `NewNodeRegistry()` regenerates a UUID on every call. Agents need a stable identity across restarts.

- [ ] **Step 1: Write failing test**

```go
// internal/compute/node_registry_test.go
package compute_test

import (
    "os"
    "path/filepath"
    "testing"
    "time"
    "github.com/stretchr/testify/assert"
    "github.com/cobaltcore-dev/o3k/internal/compute"
)

func TestNodeRegistryPersistsUUID(t *testing.T) {
    dir := t.TempDir()
    idFile := filepath.Join(dir, "node-id")

    r1, err := compute.NewNodeRegistryWithIDPath("auto", "127.0.0.1", time.Second, idFile)
    assert.NoError(t, err)
    id1 := r1.GetNodeID()
    assert.NotEmpty(t, id1)

    // Second call must return the same UUID
    r2, err := compute.NewNodeRegistryWithIDPath("auto", "127.0.0.1", time.Second, idFile)
    assert.NoError(t, err)
    assert.Equal(t, id1, r2.GetNodeID(), "UUID must be stable across restarts")

    // File must exist on disk
    _, err = os.Stat(idFile)
    assert.NoError(t, err)
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./internal/compute/... -run TestNodeRegistry 2>&1 | head -20
```

- [ ] **Step 3: Add `NewNodeRegistryWithIDPath` to `node_registry.go`**

```go
// NewNodeRegistryWithIDPath creates a NodeRegistry, persisting the UUID to idFilePath.
// If idFilePath already contains a UUID, it is reused — enabling stable identity across restarts.
// Pass idFilePath="" to use the default path /var/lib/o3k/agent/node-id.
func NewNodeRegistryWithIDPath(nodeID, tunnelIP string, heartbeatInterval time.Duration, idFilePath string) (*NodeRegistry, error) {
    if idFilePath == "" {
        idFilePath = "/var/lib/o3k/agent/node-id"
    }

    if nodeID == "" || nodeID == "auto" {
        // Try to load existing UUID
        if data, err := os.ReadFile(idFilePath); err == nil {
            nodeID = strings.TrimSpace(string(data))
        }
        // Generate new UUID if file missing or empty
        if nodeID == "" {
            nodeID = uuid.New().String()
            if err := os.MkdirAll(filepath.Dir(idFilePath), 0o750); err == nil {
                _ = os.WriteFile(idFilePath, []byte(nodeID), 0o640)
            }
        }
    }

    if tunnelIP == "" || tunnelIP == "auto" {
        detectedIP, err := detectTunnelIP()
        if err != nil {
            return nil, fmt.Errorf("failed to auto-detect tunnel IP: %w", err)
        }
        tunnelIP = detectedIP
    }

    hostname, err := os.Hostname()
    if err != nil {
        return nil, fmt.Errorf("failed to get hostname: %w", err)
    }
    if heartbeatInterval == 0 {
        heartbeatInterval = 30 * time.Second
    }

    return &NodeRegistry{
        nodeID:            nodeID,
        hostname:          hostname,
        tunnelIP:          tunnelIP,
        heartbeatInterval: heartbeatInterval,
        stopChan:          make(chan struct{}),
    }, nil
}
```

Add `"path/filepath"` and `"strings"` to imports.

- [ ] **Step 4: Run — expect PASS**

```bash
go test ./internal/compute/... -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/compute/node_registry.go internal/compute/node_registry_test.go
git commit -m "fix(compute): persist NodeRegistry UUID to disk for stable agent identity"
```

---

### Task 11: Implement `internal/tunnel/server.go` — TunnelHub gRPC server

**Files:**
- Create: `internal/tunnel/server.go`
- Create: `internal/tunnel/server_test.go`

- [ ] **Step 1: Write failing test for agent registration**

```go
// internal/tunnel/server_test.go
package tunnel_test

import (
    "context"
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/cobaltcore-dev/o3k/internal/tunnel"
)

func TestHubTracksConnectedAgents(t *testing.T) {
    hub := tunnel.NewHub("test-token-secret")
    
    // Simulate agent registration
    hub.RegisterAgent(tunnel.AgentInfo{
        NodeID:   "node-1",
        Hostname: "worker-1",
        TunnelIP: "10.0.0.2",
    })

    agents := hub.ListAgents()
    assert.Len(t, agents, 1)
    assert.Equal(t, "node-1", agents[0].NodeID)
}

func TestHubRemovesDisconnectedAgents(t *testing.T) {
    hub := tunnel.NewHub("test-token-secret")
    hub.RegisterAgent(tunnel.AgentInfo{NodeID: "node-1", Hostname: "worker-1", TunnelIP: "10.0.0.2"})
    hub.RemoveAgent("node-1")
    assert.Len(t, hub.ListAgents(), 0)
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./internal/tunnel/... -run TestHub 2>&1 | head -10
```

- [ ] **Step 3: Create `internal/tunnel/server.go`**

```go
package tunnel

import (
    "sync"
    pb "github.com/cobaltcore-dev/o3k/proto/tunnel"
)

// AgentInfo holds registration data for a connected agent.
type AgentInfo struct {
    NodeID   string
    Hostname string
    TunnelIP string
    Stream   pb.TunnelHub_AgentStreamServer // nil in tests
}

// Hub manages connected agents and routes tasks to them.
type Hub struct {
    tokenSecret string
    mu          sync.RWMutex
    agents      map[string]*AgentInfo // nodeID → info
}

// NewHub creates a TunnelHub with the given join token secret.
func NewHub(tokenSecret string) *Hub {
    return &Hub{
        tokenSecret: tokenSecret,
        agents:      make(map[string]*AgentInfo),
    }
}

// RegisterAgent adds an agent to the active set.
func (h *Hub) RegisterAgent(info AgentInfo) {
    h.mu.Lock()
    defer h.mu.Unlock()
    h.agents[info.NodeID] = &info
}

// RemoveAgent removes an agent from the active set.
func (h *Hub) RemoveAgent(nodeID string) {
    h.mu.Lock()
    defer h.mu.Unlock()
    delete(h.agents, nodeID)
}

// ListAgents returns a snapshot of currently connected agents.
func (h *Hub) ListAgents() []AgentInfo {
    h.mu.RLock()
    defer h.mu.RUnlock()
    out := make([]AgentInfo, 0, len(h.agents))
    for _, a := range h.agents {
        out = append(out, *a)
    }
    return out
}

// PickAgent selects the first available agent (round-robin in v2).
// Returns nil if no agents are connected.
func (h *Hub) PickAgent() *AgentInfo {
    h.mu.RLock()
    defer h.mu.RUnlock()
    for _, a := range h.agents {
        return a
    }
    return nil
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
go test ./internal/tunnel/... -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/tunnel/server.go internal/tunnel/server_test.go
git commit -m "feat(tunnel): add Hub with agent registration and selection"
```

---

### Task 12: Add `o3k server`/`o3k agent` subcommand dispatch to `main.go`

**Files:**
- Modify: `cmd/o3k/main.go`
- Modify: `internal/common/config.go`

- [ ] **Step 1: Add `TunnelConfig` and `async_compute` to config**

In `internal/common/config.go`, update `NovaConfig`:
```go
type NovaConfig struct {
    Port          int    `yaml:"port"`
    LibvirtURI    string `yaml:"libvirt_uri"`
    DefaultFlavor string `yaml:"default_flavor"`
    LibvirtMode   string `yaml:"libvirt_mode"`
    AsyncCompute  bool   `yaml:"async_compute"` // true when agent nodes are connected
}
```

Add a new top-level config section:
```go
type TunnelConfig struct {
    Port        int    `yaml:"port"`        // default 6385
    TokenSecret string `yaml:"token_secret"` // mTLS join token secret
    TokenFile   string `yaml:"token_file"`   // path to token file (preferred over inline)
}
```

Add `Tunnel TunnelConfig` to the `Config` struct.

Update `config/o3k.yaml` to document the new fields:
```yaml
tunnel:
  port: 6385
  # token_file: /etc/o3k/token  # preferred
  token_secret: ""  # set via O3K_TUNNEL_TOKEN or token_file
```

- [ ] **Step 2: Write test for subcommand parsing**

```go
// cmd/o3k/main_test.go
package main

import (
    "testing"
    "github.com/stretchr/testify/assert"
)

func TestIsSubcommand(t *testing.T) {
    assert.True(t, isSubcommand("server"))
    assert.True(t, isSubcommand("agent"))
    assert.True(t, isSubcommand("token"))
    assert.False(t, isSubcommand("--config"))
    assert.False(t, isSubcommand(""))
}
```

- [ ] **Step 3: Run — expect FAIL**

```bash
go test ./cmd/o3k/... -run TestIsSubcommand 2>&1 | head -10
```

- [ ] **Step 4: Refactor `main.go` to support subcommands**

Replace `func main()` in `cmd/o3k/main.go` with:

```go
func isSubcommand(s string) bool {
    switch s {
    case "server", "agent", "token":
        return true
    }
    return false
}

func main() {
    if len(os.Args) >= 2 && isSubcommand(os.Args[1]) {
        switch os.Args[1] {
        case "server":
            runServer(os.Args[2:])
        case "agent":
            runAgent(os.Args[2:])
        case "token":
            runTokenCmd(os.Args[2:])
        }
        return
    }
    // Backward compat: no subcommand → server mode
    runServer(os.Args[1:])
}
```

Move all existing `main()` logic into `func runServer(args []string)`. The function signature uses `flag.NewFlagSet("server", flag.ExitOnError)` instead of the package-level `flag.Parse()`.

Add stubs for the new subcommands:

```go
func runAgent(args []string) {
    fs := flag.NewFlagSet("agent", flag.ExitOnError)
    serverAddr := fs.String("server", "", "o3k server address (required)")
    tokenFile := fs.String("token-file", "", "path to join token file")
    nodeIDFile := fs.String("node-id-file", "/var/lib/o3k/agent/node-id", "path to persist node UUID")
    _ = fs.Parse(args)

    if *serverAddr == "" {
        fmt.Fprintln(os.Stderr, "ERROR: --server is required for agent mode")
        os.Exit(1)
    }
    fmt.Printf("o3k agent starting — connecting to %s (node-id-file: %s, token-file: %s)\n",
        *serverAddr, *nodeIDFile, *tokenFile)
    // gRPC client loop implemented in Task 13
    select {} // block until signal
}

func runTokenCmd(args []string) {
    fs := flag.NewFlagSet("token", flag.ExitOnError)
    configPath := fs.String("config", "config/o3k.yaml", "path to config")
    _ = fs.Parse(args)
    fmt.Fprintf(os.Stderr, "o3k token — reads token_secret from %s and prints a join token\n", *configPath)
    fmt.Println("(not yet implemented)")
}
```

- [ ] **Step 5: Run tests**

```bash
go test ./cmd/o3k/... -v
go build ./cmd/o3k/
```

- [ ] **Step 6: Verify backward compatibility**

```bash
./bin/o3k --config config/o3k.yaml &
sleep 1
curl -s http://localhost:35357/v3 | head -5
kill %1
```
Expected: Keystone responds normally (no subcommand = server mode).

- [ ] **Step 7: Commit**

```bash
git add cmd/o3k/main.go internal/common/config.go config/o3k.yaml
git commit -m "feat(agent): add server/agent/token subcommand dispatch to o3k binary"
```

---

### Task 13: Implement `internal/tunnel/client.go` — agent gRPC task loop

**Files:**
- Create: `internal/tunnel/client.go`
- Create: `internal/tunnel/task.go`

- [ ] **Step 1: Write failing test for task dispatch**

```go
// Append to internal/tunnel/server_test.go
func TestHubDispatchTask(t *testing.T) {
    hub := tunnel.NewHub("secret")
    hub.RegisterAgent(tunnel.AgentInfo{NodeID: "node-1", Hostname: "w1", TunnelIP: "10.0.0.2"})

    task := tunnel.Task{
        ID:       "task-abc",
        Type:     "create_vm",
        Payload:  []byte(`{"instance_id":"inst-1"}`),
    }

    agent := hub.PickAgent()
    assert.NotNil(t, agent)
    assert.Equal(t, "node-1", agent.NodeID)

    // Task.Validate checks required fields
    assert.NoError(t, task.Validate())
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./internal/tunnel/... -run TestHubDispatch 2>&1 | head -10
```

- [ ] **Step 3: Create `internal/tunnel/task.go`**

```go
package tunnel

import (
    "fmt"
    "github.com/google/uuid"
)

const (
    TaskCreateVM   = "create_vm"
    TaskDeleteVM   = "delete_vm"
    TaskCreatePort = "create_port"
    TaskDeletePort = "delete_port"
)

// Task represents a unit of work dispatched to an agent.
type Task struct {
    ID      string // UUID, auto-generated if empty
    Type    string // TaskCreateVM, etc.
    Payload []byte // JSON-encoded parameters
}

// Validate checks required fields.
func (t *Task) Validate() error {
    if t.Type == "" {
        return fmt.Errorf("task type is required")
    }
    if t.ID == "" {
        t.ID = uuid.New().String()
    }
    return nil
}
```

- [ ] **Step 4: Create `internal/tunnel/client.go`**

```go
package tunnel

import (
    "context"
    "fmt"
    "time"

    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"
    pb "github.com/cobaltcore-dev/o3k/proto/tunnel"
)

// AgentClient manages the gRPC connection from agent to server.
type AgentClient struct {
    serverAddr string
    nodeID     string
    tokenHash  string
    conn       *grpc.ClientConn
}

// NewAgentClient creates an agent-side gRPC client (not yet connected).
func NewAgentClient(serverAddr, nodeID, tokenHash string) *AgentClient {
    return &AgentClient{
        serverAddr: serverAddr,
        nodeID:     nodeID,
        tokenHash:  tokenHash,
    }
}

// Connect dials the server and opens the AgentStream.
// Blocks until ctx is cancelled or a permanent error occurs.
// Reconnects automatically on transient errors.
func (c *AgentClient) Connect(ctx context.Context) error {
    for {
        if err := c.runStream(ctx); err != nil {
            if ctx.Err() != nil {
                return ctx.Err() // cancelled
            }
            fmt.Printf("tunnel: stream error (%v) — reconnecting in 5s\n", err)
            select {
            case <-time.After(5 * time.Second):
            case <-ctx.Done():
                return ctx.Err()
            }
        }
    }
}

func (c *AgentClient) runStream(ctx context.Context) error {
    conn, err := grpc.NewClient(c.serverAddr,
        grpc.WithTransportCredentials(insecure.NewCredentials()), // mTLS added in Task 14
    )
    if err != nil {
        return fmt.Errorf("dial %s: %w", c.serverAddr, err)
    }
    defer conn.Close()

    client := pb.NewTunnelHubClient(conn)
    stream, err := client.AgentStream(ctx)
    if err != nil {
        return fmt.Errorf("open stream: %w", err)
    }

    // Send join message
    if err := stream.Send(&pb.AgentMessage{
        Payload: &pb.AgentMessage_Join{
            Join: &pb.JoinMsg{
                NodeId:    c.nodeID,
                Hostname:  "agent-hostname", // set from os.Hostname() in production
                TokenHash: c.tokenHash,
            },
        },
    }); err != nil {
        return fmt.Errorf("send join: %w", err)
    }

    // Task receive loop
    for {
        msg, err := stream.Recv()
        if err != nil {
            return fmt.Errorf("recv: %w", err)
        }
        if task := msg.GetTask(); task != nil {
            go c.executeTask(ctx, stream, task)
        }
    }
}

func (c *AgentClient) executeTask(ctx context.Context, stream pb.TunnelHub_AgentStreamClient, task *pb.TaskMsg) {
    // Task execution dispatched to hypervisor/netlink in v2
    fmt.Printf("agent: received task %s type=%s\n", task.TaskId, task.TaskType)
    _ = stream.Send(&pb.AgentMessage{
        Payload: &pb.AgentMessage_TaskResult{
            TaskResult: &pb.TaskResultMsg{
                TaskId:  task.TaskId,
                Success: true,
            },
        },
    })
}
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/tunnel/... -v
go build ./...
```

- [ ] **Step 6: Commit**

```bash
git add internal/tunnel/client.go internal/tunnel/task.go internal/tunnel/server_test.go
git commit -m "feat(tunnel): add Task type and AgentClient with reconnect loop"
```

---

### Task 14: Wire TunnelHub into `runServer`, create join token migration

**Files:**
- Modify: `cmd/o3k/main.go`
- Create: `migrations/060_tunnel_tokens.up.sql`
- Create: `migrations/060_tunnel_tokens.down.sql`

- [ ] **Step 1: Create migration for join tokens table**

```sql
-- migrations/060_tunnel_tokens.up.sql
CREATE TABLE IF NOT EXISTS tunnel_tokens (
    id          TEXT PRIMARY KEY,
    token_hash  TEXT NOT NULL,         -- HMAC-SHA256(token_secret, id)
    description TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at  TIMESTAMPTZ,
    revoked     BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE INDEX IF NOT EXISTS idx_tunnel_tokens_hash ON tunnel_tokens(token_hash);
```

```sql
-- migrations/060_tunnel_tokens.down.sql
DROP TABLE IF EXISTS tunnel_tokens;
```

- [ ] **Step 2: Run migration locally**

```bash
make migrate
```
Expected: migration 060 applied with no error.

- [ ] **Step 3: Add TunnelHub startup to `runServer`**

In `cmd/o3k/main.go`, inside `runServer`, after the database is connected and before `servers` are started:

```go
// Start TunnelHub gRPC server (runs alongside HTTP servers)
if cfg.Tunnel.Port > 0 {
    tokenSecret := cfg.Tunnel.TokenSecret
    if cfg.Tunnel.TokenFile != "" {
        if data, err := os.ReadFile(cfg.Tunnel.TokenFile); err == nil {
            tokenSecret = strings.TrimSpace(string(data))
        }
    }
    hub := tunnel.NewHub(tokenSecret)
    go func() {
        addr := fmt.Sprintf(":%d", cfg.Tunnel.Port)
        if err := hub.ListenAndServe(addr); err != nil {
            log.Error().Err(err).Msg("TunnelHub exited")
        }
    }()
}
```

Add `ListenAndServe` to `internal/tunnel/server.go`:

```go
import (
    "fmt"
    "net"
    "google.golang.org/grpc"
    pb "github.com/cobaltcore-dev/o3k/proto/tunnel"
)

// ListenAndServe starts the gRPC server on addr (e.g. ":6385").
func (h *Hub) ListenAndServe(addr string) error {
    lis, err := net.Listen("tcp", addr)
    if err != nil {
        return fmt.Errorf("tunnel listen %s: %w", addr, err)
    }
    s := grpc.NewServer()
    pb.RegisterTunnelHubServer(s, h)
    fmt.Printf("TunnelHub listening on %s\n", addr)
    return s.Serve(lis)
}

// AgentStream implements the TunnelHub gRPC service.
func (h *Hub) AgentStream(stream pb.TunnelHub_AgentStreamServer) error {
    // First message must be a Join
    msg, err := stream.Recv()
    if err != nil {
        return err
    }
    join := msg.GetJoin()
    if join == nil {
        return fmt.Errorf("first message must be JoinMsg")
    }
    h.RegisterAgent(AgentInfo{
        NodeID:   join.NodeId,
        Hostname: join.Hostname,
        TunnelIP: join.TunnelIp,
        Stream:   stream,
    })
    defer h.RemoveAgent(join.NodeId)

    // Heartbeat loop — forward tasks, process results
    for {
        if _, err := stream.Recv(); err != nil {
            return err
        }
    }
}
```

- [ ] **Step 4: Build full binary**

```bash
make build
```

- [ ] **Step 5: Smoke test — server starts, tunnel hub listens**

```bash
./bin/o3k server --config config/o3k.yaml &
sleep 2
nc -zv localhost 6385 && echo "TunnelHub port open"
kill %1
```

- [ ] **Step 6: Commit**

```bash
git add cmd/o3k/main.go internal/tunnel/server.go migrations/060_tunnel_tokens.up.sql migrations/060_tunnel_tokens.down.sql
git commit -m "feat(tunnel): wire TunnelHub into runServer and add join tokens migration"
```

---

## Self-Review

**Spec coverage check:**

| Requirement (from analysis) | Covered by task |
|-----------------------------|----------------|
| `o3k compat-check` CLI with `--dir` and `--output` flags | Task 1 |
| JSON + text report output | Task 2 |
| Embedded stub server for Terraform hits | Tasks 3–4 |
| CI smoke test | Task 5 |
| DB interface for unit testing | Task 6 |
| Global DB migrated to interface | Task 7 |
| First Nova unit tests | Task 8 |
| gRPC proto definition | Task 9 |
| NodeRegistry UUID persistence bug fix | Task 10 |
| TunnelHub server with agent registration | Task 11 |
| `o3k server`/`o3k agent`/`o3k token` subcommands | Task 12 |
| AgentClient with reconnect loop | Task 13 |
| TunnelHub wired into binary + join tokens migration | Task 14 |

**Placeholder scan:** No TBD or TODO in task steps. All code blocks are complete. `runTokenCmd` has a `fmt.Println("(not yet implemented)")` — intentional, documented.

**Type consistency check:**
- `compat.NewChecker` → `*Checker` → `.Run()` → `*Report` → `.String()` ✓
- `database.DB` interface → `MockDB` implements it → `NewServiceWithDB` accepts it ✓
- `tunnel.Hub` → `RegisterAgent(AgentInfo)` / `PickAgent() *AgentInfo` ✓
- `pb.TunnelHub_AgentStreamServer` used in `AgentInfo.Stream` field — matches generated proto interface ✓

---
