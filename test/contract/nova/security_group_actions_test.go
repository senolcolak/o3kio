package nova_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNovaAddSecurityGroup_Contract tests POST /v2.1/servers/:id/action (addSecurityGroup)
func TestNovaAddSecurityGroup_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Create test server
	server, err := servers.Create(client, servers.CreateOpts{
		Name:      "sg-action-test",
		FlavorRef: "00000000-0000-0000-0000-000000000010",
		ImageRef:  "00000000-0000-0000-0000-000000000001",
	}).Extract()
	require.NoError(t, err)
	defer servers.Delete(client, server.ID)

	// Add security group
	actionBody := `{
		"addSecurityGroup": {
			"name": "default"
		}
	}`

	url := client.ServiceURL("servers", server.ID, "action")
	req, err := http.NewRequest("POST", url, strings.NewReader(actionBody))
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusAccepted, resp.StatusCode)
}

// TestNovaRemoveSecurityGroup_Contract tests POST /v2.1/servers/:id/action (removeSecurityGroup)
func TestNovaRemoveSecurityGroup_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Create test server
	server, err := servers.Create(client, servers.CreateOpts{
		Name:      "sg-remove-test",
		FlavorRef: "00000000-0000-0000-0000-000000000010",
		ImageRef:  "00000000-0000-0000-0000-000000000001",
	}).Extract()
	require.NoError(t, err)
	defer servers.Delete(client, server.ID)

	// Add security group first
	addBody := `{
		"addSecurityGroup": {
			"name": "default"
		}
	}`
	url := client.ServiceURL("servers", server.ID, "action")
	req, _ := http.NewRequest("POST", url, strings.NewReader(addBody))
	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(req)

	// Remove security group
	removeBody := `{
		"removeSecurityGroup": {
			"name": "default"
		}
	}`

	req, err = http.NewRequest("POST", url, strings.NewReader(removeBody))
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusAccepted, resp.StatusCode)
}

// TestNovaChangePassword_Contract tests POST /v2.1/servers/:id/action (changePassword)
func TestNovaChangePassword_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Create test server
	server, err := servers.Create(client, servers.CreateOpts{
		Name:      "password-test",
		FlavorRef: "00000000-0000-0000-0000-000000000010",
		ImageRef:  "00000000-0000-0000-0000-000000000001",
	}).Extract()
	require.NoError(t, err)
	defer servers.Delete(client, server.ID)

	// Wait for server to be ACTIVE (needed for password change)
	for i := 0; i < 30; i++ {
		s, _ := servers.Get(client, server.ID).Extract()
		if s.Status == "ACTIVE" {
			break
		}
		if i == 29 {
			t.Skip("Server did not become ACTIVE in time")
		}
		time.Sleep(time.Second)
	}

	// Change admin password
	actionBody := `{
		"changePassword": {
			"adminPass": "newSecurePassword123"
		}
	}`

	url := client.ServiceURL("servers", server.ID, "action")
	req, err := http.NewRequest("POST", url, strings.NewReader(actionBody))
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusAccepted, resp.StatusCode)

	// Verify response contains no error
	respBody, _ := io.ReadAll(resp.Body)
	if len(respBody) > 0 {
		var result map[string]interface{}
		json.Unmarshal(respBody, &result)
		assert.Nil(t, result["error"])
	}
}
