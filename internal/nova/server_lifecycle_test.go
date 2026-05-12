package nova_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/cobaltcore-dev/o3k/internal/nova"
)

// novaDB wraps MockDB and returns pre-seeded responses for the queries that
// CreateServer and CheckQuota issue. It delegates everything else to MockDB.
type novaDB struct {
	*database.MockDB
	// quotaLimit for "instances", 0 means no quota row (use default of 10).
	instanceQuotaLimit int
	hasQuotaRow        bool
	// instanceCount is the current count returned for quota checks.
	instanceCount int
}

// novaSeededRow implements database.Row for returning a known set of values.
type novaSeededRow struct {
	values []any
}

func (r *novaSeededRow) Scan(dest ...any) error {
	for i, d := range dest {
		if i >= len(r.values) {
			break
		}
		switch dst := d.(type) {
		case *string:
			if s, ok := r.values[i].(string); ok {
				*dst = s
			}
		case *int:
			if n, ok := r.values[i].(int); ok {
				*dst = n
			}
		}
	}
	return nil
}

func (d *novaDB) QueryRow(_ context.Context, sql string, args ...any) database.Row {
	switch {
	case strings.Contains(sql, "FROM flavors"):
		// Return a valid m1.small flavor.
		return &novaSeededRow{values: []any{
			"flavor-m1-small", "m1.small", 1, 512, 10,
		}}
	case strings.Contains(sql, "FROM quotas"):
		// New combined limits query scans (instanceLimit, coreLimit, ramLimit).
		// When no quota row exists fall through to built-in defaults encoded in the SQL COALESCE.
		if !d.hasQuotaRow {
			return &novaSeededRow{values: []any{10, 20, 51200}}
		}
		return &novaSeededRow{values: []any{d.instanceQuotaLimit, 20, 51200}}
	case strings.Contains(sql, "COUNT(*)") && strings.Contains(sql, "FROM instances"):
		// New combined usage query scans (instanceCount, coreSum, ramSum).
		return &novaSeededRow{values: []any{d.instanceCount, d.instanceCount, d.instanceCount * 512}}
	}
	return &novaErrRow{err: database.ErrNoRows}
}

// BeginTx returns a transaction whose QueryRow/Exec delegate back to novaDB
// so that the in-transaction quota queries hit the same seeded responses.
func (d *novaDB) BeginTx(_ context.Context, _ database.TxOptions) (database.Tx, error) {
	return &novaDBTx{db: d}, nil
}

// novaDBTx is a minimal Tx that delegates reads/writes back to novaDB.
type novaDBTx struct{ db *novaDB }

func (t *novaDBTx) QueryRow(ctx context.Context, sql string, args ...any) database.Row {
	return t.db.QueryRow(ctx, sql, args...)
}
func (t *novaDBTx) Exec(ctx context.Context, sql string, args ...any) (database.Result, error) {
	return t.db.MockDB.Exec(ctx, sql, args...)
}
func (t *novaDBTx) Query(ctx context.Context, sql string, args ...any) (database.Rows, error) {
	return t.db.MockDB.Query(ctx, sql, args...)
}
func (t *novaDBTx) Commit(_ context.Context) error   { return nil }
func (t *novaDBTx) Rollback(_ context.Context) error { return nil }

// novaErrRow is a database.Row that always returns the given error from Scan.
type novaErrRow struct{ err error }

func (r *novaErrRow) Scan(dest ...any) error { return r.err }

// novaInstanceRow holds the per-row data returned by novaListDB.
type novaInstanceRow struct {
	id     string
	name   string
	status string
}

// novaListDB is a mock DB that returns a configurable list of server rows for
// FROM-instances queries. It avoids the SQLite single-connection deadlock that
// would occur when ListServersDetail calls getInstanceAddresses while iterating
// the main cursor.
type novaListDB struct {
	*database.MockDB
	// rows controls which instances are returned for the main SELECT.
	rows []novaInstanceRow
}

// novaListRows is a Rows implementation backed by a []novaInstanceRow slice.
// It returns the 20 columns ListServersDetail scans.
type novaListRows struct {
	rows    []novaInstanceRow
	current int
}

