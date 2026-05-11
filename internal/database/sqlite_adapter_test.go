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
			want: "SELECT CAST(strftime('%s', CURRENT_TIMESTAMP - created_at) AS INTEGER) / 3600",
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
