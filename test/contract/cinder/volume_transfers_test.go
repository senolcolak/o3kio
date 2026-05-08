package cinder_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCinderCreateVolumeTransfer_Contract tests POST /v3/:project_id/volume-transfers
func TestCinderCreateVolumeTransfer_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupCinderClient(t)

	// Create a test volume using raw HTTP
	createVolBody := map[string]interface{}{
		"volume": map[string]interface{}{
			"size": 1,
			"name": "test-transfer-volume",
		},
	}
	createVolBodyJSON, _ := json.Marshal(createVolBody)
	createVolReq, _ := http.NewRequest("POST", client.Endpoint+"volumes", bytes.NewReader(createVolBodyJSON))
	createVolReq.Header.Set("X-Auth-Token", client.TokenID)
	createVolReq.Header.Set("Content-Type", "application/json")

	createVolResp, err := http.DefaultClient.Do(createVolReq)
	if err != nil {
		t.Fatal(err)
	}
	defer createVolResp.Body.Close()

	createVolRespBody, _ := io.ReadAll(createVolResp.Body)
	var createVolResult struct {
		Volume struct {
			ID string `json:"id"`
		} `json:"volume"`
	}
	json.Unmarshal(createVolRespBody, &createVolResult)
	volumeID := createVolResult.Volume.ID

	// Wait for volume status to become available (goroutine takes 100ms + DB update time)
	time.Sleep(200 * time.Millisecond)

	defer func() {
		deleteReq, _ := http.NewRequest("DELETE", client.Endpoint+"volumes/"+volumeID, nil)
		deleteReq.Header.Set("X-Auth-Token", client.TokenID)
		http.DefaultClient.Do(deleteReq)
	}()

	// Test: Create volume transfer
	transfer := map[string]interface{}{
		"transfer": map[string]interface{}{
			"volume_id": volumeID,
			"name":      "test-transfer",
		},
	}

	body, _ := json.Marshal(transfer)
	url := client.Endpoint + "os-volume-transfer"
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusAccepted {
		t.Logf("Response status: %d, body: %s", resp.StatusCode, string(respBody))
	}

	assert.Equal(t, http.StatusAccepted, resp.StatusCode)
	var result struct {
		Transfer map[string]interface{} `json:"transfer"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.NotEmpty(t, result.Transfer["id"])
	assert.NotEmpty(t, result.Transfer["auth_key"])
	assert.Equal(t, volumeID, result.Transfer["volume_id"])

	// Cleanup
	if transferID, ok := result.Transfer["id"].(string); ok {
		delURL := client.Endpoint + "os-volume-transfer/" + transferID
		delReq, _ := http.NewRequest("DELETE", delURL, nil)
		delReq.Header.Set("X-Auth-Token", client.TokenID)
		http.DefaultClient.Do(delReq)
	}
}

// TestCinderListVolumeTransfers_Contract tests GET /v3/:project_id/volume-transfers
func TestCinderListVolumeTransfers_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupCinderClient(t)

	// Create volume using raw HTTP
	createVolBody := map[string]interface{}{
		"volume": map[string]interface{}{
			"size": 1,
			"name": "test-list-transfer-volume",
		},
	}
	createVolBodyJSON, _ := json.Marshal(createVolBody)
	createVolReq, _ := http.NewRequest("POST", client.Endpoint+"volumes", bytes.NewReader(createVolBodyJSON))
	createVolReq.Header.Set("X-Auth-Token", client.TokenID)
	createVolReq.Header.Set("Content-Type", "application/json")

	createVolResp, err := http.DefaultClient.Do(createVolReq)
	if err != nil {
		t.Fatal(err)
	}
	defer createVolResp.Body.Close()

	createVolRespBody, _ := io.ReadAll(createVolResp.Body)
	var createVolResult struct {
		Volume struct {
			ID string `json:"id"`
		} `json:"volume"`
	}
	json.Unmarshal(createVolRespBody, &createVolResult)
	volumeID := createVolResult.Volume.ID

	// Wait for volume status to become available (goroutine takes 100ms + DB update time)
	time.Sleep(200 * time.Millisecond)

	defer func() {
		deleteReq, _ := http.NewRequest("DELETE", client.Endpoint+"volumes/"+volumeID, nil)
		deleteReq.Header.Set("X-Auth-Token", client.TokenID)
		http.DefaultClient.Do(deleteReq)
	}()

	transfer := map[string]interface{}{
		"transfer": map[string]interface{}{
			"volume_id": volumeID,
			"name":      "test-list-transfer",
		},
	}
	transferBody, _ := json.Marshal(transfer)
	createURL := client.Endpoint + "os-volume-transfer"
	createReq, _ := http.NewRequest("POST", createURL, bytes.NewReader(transferBody))
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(createReq)

	// Test: List volume transfers
	url := client.Endpoint + "os-volume-transfer"
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		Transfers []map[string]interface{} `json:"transfers"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.NotEmpty(t, result.Transfers)
}

