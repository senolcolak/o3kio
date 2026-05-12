package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	_ "modernc.org/sqlite" // pure-Go SQLite driver, no CGO required
)

// placeholderRegex matches PostgreSQL positional parameters ($1, $2, ...).
var placeholderRegex = regexp.MustCompile(`\$\d+`)

// rewritePlaceholders converts PostgreSQL $N parameters to SQLite ? parameters.
func rewritePlaceholders(query string) string {
	return placeholderRegex.ReplaceAllString(query, "?")
}

// castRegex matches PostgreSQL type-cast suffixes (::text, ::jsonb, ::varchar(N), etc.).
// It covers all common PostgreSQL cast targets including parameterized types.
var castRegex = regexp.MustCompile(`(?i)::\s*(?:double\s+precision|timestamp(?:\s+(?:with|without)\s+time\s+zone)?|varchar(?:\(\d+\))?|numeric(?:\(\d+(?:,\s*\d+)?\))?|char(?:\(\d+\))?|smallint|bigint|integer|int|jsonb|json|boolean|uuid|real|float|interval|date|time|bytea|inet|cidr|text)`)

// rewriteDialect rewrites PostgreSQL-specific syntax to SQLite equivalents.
func rewriteDialect(query string) string {
	// ILIKE → LIKE (SQLite LIKE is case-insensitive for ASCII by default)
	query = strings.ReplaceAll(query, " ILIKE ", " LIKE ")
	query = strings.ReplaceAll(query, " ilike ", " LIKE ")

	// NOW() → CURRENT_TIMESTAMP
	query = strings.ReplaceAll(query, "NOW()", "CURRENT_TIMESTAMP")
	query = strings.ReplaceAll(query, "now()", "CURRENT_TIMESTAMP")

	// Remove PostgreSQL type casts: ::text, ::int, ::bigint, etc.
	query = castRegex.ReplaceAllString(query, "")

	// EXTRACT(EPOCH FROM (...)) → CAST(strftime('%s', ...) AS INTEGER)
	// Uses a balanced-paren scanner to correctly handle nested function calls.
	query = rewriteExtractEpoch(query)

	// Remove row-locking clauses SQLite handles via BEGIN IMMEDIATE
	query = strings.ReplaceAll(query, " FOR UPDATE SKIP LOCKED", "")
	query = strings.ReplaceAll(query, " FOR UPDATE", "")

	return query
}

// rewriteExtractEpoch rewrites all EXTRACT(EPOCH FROM (...)) occurrences in
// query to CAST(strftime('%s', ...) AS INTEGER). It uses a balanced-paren
// scanner so nested function calls inside the expression are handled correctly.
func rewriteExtractEpoch(query string) string {
	const prefix = "EXTRACT(EPOCH FROM ("
	var b strings.Builder
	for {
		idx := strings.Index(query, prefix)
		if idx == -1 {
			b.WriteString(query)
			break
		}
		// Write everything before the match.
		b.WriteString(query[:idx])

		// Find the matching closing paren for the outer '(' opened by prefix.
		// prefix ends with '(' so we start one level deep.
		rest := query[idx+len(prefix):]
		depth := 1
		closeIdx := -1
		for i, ch := range rest {
			switch ch {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					closeIdx = i
				}
			}
			if closeIdx != -1 {
				break
			}
		}
		if closeIdx == -1 {
			// Unbalanced parens — emit unchanged and stop.
			b.WriteString(query[idx:])
			break
		}
		inner := rest[:closeIdx]
		if parts := strings.SplitN(inner, " - ", 2); len(parts) == 2 {
			left := strings.TrimSpace(parts[0])
			right := strings.TrimSpace(parts[1])
			fmt.Fprintf(&b, "(CAST(strftime('%%s', %s) AS INTEGER) - CAST(strftime('%%s', %s) AS INTEGER))", left, right)
		} else {
			fmt.Fprintf(&b, "CAST(strftime('%%s', %s) AS INTEGER)", inner)
		}

		// Advance past the closing ')' of EXTRACT(...).
		// rest[closeIdx] closes the inner expression; rest[closeIdx+1] closes EXTRACT.
		after := rest[closeIdx+1:]
		if len(after) > 0 && after[0] == ')' {
			after = after[1:]
		}
		query = after
	}
	return b.String()
}

// rewrite applies placeholder and dialect rewrites in one step.
func rewrite(query string) string {
	return rewriteDialect(rewritePlaceholders(query))
}

// mapSQLError translates database/sql sentinel errors to database package errors.
func mapSQLError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNoRows
	}
	return err
}

