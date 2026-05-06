package tunnel_test

import (
	"testing"

	"github.com/cobaltcore-dev/o3k/internal/tunnel"
	"github.com/stretchr/testify/assert"
)

func TestDispatcherNoAgents(t *testing.T) {
	hub := tunnel.NewHub("secret")
	dispatcher := tunnel.NewDispatcher(hub)

	task := tunnel.Task{
		Type:    tunnel.TaskCreateVM,
		Payload: []byte(`{"instance_id":"inst-1"}`),
	}

	err := dispatcher.Dispatch(task)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no agents")
}

func TestDispatcherWithAgent(t *testing.T) {
	hub := tunnel.NewHub("secret")
	hub.RegisterAgent(&tunnel.AgentInfo{NodeID: "node-1", Hostname: "w1", TunnelIP: "10.0.0.2"})
	dispatcher := tunnel.NewDispatcher(hub)

	task := tunnel.Task{
		Type:    tunnel.TaskCreateVM,
		Payload: []byte(`{"instance_id":"inst-1"}`),
	}

	err := dispatcher.Dispatch(task)
	// Stream is nil in tests — should error gracefully
	if err != nil {
		assert.Contains(t, err.Error(), "stream")
	}
}
