package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/tunnel"
)

// defaultTaskTimeout is the maximum time Dispatch will wait for a task result
// when the caller's context carries no deadline of its own.
const defaultTaskTimeout = 5 * time.Minute

// HubAdapter adapts tunnel.Hub to the scheduler.Dispatcher interface.
type HubAdapter struct {
	hub *tunnel.Hub
}

// NewHubAdapter wraps hub so it satisfies the scheduler.Dispatcher interface.
func NewHubAdapter(hub *tunnel.Hub) *HubAdapter {
	return &HubAdapter{hub: hub}
}

// Dispatch routes the task to the agent identified by agentID (as selected by the
// scheduler algorithm), validates the task, sends it via the tunnel stream, and
// blocks until a TaskResult arrives or the context is cancelled.
// A 5-minute safety-net timeout is applied when the incoming context carries no
// deadline, preventing tasks from blocking indefinitely if an agent crashes.
func (h *HubAdapter) Dispatch(ctx context.Context, agentID string, taskType string, payload []byte, timeoutSec int) ([]byte, string, error) {
	agent := h.hub.GetAgent(agentID)
	if agent == nil {
		return nil, "", fmt.Errorf("agent %s not connected", agentID)
	}
	if agent.Stream == nil {
		return nil, "", fmt.Errorf("agent %s has no active stream", agent.NodeID)
	}
	if !h.hub.TryAcquireInflight(agent.NodeID) {
		return nil, "", fmt.Errorf("agent %s busy", agent.NodeID)
	}

	task := tunnel.Task{Type: taskType, Payload: payload}
	if err := task.Validate(); err != nil {
		h.hub.ReleaseInflight(agent.NodeID)
		return nil, "", err
	}

	resultCh := h.hub.RegisterResultChan(task.ID)

	if err := agent.SendTask(task); err != nil {
		h.hub.ReleaseInflight(agent.NodeID)
		return nil, err.Error(), err
	}

	// Apply a safety-net timeout when the caller's context has no deadline.
	// worker.go already sets context.WithTimeout(ctx, timeoutSec), so in normal
	// operation this branch is taken only in tests or direct callers.
	waitCtx := ctx
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		waitCtx, cancel = context.WithTimeout(ctx, defaultTaskTimeout)
		defer cancel()
	}

	select {
	case result := <-resultCh:
		if result.Error != "" {
			return nil, result.Error, nil
		}
		return result.Result, "", nil
	case <-waitCtx.Done():
		h.hub.ReleaseInflight(agent.NodeID)
		// Remove the map entry so it does not leak; any late result arriving
		// after cancellation will be discarded by DeliverResult's default branch.
		h.hub.UnregisterResultChan(task.ID)
		return nil, "", waitCtx.Err()
	}
}
