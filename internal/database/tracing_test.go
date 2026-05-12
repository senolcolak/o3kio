package database

import (
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestTracingAdapterPreservesBackendType(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	adapter, err := NewSQLiteAdapter(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteAdapter: %v", err)
	}

	// Set global DB to the SQLite adapter
	DB = adapter
	if got := BackendType(); got != "sqlite" {
		t.Errorf("BackendType() before wrapping = %q, want \"sqlite\"", got)
	}

	// Wrap with TracingAdapter (simulates main.go:372)
	DB = NewTracingAdapter(DB)

	// BackendType must still return "sqlite" after wrapping
	if got := BackendType(); got != "sqlite" {
		t.Errorf("BackendType() after wrapping = %q, want \"sqlite\"", got)
	}
}

func TestTracingAdapterHealthCheckWorks(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	adapter, err := NewSQLiteAdapter(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteAdapter: %v", err)
	}

	DB = NewTracingAdapter(adapter)

	// HealthCheck must not panic and must not return error for SQLite
	if err := HealthCheck(t.Context()); err != nil {
		t.Errorf("HealthCheck() after wrapping returned error: %v", err)
	}
}

func TestTracingAdapterCloseDoesNotPanic(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	adapter, err := NewSQLiteAdapter(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteAdapter: %v", err)
	}

	DB = NewTracingAdapter(adapter)

	// Close must not panic when DB is wrapped
	Close()
}

func TestTracingAdapterMigrateSQLiteFSWorks(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	adapter, err := NewSQLiteAdapter(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteAdapter: %v", err)
	}

	DB = NewTracingAdapter(adapter)

	// Use an empty but non-nil FS with a "sqlite" directory.
	// MigrateSQLiteFS must be able to unwrap TracingAdapter to find SQLiteAdapter.
	// An empty FS means 0 migrations applied — no error expected.
	fsys := fstest.MapFS{
		"sqlite/.keep": &fstest.MapFile{},
	}
	if err := MigrateSQLiteFS(fsys); err != nil {
		t.Errorf("MigrateSQLiteFS failed after wrapping with TracingAdapter: %v", err)
	}
}

func TestUnwrapDB(t *testing.T) {
	dir := t.TempDir()
	adapter, err := NewSQLiteAdapter(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter: %v", err)
	}
	t.Cleanup(adapter.Close)

	// Single wrap
	wrapped := NewTracingAdapter(adapter)
	if _, ok := unwrapDB(wrapped).(*SQLiteAdapter); !ok {
		t.Error("unwrapDB failed to reach SQLiteAdapter through single TracingAdapter")
	}

	// Double wrap (future-proofing)
	doubleWrapped := NewTracingAdapter(wrapped)
	if _, ok := unwrapDB(doubleWrapped).(*SQLiteAdapter); !ok {
		t.Error("unwrapDB failed to reach SQLiteAdapter through double TracingAdapter")
	}

	// No wrap
	if _, ok := unwrapDB(adapter).(*SQLiteAdapter); !ok {
		t.Error("unwrapDB failed on unwrapped adapter")
	}
}
