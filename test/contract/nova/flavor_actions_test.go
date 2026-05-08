package nova_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/flavors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNovaFlavorAddTenantAccess_Contract tests POST /v2.1/flavors/:id/action (addTenantAccess)
func TestNovaFlavorAddTenantAccess_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Create private flavor
	isPublicFalse := false
	flavor, err := flavors.Create(client, flavors.CreateOpts{
		Name:     "test-flavor-private",
		RAM:      512,
		VCPUs:    1,
		Disk:     gophercloud.IntToPointer(5),
		IsPublic: &isPublicFalse,
	}).Extract()
	require.NoError(t, err)
	defer flavors.Delete(client, flavor.ID)

	// Get current project ID (using default project from seed data)
	projectID := "550e8400-e29b-41d4-a716-446655440000" // default project

	// Test: Add tenant access
	action := map[string]interface{}{
		"addTenantAccess": map[string]string{
			"tenant": projectID,
		},
	}
	body, _ := json.Marshal(action)
	url := client.ServiceURL("flavors", flavor.ID, "action")
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify access was added
	accessURL := client.ServiceURL("flavors", flavor.ID, "os-flavor-access")
	accessReq, _ := http.NewRequest("GET", accessURL, nil)
	accessReq.Header.Set("X-Auth-Token", client.TokenID)
	accessResp, err := http.DefaultClient.Do(accessReq)
	if err != nil {
		t.Fatal(err)
	}
	defer accessResp.Body.Close()

	accessBody, _ := io.ReadAll(accessResp.Body)
	var accessResult struct {
		FlavorAccess []struct {
			FlavorID  string `json:"flavor_id"`
			TenantID  string `json:"tenant_id"`
		} `json:"flavor_access"`
	}
	json.Unmarshal(accessBody, &accessResult)
	assert.NotEmpty(t, accessResult.FlavorAccess)
	assert.Equal(t, projectID, accessResult.FlavorAccess[0].TenantID)
}

// TestNovaFlavorRemoveTenantAccess_Contract tests POST /v2.1/flavors/:id/action (removeTenantAccess)
func TestNovaFlavorRemoveTenantAccess_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Create private flavor
	isPublicFalse := false
	flavor, err := flavors.Create(client, flavors.CreateOpts{
		Name:     "test-flavor-remove-access",
		RAM:      512,
		VCPUs:    1,
		Disk:     gophercloud.IntToPointer(5),
		IsPublic: &isPublicFalse,
	}).Extract()
	require.NoError(t, err)
	defer flavors.Delete(client, flavor.ID)

	// Get current project ID (using default project from seed data)
	projectID := "550e8400-e29b-41d4-a716-446655440000" // default project

	// Add tenant access first
	addAction := map[string]interface{}{
		"addTenantAccess": map[string]string{
			"tenant": projectID,
		},
	}
	addBody, _ := json.Marshal(addAction)
	addURL := client.ServiceURL("flavors", flavor.ID, "action")
	addReq, _ := http.NewRequest("POST", addURL, bytes.NewReader(addBody))
	addReq.Header.Set("X-Auth-Token", client.TokenID)
	addReq.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(addReq)

	// Test: Remove tenant access
	removeAction := map[string]interface{}{
		"removeTenantAccess": map[string]string{
			"tenant": projectID,
		},
	}
	removeBody, _ := json.Marshal(removeAction)
	removeURL := client.ServiceURL("flavors", flavor.ID, "action")
	removeReq, err := http.NewRequest("POST", removeURL, bytes.NewReader(removeBody))
	require.NoError(t, err)

	removeReq.Header.Set("X-Auth-Token", client.TokenID)
	removeReq.Header.Set("Content-Type", "application/json")

	removeResp, err := http.DefaultClient.Do(removeReq)
	require.NoError(t, err)
	defer removeResp.Body.Close()

	assert.Equal(t, http.StatusOK, removeResp.StatusCode)

	// Verify access was removed
	accessURL := client.ServiceURL("flavors", flavor.ID, "os-flavor-access")
	accessReq, _ := http.NewRequest("GET", accessURL, nil)
	accessReq.Header.Set("X-Auth-Token", client.TokenID)
	accessResp, err := http.DefaultClient.Do(accessReq)
	if err != nil {
		t.Fatal(err)
	}
	defer accessResp.Body.Close()

	accessBody, _ := io.ReadAll(accessResp.Body)
	var accessResult struct {
		FlavorAccess []struct {
			FlavorID string `json:"flavor_id"`
			TenantID string `json:"tenant_id"`
		} `json:"flavor_access"`
	}
	json.Unmarshal(accessBody, &accessResult)
	assert.Empty(t, accessResult.FlavorAccess)
}

// TestNovaGetFlavorAccess_Contract tests GET /v2.1/flavors/:id/os-flavor-access
func TestNovaGetFlavorAccess_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Create private flavor
	isPublicFalse := false
	flavor, err := flavors.Create(client, flavors.CreateOpts{
		Name:     "test-flavor-get-access",
		RAM:      512,
		VCPUs:    1,
		Disk:     gophercloud.IntToPointer(5),
		IsPublic: &isPublicFalse,
	}).Extract()
	require.NoError(t, err)
	defer flavors.Delete(client, flavor.ID)

	// Test: Get flavor access (should be empty initially)
	url := client.ServiceURL("flavors", flavor.ID, "os-flavor-access")
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		FlavorAccess []struct {
			FlavorID string `json:"flavor_id"`
			TenantID string `json:"tenant_id"`
		} `json:"flavor_access"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)
	assert.NotNil(t, result.FlavorAccess) // Should be empty array, not null
}
