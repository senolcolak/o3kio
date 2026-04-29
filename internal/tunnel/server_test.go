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

func TestHubDispatchTask(t *testing.T) {
	hub := tunnel.NewHub("secret")
	hub.RegisterAgent(tunnel.AgentInfo{NodeID: "node-1", Hostname: "w1", TunnelIP: "10.0.0.2"})

	task := tunnel.Task{
		ID:      "task-abc",
		Type:    "create_vm",
		Payload: []byte(`{"instance_id":"inst-1"}`),
	}

	agent := hub.PickAgent()
	assert.NotNil(t, agent)
	assert.Equal(t, "node-1", agent.NodeID)

	assert.NoError(t, task.Validate())
}

func TestHubVerifiesToken(t *testing.T) {
	secret := "test-secret"
	hub := tunnel.NewHub(secret)

	validHash := tunnel.GenerateTokenHash(secret, "node-1")
	assert.True(t, hub.VerifyJoin("node-1", validHash))
	assert.False(t, hub.VerifyJoin("node-1", "bad-hash"))
	assert.False(t, hub.VerifyJoin("node-2", validHash))
}
