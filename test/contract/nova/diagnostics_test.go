package nova_test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNovaServerDiagnostics_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Create a test server first
	createOpts := servers.CreateOpts{
		Name:      "test-server-diagnostics",
		FlavorRef: "00000000-0000-0000-0000-000000000010", // m1.tiny
		ImageRef:  "00000000-0000-0000-0000-000000000001",
	}

	server, err := servers.Create(client, createOpts).Extract()
	require.NoError(t, err)
	defer servers.Delete(client, server.ID)

	// Get server diagnostics via direct HTTP
	url := client.ServiceURL("servers", server.ID, "diagnostics")
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	require.NoError(t, err)

	// Should have basic diagnostic fields
	assert.Contains(t, result, "state")
	assert.Contains(t, result, "driver")
}