// SQLiteAdapter wraps a *sql.DB (modernc SQLite) and implements DBIF.
type SQLiteAdapter struct {
	db *sql.DB
}

// NewSQLiteAdapter opens a SQLite database at dbPath and returns a DBIF-compatible adapter.
// WAL mode and a 30-second busy timeout are enabled by default.
func NewSQLiteAdapter(dbPath string) (*SQLiteAdapter, error) {
	dsn := dbPath + "?_journal=WAL&_busy_timeout=30000&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	// SQLite supports only one concurrent writer.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite database: %w", err)
	}
	return &SQLiteAdapter{db: db}, nil
}

// Close releases the underlying SQLite connection.
func (a *SQLiteAdapter) Close() { a.db.Close() }

// Ping verifies the SQLite connection is still alive by executing SELECT 1.
func (a *SQLiteAdapter) Ping(ctx context.Context) error {
	return a.db.PingContext(ctx)
}

func (a *SQLiteAdapter) Exec(ctx context.Context, query string, args ...any) (Result, error) {
	res, err := a.db.ExecContext(ctx, rewrite(query), args...)
	if err != nil {
		return nil, mapSQLError(err)
	}
	return &sqlResult{res: res}, nil
}

func (a *SQLiteAdapter) QueryRow(ctx context.Context, query string, args ...any) Row {
	return &sqlRow{row: a.db.QueryRowContext(ctx, rewrite(query), args...)}
}

func (a *SQLiteAdapter) Query(ctx context.Context, query string, args ...any) (Rows, error) {
	rows, err := a.db.QueryContext(ctx, rewrite(query), args...)
	if err != nil {
		return nil, mapSQLError(err)
	}
	return &sqlRows{rows: rows}, nil
}

// BeginTx starts a transaction. opts.ReadOnly is forwarded to the driver.
// opts.IsoLevel is intentionally not passed per-transaction: the DSN already
// includes _txlock=immediate, which promotes all write transactions to
// IMMEDIATE mode globally, preventing SQLITE_BUSY from deferred lock
// upgrades. Read-only transactions still use shared (deferred) locks because
// the SQLite driver honours ReadOnly=true and issues BEGIN without IMMEDIATE.
func (a *SQLiteAdapter) BeginTx(ctx context.Context, opts TxOptions) (Tx, error) {
	tx, err := a.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: opts.ReadOnly})
	if err != nil {
		return nil, mapSQLError(err)
	}
	return &sqlTx{tx: tx}, nil
}

// sqlResult wraps sql.Result.
type sqlResult struct{ res sql.Result }

func (r *sqlResult) RowsAffected() int64 {
	n, _ := r.res.RowsAffected()
	return n
}

// sqlRow wraps *sql.Row.
type sqlRow struct{ row *sql.Row }

func (r *sqlRow) Scan(dest ...any) error { return mapSQLError(r.row.Scan(dest...)) }

// sqlRows wraps *sql.Rows.
type sqlRows struct{ rows *sql.Rows }

func (r *sqlRows) Next() bool             { return r.rows.Next() }
func (r *sqlRows) Scan(dest ...any) error { return mapSQLError(r.rows.Scan(dest...)) }
func (r *sqlRows) Close()                 { r.rows.Close() }
func (r *sqlRows) Err() error             { return mapSQLError(r.rows.Err()) }

// sqlTx wraps *sql.Tx.
type sqlTx struct{ tx *sql.Tx }

func (t *sqlTx) Exec(ctx context.Context, query string, args ...any) (Result, error) {
	res, err := t.tx.ExecContext(ctx, rewrite(query), args...)
	if err != nil {
		return nil, mapSQLError(err)
	}
	return &sqlResult{res: res}, nil
}

func (t *sqlTx) QueryRow(ctx context.Context, query string, args ...any) Row {
	return &sqlRow{row: t.tx.QueryRowContext(ctx, rewrite(query), args...)}
}

func (t *sqlTx) Query(ctx context.Context, query string, args ...any) (Rows, error) {
	rows, err := t.tx.QueryContext(ctx, rewrite(query), args...)
	if err != nil {
		return nil, mapSQLError(err)
	}
	return &sqlRows{rows: rows}, nil
}

func (t *sqlTx) Commit(_ context.Context) error   { return t.tx.Commit() }
func (t *sqlTx) Rollback(_ context.Context) error { return t.tx.Rollback() }

// migrationVersionRegex extracts the numeric prefix from migration filenames like "001_initial_schema.up.sql".
var migrationVersionRegex = regexp.MustCompile(`^(\d+)_.+\.up\.sql$`)

