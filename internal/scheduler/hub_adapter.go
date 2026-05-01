package scheduler

import (
	"context"
	"fmt"

	"github.com/cobaltcore-dev/o3k/internal/tunnel"
)

// HubAdapter adapts tunnel.Hub to the scheduler.Dispatcher interface.
type HubAdapter struct {
	hub *tunnel.Hub
}

// NewHubAdapter wraps hub so it satisfies the scheduler.Dispatcher interface.
func NewHubAdapter(hub *tunnel.Hub) *HubAdapter {
	return &HubAdapter{hub: hub}
}

// Dispatch picks an available agent, validates the task, sends it via the tunnel
// stream, and blocks until a TaskResult arrives or the context is cancelled.
func (h *HubAdapter) Dispatch(ctx context.Context, agentID string, taskType string, payload []byte, timeoutSec int) ([]byte, string, error) {
	agent := h.hub.PickAgent()
	if agent == nil {
		return nil, "", fmt.Errorf("no agents connected")
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

	d := tunnel.NewDispatcher(h.hub)
	if err := d.Dispatch(task); err != nil {
		h.hub.ReleaseInflight(agent.NodeID)
		return nil, err.Error(), err
	}

	select {
	case result := <-resultCh:
		if result.Error != "" {
			return nil, result.Error, nil
		}
		return result.Result, "", nil
	case <-ctx.Done():
		h.hub.ReleaseInflight(agent.NodeID)
		return nil, "", ctx.Err()
	}
}
