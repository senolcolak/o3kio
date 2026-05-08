package database

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgxAdapter wraps a pgxpool.Pool to implement DBIF.
type PgxAdapter struct {
	pool *pgxpool.Pool
}

// NewPgxAdapter wraps pool in a DBIF-compatible adapter.
func NewPgxAdapter(pool *pgxpool.Pool) *PgxAdapter {
	return &PgxAdapter{pool: pool}
}

// Pool returns the underlying pgxpool for stats/health checks.
func (a *PgxAdapter) Pool() *pgxpool.Pool {
	return a.pool
}

func (a *PgxAdapter) Exec(ctx context.Context, sql string, args ...any) (Result, error) {
	tag, err := a.pool.Exec(ctx, sql, args...)
	if err != nil {
		return nil, mapPgxError(err)
	}
	return &pgxResult{tag: tag}, nil
}

func (a *PgxAdapter) QueryRow(ctx context.Context, sql string, args ...any) Row {
	return &pgxRow{row: a.pool.QueryRow(ctx, sql, args...)}
}

func (a *PgxAdapter) Query(ctx context.Context, sql string, args ...any) (Rows, error) {
	rows, err := a.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, mapPgxError(err)
	}
	return &pgxRows{rows: rows}, nil
}

func (a *PgxAdapter) BeginTx(ctx context.Context, opts TxOptions) (Tx, error) {
	pgxOpts := pgx.TxOptions{}
	switch opts.IsoLevel {
	case "serializable":
		pgxOpts.IsoLevel = pgx.Serializable
	case "repeatable_read":
		pgxOpts.IsoLevel = pgx.RepeatableRead
	case "read_committed":
		pgxOpts.IsoLevel = pgx.ReadCommitted
	}
	pgxOpts.AccessMode = pgx.ReadWrite
	if opts.ReadOnly {
		pgxOpts.AccessMode = pgx.ReadOnly
	}

	tx, err := a.pool.BeginTx(ctx, pgxOpts)
	if err != nil {
		return nil, mapPgxError(err)
	}
	return &pgxTx{tx: tx}, nil
}

// pgxResult wraps pgconn.CommandTag.
type pgxResult struct {
	tag pgconn.CommandTag
}

func (r *pgxResult) RowsAffected() int64 {
	return r.tag.RowsAffected()
}

// pgxRow wraps pgx.Row.
type pgxRow struct {
	row pgx.Row
}

func (r *pgxRow) Scan(dest ...any) error {
	return mapPgxError(r.row.Scan(dest...))
}

// pgxRows wraps pgx.Rows.
type pgxRows struct {
	rows pgx.Rows
}

func (r *pgxRows) Next() bool            { return r.rows.Next() }
func (r *pgxRows) Scan(dest ...any) error { return mapPgxError(r.rows.Scan(dest...)) }
func (r *pgxRows) Close()                { r.rows.Close() }
func (r *pgxRows) Err() error            { return mapPgxError(r.rows.Err()) }

// pgxTx wraps pgx.Tx.
type pgxTx struct {
	tx pgx.Tx
}

func (t *pgxTx) Exec(ctx context.Context, sql string, args ...any) (Result, error) {
	tag, err := t.tx.Exec(ctx, sql, args...)
	if err != nil {
		return nil, mapPgxError(err)
	}
	return &pgxResult{tag: tag}, nil
}

func (t *pgxTx) QueryRow(ctx context.Context, sql string, args ...any) Row {
	return &pgxRow{row: t.tx.QueryRow(ctx, sql, args...)}
}

func (t *pgxTx) Query(ctx context.Context, sql string, args ...any) (Rows, error) {
	rows, err := t.tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, mapPgxError(err)
	}
	return &pgxRows{rows: rows}, nil
}

func (t *pgxTx) Commit(ctx context.Context) error   { return t.tx.Commit(ctx) }
func (t *pgxTx) Rollback(ctx context.Context) error { return t.tx.Rollback(ctx) }

// mapPgxError translates pgx errors to database package errors.
func mapPgxError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNoRows
	}
	return err
}
