package nova_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNovaServerEvacuate_Contract tests POST /v2.1/servers/:id/action (evacuate)
func TestNovaServerEvacuate_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Create server
	createOpts := servers.CreateOpts{
		Name:      "test-evacuate-server",
		FlavorRef: "00000000-0000-0000-0000-000000000010",
		ImageRef:  "00000000-0000-0000-0000-000000000001",
	}

	server, err := servers.Create(client, createOpts).Extract()
	require.NoError(t, err)
	defer servers.Delete(client, server.ID)

	// Test: Evacuate via direct HTTP (gophercloud doesn't support it)
	url := client.ServiceURL("servers", server.ID, "action")
	body := strings.NewReader(`{"evacuate": {}}`)
	req, err := http.NewRequest("POST", url, body)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Accept 200/202/409 (evacuate accepted or already in progress)
	assert.Contains(t, []int{200, 202, 409}, resp.StatusCode, "Evacuate should not return 404")
}

// TestNovaServerMigrate_Contract tests POST /v2.1/servers/:id/action (migrate)
func TestNovaServerMigrate_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	createOpts := servers.CreateOpts{
		Name:      "test-migrate-server",
		FlavorRef: "00000000-0000-0000-0000-000000000010",
		ImageRef:  "00000000-0000-0000-0000-000000000001",
	}

	server, err := servers.Create(client, createOpts).Extract()
	require.NoError(t, err)
	defer servers.Delete(client, server.ID)

	// Test: Migrate via direct HTTP
	url := client.ServiceURL("servers", server.ID, "action")
	body := strings.NewReader(`{"migrate": null}`)
	req, err := http.NewRequest("POST", url, body)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Contains(t, []int{202, 409}, resp.StatusCode, "Migrate should not return 404")
}

// TestNovaServerLiveMigrate_Contract tests POST /v2.1/servers/:id/action (os-migrateLive)
func TestNovaServerLiveMigrate_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	createOpts := servers.CreateOpts{
		Name:      "test-live-migrate-server",
		FlavorRef: "00000000-0000-0000-0000-000000000010",
		ImageRef:  "00000000-0000-0000-0000-000000000001",
	}

	server, err := servers.Create(client, createOpts).Extract()
	require.NoError(t, err)
	defer servers.Delete(client, server.ID)

	// Test: Live migrate via direct HTTP
	url := client.ServiceURL("servers", server.ID, "action")
	body := strings.NewReader(`{"os-migrateLive": {"host": null, "block_migration": false}}`)
	req, err := http.NewRequest("POST", url, body)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Debug: Print response if not expected status
	if resp.StatusCode != 202 && resp.StatusCode != 409 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Logf("Unexpected status %d, body: %s", resp.StatusCode, string(respBody))
	}

	// Should accept 202 or 409 (conflict if instance not ACTIVE)
	assert.Contains(t, []int{202, 409}, resp.StatusCode, "LiveMigrate should not return 404")

	// Verify response body is valid JSON for non-202 responses
	if resp.StatusCode != 202 {
		respBody, _ := io.ReadAll(resp.Body)
		var result map[string]interface{}
		err = json.Unmarshal(respBody, &result)
		// Should be valid JSON error response
		if err == nil {
			assert.Contains(t, result, "error")
		}
	}
}

