package database

import (
	"context"
	"strings"
	"sync"
)

// MockDB is a test double for DBIF. Use NewMockDB() to create one,
// register expected SQL prefixes with OnExec, and assert with ExecCalled.
type MockDB struct {
	mu        sync.Mutex
	execRules map[string]error
	execCalls []string
}

// NewMockDB returns a ready-to-use MockDB.
func NewMockDB() *MockDB {
	return &MockDB{execRules: make(map[string]error)}
}

// OnExec registers an expected SQL prefix. When Exec is called with a statement
// that contains prefix, the provided error (may be nil) is returned.
func (m *MockDB) OnExec(prefix string, err error) {
	m.mu.Lock()
	m.execRules[prefix] = err
	m.mu.Unlock()
}

// ExecCalled reports whether Exec was called with a statement containing prefix.
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

// Exec implements DBIF.
func (m *MockDB) Exec(ctx context.Context, sql string, args ...any) (Result, error) {
	m.mu.Lock()
	m.execCalls = append(m.execCalls, sql)
	for prefix, err := range m.execRules {
		if strings.Contains(sql, prefix) {
			m.mu.Unlock()
			return &mockResult{}, err
		}
	}
	m.mu.Unlock()
	return &mockResult{rows: 1}, nil
}

// QueryRow implements DBIF. Returns a row that always returns ErrNoRows on Scan.
func (m *MockDB) QueryRow(ctx context.Context, sql string, args ...any) Row {
	return &mockRow{err: ErrNoRows}
}

// Query implements DBIF. Returns an empty result set.
func (m *MockDB) Query(ctx context.Context, sql string, args ...any) (Rows, error) {
	return &mockRows{}, nil
}

// BeginTx implements DBIF. Returns a no-op transaction.
func (m *MockDB) BeginTx(ctx context.Context, opts TxOptions) (Tx, error) {
	return &mockTx{db: m}, nil
}

// mockResult implements Result.
type mockResult struct {
	rows int64
}

func (r *mockResult) RowsAffected() int64 { return r.rows }

// mockRow implements Row.
type mockRow struct{ err error }

func (r *mockRow) Scan(dest ...any) error { return r.err }

// mockRows implements Rows.
type mockRows struct{}

func (r *mockRows) Next() bool            { return false }
func (r *mockRows) Scan(dest ...any) error { return nil }
func (r *mockRows) Close()                {}
func (r *mockRows) Err() error            { return nil }

// mockTx implements Tx by delegating reads/writes back to the owning MockDB.
type mockTx struct {
	db *MockDB
}

func (t *mockTx) Exec(ctx context.Context, sql string, args ...any) (Result, error) {
	return t.db.Exec(ctx, sql, args...)
}

func (t *mockTx) QueryRow(ctx context.Context, sql string, args ...any) Row {
	return t.db.QueryRow(ctx, sql, args...)
}

func (t *mockTx) Query(ctx context.Context, sql string, args ...any) (Rows, error) {
	return t.db.Query(ctx, sql, args...)
}

func (t *mockTx) Commit(ctx context.Context) error   { return nil }
func (t *mockTx) Rollback(ctx context.Context) error { return nil }
