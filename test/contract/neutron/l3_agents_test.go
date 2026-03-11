package neutron_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os/exec"
	"testing"

	"github.com/google/uuid"
	"github.com/gophercloud/gophercloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNeutronListAgents_Contract tests GET /v2.0/agents
func TestNeutronListAgents_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNeutronClient(t)

	url := client.ServiceURL("agents")
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		Agents []map[string]interface{} `json:"agents"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.NotNil(t, result.Agents)
}

// TestNeutronGetAgent_Contract tests GET /v2.0/agents/:id
func TestNeutronGetAgent_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNeutronClient(t)

	// Create test agent
	agentID := createTestAgent(t, "L3 agent", "neutron-l3-agent")

	url := client.ServiceURL("agents", agentID)
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		Agent map[string]interface{} `json:"agent"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.Equal(t, agentID, result.Agent["id"])

	// Cleanup
	cleanupTestAgent(t, agentID)
}

// TestNeutronListL3AgentsOnRouter_Contract tests GET /v2.0/routers/:id/l3-agents
func TestNeutronListL3AgentsOnRouter_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNeutronClient(t)

	// Create test router
	routerID := createTestRouter(t, client, "test-router-l3")

	url := client.ServiceURL("routers", routerID, "l3-agents")
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		Agents []map[string]interface{} `json:"agents"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.NotNil(t, result.Agents)

	// Cleanup
	cleanupTestRouter(t, client, routerID)
}

// TestNeutronAddL3AgentToRouter_Contract tests POST /v2.0/routers/:id/l3-agents
func TestNeutronAddL3AgentToRouter_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNeutronClient(t)

	// Create test router and agent
	routerID := createTestRouter(t, client, "test-router-add-agent")
	agentID := createTestAgent(t, "L3 agent", "neutron-l3-agent")

	// Add agent to router
	payload := map[string]interface{}{
		"agent_id": agentID,
	}

	body, _ := json.Marshal(payload)
	url := client.ServiceURL("routers", routerID, "l3-agents")
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	// Cleanup
	cleanupTestAgent(t, agentID)
	cleanupTestRouter(t, client, routerID)
}

// Helper to create test agent
func createTestAgent(t *testing.T, description, binary string) string {
	t.Helper()

	agentID := uuid.New().String()
	cmd := `docker exec o3k-postgres psql -U lightstack -d lightstack -c "INSERT INTO neutron_agents (id, agent_type, \"binary\", host, description, admin_state_up, alive, created_at) VALUES ('` + agentID + `', 'L3 agent', '` + binary + `', 'compute-1', '` + description + `', true, true, NOW()) ON CONFLICT DO NOTHING;"`
	exec.Command("sh", "-c", cmd).Run()

	return agentID
}

// Helper to cleanup test agent
func cleanupTestAgent(t *testing.T, agentID string) {
	t.Helper()

	cmd := `docker exec o3k-postgres psql -U lightstack -d lightstack -c "DELETE FROM neutron_agents WHERE id='` + agentID + `';"`
	exec.Command("sh", "-c", cmd).Run()
}

// Helper to create test router
func createTestRouter(t *testing.T, client *gophercloud.ServiceClient, name string) string {
	t.Helper()

	payload := map[string]interface{}{
		"router": map[string]interface{}{
			"name": name,
		},
	}

	body, _ := json.Marshal(payload)
	url := client.ServiceURL("routers")
	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to create router: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		Router map[string]interface{} `json:"router"`
	}
	json.Unmarshal(respBody, &result)

	return result.Router["id"].(string)
}

// Helper to cleanup test router
func cleanupTestRouter(t *testing.T, client *gophercloud.ServiceClient, routerID string) {
	t.Helper()

	url := client.ServiceURL("routers", routerID)
	req, _ := http.NewRequest("DELETE", url, nil)
	req.Header.Set("X-Auth-Token", client.TokenID)
	http.DefaultClient.Do(req)
}
