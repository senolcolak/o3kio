package cinder_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gophercloud/gophercloud/openstack/blockstorage/v3/volumes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCinderListBackups_Contract tests GET /v3/:project_id/backups
func TestCinderListBackups_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupCinderClient(t)

	url := client.Endpoint + "backups"
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
	assert.Contains(t, result, "backups")
}

// TestCinderCreateBackup_Contract tests POST /v3/:project_id/backups
func TestCinderCreateBackup_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupCinderClient(t)

	// Create volume first using gophercloud
	volume, err := volumes.Create(client, volumes.CreateOpts{
		Size: 1,
		Name: "test-backup-volume",
	}).Extract()
	require.NoError(t, err)
	defer volumes.Delete(client, volume.ID, volumes.DeleteOpts{})

	// Test: Create backup
	url := client.Endpoint + "backups"
	body := strings.NewReader(`{"backup": {"volume_id": "` + volume.ID + `", "name": "test-backup"}}`)
	req, err := http.NewRequest("POST", url, body)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusAccepted {
		t.Logf("Unexpected status %d, body: %s", resp.StatusCode, string(respBody))
	}

	assert.Equal(t, http.StatusAccepted, resp.StatusCode)

	var result map[string]interface{}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)
	assert.Contains(t, result, "backup")

	// Clean up
	if backup, ok := result["backup"].(map[string]interface{}); ok {
		if backupID, ok := backup["id"].(string); ok {
			deleteURL := client.Endpoint + "backups/" + backupID
			delReq, _ := http.NewRequest("DELETE", deleteURL, nil)
			delReq.Header.Set("X-Auth-Token", client.TokenID)
			http.DefaultClient.Do(delReq)
		}
	}
}

// TestCinderGetBackup_Contract tests GET /v3/:project_id/backups/:id
func TestCinderGetBackup_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupCinderClient(t)

	// Create volume
	volume, err := volumes.Create(client, volumes.CreateOpts{
		Size: 1,
		Name: "test-get-backup-volume",
	}).Extract()
	require.NoError(t, err)
	defer volumes.Delete(client, volume.ID, volumes.DeleteOpts{})

	// Create backup
	createURL := client.Endpoint + "backups"
	createBody := strings.NewReader(`{"backup": {"volume_id": "` + volume.ID + `", "name": "test-get-backup"}}`)
	createReq, _ := http.NewRequest("POST", createURL, createBody)
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := http.DefaultClient.Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()

	var createResult map[string]interface{}
	json.NewDecoder(createResp.Body).Decode(&createResult)
	backup := createResult["backup"].(map[string]interface{})
	backupID := backup["id"].(string)

	// Test: Get backup
	url := client.Endpoint + "backups/" + backupID
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

// TestCinderDeleteBackup_Contract tests DELETE /v3/:project_id/backups/:id
func TestCinderDeleteBackup_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupCinderClient(t)

	// Create volume
	volume, err := volumes.Create(client, volumes.CreateOpts{
		Size: 1,
		Name: "test-delete-backup-volume",
	}).Extract()
	require.NoError(t, err)
	defer volumes.Delete(client, volume.ID, volumes.DeleteOpts{})

	// Create backup
	createURL := client.Endpoint + "backups"
	createBody := strings.NewReader(`{"backup": {"volume_id": "` + volume.ID + `", "name": "test-delete-backup"}}`)
	createReq, _ := http.NewRequest("POST", createURL, createBody)
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := http.DefaultClient.Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()

	var createResult map[string]interface{}
	json.NewDecoder(createResp.Body).Decode(&createResult)
	backup := createResult["backup"].(map[string]interface{})
	backupID := backup["id"].(string)

	// Test: Delete backup
	url := client.Endpoint + "backups/" + backupID
	req, err := http.NewRequest("DELETE", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusAccepted, resp.StatusCode)
}

// TestCinderRestoreBackup_Contract tests POST /v3/:project_id/backups/:id/restore
func TestCinderRestoreBackup_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupCinderClient(t)

	// Create volume
	volume, err := volumes.Create(client, volumes.CreateOpts{
		Size: 1,
		Name: "test-restore-backup-volume",
	}).Extract()
	require.NoError(t, err)
	defer volumes.Delete(client, volume.ID, volumes.DeleteOpts{})

	// Create backup
	createURL := client.Endpoint + "backups"
	createBody := strings.NewReader(`{"backup": {"volume_id": "` + volume.ID + `", "name": "test-restore-backup"}}`)
	createReq, _ := http.NewRequest("POST", createURL, createBody)
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := http.DefaultClient.Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()

	var createResult map[string]interface{}
	json.NewDecoder(createResp.Body).Decode(&createResult)
	backup := createResult["backup"].(map[string]interface{})
	backupID := backup["id"].(string)

	// Test: Restore backup
	url := client.Endpoint + "backups/" + backupID + "/restore"
	body := strings.NewReader(`{"restore": {}}`)
	req, err := http.NewRequest("POST", url, body)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusAccepted, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)
	assert.Contains(t, result, "restore")

	// Clean up
	delBackupURL := client.Endpoint + "backups/" + backupID
	delBackupReq, _ := http.NewRequest("DELETE", delBackupURL, nil)
	delBackupReq.Header.Set("X-Auth-Token", client.TokenID)
	http.DefaultClient.Do(delBackupReq)

	// Delete restored volume if different from original
	if restore, ok := result["restore"].(map[string]interface{}); ok {
		if restoredVolID, ok := restore["volume_id"].(string); ok && restoredVolID != volume.ID {
			volumes.Delete(client, restoredVolID, volumes.DeleteOpts{})
		}
	}
}

// TestCinderGetBackupDetail_Contract tests GET /v3/:project_id/backups/detail
func TestCinderGetBackupDetail_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupCinderClient(t)

	url := client.Endpoint + "backups/detail"
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
	assert.Contains(t, result, "backups")
}