// TestCinderGetVolumeTransfer_Contract tests GET /v3/:project_id/volume-transfers/:id
func TestCinderGetVolumeTransfer_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupCinderClient(t)

	// Create volume using raw HTTP
	createVolBody := map[string]interface{}{
		"volume": map[string]interface{}{
			"size": 1,
			"name": "test-get-transfer-volume",
		},
	}
	createVolBodyJSON, _ := json.Marshal(createVolBody)
	createVolReq, _ := http.NewRequest("POST", client.Endpoint+"volumes", bytes.NewReader(createVolBodyJSON))
	createVolReq.Header.Set("X-Auth-Token", client.TokenID)
	createVolReq.Header.Set("Content-Type", "application/json")

	createVolResp, err := http.DefaultClient.Do(createVolReq)
	if err != nil {
		t.Fatal(err)
	}
	defer createVolResp.Body.Close()

	createVolRespBody, _ := io.ReadAll(createVolResp.Body)
	var createVolResult struct {
		Volume struct {
			ID string `json:"id"`
		} `json:"volume"`
	}
	json.Unmarshal(createVolRespBody, &createVolResult)
	volumeID := createVolResult.Volume.ID

	// Wait for volume status to become available (goroutine takes 100ms + DB update time)
	time.Sleep(200 * time.Millisecond)

	defer func() {
		deleteReq, _ := http.NewRequest("DELETE", client.Endpoint+"volumes/"+volumeID, nil)
		deleteReq.Header.Set("X-Auth-Token", client.TokenID)
		http.DefaultClient.Do(deleteReq)
	}()

	transfer := map[string]interface{}{
		"transfer": map[string]interface{}{
			"volume_id": volumeID,
			"name":      "test-get-transfer",
		},
	}
	transferBody, _ := json.Marshal(transfer)
	createURL := client.Endpoint + "os-volume-transfer"
	createReq, _ := http.NewRequest("POST", createURL, bytes.NewReader(transferBody))
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatal(err)
	}
	defer createResp.Body.Close()

	createBody, _ := io.ReadAll(createResp.Body)
	var createResult struct {
		Transfer map[string]interface{} `json:"transfer"`
	}
	json.Unmarshal(createBody, &createResult)
	transferID := createResult.Transfer["id"].(string)

	// Test: Get volume transfer
	url := client.Endpoint + "os-volume-transfer/" + transferID
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		Transfer map[string]interface{} `json:"transfer"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.Equal(t, transferID, result.Transfer["id"])
	assert.Equal(t, volumeID, result.Transfer["volume_id"])

	// Cleanup
	delReq, _ := http.NewRequest("DELETE", url, nil)
	delReq.Header.Set("X-Auth-Token", client.TokenID)
	http.DefaultClient.Do(delReq)
}

// TestCinderDeleteVolumeTransfer_Contract tests DELETE /v3/:project_id/volume-transfers/:id
func TestCinderDeleteVolumeTransfer_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupCinderClient(t)

	// Create volume using raw HTTP
	createVolBody := map[string]interface{}{
		"volume": map[string]interface{}{
			"size": 1,
			"name": "test-delete-transfer-volume",
		},
	}
	createVolBodyJSON, _ := json.Marshal(createVolBody)
	createVolReq, _ := http.NewRequest("POST", client.Endpoint+"volumes", bytes.NewReader(createVolBodyJSON))
	createVolReq.Header.Set("X-Auth-Token", client.TokenID)
	createVolReq.Header.Set("Content-Type", "application/json")

	createVolResp, err := http.DefaultClient.Do(createVolReq)
	if err != nil {
		t.Fatal(err)
	}
	defer createVolResp.Body.Close()

	createVolRespBody, _ := io.ReadAll(createVolResp.Body)
	var createVolResult struct {
		Volume struct {
			ID string `json:"id"`
		} `json:"volume"`
	}
	json.Unmarshal(createVolRespBody, &createVolResult)
	volumeID := createVolResult.Volume.ID

	// Wait for volume status to become available (goroutine takes 100ms + DB update time)
	time.Sleep(200 * time.Millisecond)

	defer func() {
		deleteReq, _ := http.NewRequest("DELETE", client.Endpoint+"volumes/"+volumeID, nil)
		deleteReq.Header.Set("X-Auth-Token", client.TokenID)
		http.DefaultClient.Do(deleteReq)
	}()

	transfer := map[string]interface{}{
		"transfer": map[string]interface{}{
			"volume_id": volumeID,
			"name":      "test-delete-transfer",
		},
	}
	transferBody, _ := json.Marshal(transfer)
	createURL := client.Endpoint + "os-volume-transfer"
	createReq, _ := http.NewRequest("POST", createURL, bytes.NewReader(transferBody))
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatal(err)
	}
	defer createResp.Body.Close()

	createBody, _ := io.ReadAll(createResp.Body)
	var createResult struct {
		Transfer map[string]interface{} `json:"transfer"`
	}
	json.Unmarshal(createBody, &createResult)
	transferID := createResult.Transfer["id"].(string)

	// Test: Delete volume transfer
	url := client.Endpoint + "os-volume-transfer/" + transferID
	req, err := http.NewRequest("DELETE", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusAccepted, resp.StatusCode)
}

// TestCinderAcceptVolumeTransfer_Contract tests POST /v3/:project_id/volume-transfers/:id/accept
func TestCinderAcceptVolumeTransfer_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupCinderClient(t)

	// Create volume using raw HTTP
	createVolBody := map[string]interface{}{
		"volume": map[string]interface{}{
			"size": 1,
			"name": "test-accept-transfer-volume",
		},
	}
	createVolBodyJSON, _ := json.Marshal(createVolBody)
	createVolReq, _ := http.NewRequest("POST", client.Endpoint+"volumes", bytes.NewReader(createVolBodyJSON))
	createVolReq.Header.Set("X-Auth-Token", client.TokenID)
	createVolReq.Header.Set("Content-Type", "application/json")

	createVolResp, err := http.DefaultClient.Do(createVolReq)
	if err != nil {
		t.Fatal(err)
	}
	defer createVolResp.Body.Close()

	createVolRespBody, _ := io.ReadAll(createVolResp.Body)
	var createVolResult struct {
		Volume struct {
			ID string `json:"id"`
		} `json:"volume"`
	}
	json.Unmarshal(createVolRespBody, &createVolResult)
	volumeID := createVolResult.Volume.ID

	// Wait for volume status to become available (goroutine takes 100ms + DB update time)
	time.Sleep(200 * time.Millisecond)

	defer func() {
		deleteReq, _ := http.NewRequest("DELETE", client.Endpoint+"volumes/"+volumeID, nil)
		deleteReq.Header.Set("X-Auth-Token", client.TokenID)
		http.DefaultClient.Do(deleteReq)
	}()

	transfer := map[string]interface{}{
		"transfer": map[string]interface{}{
			"volume_id": volumeID,
			"name":      "test-accept-transfer",
		},
	}
	transferBody, _ := json.Marshal(transfer)
	createURL := client.Endpoint + "os-volume-transfer"
	createReq, _ := http.NewRequest("POST", createURL, bytes.NewReader(transferBody))
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatal(err)
	}
	defer createResp.Body.Close()

	createBody, _ := io.ReadAll(createResp.Body)
	var createResult struct {
		Transfer map[string]interface{} `json:"transfer"`
	}
	json.Unmarshal(createBody, &createResult)
	transferID := createResult.Transfer["id"].(string)
	authKey := createResult.Transfer["auth_key"].(string)

	// Test: Accept volume transfer
	accept := map[string]interface{}{
		"accept": map[string]interface{}{
			"auth_key": authKey,
		},
	}
	acceptBody, _ := json.Marshal(accept)
	url := client.Endpoint + "os-volume-transfer/" + transferID + "/accept"
	req, err := http.NewRequest("POST", url, bytes.NewReader(acceptBody))
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusAccepted {
		t.Logf("Response status: %d, body: %s", resp.StatusCode, string(respBody))
	}

	assert.Equal(t, http.StatusAccepted, resp.StatusCode)
	var result struct {
		Transfer map[string]interface{} `json:"transfer"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.Equal(t, transferID, result.Transfer["id"])
	assert.Equal(t, volumeID, result.Transfer["volume_id"])
}
