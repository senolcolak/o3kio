package nova_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNovaServerCreate_Contract tests basic server creation
// This is a HIGH priority test validating core Nova functionality
func TestNovaServerCreate_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Create server with minimal required fields (use UUIDs like other tests)
	createOpts := servers.CreateOpts{
		Name:      "test-server-basic",
		FlavorRef: "00000000-0000-0000-0000-000000000010", // m1.small
		ImageRef:  "00000000-0000-0000-0000-000000000001", // cirros
	}

	server, err := servers.Create(client, createOpts).Extract()
	require.NoError(t, err, "Failed to create server")
	require.NotNil(t, server)
	assert.NotEmpty(t, server.ID)
	assert.Equal(t, "test-server-basic", server.Name)
	assert.NotEmpty(t, server.Status)

	// Cleanup
	defer func() {
		err := servers.Delete(client, server.ID).ExtractErr()
		if err != nil {
			t.Logf("Cleanup failed: %v", err)
		}
	}()

	// Verify server exists
	fetchedServer, err := servers.Get(client, server.ID).Extract()
	require.NoError(t, err, "Failed to fetch created server")
	assert.Equal(t, server.ID, fetchedServer.ID)
	assert.Equal(t, server.Name, fetchedServer.Name)
}

// TestNovaServerList_Contract tests listing servers
func TestNovaServerList_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// List all servers (servers.List already includes details by default)
	allPages, err := servers.List(client, servers.ListOpts{}).AllPages()
	require.NoError(t, err, "Failed to list servers")

	allServers, err := servers.ExtractServers(allPages)
	require.NoError(t, err, "Failed to extract servers from pages")
	// Empty list is valid - just verify no error occurred
	assert.GreaterOrEqual(t, len(allServers), 0, "Server list length should be >= 0")
}

// TestNovaServerGet_Contract tests fetching a specific server
func TestNovaServerGet_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Create a test server
	createOpts := servers.CreateOpts{
		Name:      "test-server-get",
		FlavorRef: "00000000-0000-0000-0000-000000000010",
		ImageRef:  "00000000-0000-0000-0000-000000000001",
	}

	server, err := servers.Create(client, createOpts).Extract()
	require.NoError(t, err, "Failed to create test server")

	defer func() {
		_ = servers.Delete(client, server.ID).ExtractErr()
	}()

	// Get server by ID
	fetchedServer, err := servers.Get(client, server.ID).Extract()
	require.NoError(t, err, "Failed to get server by ID")
	assert.Equal(t, server.ID, fetchedServer.ID)
	assert.Equal(t, server.Name, fetchedServer.Name)
	assert.NotEmpty(t, fetchedServer.Status)
	assert.NotEmpty(t, fetchedServer.Created)
	assert.NotEmpty(t, fetchedServer.Updated)
}

// TestNovaServerDelete_Contract tests server deletion
func TestNovaServerDelete_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Create a test server
	createOpts := servers.CreateOpts{
		Name:      "test-server-delete",
		FlavorRef: "00000000-0000-0000-0000-000000000010",
		ImageRef:  "00000000-0000-0000-0000-000000000001",
	}

	server, err := servers.Create(client, createOpts).Extract()
	require.NoError(t, err, "Failed to create test server")

	// Delete server
	err = servers.Delete(client, server.ID).ExtractErr()
	require.NoError(t, err, "Failed to delete server")

	// Verify server is deleted (should return 404)
	_, err = servers.Get(client, server.ID).Extract()
	assert.Error(t, err, "Expected error when fetching deleted server")

	// Gophercloud returns ErrDefault404 for 404 responses
	if err != nil {
		_, ok := err.(gophercloud.ErrDefault404)
		assert.True(t, ok, "Expected 404 error type")
	}
}

