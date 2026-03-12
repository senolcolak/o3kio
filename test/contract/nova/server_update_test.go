package nova_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNovaUpdateServer_Contract tests PATCH /v2.1/servers/:id
func TestNovaUpdateServer_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Create test server
	createOpts := servers.CreateOpts{
		Name:      "test-server-update",
		FlavorRef: "00000000-0000-0000-0000-000000000010",
		ImageRef:  "00000000-0000-0000-0000-000000000001",
	}
	server, err := servers.Create(client, createOpts).Extract()
	require.NoError(t, err)
	defer servers.Delete(client, server.ID)

	// Update server name
	payload := map[string]interface{}{
		"server": map[string]interface{}{
			"name": "updated-server-name",
		},
	}

	body, _ := json.Marshal(payload)
	url := client.ServiceURL("servers", server.ID)
	req, err := http.NewRequest("PATCH", url, bytes.NewReader(body))
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		Server map[string]interface{} `json:"server"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.Equal(t, "updated-server-name", result.Server["name"])
	assert.Equal(t, server.ID, result.Server["id"])

	// Verify with GET
	updatedServer, err := servers.Get(client, server.ID).Extract()
	require.NoError(t, err)
	assert.Equal(t, "updated-server-name", updatedServer.Name)
}

// TestNovaUpdateNonExistentServer_Contract tests updating non-existent server
func TestNovaUpdateNonExistentServer_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Try to update non-existent server
	payload := map[string]interface{}{
		"server": map[string]interface{}{
			"name": "should-fail",
		},
	}

	body, _ := json.Marshal(payload)
	url := client.ServiceURL("servers", "00000000-0000-0000-0000-999999999999")
	req, err := http.NewRequest("PATCH", url, bytes.NewReader(body))
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}
