package cinder_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCinderListVolumeTypes_Contract tests GET /v3/:project_id/types
func TestCinderListVolumeTypes_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupCinderClient(t)

	url := client.Endpoint + "types"
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)
	assert.Contains(t, result, "volume_types")
}

// TestCinderCreateVolumeType_Contract tests POST /v3/:project_id/types
func TestCinderCreateVolumeType_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupCinderClient(t)

	url := client.Endpoint + "types"
	body := strings.NewReader(`{"volume_type": {"name": "test-volume-type", "description": "Test volume type"}}`)
	req, err := http.NewRequest("POST", url, body)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)
	assert.Contains(t, result, "volume_type")

	// Clean up
	if volType, ok := result["volume_type"].(map[string]interface{}); ok {
		if typeID, ok := volType["id"].(string); ok {
			deleteURL := client.Endpoint + "types/" + typeID
			delReq, _ := http.NewRequest("DELETE", deleteURL, nil)
			delReq.Header.Set("X-Auth-Token", client.TokenID)
			http.DefaultClient.Do(delReq)
		}
	}
}

// TestCinderGetVolumeType_Contract tests GET /v3/:project_id/types/:id
func TestCinderGetVolumeType_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupCinderClient(t)

	// Create volume type
	createURL := client.Endpoint + "types"
	createBody := strings.NewReader(`{"volume_type": {"name": "test-get-volume-type"}}`)
	createReq, _ := http.NewRequest("POST", createURL, createBody)
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := http.DefaultClient.Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()

	var createResult map[string]interface{}
	json.NewDecoder(createResp.Body).Decode(&createResult)
	volType := createResult["volume_type"].(map[string]interface{})
	typeID := volType["id"].(string)

	// Test: Get volume type
	url := client.Endpoint + "types/" + typeID
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Clean up
	delReq, _ := http.NewRequest("DELETE", url, nil)
	delReq.Header.Set("X-Auth-Token", client.TokenID)
	http.DefaultClient.Do(delReq)
}

// TestCinderUpdateVolumeType_Contract tests PUT /v3/:project_id/types/:id
func TestCinderUpdateVolumeType_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupCinderClient(t)

	// Create volume type
	createURL := client.Endpoint + "types"
	createBody := strings.NewReader(`{"volume_type": {"name": "test-update-volume-type"}}`)
	createReq, _ := http.NewRequest("POST", createURL, createBody)
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := http.DefaultClient.Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()

	var createResult map[string]interface{}
	json.NewDecoder(createResp.Body).Decode(&createResult)
	volType := createResult["volume_type"].(map[string]interface{})
	typeID := volType["id"].(string)

	// Test: Update volume type
	url := client.Endpoint + "types/" + typeID
	body := strings.NewReader(`{"volume_type": {"description": "Updated description"}}`)
	req, err := http.NewRequest("PUT", url, body)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Clean up
	delReq, _ := http.NewRequest("DELETE", url, nil)
	delReq.Header.Set("X-Auth-Token", client.TokenID)
	http.DefaultClient.Do(delReq)
}

// TestCinderDeleteVolumeType_Contract tests DELETE /v3/:project_id/types/:id
func TestCinderDeleteVolumeType_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupCinderClient(t)

	// Create volume type
	createURL := client.Endpoint + "types"
	createBody := strings.NewReader(`{"volume_type": {"name": "test-delete-volume-type"}}`)
	createReq, _ := http.NewRequest("POST", createURL, createBody)
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := http.DefaultClient.Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()

	var createResult map[string]interface{}
	json.NewDecoder(createResp.Body).Decode(&createResult)
	volType := createResult["volume_type"].(map[string]interface{})
	typeID := volType["id"].(string)

	// Test: Delete volume type
	url := client.Endpoint + "types/" + typeID
	req, err := http.NewRequest("DELETE", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusAccepted, resp.StatusCode)
}

// TestCinderListVolumeTypeExtraSpecs_Contract tests GET /v3/:project_id/types/:id/extra_specs
func TestCinderListVolumeTypeExtraSpecs_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupCinderClient(t)

	// Create volume type
	createURL := client.Endpoint + "types"
	createBody := strings.NewReader(`{"volume_type": {"name": "test-extra-specs-type"}}`)
	createReq, _ := http.NewRequest("POST", createURL, createBody)
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := http.DefaultClient.Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()

	var createResult map[string]interface{}
	json.NewDecoder(createResp.Body).Decode(&createResult)
	volType := createResult["volume_type"].(map[string]interface{})
	typeID := volType["id"].(string)

	// Test: List extra specs
	url := client.Endpoint + "types/" + typeID + "/extra_specs"
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Clean up
	delURL := client.Endpoint + "types/" + typeID
	delReq, _ := http.NewRequest("DELETE", delURL, nil)
	delReq.Header.Set("X-Auth-Token", client.TokenID)
	http.DefaultClient.Do(delReq)
}
