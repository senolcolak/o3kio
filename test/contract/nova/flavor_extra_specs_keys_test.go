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

// TestNovaGetFlavorExtraSpecKey_Contract tests GET /v2.1/flavors/:id/os-extra_specs/:key
func TestNovaGetFlavorExtraSpecKey_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Create test flavor
	flavor, err := flavors.Create(client, flavors.CreateOpts{
		Name:  "test-flavor-get-key",
		RAM:   512,
		VCPUs: 1,
		Disk:  gophercloud.IntToPointer(5),
	}).Extract()
	require.NoError(t, err)
	defer flavors.Delete(client, flavor.ID)

	// Create extra specs
	extraSpecs := map[string]interface{}{
		"extra_specs": map[string]string{
			"hw:cpu_policy": "dedicated",
			"hw:numa_nodes": "2",
		},
	}
	body, _ := json.Marshal(extraSpecs)
	createURL := client.ServiceURL("flavors", flavor.ID, "os-extra_specs")
	createReq, _ := http.NewRequest("POST", createURL, bytes.NewReader(body))
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(createReq)

	// Test: Get specific key
	url := client.ServiceURL("flavors", flavor.ID, "os-extra_specs", "hw:cpu_policy")
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
	assert.Equal(t, "dedicated", result["hw:cpu_policy"])
}

// TestNovaUpdateFlavorExtraSpecKey_Contract tests PUT /v2.1/flavors/:id/os-extra_specs/:key
func TestNovaUpdateFlavorExtraSpecKey_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Create test flavor
	flavor, err := flavors.Create(client, flavors.CreateOpts{
		Name:  "test-flavor-update-key",
		RAM:   512,
		VCPUs: 1,
		Disk:  gophercloud.IntToPointer(5),
	}).Extract()
	require.NoError(t, err)
	defer flavors.Delete(client, flavor.ID)

	// Create extra specs
	extraSpecs := map[string]interface{}{
		"extra_specs": map[string]string{
			"hw:cpu_policy": "shared",
		},
	}
	body, _ := json.Marshal(extraSpecs)
	createURL := client.ServiceURL("flavors", flavor.ID, "os-extra_specs")
	createReq, _ := http.NewRequest("POST", createURL, bytes.NewReader(body))
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(createReq)

	// Test: Update specific key
	updateData := map[string]string{
		"hw:cpu_policy": "dedicated",
	}
	updateBody, _ := json.Marshal(updateData)
	url := client.ServiceURL("flavors", flavor.ID, "os-extra_specs", "hw:cpu_policy")
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
	assert.Equal(t, "dedicated", result["hw:cpu_policy"])

	// Verify update
	getURL := client.ServiceURL("flavors", flavor.ID, "os-extra_specs")
	getReq, _ := http.NewRequest("GET", getURL, nil)
	getReq.Header.Set("X-Auth-Token", client.TokenID)
	getResp, err := http.DefaultClient.Do(getReq)
	if err != nil {
		t.Fatal(err)
	}
	defer getResp.Body.Close()

	getBody, _ := io.ReadAll(getResp.Body)
	var getResult struct {
		ExtraSpecs map[string]string `json:"extra_specs"`
	}
	json.Unmarshal(getBody, &getResult)
	assert.Equal(t, "dedicated", getResult.ExtraSpecs["hw:cpu_policy"])
}

// TestNovaDeleteFlavorExtraSpecKey_Contract tests DELETE /v2.1/flavors/:id/os-extra_specs/:key
func TestNovaDeleteFlavorExtraSpecKey_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Create test flavor
	flavor, err := flavors.Create(client, flavors.CreateOpts{
		Name:  "test-flavor-delete-key",
		RAM:   512,
		VCPUs: 1,
		Disk:  gophercloud.IntToPointer(5),
	}).Extract()
	require.NoError(t, err)
	defer flavors.Delete(client, flavor.ID)

	// Create extra specs
	extraSpecs := map[string]interface{}{
		"extra_specs": map[string]string{
			"hw:cpu_policy": "dedicated",
			"hw:numa_nodes": "1",
		},
	}
	body, _ := json.Marshal(extraSpecs)
	createURL := client.ServiceURL("flavors", flavor.ID, "os-extra_specs")
	createReq, _ := http.NewRequest("POST", createURL, bytes.NewReader(body))
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(createReq)

	// Test: Delete specific key
	url := client.ServiceURL("flavors", flavor.ID, "os-extra_specs", "hw:cpu_policy")
	req, err := http.NewRequest("DELETE", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	// Verify deletion
	getURL := client.ServiceURL("flavors", flavor.ID, "os-extra_specs")
	getReq, _ := http.NewRequest("GET", getURL, nil)
	getReq.Header.Set("X-Auth-Token", client.TokenID)
	getResp, err := http.DefaultClient.Do(getReq)
	if err != nil {
		t.Fatal(err)
	}
	defer getResp.Body.Close()

	getBody, _ := io.ReadAll(getResp.Body)
	var getResult struct {
		ExtraSpecs map[string]string `json:"extra_specs"`
	}
	json.Unmarshal(getBody, &getResult)
	assert.NotContains(t, getResult.ExtraSpecs, "hw:cpu_policy")
	assert.Contains(t, getResult.ExtraSpecs, "hw:numa_nodes") // Other key remains
}