// MigrateSQLiteFS applies SQLite migration files from an fs.FS (typically an
// embed.FS) under the "sqlite" directory, in sorted order.  It tracks applied
// migrations in a schema_migrations table and is idempotent.
//
// Use this in place of MigrateSQLite when the binary embeds migrations via
// go:embed so that no migrations/ directory is required at runtime.
func MigrateSQLiteFS(fsys fs.FS) error {
	adapter, ok := unwrapDB(DB).(*SQLiteAdapter)
	if !ok {
		return fmt.Errorf("MigrateSQLiteFS called but DB is not SQLite")
	}
	db := adapter.db

	// Create schema_migrations tracking table if not exists.
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TEXT DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}

	// Read migration entries from the embedded FS.
	entries, err := fs.ReadDir(fsys, "sqlite")
	if err != nil {
		return fmt.Errorf("read sqlite migrations from embedded FS: %w", err)
	}

	// Filter and sort *.up.sql files by numeric version prefix.
	type migration struct {
		version  string
		filename string
	}
	var migrations []migration
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		matches := migrationVersionRegex.FindStringSubmatch(entry.Name())
		if matches == nil {
			continue
		}
		migrations = append(migrations, migration{version: matches[1], filename: entry.Name()})
	}
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].version < migrations[j].version
	})

	// Apply each migration that hasn't been applied yet.
	applied := 0
	for _, m := range migrations {
		var exists string
		err := db.QueryRow("SELECT version FROM schema_migrations WHERE version = ?", m.version).Scan(&exists)
		if err == nil {
			// Already applied.
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("check migration %s: %w", m.version, err)
		}

		// Read and execute migration inside a transaction.
		content, err := fs.ReadFile(fsys, filepath.Join("sqlite", m.filename))
		if err != nil {
			return fmt.Errorf("read embedded migration file %s: %w", m.filename, err)
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin transaction for migration %s: %w", m.version, err)
		}

		if _, err := tx.Exec(string(content)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("execute migration %s: %w", m.version, err)
		}

		if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", m.version); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %s: %w", m.version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", m.version, err)
		}
		applied++
	}

	if applied == 0 {
		fmt.Println("SQLite database is already up to date")
	} else {
		fmt.Printf("Applied %d SQLite migration(s)\n", applied)
	}
	return nil
}

// MigrateSQLite applies SQLite migration files from {migrationsPath}/sqlite/ in sorted order.
// It tracks applied migrations in a schema_migrations table and is idempotent.
func MigrateSQLite(migrationsPath string) error {
	adapter, ok := unwrapDB(DB).(*SQLiteAdapter)
	if !ok {
		return fmt.Errorf("MigrateSQLite called but DB is not SQLite")
	}
	db := adapter.db

	sqliteDir := filepath.Join(migrationsPath, "sqlite")
	if _, err := os.Stat(sqliteDir); os.IsNotExist(err) {
		return fmt.Errorf("sqlite migrations directory does not exist: %s", sqliteDir)
	}

	// Create schema_migrations tracking table if not exists.
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TEXT DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}

	// Read migration files.
	entries, err := os.ReadDir(sqliteDir)
	if err != nil {
		return fmt.Errorf("read sqlite migrations directory: %w", err)
	}

	// Filter and sort *.up.sql files by numeric version prefix.
	type migration struct {
		version  string
		filename string
	}
	var migrations []migration
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		matches := migrationVersionRegex.FindStringSubmatch(entry.Name())
		if matches == nil {
			continue
		}
		migrations = append(migrations, migration{version: matches[1], filename: entry.Name()})
	}
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].version < migrations[j].version
	})

	// Apply each migration that hasn't been applied yet.
	applied := 0
	for _, m := range migrations {
		var exists string
		err := db.QueryRow("SELECT version FROM schema_migrations WHERE version = ?", m.version).Scan(&exists)
		if err == nil {
			// Already applied.
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("check migration %s: %w", m.version, err)
		}

		// Read and execute migration inside a transaction.
		content, err := os.ReadFile(filepath.Join(sqliteDir, m.filename))
		if err != nil {
			return fmt.Errorf("read migration file %s: %w", m.filename, err)
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin transaction for migration %s: %w", m.version, err)
		}

		if _, err := tx.Exec(string(content)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("execute migration %s: %w", m.version, err)
		}

		if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", m.version); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %s: %w", m.version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", m.version, err)
		}
		applied++
	}

	if applied == 0 {
		fmt.Println("SQLite database is already up to date")
	} else {
		fmt.Printf("Applied %d SQLite migration(s)\n", applied)
	}
	return nil
}
