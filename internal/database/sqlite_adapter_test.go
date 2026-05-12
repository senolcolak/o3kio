package database

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func TestRewritePlaceholders(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "single param",
			in:   "SELECT * FROM x WHERE id = $1",
			want: "SELECT * FROM x WHERE id = ?",
		},
		{
			name: "two params",
			in:   "WHERE a = $1 AND b = $2",
			want: "WHERE a = ? AND b = ?",
		},
		{
			name: "eleven params",
			in:   "INSERT INTO t VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)",
			want: "INSERT INTO t VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		},
		{
			name: "no params",
			in:   "SELECT * FROM t",
			want: "SELECT * FROM t",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rewritePlaceholders(tt.in)
			if got != tt.want {
				t.Errorf("rewritePlaceholders(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestRewriteDialect(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "ILIKE uppercase",
			in:   "WHERE name ILIKE $1",
			want: "WHERE name LIKE $1",
		},
		{
			name: "ilike lowercase",
			in:   "WHERE name ilike $1",
			want: "WHERE name LIKE $1",
		},
		{
			name: "NOW()",
			in:   "WHERE updated_at >= NOW()",
			want: "WHERE updated_at >= CURRENT_TIMESTAMP",
		},
		{
			name: "now() lowercase",
			in:   "INSERT INTO t (created_at) VALUES (now())",
			want: "INSERT INTO t (created_at) VALUES (CURRENT_TIMESTAMP)",
		},
		{
			name: "cast ::text",
			in:   "SELECT id::text FROM t",
			want: "SELECT id FROM t",
		},
		{
			name: "cast ::bigint",
			in:   "SELECT x::bigint FROM t",
			want: "SELECT x FROM t",
		},
		{
			name: "cast ::jsonb",
			in:   "SELECT data::jsonb FROM t",
			want: "SELECT data FROM t",
		},
		{
			name: "FOR UPDATE SKIP LOCKED",
			in:   "SELECT x FROM t FOR UPDATE SKIP LOCKED",
			want: "SELECT x FROM t",
		},
		{
			name: "FOR UPDATE",
			in:   "SELECT x FROM t FOR UPDATE",
			want: "SELECT x FROM t",
		},
		{
			name: "EXTRACT EPOCH",
			in:   "SELECT EXTRACT(EPOCH FROM (NOW() - created_at)) / 3600",
			want: "SELECT (CAST(strftime('%s', CURRENT_TIMESTAMP) AS INTEGER) - CAST(strftime('%s', created_at) AS INTEGER)) / 3600",
		},
		{
			name: "EXTRACT EPOCH nested function call",
			in:   "SELECT EXTRACT(EPOCH FROM (COALESCE(updated_at, created_at)))",
			want: "SELECT CAST(strftime('%s', COALESCE(updated_at, created_at)) AS INTEGER)",
		},
		{
			name: "EXTRACT EPOCH multiple occurrences",
			in:   "SELECT EXTRACT(EPOCH FROM (a)), EXTRACT(EPOCH FROM (b))",
			want: "SELECT CAST(strftime('%s', a) AS INTEGER), CAST(strftime('%s', b) AS INTEGER)",
		},
		{
			name: "no rewrite needed",
			in:   "SELECT id, name FROM users WHERE active = 1",
			want: "SELECT id, name FROM users WHERE active = 1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rewriteDialect(tt.in)
			if got != tt.want {
				t.Errorf("rewriteDialect(%q)\n  got  %q\n  want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestRewrite(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "placeholder + ilike",
			in:   "WHERE name ILIKE $1",
			want: "WHERE name LIKE ?",
		},
		{
			name: "placeholder + cast",
			in:   "SELECT id::text FROM t WHERE project_id = $1",
			want: "SELECT id FROM t WHERE project_id = ?",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rewrite(tt.in)
			if got != tt.want {
				t.Errorf("rewrite(%q)\n  got  %q\n  want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestMapSQLError(t *testing.T) {
	t.Run("nil passes through", func(t *testing.T) {
		if mapSQLError(nil) != nil {
			t.Error("expected nil")
		}
	})
	t.Run("sql.ErrNoRows mapped to ErrNoRows", func(t *testing.T) {
		err := mapSQLError(sql.ErrNoRows)
		if !errors.Is(err, ErrNoRows) {
			t.Errorf("expected ErrNoRows, got %v", err)
		}
	})
	t.Run("other errors pass through", func(t *testing.T) {
		sentinel := errors.New("some db error")
		if mapSQLError(sentinel) != sentinel {
			t.Error("expected sentinel error unchanged")
		}
	})
}

func TestSQLiteAdapter_BasicRoundtrip(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	adapter, err := NewSQLiteAdapter(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteAdapter: %v", err)
	}
	t.Cleanup(adapter.Close)

	ctx := t.Context()

	// Create table.
	_, err = adapter.Exec(ctx, `CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	// Insert with PostgreSQL-style placeholder rewrite.
	res, err := adapter.Exec(ctx, `INSERT INTO users (id, name) VALUES ($1, $2)`, 1, "alice")
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if n := res.RowsAffected(); n != 1 {
		t.Errorf("rows affected = %d, want 1", n)
	}

	// QueryRow with placeholder rewrite.
	var name string
	err = adapter.QueryRow(ctx, `SELECT name FROM users WHERE id = $1`, 1).Scan(&name)
	if err != nil {
		t.Fatalf("query row: %v", err)
	}
	if name != "alice" {
		t.Errorf("name = %q, want alice", name)
	}

	// ErrNoRows on missing row.
	err = adapter.QueryRow(ctx, `SELECT name FROM users WHERE id = $1`, 99).Scan(&name)
	if !errors.Is(err, ErrNoRows) {
		t.Errorf("expected ErrNoRows, got %v", err)
	}
}

func TestSQLiteAdapter_Query(t *testing.T) {
	dir := t.TempDir()
	adapter, err := NewSQLiteAdapter(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter: %v", err)
	}
	t.Cleanup(adapter.Close)

	ctx := t.Context()

	_, err = adapter.Exec(ctx, `CREATE TABLE items (id INTEGER, val TEXT)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	for i, v := range []string{"a", "b", "c"} {
		_, err = adapter.Exec(ctx, `INSERT INTO items VALUES ($1, $2)`, i+1, v)
		if err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}

	rows, err := adapter.Query(ctx, `SELECT id, val FROM items ORDER BY id`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	var got []string
	for rows.Next() {
		var id int
		var val string
		if err := rows.Scan(&id, &val); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, val)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows err: %v", err)
	}
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Errorf("got %v, want [a b c]", got)
	}
}

func TestSQLiteAdapter_Transaction(t *testing.T) {
	dir := t.TempDir()
	adapter, err := NewSQLiteAdapter(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter: %v", err)
	}
	t.Cleanup(adapter.Close)

	ctx := t.Context()

	_, err = adapter.Exec(ctx, `CREATE TABLE kv (k TEXT PRIMARY KEY, v TEXT)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	t.Run("commit", func(t *testing.T) {
		tx, err := adapter.BeginTx(ctx, TxOptions{})
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		_, err = tx.Exec(ctx, `INSERT INTO kv VALUES ($1, $2)`, "key1", "val1")
		if err != nil {
			t.Fatalf("tx exec: %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit: %v", err)
		}

		var v string
		err = adapter.QueryRow(ctx, `SELECT v FROM kv WHERE k = $1`, "key1").Scan(&v)
		if err != nil {
			t.Fatalf("query after commit: %v", err)
		}
		if v != "val1" {
			t.Errorf("v = %q, want val1", v)
		}
	})

	t.Run("rollback", func(t *testing.T) {
		tx, err := adapter.BeginTx(ctx, TxOptions{})
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		_, err = tx.Exec(ctx, `INSERT INTO kv VALUES ($1, $2)`, "key2", "val2")
		if err != nil {
			t.Fatalf("tx exec: %v", err)
		}
		if err := tx.Rollback(ctx); err != nil {
			t.Fatalf("rollback: %v", err)
		}

		var v string
		err = adapter.QueryRow(ctx, `SELECT v FROM kv WHERE k = $1`, "key2").Scan(&v)
		if !errors.Is(err, ErrNoRows) {
			t.Errorf("expected ErrNoRows after rollback, got v=%q err=%v", v, err)
		}
	})
}

// Compile-time check: SQLiteAdapter satisfies DBIF.
var _ DBIF = (*SQLiteAdapter)(nil)

// ---------------------------------------------------------------------------
// castRegex and rewriteExtractEpoch targeted tests
// ---------------------------------------------------------------------------

// TestCastRegexExpandedTypes verifies that all extended PostgreSQL type-cast
// suffixes are stripped by rewriteDialect.
func TestCastRegexExpandedTypes(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "::numeric(10,2)",
			in:   "SELECT price::numeric(10,2) FROM products",
			want: "SELECT price FROM products",
		},
		{
			name: "::varchar(255)",
			in:   "SELECT name::varchar(255) FROM t",
			want: "SELECT name FROM t",
		},
		{
			name: "::double precision",
			in:   "SELECT val::double precision FROM t",
			want: "SELECT val FROM t",
		},
		{
			name: "::timestamp with time zone",
			in:   "SELECT created_at::timestamp with time zone FROM t",
			want: "SELECT created_at FROM t",
		},
		{
			name: "::inet",
			in:   "SELECT addr::inet FROM t",
			want: "SELECT addr FROM t",
		},
		{
			name: "::bytea",
			in:   "SELECT data::bytea FROM t",
			want: "SELECT data FROM t",
		},
		{
			name: "multiple casts in one query",
			in:   "SELECT a::int, b::text, c::numeric(5,2) FROM t",
			want: "SELECT a, b, c FROM t",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rewriteDialect(tt.in)
			if got != tt.want {
				t.Errorf("rewriteDialect(%q)\n  got  %q\n  want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestExtractEpochDateSubtraction verifies that EXTRACT(EPOCH FROM (a - b))
// is rewritten to the two-argument strftime subtraction form.
func TestExtractEpochDateSubtraction(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "now minus column",
			in:   "SELECT EXTRACT(EPOCH FROM (NOW() - created_at)) / 3600",
			want: "SELECT (CAST(strftime('%s', CURRENT_TIMESTAMP) AS INTEGER) - CAST(strftime('%s', created_at) AS INTEGER)) / 3600",
		},
		{
			name: "column minus column",
			in:   "SELECT EXTRACT(EPOCH FROM (updated_at - created_at))",
			want: "SELECT (CAST(strftime('%s', updated_at) AS INTEGER) - CAST(strftime('%s', created_at) AS INTEGER))",
		},
		{
			name: "spaced operands",
			in:   "SELECT EXTRACT(EPOCH FROM (  end_time  -  start_time  ))",
			want: "SELECT (CAST(strftime('%s', end_time) AS INTEGER) - CAST(strftime('%s', start_time) AS INTEGER))",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rewriteDialect(tt.in)
			if got != tt.want {
				t.Errorf("rewriteDialect(%q)\n  got  %q\n  want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestExtractEpochNoSubtraction verifies that EXTRACT(EPOCH FROM (col)) with
// no subtraction still produces a single-argument strftime cast (no regression).
func TestExtractEpochNoSubtraction(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "single column",
			in:   "SELECT EXTRACT(EPOCH FROM (created_at))",
			want: "SELECT CAST(strftime('%s', created_at) AS INTEGER)",
		},
		{
			name: "nested function call",
			in:   "SELECT EXTRACT(EPOCH FROM (COALESCE(updated_at, created_at)))",
			want: "SELECT CAST(strftime('%s', COALESCE(updated_at, created_at)) AS INTEGER)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rewriteDialect(tt.in)
			if got != tt.want {
				t.Errorf("rewriteDialect(%q)\n  got  %q\n  want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestExtractEpochMultipleWithSubtraction verifies that a query with several
// EXTRACT calls — some with subtraction, some without — are each rewritten
// correctly without interfering with each other.
func TestExtractEpochMultipleWithSubtraction(t *testing.T) {
	in := "SELECT EXTRACT(EPOCH FROM (end_at - start_at)), EXTRACT(EPOCH FROM (now)), EXTRACT(EPOCH FROM (finish - begin))"
	want := "SELECT (CAST(strftime('%s', end_at) AS INTEGER) - CAST(strftime('%s', start_at) AS INTEGER)), CAST(strftime('%s', now) AS INTEGER), (CAST(strftime('%s', finish) AS INTEGER) - CAST(strftime('%s', begin) AS INTEGER))"
	got := rewriteDialect(in)
	if got != want {
		t.Errorf("rewriteDialect multiple EXTRACT\n  got  %q\n  want %q", got, want)
	}
}

func TestSQLiteAdapter_OnConflictDoNothing(t *testing.T) {
	dir := t.TempDir()
	adapter, err := NewSQLiteAdapter(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter: %v", err)
	}
	t.Cleanup(adapter.Close)
	ctx := t.Context()

	_, err = adapter.Exec(ctx, `CREATE TABLE kv (k TEXT PRIMARY KEY, v TEXT)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	// First insert
	_, err = adapter.Exec(ctx, `INSERT INTO kv (k, v) VALUES ($1, $2)`, "key1", "first")
	if err != nil {
		t.Fatalf("first insert: %v", err)
	}

	// Duplicate insert with ON CONFLICT DO NOTHING — must not error
	_, err = adapter.Exec(ctx, `INSERT INTO kv (k, v) VALUES ($1, $2) ON CONFLICT (k) DO NOTHING`, "key1", "second")
	if err != nil {
		t.Fatalf("ON CONFLICT DO NOTHING: %v", err)
	}

	// Value should still be "first"
	var v string
	err = adapter.QueryRow(ctx, `SELECT v FROM kv WHERE k = $1`, "key1").Scan(&v)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if v != "first" {
		t.Errorf("value = %q, want \"first\" (DO NOTHING should preserve original)", v)
	}
}

func TestSQLiteAdapter_OnConflictDoUpdate(t *testing.T) {
	dir := t.TempDir()
	adapter, err := NewSQLiteAdapter(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter: %v", err)
	}
	t.Cleanup(adapter.Close)
	ctx := t.Context()

	_, err = adapter.Exec(ctx, `CREATE TABLE kv (k TEXT PRIMARY KEY, v TEXT, updated_at TEXT)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	// First insert
	_, err = adapter.Exec(ctx, `INSERT INTO kv (k, v, updated_at) VALUES ($1, $2, $3)`, "key1", "first", "2024-01-01")
	if err != nil {
		t.Fatalf("first insert: %v", err)
	}

	// Upsert with ON CONFLICT DO UPDATE
	_, err = adapter.Exec(ctx,
		`INSERT INTO kv (k, v, updated_at) VALUES ($1, $2, $3) ON CONFLICT (k) DO UPDATE SET v = EXCLUDED.v, updated_at = EXCLUDED.updated_at`,
		"key1", "updated", "2024-06-01")
	if err != nil {
		t.Fatalf("ON CONFLICT DO UPDATE: %v", err)
	}

	var v, ts string
	err = adapter.QueryRow(ctx, `SELECT v, updated_at FROM kv WHERE k = $1`, "key1").Scan(&v, &ts)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if v != "updated" {
		t.Errorf("value = %q, want \"updated\"", v)
	}
	if ts != "2024-06-01" {
		t.Errorf("updated_at = %q, want \"2024-06-01\"", ts)
	}
}

func TestSQLiteAdapter_Returning(t *testing.T) {
	dir := t.TempDir()
	adapter, err := NewSQLiteAdapter(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter: %v", err)
	}
	t.Cleanup(adapter.Close)
	ctx := t.Context()

	_, err = adapter.Exec(ctx, `CREATE TABLE items (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT, active INTEGER DEFAULT 1)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	// INSERT with RETURNING
	var id int
	var name string
	err = adapter.QueryRow(ctx,
		`INSERT INTO items (name) VALUES ($1) RETURNING id, name`, "test-item",
	).Scan(&id, &name)
	if err != nil {
		t.Fatalf("INSERT RETURNING: %v", err)
	}
	if id != 1 {
		t.Errorf("id = %d, want 1", id)
	}
	if name != "test-item" {
		t.Errorf("name = %q, want \"test-item\"", name)
	}

	// UPDATE with RETURNING
	var newName string
	err = adapter.QueryRow(ctx,
		`UPDATE items SET name = $1 WHERE id = $2 RETURNING name`, "renamed", 1,
	).Scan(&newName)
	if err != nil {
		t.Fatalf("UPDATE RETURNING: %v", err)
	}
	if newName != "renamed" {
		t.Errorf("name after update = %q, want \"renamed\"", newName)
	}
}
