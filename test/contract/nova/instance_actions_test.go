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

// TestNovaListInstanceActions_Contract tests GET /v2.1/servers/:id/os-instance-actions
func TestNovaListInstanceActions_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Create a test server
	server, err := servers.Create(client, servers.CreateOpts{
		Name:      "test-server-actions",
		FlavorRef: "00000000-0000-0000-0000-000000000010", // m1.tiny
		ImageRef:  "00000000-0000-0000-0000-000000000001",
	}).Extract()
	require.NoError(t, err)
	defer servers.Delete(client, server.ID)

	// Perform an action (reboot) to create action history
	servers.Reboot(client, server.ID, servers.RebootOpts{Type: servers.SoftReboot})

	// Test: List instance actions
	url := client.ServiceURL("servers", server.ID, "os-instance-actions")
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		InstanceActions []struct {
			Action    string `json:"action"`
			RequestID string `json:"request_id"`
			UserID    string `json:"user_id"`
			ProjectID string `json:"project_id"`
			StartTime string `json:"start_time"`
		} `json:"instanceActions"`
	}
	err = json.Unmarshal(body, &result)
	require.NoError(t, err)

	// Should have at least the create action
	assert.NotEmpty(t, result.InstanceActions)

	// Verify action structure
	if len(result.InstanceActions) > 0 {
		action := result.InstanceActions[0]
		assert.NotEmpty(t, action.Action)
		assert.NotEmpty(t, action.StartTime)
	}
}

// TestNovaGetInstanceAction_Contract tests GET /v2.1/servers/:id/os-instance-actions/:request_id
func TestNovaGetInstanceAction_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Create a test server
	server, err := servers.Create(client, servers.CreateOpts{
		Name:      "test-server-action-detail",
		FlavorRef: "00000000-0000-0000-0000-000000000010",
		ImageRef:  "00000000-0000-0000-0000-000000000001",
	}).Extract()
	require.NoError(t, err)
	defer servers.Delete(client, server.ID)

	// List actions to get a request_id
	listURL := client.ServiceURL("servers", server.ID, "os-instance-actions")
	listReq, _ := http.NewRequest("GET", listURL, nil)
	listReq.Header.Set("X-Auth-Token", client.TokenID)
	listResp, _ := http.DefaultClient.Do(listReq)
	defer listResp.Body.Close()

	listBody, _ := io.ReadAll(listResp.Body)
	var listResult struct {
		InstanceActions []struct {
			RequestID string `json:"request_id"`
		} `json:"instanceActions"`
	}
	json.Unmarshal(listBody, &listResult)

	if len(listResult.InstanceActions) == 0 {
		t.Skip("No actions found to test detail endpoint")
	}

	requestID := listResult.InstanceActions[0].RequestID

	// Test: Get specific instance action
	url := client.ServiceURL("servers", server.ID, "os-instance-actions", requestID)
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		InstanceAction struct {
			Action    string `json:"action"`
			RequestID string `json:"request_id"`
			Message   string `json:"message"`
			StartTime string `json:"start_time"`
		} `json:"instanceAction"`
	}
	err = json.Unmarshal(body, &result)
	require.NoError(t, err)

	assert.Equal(t, requestID, result.InstanceAction.RequestID)
	assert.NotEmpty(t, result.InstanceAction.Action)
}
