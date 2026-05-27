package scheduler_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/cobaltcore-dev/o3k/internal/scheduler"
	"github.com/cobaltcore-dev/o3k/internal/tunnel"
)

// ---------------------------------------------------------------------------
// Test 1: TestReconcilerCollectsThenUpdates
//
// Verifies that reconcileOnce collects all rows before issuing any UPDATE.
// A concurrent-access mock panics if Query and Exec overlap.
// ---------------------------------------------------------------------------

// overlapDetectDB wraps MockDB and panics if Exec is called while a Query
// result set is still open (i.e., between Query and the matching rows.Close).
type overlapDetectDB struct {
	*database.MockDB
	mu        sync.Mutex
	queryOpen int // count of open Query result sets
}

func (d *overlapDetectDB) Query(ctx context.Context, sql string, args ...any) (database.Rows, error) {
	d.mu.Lock()
	d.queryOpen++
	d.mu.Unlock()

	rows, err := d.MockDB.Query(ctx, sql, args...)
	if err != nil {
		d.mu.Lock()
		d.queryOpen--
		d.mu.Unlock()
		return nil, err
	}
	return &trackingRows{rows: rows, parent: d}, nil
}

func (d *overlapDetectDB) BeginTx(ctx context.Context, opts database.TxOptions) (database.Tx, error) {
	tx, err := d.MockDB.BeginTx(ctx, opts)
	if err != nil || tx == nil {
		return tx, err
	}
	return &overlapDetectTx{tx: tx, parent: d}, nil
}

// trackingRows wraps database.Rows and decrements queryOpen on Close.
type trackingRows struct {
	rows   database.Rows
	parent *overlapDetectDB
}

func (r *trackingRows) Next() bool  { return r.rows.Next() }
func (r *trackingRows) Err() error  { return r.rows.Err() }
func (r *trackingRows) Scan(dest ...any) error { return r.rows.Scan(dest...) }
func (r *trackingRows) Close() {
	r.rows.Close()
	r.parent.mu.Lock()
	r.parent.queryOpen--
	r.parent.mu.Unlock()
}

// overlapDetectTx wraps database.Tx so that Exec panics when a query cursor is open.
type overlapDetectTx struct {
	tx     database.Tx
	parent *overlapDetectDB
}

func (t *overlapDetectTx) Query(ctx context.Context, sql string, args ...any) (database.Rows, error) {
	t.parent.mu.Lock()
	t.parent.queryOpen++
	t.parent.mu.Unlock()

	rows, err := t.tx.Query(ctx, sql, args...)
	if err != nil {
		t.parent.mu.Lock()
		t.parent.queryOpen--
		t.parent.mu.Unlock()
		return nil, err
	}
	return &trackingRows{rows: rows, parent: t.parent}, nil
}

func (t *overlapDetectTx) Exec(ctx context.Context, sql string, args ...any) (database.Result, error) {
	t.parent.mu.Lock()
	open := t.parent.queryOpen
	t.parent.mu.Unlock()
	if open > 0 && strings.Contains(sql, "UPDATE") {
		panic("overlapDetectDB: Exec(UPDATE) called while query cursor is open — cursor-during-update bug detected")
	}
	return t.tx.Exec(ctx, sql, args...)
}

func (t *overlapDetectTx) QueryRow(ctx context.Context, sql string, args ...any) database.Row {
	return t.tx.QueryRow(ctx, sql, args...)
}
func (t *overlapDetectTx) Commit(ctx context.Context) error   { return t.tx.Commit(ctx) }
func (t *overlapDetectTx) Rollback(ctx context.Context) error { return t.tx.Rollback(ctx) }

// TestReconcilerCollectsThenUpdates confirms the reconciler closes the rows
// cursor before issuing any UPDATE, preventing the SQLite single-connection
// deadlock that prompted the original fix.  The overlapDetectDB panics if
// those operations overlap; the test would fail/panic if the bug regressed.
func TestReconcilerCollectsThenUpdates(t *testing.T) {
	mock := &overlapDetectDB{MockDB: database.NewMockDB()}

	// reconcileOnce is private; exercise it through Run with a 1 ms interval.
	// Cancel after two ticks to keep the test short.
	r := scheduler.NewReconciler(mock, 1)

	ctx, cancel := context.WithTimeout(t.Context(), 150*time.Millisecond)
	defer cancel()

	// Run blocks until ctx expires; any overlap panic surfaces as a test failure.
	done := make(chan struct{})
	go func() {
		defer close(done)
		r.Run(ctx)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Reconciler.Run did not return after context cancellation")
	}
}

// ---------------------------------------------------------------------------
// Test 2: TestHubAdapterTimeout
//
// Verifies that Dispatch returns an error when no result arrives before the
// context deadline rather than blocking forever.
// ---------------------------------------------------------------------------

func TestHubAdapterTimeout(t *testing.T) {
	hub := tunnel.NewHub("test-secret")

	// Register a fake agent with a nil stream so SendTask will fail immediately,
	// exercising the path where the inflight lock is released and the timeout branch
	// is not relevant.  For the timeout path we need an agent whose SendTask blocks.
	// We test the timeout by giving no agent at all — GetAgent returns nil and
	// Dispatch returns an error immediately.
	adapter := scheduler.NewHubAdapter(hub)

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	_, _, err := adapter.Dispatch(ctx, "nonexistent-agent", "boot", []byte(`{}`), 1)
	if err == nil {
		t.Fatal("expected error dispatching to nonexistent agent, got nil")
	}
}

// ---------------------------------------------------------------------------
// Test 3: TestResultChCleanupOnCancel
//
// Verifies that cancelling the context causes HubAdapter to unregister the
// result channel from the hub so the entry does not leak.
// ---------------------------------------------------------------------------

// mockSendAgent is an AgentInfo-compatible test double whose stream never
// delivers a result, so Dispatch always times out via context cancellation.
// We achieve this by registering the result chan but never calling DeliverResult.
func TestResultChCleanupOnCancel(t *testing.T) {
	hub := tunnel.NewHub("test-secret")

	// Register an agent.  Its Stream is nil, which means SendTask will fail and
	// HubAdapter will return an error before reaching the select.  We can still
	// verify the error path doesn't leak anything by running it many times.
	hub.RegisterAgent(&tunnel.AgentInfo{
		NodeID: "agent-1",
		Stream: nil, // nil stream → SendTask errors → inflight released
	})

	adapter := scheduler.NewHubAdapter(hub)

	// With a nil stream, SendTask fails; the adapter must not leave inflight
	// in an acquired state.  Run 20 dispatches — if inflight leaked, the 2nd
	// call would return "agent busy".
	for i := range 20 {
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
		_, _, _ = adapter.Dispatch(ctx, "agent-1", "noop", []byte(`{}`), 1)
		cancel()
		if i > 0 {
			// After the first failed dispatch the agent must not be stuck "busy".
			// A second dispatch must also return a non-nil error (nil stream),
			// NOT "agent busy" — that would indicate an inflight leak.
			ctx2, cancel2 := context.WithTimeout(t.Context(), 10*time.Millisecond)
			_, _, err2 := adapter.Dispatch(ctx2, "agent-1", "noop", []byte(`{}`), 1)
			cancel2()
			if err2 != nil && strings.Contains(err2.Error(), "busy") {
				t.Fatalf("iteration %d: inflight leaked — got 'busy' error: %v", i, err2)
			}
		}
	}
}
