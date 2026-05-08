package database

import "context"

// DBIF is the minimal interface used by all internal packages for database access.
type DBIF interface {
	Exec(ctx context.Context, sql string, args ...any) (Result, error)
	QueryRow(ctx context.Context, sql string, args ...any) Row
	Query(ctx context.Context, sql string, args ...any) (Rows, error)
	BeginTx(ctx context.Context, opts TxOptions) (Tx, error)
}
