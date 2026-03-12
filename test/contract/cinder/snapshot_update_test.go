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

// TestCinderUpdateSnapshot_Contract tests PUT /v3/:project/snapshots/:id
func TestCinderUpdateSnapshot_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupCinderClient(t)

	// Create test volume first
	createVolBody := `{"volume": {"size": 1, "name": "snapshot-update-vol"}}`
	createVolURL := client.ServiceURL("volumes")
	createVolReq, _ := http.NewRequest("POST", createVolURL, strings.NewReader(createVolBody))
	createVolReq.Header.Set("X-Auth-Token", client.TokenID)
	createVolReq.Header.Set("Content-Type", "application/json")

	createVolResp, err := http.DefaultClient.Do(createVolReq)
	require.NoError(t, err)
	defer createVolResp.Body.Close()

	createVolRespBody, _ := io.ReadAll(createVolResp.Body)
	var createVolResult struct {
		Volume struct {
			ID string `json:"id"`
		} `json:"volume"`
	}
	json.Unmarshal(createVolRespBody, &createVolResult)
	volumeID := createVolResult.Volume.ID

	defer func() {
		deleteReq, _ := http.NewRequest("DELETE", client.ServiceURL("volumes", volumeID), nil)
		deleteReq.Header.Set("X-Auth-Token", client.TokenID)
		http.DefaultClient.Do(deleteReq)
	}()

	// Create snapshot
	createSnapBody := `{"snapshot": {"volume_id": "` + volumeID + `", "name": "original-name", "description": "original description"}}`
	createSnapURL := client.ServiceURL("snapshots")
	createSnapReq, _ := http.NewRequest("POST", createSnapURL, strings.NewReader(createSnapBody))
	createSnapReq.Header.Set("X-Auth-Token", client.TokenID)
	createSnapReq.Header.Set("Content-Type", "application/json")

	createSnapResp, err := http.DefaultClient.Do(createSnapReq)
	require.NoError(t, err)
	defer createSnapResp.Body.Close()

	createSnapRespBody, _ := io.ReadAll(createSnapResp.Body)
	var createSnapResult struct {
		Snapshot struct {
			ID string `json:"id"`
		} `json:"snapshot"`
	}
	json.Unmarshal(createSnapRespBody, &createSnapResult)
	snapshotID := createSnapResult.Snapshot.ID

	defer func() {
		deleteReq, _ := http.NewRequest("DELETE", client.ServiceURL("snapshots", snapshotID), nil)
		deleteReq.Header.Set("X-Auth-Token", client.TokenID)
		http.DefaultClient.Do(deleteReq)
	}()

	// Update snapshot name and description
	updateBody := `{"snapshot": {"name": "updated-name", "description": "updated description"}}`
	url := client.ServiceURL("snapshots", snapshotID)
	req, err := http.NewRequest("PUT", url, strings.NewReader(updateBody))
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should return 200 OK
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		Snapshot struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"snapshot"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	// Should have updated fields
	assert.Equal(t, snapshotID, result.Snapshot.ID)
	assert.Equal(t, "updated-name", result.Snapshot.Name)
	assert.Equal(t, "updated description", result.Snapshot.Description)
}
