package database

import (
	"context"
	"errors"
)

// ErrNoRows is returned when a query expects one row but gets none.
// Services should use errors.Is(err, database.ErrNoRows) instead of pgx.ErrNoRows.
var ErrNoRows = errors.New("no rows in result set")

// Result represents the result of an Exec operation.
type Result interface {
	RowsAffected() int64
}

// Row represents a single database row.
type Row interface {
	Scan(dest ...any) error
}

// Rows represents multiple database rows from a query.
type Rows interface {
	Next() bool
	Scan(dest ...any) error
	Close()
	Err() error
}

// Tx represents a database transaction.
type Tx interface {
	Exec(ctx context.Context, sql string, args ...any) (Result, error)
	QueryRow(ctx context.Context, sql string, args ...any) Row
	Query(ctx context.Context, sql string, args ...any) (Rows, error)
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

// TxOptions configures transaction behavior.
type TxOptions struct {
	IsoLevel string // "serializable", "repeatable_read", "read_committed", "read_uncommitted"
	ReadOnly bool
}
