package cinder

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCinderListGroups_Contract tests GET /v3/:project/groups
func TestCinderListGroups_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupCinderClient(t)

	url := client.ServiceURL("groups")
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		Groups []map[string]interface{} `json:"groups"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.NotNil(t, result.Groups)
}

// TestCinderCreateGroup_Contract tests POST /v3/:project/groups
func TestCinderCreateGroup_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupCinderClient(t)

	payload := map[string]interface{}{
		"group": map[string]interface{}{
			"name":        "test-group",
			"description": "Test volume group",
			"group_type":  "default",
		},
	}

	body, _ := json.Marshal(payload)
	url := client.ServiceURL("groups")
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusAccepted, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		Group map[string]interface{} `json:"group"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.NotEmpty(t, result.Group["id"])
	assert.Equal(t, "test-group", result.Group["name"])
	assert.Equal(t, "Test volume group", result.Group["description"])

	// Cleanup
	groupID := result.Group["id"].(string)
	cleanupTestGroup(t, client, groupID)
}

// TestCinderGetGroup_Contract tests GET /v3/:project/groups/:id
func TestCinderGetGroup_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupCinderClient(t)

	// Create test group
	groupID := createTestGroup(t, client)

	url := client.ServiceURL("groups", groupID)
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		Group map[string]interface{} `json:"group"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.Equal(t, groupID, result.Group["id"])

	// Cleanup
	cleanupTestGroup(t, client, groupID)
}

// TestCinderUpdateGroup_Contract tests PUT /v3/:project/groups/:id
func TestCinderUpdateGroup_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupCinderClient(t)

	// Create test group
	groupID := createTestGroup(t, client)

	payload := map[string]interface{}{
		"group": map[string]interface{}{
			"name":        "updated-group-name",
			"description": "Updated group description",
		},
	}

	body, _ := json.Marshal(payload)
	url := client.ServiceURL("groups", groupID)
	req, err := http.NewRequest("PUT", url, bytes.NewReader(body))
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		Group map[string]interface{} `json:"group"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.Equal(t, "updated-group-name", result.Group["name"])
	assert.Equal(t, "Updated group description", result.Group["description"])

	// Cleanup
	cleanupTestGroup(t, client, groupID)
}

// TestCinderDeleteGroup_Contract tests DELETE /v3/:project/groups/:id
func TestCinderDeleteGroup_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupCinderClient(t)

	// Create test group
	groupID := createTestGroup(t, client)

	url := client.ServiceURL("groups", groupID)
	req, err := http.NewRequest("DELETE", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusAccepted, resp.StatusCode)
}

// Helper to create test group
func createTestGroup(t *testing.T, client *gophercloud.ServiceClient) string {
	t.Helper()

	payload := map[string]interface{}{
		"group": map[string]interface{}{
			"name":        "test-group",
			"description": "Test group",
			"group_type":  "default",
		},
	}

	body, _ := json.Marshal(payload)
	url := client.ServiceURL("groups")
	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to create group: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		Group map[string]interface{} `json:"group"`
	}
	json.Unmarshal(respBody, &result)

	return result.Group["id"].(string)
}

// Helper to cleanup test group
func cleanupTestGroup(t *testing.T, client *gophercloud.ServiceClient, groupID string) {
	t.Helper()

	url := client.ServiceURL("groups", groupID)
	req, _ := http.NewRequest("DELETE", url, nil)
	req.Header.Set("X-Auth-Token", client.TokenID)
	http.DefaultClient.Do(req)
}