// TestNovaServerReboot_Contract tests server reboot action
func TestNovaServerReboot_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Create a test server
	createOpts := servers.CreateOpts{
		Name:      "test-server-reboot",
		FlavorRef: "00000000-0000-0000-0000-000000000010",
		ImageRef:  "00000000-0000-0000-0000-000000000001",
	}

	server, err := servers.Create(client, createOpts).Extract()
	require.NoError(t, err, "Failed to create test server")

	defer func() {
		_ = servers.Delete(client, server.ID).ExtractErr()
	}()

	// Test soft reboot using raw HTTP (gophercloud doesn't have Reboot method)
	rebootBody := `{"reboot": {"type": "SOFT"}}`
	url := client.ServiceURL("servers", server.ID, "action")
	req, err := http.NewRequest("POST", url, strings.NewReader(rebootBody))
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusAccepted, resp.StatusCode, "Soft reboot should return 202")

	// Verify server still exists after reboot
	fetchedServer, err := servers.Get(client, server.ID).Extract()
	require.NoError(t, err, "Failed to fetch server after reboot")
	assert.Equal(t, server.ID, fetchedServer.ID)

	// Test hard reboot
	hardRebootBody := `{"reboot": {"type": "HARD"}}`
	req2, err := http.NewRequest("POST", url, strings.NewReader(hardRebootBody))
	require.NoError(t, err)
	req2.Header.Set("X-Auth-Token", client.TokenID)
	req2.Header.Set("Content-Type", "application/json")

	resp2, err := http.DefaultClient.Do(req2)
	require.NoError(t, err)
	defer resp2.Body.Close()
	assert.Equal(t, http.StatusAccepted, resp2.StatusCode, "Hard reboot should return 202")
}

// TestNovaServerUpdate_Contract tests server update operation
func TestNovaServerUpdate_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Create a test server
	createOpts := servers.CreateOpts{
		Name:      "test-server-original",
		FlavorRef: "00000000-0000-0000-0000-000000000010",
		ImageRef:  "00000000-0000-0000-0000-000000000001",
	}

	server, err := servers.Create(client, createOpts).Extract()
	require.NoError(t, err, "Failed to create test server")

	defer func() {
		_ = servers.Delete(client, server.ID).ExtractErr()
	}()

	// Update server name
	newName := "test-server-updated"
	updateOpts := servers.UpdateOpts{
		Name: newName, // Use string directly, not pointer
	}

	updatedServer, err := servers.Update(client, server.ID, updateOpts).Extract()
	require.NoError(t, err, "Failed to update server")
	assert.Equal(t, newName, updatedServer.Name)

	// Verify update persisted
	fetchedServer, err := servers.Get(client, server.ID).Extract()
	require.NoError(t, err, "Failed to fetch updated server")
	assert.Equal(t, newName, fetchedServer.Name)
}

// TestNovaServerLifecycle_Contract tests complete server lifecycle
func TestNovaServerLifecycle_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// 1. Create
	createOpts := servers.CreateOpts{
		Name:      "test-server-lifecycle",
		FlavorRef: "00000000-0000-0000-0000-000000000010",
		ImageRef:  "00000000-0000-0000-0000-000000000001",
	}

	server, err := servers.Create(client, createOpts).Extract()
	require.NoError(t, err, "Failed to create server")
	defer func() {
		_ = servers.Delete(client, server.ID).ExtractErr()
	}()

	// 2. Stop using raw HTTP
	stopBody := `{"os-stop": null}`
	url := client.ServiceURL("servers", server.ID, "action")
	req, err := http.NewRequest("POST", url, strings.NewReader(stopBody))
	require.NoError(t, err)
	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()

	// 3. Start using raw HTTP
	startBody := `{"os-start": null}`
	req2, err := http.NewRequest("POST", url, strings.NewReader(startBody))
	require.NoError(t, err)
	req2.Header.Set("X-Auth-Token", client.TokenID)
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := http.DefaultClient.Do(req2)
	require.NoError(t, err)
	resp2.Body.Close()

	// 4. Reboot
	rebootBody := `{"reboot": {"type": "SOFT"}}`
	req3, err := http.NewRequest("POST", url, strings.NewReader(rebootBody))
	require.NoError(t, err)
	req3.Header.Set("X-Auth-Token", client.TokenID)
	req3.Header.Set("Content-Type", "application/json")
	resp3, err := http.DefaultClient.Do(req3)
	require.NoError(t, err)
	resp3.Body.Close()

	// 5. Verify final state
	finalServer, err := servers.Get(client, server.ID).Extract()
	require.NoError(t, err, "Failed to fetch server after lifecycle operations")
	assert.Equal(t, server.ID, finalServer.ID)

	// 6. Delete
	err = servers.Delete(client, server.ID).ExtractErr()
	require.NoError(t, err, "Failed to delete server")
}
