package tunnel_test

import (
	"testing"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/tunnel"
	"github.com/stretchr/testify/assert"
)

func TestHubTracksConnectedAgents(t *testing.T) {
	hub := tunnel.NewHub("test-token-secret")

	hub.RegisterAgent(&tunnel.AgentInfo{
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
	hub.RegisterAgent(&tunnel.AgentInfo{NodeID: "node-1", Hostname: "worker-1", TunnelIP: "10.0.0.2"})
	hub.RemoveAgent("node-1")
	assert.Len(t, hub.ListAgents(), 0)
}

func TestHubDispatchTask(t *testing.T) {
	hub := tunnel.NewHub("secret")
	hub.RegisterAgent(&tunnel.AgentInfo{NodeID: "node-1", Hostname: "w1", TunnelIP: "10.0.0.2"})

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

func TestHubSetTLSConfig(t *testing.T) {
	ca, err := tunnel.GenerateCA()
	assert.NoError(t, err)
	serverCert, err := tunnel.SignCert(ca, "o3k-server")
	assert.NoError(t, err)
	tlsCfg, err := tunnel.ServerTLSConfig(ca, serverCert)
	assert.NoError(t, err)

	hub := tunnel.NewHub("secret")
	hub.SetTLSConfig(tlsCfg)
	// Verify it doesn't panic — full E2E in Task 7.
}

func TestAgentClientSetTLSConfig(t *testing.T) {
	ca, err := tunnel.GenerateCA()
	assert.NoError(t, err)
	clientCert, err := tunnel.SignCert(ca, "agent-1")
	assert.NoError(t, err)
	tlsCfg, err := tunnel.ClientTLSConfig(ca, clientCert)
	assert.NoError(t, err)

	client := tunnel.NewAgentClient("127.0.0.1:6385", "node-1", "hash")
	client.SetTLSConfig(tlsCfg)
	// No panic — full E2E would require running server.
}

func TestHubInflightTracking(t *testing.T) {
	hub := tunnel.NewHub("secret")
	hub.RegisterAgent(&tunnel.AgentInfo{NodeID: "node-1", Hostname: "w1", TunnelIP: "10.0.0.2"})

	assert.True(t, hub.TryAcquireInflight("node-1"))
	assert.False(t, hub.TryAcquireInflight("node-1"))

	hub.ReleaseInflight("node-1")
	assert.True(t, hub.TryAcquireInflight("node-1"))
}

func TestHubResultChannelDelivery(t *testing.T) {
	hub := tunnel.NewHub("secret")

	ch := hub.RegisterResultChan("task-123")
	hub.DeliverResult(tunnel.ResultMsg{
		TaskID:  "task-123",
		Success: true,
		Result:  []byte(`{"ok":true}`),
	})

	select {
	case msg := <-ch:
		assert.True(t, msg.Success)
		assert.Equal(t, "task-123", msg.TaskID)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for result")
	}
}