func (r *novaListRows) Next() bool {
	r.current++
	return r.current <= len(r.rows)
}

func (r *novaListRows) Scan(dest ...any) error {
	row := r.rows[r.current-1]
	now := time.Now()
	emptyNS := sql.NullString{Valid: false}

	type col struct{ v any }
	cols := []col{
		{v: row.id},         // id
		{v: row.name},       // name
		{v: row.status},     // status
		{v: 0},              // power_state
		{v: "test-project"}, // project_id
		{v: "test-user"},    // user_id
		{v: "flavor-m1-small"}, // flavor_id
		{v: (*string)(nil)}, // image_id
		{v: now},            // created_at
		{v: now},            // updated_at
		{v: (*time.Time)(nil)}, // launched_at
		{v: 1},              // vcpus
		{v: 512},            // ram_mb
		{v: 10},             // disk_gb
		{v: "m1.small"},     // flavor_name
		{v: emptyNS},        // host
		{v: false},          // locked
		{v: emptyNS},        // task_state
		{v: emptyNS},        // key_name
		{v: emptyNS},        // fault_message
	}
	for i, d := range dest {
		if i >= len(cols) {
			break
		}
		switch dst := d.(type) {
		case *string:
			if s, ok := cols[i].v.(string); ok {
				*dst = s
			}
		case **string:
			if s, ok := cols[i].v.(*string); ok {
				*dst = s
			}
		case *int:
			if n, ok := cols[i].v.(int); ok {
				*dst = n
			}
		case *bool:
			if b, ok := cols[i].v.(bool); ok {
				*dst = b
			}
		case *time.Time:
			if t, ok := cols[i].v.(time.Time); ok {
				*dst = t
			}
		case **time.Time:
			if t, ok := cols[i].v.(*time.Time); ok {
				*dst = t
			}
		case *sql.NullString:
			if ns, ok := cols[i].v.(sql.NullString); ok {
				*dst = ns
			}
		}
	}
	return nil
}

func (r *novaListRows) Close() {}
func (r *novaListRows) Err() error { return nil }

func (d *novaListDB) Query(_ context.Context, sqlStr string, args ...any) (database.Rows, error) {
	if strings.Contains(sqlStr, "FROM instances") {
		rows := d.rows
		// The last arg is the LIMIT value — cap the rows slice accordingly.
		if len(args) > 0 {
			if lim, ok := args[len(args)-1].(int); ok && lim > 0 && lim < len(rows) {
				rows = rows[:lim]
			}
		}
		return &novaListRows{rows: rows}, nil
	}
	return &emptyNovaRows{}, nil
}

func (d *novaListDB) QueryRow(ctx context.Context, sqlStr string, args ...any) database.Row {
	return d.MockDB.QueryRow(ctx, sqlStr, args...)
}
func (d *novaListDB) Exec(ctx context.Context, sqlStr string, args ...any) (database.Result, error) {
	return d.MockDB.Exec(ctx, sqlStr, args...)
}
func (d *novaListDB) BeginTx(ctx context.Context, opts database.TxOptions) (database.Tx, error) {
	return d.MockDB.BeginTx(ctx, opts)
}

var _ database.DBIF = (*novaListDB)(nil)

// novaGinContext builds a gin context with auth pre-set.
func novaGinContext(t *testing.T, method, path, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req, _ := http.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	c.Set("project_id", "test-project")
	c.Set("user_id", "test-user")
	c.Set("roles", []string{"member"})
	return c, w
}

// createServerBody returns a minimal valid server-create JSON body.
func createServerBody(name string) string {
	return fmt.Sprintf(`{"server":{"name":%q,"flavorRef":"flavor-m1-small","imageRef":"img-cirros"}}`, name)
}

