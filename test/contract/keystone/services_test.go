package keystone_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/gophercloud/gophercloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestKeystoneListServices_Contract tests GET /v3/services
func TestKeystoneListServices_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupKeystoneClient(t)

	url := client.ServiceURL("services")
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		Services []map[string]interface{} `json:"services"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.NotNil(t, result.Services)
	// Should have at least keystone, nova, neutron, cinder, glance
	assert.GreaterOrEqual(t, len(result.Services), 5)
}

// TestKeystoneCreateService_Contract tests POST /v3/services
func TestKeystoneCreateService_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupKeystoneClient(t)

	payload := map[string]interface{}{
		"service": map[string]interface{}{
			"type":        "test-service",
			"name":        "test",
			"description": "Test Service",
			"enabled":     true,
		},
	}

	body, _ := json.Marshal(payload)
	url := client.ServiceURL("services")
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		Service map[string]interface{} `json:"service"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.NotEmpty(t, result.Service["id"])
	assert.Equal(t, "test-service", result.Service["type"])

	// Cleanup
	serviceID := result.Service["id"].(string)
	cleanupTestService(t, client, serviceID)
}

// TestKeystoneListEndpoints_Contract tests GET /v3/endpoints
func TestKeystoneListEndpoints_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupKeystoneClient(t)

	url := client.ServiceURL("endpoints")
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		Endpoints []map[string]interface{} `json:"endpoints"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.NotNil(t, result.Endpoints)
	// Should have multiple endpoints (public, internal for each service)
	assert.GreaterOrEqual(t, len(result.Endpoints), 5)
}

// Helper to cleanup test service
func cleanupTestService(t *testing.T, client *gophercloud.ServiceClient, serviceID string) {
	t.Helper()

	url := client.ServiceURL("services", serviceID)
	req, _ := http.NewRequest("DELETE", url, nil)
	req.Header.Set("X-Auth-Token", client.TokenID)
	http.DefaultClient.Do(req)
}
