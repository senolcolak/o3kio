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

// TestNovaListServerTags_Contract tests GET /v2.1/servers/:id/tags
func TestNovaListServerTags_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Create test server
	createOpts := servers.CreateOpts{
		Name:      "test-tags-list",
		FlavorRef: "00000000-0000-0000-0000-000000000010",
		ImageRef:  "00000000-0000-0000-0000-000000000001",
	}
	server, err := servers.Create(client, createOpts).Extract()
	require.NoError(t, err)
	defer servers.Delete(client, server.ID)

	url := client.ServiceURL("servers", server.ID, "tags")
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		Tags []string `json:"tags"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.NotNil(t, result.Tags)
}

// TestNovaReplaceServerTags_Contract tests PUT /v2.1/servers/:id/tags
func TestNovaReplaceServerTags_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Create test server
	createOpts := servers.CreateOpts{
		Name:      "test-tags-replace",
		FlavorRef: "00000000-0000-0000-0000-000000000010",
		ImageRef:  "00000000-0000-0000-0000-000000000001",
	}
	server, err := servers.Create(client, createOpts).Extract()
	require.NoError(t, err)
	defer servers.Delete(client, server.ID)

	// Replace tags
	payload := map[string]interface{}{
		"tags": []string{"tag1", "tag2", "tag3"},
	}

	body, _ := json.Marshal(payload)
	url := client.ServiceURL("servers", server.ID, "tags")
	req, err := http.NewRequest("PUT", url, bytes.NewReader(body))
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		Tags []string `json:"tags"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.ElementsMatch(t, []string{"tag1", "tag2", "tag3"}, result.Tags)
}

// TestNovaAddServerTag_Contract tests PUT /v2.1/servers/:id/tags/:tag
func TestNovaAddServerTag_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Create test server
	createOpts := servers.CreateOpts{
		Name:      "test-tags-add",
		FlavorRef: "00000000-0000-0000-0000-000000000010",
		ImageRef:  "00000000-0000-0000-0000-000000000001",
	}
	server, err := servers.Create(client, createOpts).Extract()
	require.NoError(t, err)
	defer servers.Delete(client, server.ID)

	// Add tag
	url := client.ServiceURL("servers", server.ID, "tags", "new-tag")
	req, err := http.NewRequest("PUT", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	// Verify tag was added
	listReq, _ := http.NewRequest("GET", client.ServiceURL("servers", server.ID, "tags"), nil)
	listReq.Header.Set("X-Auth-Token", client.TokenID)
	listResp, err := http.DefaultClient.Do(listReq)
	if err != nil {
		t.Fatal(err)
	}
	defer listResp.Body.Close()

	listRespBody, _ := io.ReadAll(listResp.Body)
	var listResult struct {
		Tags []string `json:"tags"`
	}
	json.Unmarshal(listRespBody, &listResult)

	assert.Contains(t, listResult.Tags, "new-tag")
}

// TestNovaDeleteServerTag_Contract tests DELETE /v2.1/servers/:id/tags/:tag
func TestNovaDeleteServerTag_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Create test server
	createOpts := servers.CreateOpts{
		Name:      "test-tags-delete",
		FlavorRef: "00000000-0000-0000-0000-000000000010",
		ImageRef:  "00000000-0000-0000-0000-000000000001",
	}
	server, err := servers.Create(client, createOpts).Extract()
	require.NoError(t, err)
	defer servers.Delete(client, server.ID)

	// Add a tag first
	addReq, _ := http.NewRequest("PUT", client.ServiceURL("servers", server.ID, "tags", "to-delete"), nil)
	addReq.Header.Set("X-Auth-Token", client.TokenID)
	http.DefaultClient.Do(addReq)

	// Delete tag
	url := client.ServiceURL("servers", server.ID, "tags", "to-delete")
	req, err := http.NewRequest("DELETE", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}

// TestNovaDeleteAllServerTags_Contract tests DELETE /v2.1/servers/:id/tags
func TestNovaDeleteAllServerTags_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Create test server
	createOpts := servers.CreateOpts{
		Name:      "test-tags-delete-all",
		FlavorRef: "00000000-0000-0000-0000-000000000010",
		ImageRef:  "00000000-0000-0000-0000-000000000001",
	}
	server, err := servers.Create(client, createOpts).Extract()
	require.NoError(t, err)
	defer servers.Delete(client, server.ID)

	// Add some tags
	replacePayload := map[string]interface{}{
		"tags": []string{"tag1", "tag2"},
	}
	replaceBody, _ := json.Marshal(replacePayload)
	replaceReq, _ := http.NewRequest("PUT", client.ServiceURL("servers", server.ID, "tags"), bytes.NewReader(replaceBody))
	replaceReq.Header.Set("X-Auth-Token", client.TokenID)
	replaceReq.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(replaceReq)

	// Delete all tags
	url := client.ServiceURL("servers", server.ID, "tags")
	req, err := http.NewRequest("DELETE", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	// Verify all tags removed
	listReq, _ := http.NewRequest("GET", client.ServiceURL("servers", server.ID, "tags"), nil)
	listReq.Header.Set("X-Auth-Token", client.TokenID)
	listResp, err := http.DefaultClient.Do(listReq)
	if err != nil {
		t.Fatal(err)
	}
	defer listResp.Body.Close()

	listRespBody, _ := io.ReadAll(listResp.Body)
	var listResult struct {
		Tags []string `json:"tags"`
	}
	json.Unmarshal(listRespBody, &listResult)

	assert.Empty(t, listResult.Tags)
}