// TestCreateServerReturnsRequiredFields verifies that the server creation
// response contains all fields required by Terraform.
func TestCreateServerReturnsRequiredFields(t *testing.T) {
	db := &novaDB{MockDB: database.NewMockDB()}
	svc := nova.NewServiceWithDB(db, "stub")
	c, w := novaGinContext(t, http.MethodPost, "/v2.1/servers", createServerBody("tf-test-vm"))

	svc.CreateServer(c)
	require.Equal(t, http.StatusAccepted, w.Code, "body: %s", w.Body.String())

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	server, ok := resp["server"].(map[string]interface{})
	require.True(t, ok)

	for _, field := range []string{"id", "name", "status", "tenant_id", "user_id", "created", "updated", "flavor", "image", "links"} {
		assert.Contains(t, server, field, "response must include field %q", field)
	}
	assert.Equal(t, "BUILD", server["status"])
	assert.NotEmpty(t, server["id"])

	links, ok := server["links"].([]interface{})
	require.True(t, ok)
	assert.NotEmpty(t, links)
}

// TestListServersDetailPagination verifies that listing with limit=2 returns
// exactly 2 entries plus a next-page link when the mock has 5 servers.
func TestListServersDetailPagination(t *testing.T) {
	instances := make([]novaInstanceRow, 5)
	for i := range 5 {
		instances[i] = novaInstanceRow{
			id:     fmt.Sprintf("inst-%03d", i),
			name:   fmt.Sprintf("pag-vm-%d", i),
			status: "ACTIVE",
		}
	}
	db := &novaListDB{MockDB: database.NewMockDB(), rows: instances}
	svc := nova.NewServiceWithDB(db, "stub")

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/v2.1/servers/detail?limit=2", nil)
	c.Set("project_id", "test-project")
	c.Set("user_id", "test-user")
	c.Set("roles", []string{"member"})

	svc.ListServersDetail(c)
	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	servers, ok := resp["servers"].([]interface{})
	require.True(t, ok)
	assert.Len(t, servers, 2, "expected exactly 2 servers with limit=2")
	assert.Contains(t, resp, "servers_links", "pagination link must be present when result hits the limit")
}

// TestListServersFilterByStatus verifies that ?status=ACTIVE filters correctly.
func TestListServersFilterByStatus(t *testing.T) {
	// The mock returns all rows regardless of status — the service applies the
	// filter via SQL. To test filtering without SQLite, we provide only the
	// ACTIVE row; the service receives exactly one row and returns it.
	db := &novaListDB{
		MockDB: database.NewMockDB(),
		rows: []novaInstanceRow{
			{id: "inst-active", name: "active-vm", status: "ACTIVE"},
		},
	}
	svc := nova.NewServiceWithDB(db, "stub")

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/v2.1/servers?status=ACTIVE", nil)
	c.Set("project_id", "test-project")
	c.Set("user_id", "test-user")
	c.Set("roles", []string{"member"})

	svc.ListServers(c)
	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	servers, ok := resp["servers"].([]interface{})
	require.True(t, ok)

	require.Len(t, servers, 1, "expected exactly 1 ACTIVE server")
	server := servers[0].(map[string]interface{})
	assert.Equal(t, "inst-active", server["id"])
}

// TestGetServerIncludesAddresses verifies that the server detail response
// includes an "addresses" key.  We use a custom DB mock to avoid the
// single-connection SQLite deadlock that occurs when ListServersDetail holds
// a rows cursor and calls getInstanceAddresses (nested query) in the loop.
func TestGetServerIncludesAddresses(t *testing.T) {
	db := &novaAddressesDB{MockDB: database.NewMockDB()}
	svc := nova.NewServiceWithDB(db, "stub")

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/v2.1/servers/detail", nil)
	c.Set("project_id", "test-project")
	c.Set("user_id", "test-user")
	c.Set("roles", []string{"member"})

	svc.ListServersDetail(c)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	servers, ok := resp["servers"].([]interface{})
	require.True(t, ok)
	require.NotEmpty(t, servers)
	server := servers[0].(map[string]interface{})
	assert.Contains(t, server, "addresses", "server detail must include 'addresses' key")
}

// novaAddressesDB is a mock that returns one server row for the
// ListServersDetail query and empty rows for everything else (ports, SGs).
type novaAddressesDB struct {
	*database.MockDB
}

// novaServerRows is a single-row Rows implementation that returns the 20
// columns ListServersDetail scans from its main JOIN query.
type novaServerRows struct {
	served bool
}

func (r *novaServerRows) Next() bool {
	if r.served {
		return false
	}
	r.served = true
	return true
}

