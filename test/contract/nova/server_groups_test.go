package nova_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNovaCreateServerGroup_Contract tests POST /v2.1/:project_id/os-server-groups
func TestNovaCreateServerGroup_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	group := map[string]interface{}{
		"server_group": map[string]interface{}{
			"name":     "test-server-group",
			"policies": []string{"anti-affinity"},
		},
	}

	body, _ := json.Marshal(group)
	url := client.Endpoint + "os-server-groups"
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		ServerGroup map[string]interface{} `json:"server_group"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.NotEmpty(t, result.ServerGroup["id"])
	assert.Equal(t, "test-server-group", result.ServerGroup["name"])

	policies := result.ServerGroup["policies"].([]interface{})
	assert.Equal(t, 1, len(policies))
	assert.Equal(t, "anti-affinity", policies[0])

	// Cleanup
	if groupID, ok := result.ServerGroup["id"].(string); ok {
		delReq, _ := http.NewRequest("DELETE", client.Endpoint+"os-server-groups/"+groupID, nil)
		delReq.Header.Set("X-Auth-Token", client.TokenID)
		http.DefaultClient.Do(delReq)
	}
}

// TestNovaListServerGroups_Contract tests GET /v2.1/:project_id/os-server-groups
func TestNovaListServerGroups_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Create a test group first
	group := map[string]interface{}{
		"server_group": map[string]interface{}{
			"name":     "test-list-group",
			"policies": []string{"affinity"},
		},
	}
	groupBody, _ := json.Marshal(group)
	createReq, _ := http.NewRequest("POST", client.Endpoint+"os-server-groups", bytes.NewReader(groupBody))
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(createReq)

	// Test: List server groups
	url := client.Endpoint + "os-server-groups"
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		ServerGroups []map[string]interface{} `json:"server_groups"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.NotEmpty(t, result.ServerGroups)
}

// TestNovaGetServerGroup_Contract tests GET /v2.1/:project_id/os-server-groups/:id
func TestNovaGetServerGroup_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Create a test group
	group := map[string]interface{}{
		"server_group": map[string]interface{}{
			"name":     "test-get-group",
			"policies": []string{"anti-affinity"},
		},
	}
	groupBody, _ := json.Marshal(group)
	createReq, _ := http.NewRequest("POST", client.Endpoint+"os-server-groups", bytes.NewReader(groupBody))
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatal(err)
	}
	defer createResp.Body.Close()

	createBody, _ := io.ReadAll(createResp.Body)
	var createResult struct {
		ServerGroup map[string]interface{} `json:"server_group"`
	}
	json.Unmarshal(createBody, &createResult)
	groupID := createResult.ServerGroup["id"].(string)

	// Test: Get server group
	url := client.Endpoint + "os-server-groups/" + groupID
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		ServerGroup map[string]interface{} `json:"server_group"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.Equal(t, groupID, result.ServerGroup["id"])
	assert.Equal(t, "test-get-group", result.ServerGroup["name"])

	// Cleanup
	delReq, _ := http.NewRequest("DELETE", url, nil)
	delReq.Header.Set("X-Auth-Token", client.TokenID)
	http.DefaultClient.Do(delReq)
}

// TestNovaDeleteServerGroup_Contract tests DELETE /v2.1/:project_id/os-server-groups/:id
func TestNovaDeleteServerGroup_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Create a test group
	group := map[string]interface{}{
		"server_group": map[string]interface{}{
			"name":     "test-delete-group",
			"policies": []string{"affinity"},
		},
	}
	groupBody, _ := json.Marshal(group)
	createReq, _ := http.NewRequest("POST", client.Endpoint+"os-server-groups", bytes.NewReader(groupBody))
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatal(err)
	}
	defer createResp.Body.Close()

	createBody, _ := io.ReadAll(createResp.Body)
	var createResult struct {
		ServerGroup map[string]interface{} `json:"server_group"`
	}
	json.Unmarshal(createBody, &createResult)
	groupID := createResult.ServerGroup["id"].(string)

	// Test: Delete server group
	url := client.Endpoint + "os-server-groups/" + groupID
	req, err := http.NewRequest("DELETE", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}
