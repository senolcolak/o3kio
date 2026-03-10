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

func TestNovaFlavorExtraSpecs_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Create a test flavor first
	createOpts := flavors.CreateOpts{
		Name:  "test-flavor-extraspecs",
		RAM:   512,
		VCPUs: 1,
		Disk:  gophercloud.IntToPointer(5),
	}

	flavor, err := flavors.Create(client, createOpts).Extract()
	require.NoError(t, err)
	defer flavors.Delete(client, flavor.ID)

	// Create extra specs via direct HTTP
	extraSpecs := map[string]interface{}{
		"extra_specs": map[string]string{
			"hw:cpu_policy":        "dedicated",
			"hw:numa_nodes":        "1",
			"quota:disk_read_iops": "1000",
		},
	}

	body, _ := json.Marshal(extraSpecs)
	url := client.ServiceURL("flavors", flavor.ID, "os-extra_specs")
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Get extra specs
	getURL := client.ServiceURL("flavors", flavor.ID, "os-extra_specs")
	getReq, err := http.NewRequest("GET", getURL, nil)
	require.NoError(t, err)

	getReq.Header.Set("X-Auth-Token", client.TokenID)

	getResp, err := http.DefaultClient.Do(getReq)
	require.NoError(t, err)
	defer getResp.Body.Close()

	assert.Equal(t, http.StatusOK, getResp.StatusCode)

	getBody, err := io.ReadAll(getResp.Body)
	require.NoError(t, err)

	var result struct {
		ExtraSpecs map[string]string `json:"extra_specs"`
	}

	err = json.Unmarshal(getBody, &result)
	require.NoError(t, err)
	assert.NotEmpty(t, result.ExtraSpecs)
	assert.Equal(t, "dedicated", result.ExtraSpecs["hw:cpu_policy"])
	assert.Equal(t, "1", result.ExtraSpecs["hw:numa_nodes"])
	assert.Equal(t, "1000", result.ExtraSpecs["quota:disk_read_iops"])
}