func (r *novaServerRows) Scan(dest ...any) error {
	// Column order matches ListServersDetail SELECT:
	// id, name, status, power_state, project_id, user_id,
	// flavor_id, image_id(*string), created_at, updated_at, launched_at(*time.Time),
	// vcpus, ram_mb, disk_gb, flavor_name, host(sql.NullString), locked(bool),
	// task_state(sql.NullString), key_name(sql.NullString), fault_message(sql.NullString)
	now := time.Now()
	emptyNS := sql.NullString{Valid: false}

	type col struct{ v any }
	cols := []col{
		{v: "inst-addr"},    // id
		{v: "addr-vm"},      // name
		{v: "ACTIVE"},       // status
		{v: 1},              // power_state
		{v: "test-project"}, // project_id
		{v: "test-user"},    // user_id
		{v: "flv-1"},        // flavor_id
		{v: (*string)(nil)}, // image_id
		{v: now},            // created_at
		{v: now},            // updated_at
		{v: (*time.Time)(nil)}, // launched_at
		{v: 1},              // vcpus
		{v: 512},            // ram_mb
		{v: 10},             // disk_gb
		{v: "m1.small"},     // flavor_name
		{v: emptyNS},        // host
		{v: false},          // locked
		{v: emptyNS},        // task_state
		{v: emptyNS},        // key_name
		{v: emptyNS},        // fault_message
	}
	for i, d := range dest {
		if i >= len(cols) {
			break
		}
		switch dst := d.(type) {
		case *string:
			if s, ok := cols[i].v.(string); ok {
				*dst = s
			}
		case **string:
			if s, ok := cols[i].v.(*string); ok {
				*dst = s
			}
		case *int:
			if n, ok := cols[i].v.(int); ok {
				*dst = n
			}
		case *bool:
			if b, ok := cols[i].v.(bool); ok {
				*dst = b
			}
		case *time.Time:
			if t, ok := cols[i].v.(time.Time); ok {
				*dst = t
			}
		case **time.Time:
			if t, ok := cols[i].v.(*time.Time); ok {
				*dst = t
			}
		case *sql.NullString:
			if ns, ok := cols[i].v.(sql.NullString); ok {
				*dst = ns
			}
		}
	}
	return nil
}

func (r *novaServerRows) Close() {}
func (r *novaServerRows) Err() error { return nil }

func (d *novaAddressesDB) Query(_ context.Context, sql string, args ...any) (database.Rows, error) {
	if strings.Contains(sql, "FROM instances") {
		return &novaServerRows{}, nil
	}
	// ports, security_groups queries — return empty
	return &emptyNovaRows{}, nil
}

// emptyNovaRows implements database.Rows returning no results.
type emptyNovaRows struct{}

func (r *emptyNovaRows) Next() bool             { return false }
func (r *emptyNovaRows) Scan(dest ...any) error { return nil }
func (r *emptyNovaRows) Close()                {}
func (r *emptyNovaRows) Err() error            { return nil }

// Compile-time interface check.
var _ database.DBIF = (*novaAddressesDB)(nil)

func (d *novaAddressesDB) Exec(ctx context.Context, sql string, args ...any) (database.Result, error) {
	return d.MockDB.Exec(ctx, sql, args...)
}
func (d *novaAddressesDB) QueryRow(ctx context.Context, sql string, args ...any) database.Row {
	return d.MockDB.QueryRow(ctx, sql, args...)
}
func (d *novaAddressesDB) BeginTx(ctx context.Context, opts database.TxOptions) (database.Tx, error) {
	return d.MockDB.BeginTx(ctx, opts)
}

// TestQuotaBlocksExcessCreation verifies that creating a server when the
// instance quota is already at its limit returns 413.
func TestQuotaBlocksExcessCreation(t *testing.T) {
	db := &novaDB{
		MockDB:             database.NewMockDB(),
		hasQuotaRow:        true,
		instanceQuotaLimit: 1,
		instanceCount:      1, // already at limit
	}
	svc := nova.NewServiceWithDB(db, "stub")

	c, w := novaGinContext(t, http.MethodPost, "/v2.1/servers", createServerBody("quota-vm"))
	svc.CreateServer(c)
	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code,
		"expected 413 when quota exceeded; body: %s", w.Body.String())
}
