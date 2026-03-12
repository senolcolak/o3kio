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

// TestCinderGetVolumeMetadata_Contract tests GET /v3/:project/volumes/:id/metadata
func TestCinderGetVolumeMetadata_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupCinderClient(t)

	// Create test volume using raw HTTP
	createBody := `{"volume": {"size": 1, "name": "metadata-test"}}`
	createURL := client.ServiceURL("volumes")
	createReq, _ := http.NewRequest("POST", createURL, strings.NewReader(createBody))
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := http.DefaultClient.Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()

	createRespBody, _ := io.ReadAll(createResp.Body)
	var createResult struct {
		Volume struct {
			ID string `json:"id"`
		} `json:"volume"`
	}
	json.Unmarshal(createRespBody, &createResult)
	volumeID := createResult.Volume.ID

	defer func() {
		deleteReq, _ := http.NewRequest("DELETE", client.ServiceURL("volumes", volumeID), nil)
		deleteReq.Header.Set("X-Auth-Token", client.TokenID)
		http.DefaultClient.Do(deleteReq)
	}()

	// Get metadata (should be empty initially)
	url := client.ServiceURL("volumes", volumeID, "metadata")
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

	// Should have metadata object
	assert.NotNil(t, result["metadata"])
}

// TestCinderSetVolumeMetadataKey_Contract tests PUT /v3/:project/volumes/:id/metadata/:key
func TestCinderSetVolumeMetadataKey_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupCinderClient(t)

	// Create test volume
	createBody := `{"volume": {"size": 1, "name": "metadata-key-test"}}`
	createURL := client.ServiceURL("volumes")
	createReq, _ := http.NewRequest("POST", createURL, strings.NewReader(createBody))
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := http.DefaultClient.Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()

	createRespBody, _ := io.ReadAll(createResp.Body)
	var createResult struct {
		Volume struct {
			ID string `json:"id"`
		} `json:"volume"`
	}
	json.Unmarshal(createRespBody, &createResult)
	volumeID := createResult.Volume.ID

	defer func() {
		deleteReq, _ := http.NewRequest("DELETE", client.ServiceURL("volumes", volumeID), nil)
		deleteReq.Header.Set("X-Auth-Token", client.TokenID)
		http.DefaultClient.Do(deleteReq)
	}()

	// Set metadata key
	metadataBody := `{"meta": {"environment": "production"}}`
	url := client.ServiceURL("volumes", volumeID, "metadata", "environment")
	req, err := http.NewRequest("PUT", url, strings.NewReader(metadataBody))
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should return 200 OK
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestCinderGetVolumeMetadataKey_Contract tests GET /v3/:project/volumes/:id/metadata/:key
func TestCinderGetVolumeMetadataKey_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupCinderClient(t)

	// Create test volume
	createBody := `{"volume": {"size": 1, "name": "metadata-get-key-test"}}`
	createURL := client.ServiceURL("volumes")
	createReq, _ := http.NewRequest("POST", createURL, strings.NewReader(createBody))
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := http.DefaultClient.Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()

	createRespBody, _ := io.ReadAll(createResp.Body)
	var createResult struct {
		Volume struct {
			ID string `json:"id"`
		} `json:"volume"`
	}
	json.Unmarshal(createRespBody, &createResult)
	volumeID := createResult.Volume.ID

	defer func() {
		deleteReq, _ := http.NewRequest("DELETE", client.ServiceURL("volumes", volumeID), nil)
		deleteReq.Header.Set("X-Auth-Token", client.TokenID)
		http.DefaultClient.Do(deleteReq)
	}()

	// Set a metadata key first
	metadataBody := `{"meta": {"test-key": "test-value"}}`
	setURL := client.ServiceURL("volumes", volumeID, "metadata", "test-key")
	setReq, _ := http.NewRequest("PUT", setURL, strings.NewReader(metadataBody))
	setReq.Header.Set("X-Auth-Token", client.TokenID)
	setReq.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(setReq)

	// Get the metadata key
	url := client.ServiceURL("volumes", volumeID, "metadata", "test-key")
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

	// Should have meta object with the key
	assert.NotNil(t, result["meta"])
}

// TestCinderUpdateAllVolumeMetadata_Contract tests POST /v3/:project/volumes/:id/metadata
func TestCinderUpdateAllVolumeMetadata_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupCinderClient(t)

	// Create test volume
	createBody := `{"volume": {"size": 1, "name": "metadata-all-test"}}`
	createURL := client.ServiceURL("volumes")
	createReq, _ := http.NewRequest("POST", createURL, strings.NewReader(createBody))
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := http.DefaultClient.Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()

	createRespBody, _ := io.ReadAll(createResp.Body)
	var createResult struct {
		Volume struct {
			ID string `json:"id"`
		} `json:"volume"`
	}
	json.Unmarshal(createRespBody, &createResult)
	volumeID := createResult.Volume.ID

	defer func() {
		deleteReq, _ := http.NewRequest("DELETE", client.ServiceURL("volumes", volumeID), nil)
		deleteReq.Header.Set("X-Auth-Token", client.TokenID)
		http.DefaultClient.Do(deleteReq)
	}()

	// Update all metadata
	metadataBody := `{"metadata": {"key1": "value1", "key2": "value2"}}`
	url := client.ServiceURL("volumes", volumeID, "metadata")
	req, err := http.NewRequest("POST", url, strings.NewReader(metadataBody))
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should return 200 OK
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestCinderDeleteVolumeMetadataKey_Contract tests DELETE /v3/:project/volumes/:id/metadata/:key
func TestCinderDeleteVolumeMetadataKey_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupCinderClient(t)

	// Create test volume
	createBody := `{"volume": {"size": 1, "name": "metadata-delete-test"}}`
	createURL := client.ServiceURL("volumes")
	createReq, _ := http.NewRequest("POST", createURL, strings.NewReader(createBody))
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := http.DefaultClient.Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()

	createRespBody, _ := io.ReadAll(createResp.Body)
	var createResult struct {
		Volume struct {
			ID string `json:"id"`
		} `json:"volume"`
	}
	json.Unmarshal(createRespBody, &createResult)
	volumeID := createResult.Volume.ID

	defer func() {
		deleteReq, _ := http.NewRequest("DELETE", client.ServiceURL("volumes", volumeID), nil)
		deleteReq.Header.Set("X-Auth-Token", client.TokenID)
		http.DefaultClient.Do(deleteReq)
	}()

	// Set a metadata key first
	metadataBody := `{"meta": {"delete-me": "value"}}`
	setURL := client.ServiceURL("volumes", volumeID, "metadata", "delete-me")
	setReq, _ := http.NewRequest("PUT", setURL, strings.NewReader(metadataBody))
	setReq.Header.Set("X-Auth-Token", client.TokenID)
	setReq.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(setReq)

	// Delete the metadata key
	url := client.ServiceURL("volumes", volumeID, "metadata", "delete-me")
	req, err := http.NewRequest("DELETE", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should return 204 No Content
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}
