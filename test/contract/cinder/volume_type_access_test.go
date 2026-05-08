package cinder_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/gophercloud/gophercloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCinderVolumeTypeAccessCRUD tests volume type access operations
func TestCinderVolumeTypeAccessCRUD(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupCinderClient(t)

	// Create a private volume type
	volumeType := map[string]interface{}{
		"volume_type": map[string]interface{}{
			"name": "private-type-test",
			"os-volume-type-access:is_public": false,
		},
	}

	body, _ := json.Marshal(volumeType)
	url := client.ServiceURL("types")
	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var createResult struct {
		VolumeType map[string]interface{} `json:"volume_type"`
	}
	json.Unmarshal(respBody, &createResult)
	typeID := createResult.VolumeType["id"].(string)
	require.NotEmpty(t, typeID)

	// TEST 1: List volume type access (should be empty initially)
	url = client.ServiceURL("types", typeID, "os-volume-type-access")
	req, err = http.NewRequest("GET", url, nil)
	require.NoError(t, err)
	req.Header.Set("X-Auth-Token", client.TokenID)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// TEST 2: Add project access
	projectID := getProjectIDFromToken(t, client)
	addAccess := map[string]interface{}{
		"addProjectAccess": map[string]interface{}{
			"project": projectID,
		},
	}
	body, _ = json.Marshal(addAccess)
	url = client.ServiceURL("types", typeID, "action")
	req, _ = http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)

	// Verify access was added
	url = client.ServiceURL("types", typeID, "os-volume-type-access")
	req, _ = http.NewRequest("GET", url, nil)
	req.Header.Set("X-Auth-Token", client.TokenID)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, _ = io.ReadAll(resp.Body)
	var listResult struct {
		VolumeTypeAccess []map[string]interface{} `json:"volume_type_access"`
	}
	json.Unmarshal(respBody, &listResult)

	found := false
	for _, access := range listResult.VolumeTypeAccess {
		if access["project_id"].(string) == projectID {
			found = true
			break
		}
	}
	assert.True(t, found, "Project access should be added")

	// TEST 3: Remove project access
	removeAccess := map[string]interface{}{
		"removeProjectAccess": map[string]interface{}{
			"project": projectID,
		},
	}
	body, _ = json.Marshal(removeAccess)
	url = client.ServiceURL("types", typeID, "action")
	req, _ = http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)

	// Verify access was removed
	url = client.ServiceURL("types", typeID, "os-volume-type-access")
	req, _ = http.NewRequest("GET", url, nil)
	req.Header.Set("X-Auth-Token", client.TokenID)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, _ = io.ReadAll(resp.Body)
	json.Unmarshal(respBody, &listResult)

	found = false
	for _, access := range listResult.VolumeTypeAccess {
		if access["project_id"].(string) == projectID {
			found = true
			break
		}
	}
	assert.False(t, found, "Project access should be removed")

	// Cleanup
	url = client.ServiceURL("types", typeID)
	req, _ = http.NewRequest("DELETE", url, nil)
	req.Header.Set("X-Auth-Token", client.TokenID)
	http.DefaultClient.Do(req)
}

// Helper to extract project ID from token context
func getProjectIDFromToken(t *testing.T, client *gophercloud.ServiceClient) string {
	// Get projects from keystone
	req, _ := http.NewRequest("GET", "http://localhost:35357/v3/projects", nil)
	req.Header.Set("X-Auth-Token", client.TokenID)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		Projects []map[string]interface{} `json:"projects"`
	}
	json.Unmarshal(respBody, &result)

	if len(result.Projects) > 0 {
		return result.Projects[0]["id"].(string)
	}

	t.Skip("No projects available")
	return ""
}
