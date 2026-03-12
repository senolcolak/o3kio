package neutron_test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNeutronGetAutoAllocatedTopology_Contract tests GET /v2.0/auto-allocated-topology/:project
func TestNeutronGetAutoAllocatedTopology_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNeutronClient(t)

	// Get auto-allocated topology (returns existing or dry-run info)
	url := client.ServiceURL("auto-allocated-topology", "default")
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Either 200 OK (exists) or 404 (not created yet)
	assert.Contains(t, []int{http.StatusOK, http.StatusNotFound}, resp.StatusCode)

	if resp.StatusCode == http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		var result struct {
			AutoAllocatedTopology map[string]interface{} `json:"auto_allocated_topology"`
		}
		err = json.Unmarshal(respBody, &result)
		require.NoError(t, err)
		assert.NotNil(t, result.AutoAllocatedTopology)
	}
}

// TestNeutronCreateAutoAllocatedTopology_Contract tests POST /v2.0/auto-allocated-topology/:project
func TestNeutronCreateAutoAllocatedTopology_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNeutronClient(t)

	// Create auto-allocated topology
	url := client.ServiceURL("auto-allocated-topology", "default")
	req, err := http.NewRequest("POST", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should return 200 OK or 201 Created
	assert.Contains(t, []int{http.StatusOK, http.StatusCreated}, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		AutoAllocatedTopology map[string]interface{} `json:"auto_allocated_topology"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	// Response should contain network_id
	assert.NotEmpty(t, result.AutoAllocatedTopology["id"])
}

// TestNeutronDeleteAutoAllocatedTopology_Contract tests DELETE /v2.0/auto-allocated-topology/:project
func TestNeutronDeleteAutoAllocatedTopology_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNeutronClient(t)

	// Create topology first
	createURL := client.ServiceURL("auto-allocated-topology", "default")
	createReq, _ := http.NewRequest("POST", createURL, nil)
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	http.DefaultClient.Do(createReq)

	// Delete auto-allocated topology
	url := client.ServiceURL("auto-allocated-topology", "default")
	req, err := http.NewRequest("DELETE", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should return 204 No Content or 404 (if doesn't exist)
	assert.Contains(t, []int{http.StatusNoContent, http.StatusNotFound}, resp.StatusCode)
}
