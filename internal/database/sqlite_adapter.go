package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"

	_ "modernc.org/sqlite" // pure-Go SQLite driver, no CGO required
)

// placeholderRegex matches PostgreSQL positional parameters ($1, $2, ...).
var placeholderRegex = regexp.MustCompile(`\$\d+`)

// rewritePlaceholders converts PostgreSQL $N parameters to SQLite ? parameters.
func rewritePlaceholders(query string) string {
	return placeholderRegex.ReplaceAllString(query, "?")
}

// castRegex matches PostgreSQL type-cast suffixes (::text, ::jsonb, etc.).
var castRegex = regexp.MustCompile(`::(text|int|bigint|integer|jsonb|timestamp|boolean|uuid)`)

// extractEpochRegex matches EXTRACT(EPOCH FROM (...)) expressions.
var extractEpochRegex = regexp.MustCompile(`EXTRACT\(EPOCH FROM \(([^)]+)\)\)`)

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
	query = extractEpochRegex.ReplaceAllStringFunc(query, rewriteExtractEpoch)

	// Remove row-locking clauses SQLite handles via BEGIN IMMEDIATE
	query = strings.ReplaceAll(query, " FOR UPDATE SKIP LOCKED", "")
	query = strings.ReplaceAll(query, " FOR UPDATE", "")

	return query
}

func rewriteExtractEpoch(match string) string {
	sub := extractEpochRegex.FindStringSubmatch(match)
	if len(sub) > 1 {
		return fmt.Sprintf("CAST(strftime('%%s', %s) AS INTEGER)", sub[1])
	}
	return match
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
