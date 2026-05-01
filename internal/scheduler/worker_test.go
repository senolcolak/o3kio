package scheduler_test

import (
	"context"
	"testing"

	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/cobaltcore-dev/o3k/internal/scheduler"
	"github.com/stretchr/testify/assert"
)

type mockDispatcher struct {
	called bool
}

func (m *mockDispatcher) Dispatch(ctx context.Context, agentID string, taskType string, payload []byte, timeoutSec int) ([]byte, string, error) {
	m.called = true
	return []byte(`{"status":"ok"}`), "", nil
}

func TestNewWorker(t *testing.T) {
	mock := database.NewMockDB()
	d := &mockDispatcher{}
	w := scheduler.NewWorker(mock, d)
	assert.NotNil(t, w)
}

func TestWorker_ProcessOne_NoTasks(t *testing.T) {
	// MockDB.BeginTx returns (nil, nil) — claimTask exits early and the
	// dispatcher must not be called.
	mock := database.NewMockDB()
	d := &mockDispatcher{}
	w := scheduler.NewWorker(mock, d)

	w.ProcessOne(t.Context())

	assert.False(t, d.called, "dispatcher should not be called when no tasks are available")
}
