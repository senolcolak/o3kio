package cinder_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/gophercloud/gophercloud/openstack/blockstorage/v3/volumetypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCinderCreateVolumeTypeExtraSpecs_Contract tests POST /v3/:project_id/types/:id/extra_specs
func TestCinderCreateVolumeTypeExtraSpecs_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupCinderClient(t)

	// Create volume type first
	vt, err := volumetypes.Create(client, volumetypes.CreateOpts{
		Name: "test-extra-specs-type",
	}).Extract()
	require.NoError(t, err)
	defer volumetypes.Delete(client, vt.ID)

	// Test: Create extra specs
	extraSpecs := map[string]interface{}{
		"extra_specs": map[string]string{
			"volume_backend_name": "rbd",
			"replication_enabled": "true",
		},
	}

	body, _ := json.Marshal(extraSpecs)
	url := client.Endpoint + "types/" + vt.ID + "/extra_specs"
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
		ExtraSpecs map[string]string `json:"extra_specs"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.Equal(t, "rbd", result.ExtraSpecs["volume_backend_name"])
	assert.Equal(t, "true", result.ExtraSpecs["replication_enabled"])
}

// TestCinderGetVolumeTypeExtraSpecKey_Contract tests GET /v3/:project_id/types/:id/extra_specs/:key
func TestCinderGetVolumeTypeExtraSpecKey_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupCinderClient(t)

	// Create volume type with extra specs
	vt, err := volumetypes.Create(client, volumetypes.CreateOpts{
		Name: "test-get-key-type",
	}).Extract()
	require.NoError(t, err)
	defer volumetypes.Delete(client, vt.ID)

	// Create extra specs first
	extraSpecs := map[string]interface{}{
		"extra_specs": map[string]string{
			"volume_backend_name": "local",
		},
	}
	body, _ := json.Marshal(extraSpecs)
	createReq, _ := http.NewRequest("POST", client.Endpoint+"types/"+vt.ID+"/extra_specs", bytes.NewReader(body))
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(createReq)

	// Test: Get single key
	url := client.Endpoint + "types/" + vt.ID + "/extra_specs/volume_backend_name"
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]string
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.Equal(t, "local", result["volume_backend_name"])
}

// TestCinderUpdateVolumeTypeExtraSpecKey_Contract tests PUT /v3/:project_id/types/:id/extra_specs/:key
func TestCinderUpdateVolumeTypeExtraSpecKey_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupCinderClient(t)

	// Create volume type with extra specs
	vt, err := volumetypes.Create(client, volumetypes.CreateOpts{
		Name: "test-update-key-type",
	}).Extract()
	require.NoError(t, err)
	defer volumetypes.Delete(client, vt.ID)

	// Create initial extra spec
	extraSpecs := map[string]interface{}{
		"extra_specs": map[string]string{
			"volume_backend_name": "local",
		},
	}
	body, _ := json.Marshal(extraSpecs)
	createReq, _ := http.NewRequest("POST", client.Endpoint+"types/"+vt.ID+"/extra_specs", bytes.NewReader(body))
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(createReq)

	// Test: Update single key
	update := map[string]string{
		"volume_backend_name": "rbd",
	}
	updateBody, _ := json.Marshal(update)
	url := client.Endpoint + "types/" + vt.ID + "/extra_specs/volume_backend_name"
	req, err := http.NewRequest("PUT", url, bytes.NewReader(updateBody))
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]string
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.Equal(t, "rbd", result["volume_backend_name"])
}

// TestCinderDeleteVolumeTypeExtraSpecKey_Contract tests DELETE /v3/:project_id/types/:id/extra_specs/:key
func TestCinderDeleteVolumeTypeExtraSpecKey_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupCinderClient(t)

	// Create volume type with extra specs
	vt, err := volumetypes.Create(client, volumetypes.CreateOpts{
		Name: "test-delete-key-type",
	}).Extract()
	require.NoError(t, err)
	defer volumetypes.Delete(client, vt.ID)

	// Create extra specs
	extraSpecs := map[string]interface{}{
		"extra_specs": map[string]string{
			"volume_backend_name": "local",
		},
	}
	body, _ := json.Marshal(extraSpecs)
	createReq, _ := http.NewRequest("POST", client.Endpoint+"types/"+vt.ID+"/extra_specs", bytes.NewReader(body))
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(createReq)

	// Test: Delete single key
	url := client.Endpoint + "types/" + vt.ID + "/extra_specs/volume_backend_name"
	req, err := http.NewRequest("DELETE", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}
