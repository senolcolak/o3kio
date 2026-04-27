package tunnel_test

import (
	"testing"

	"github.com/cobaltcore-dev/o3k/internal/tunnel"
	"github.com/stretchr/testify/assert"
)

func TestHubTracksConnectedAgents(t *testing.T) {
	hub := tunnel.NewHub("test-token-secret")

	hub.RegisterAgent(tunnel.AgentInfo{
		NodeID:   "node-1",
		Hostname: "worker-1",
		TunnelIP: "10.0.0.2",
	})

	agents := hub.ListAgents()
	assert.Len(t, agents, 1)
	assert.Equal(t, "node-1", agents[0].NodeID)
}

func TestHubRemovesDisconnectedAgents(t *testing.T) {
	hub := tunnel.NewHub("test-token-secret")
	hub.RegisterAgent(tunnel.AgentInfo{NodeID: "node-1", Hostname: "worker-1", TunnelIP: "10.0.0.2"})
	hub.RemoveAgent("node-1")
	assert.Len(t, hub.ListAgents(), 0)
}
