package database

import (
	"context"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

// QueryRow implements DBIF. Returns a row that always returns pgx.ErrNoRows on Scan.
func (m *MockDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return &mockRow{err: pgx.ErrNoRows}
}

// Query implements DBIF. Returns an empty result set.
func (m *MockDB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return &mockRows{}, nil
}

// BeginTx implements DBIF. Returns nil — tests that need transaction behaviour
// should stub this separately.
func (m *MockDB) BeginTx(ctx context.Context, opts pgx.TxOptions) (pgx.Tx, error) {
	return nil, nil
}

// mockRow implements pgx.Row.
type mockRow struct{ err error }

func (r *mockRow) Scan(dest ...any) error { return r.err }

// mockRows implements pgx.Rows.
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
