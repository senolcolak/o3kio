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

// TestKeystoneGetService_Contract tests GET /v3/services/:id
func TestKeystoneGetService_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)
	client := setupKeystoneClient(t)

	// Use existing seeded service (Keystone)
	serviceID := "00000000-0000-0000-0000-000000000010"

	url := client.ServiceURL("services", serviceID)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result struct {
		Service struct {
			ID   string `json:"id"`
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"service"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, serviceID, result.Service.ID)
	assert.Equal(t, "identity", result.Service.Type)
}

// TestKeystoneUpdateService_Contract tests PATCH /v3/services/:id
func TestKeystoneUpdateService_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)
	client := setupKeystoneClient(t)

	// Create test service
	createPayload := map[string]interface{}{
		"service": map[string]interface{}{
			"type": "test-update-type",
			"name": "test-update",
		},
	}
	createBody, _ := json.Marshal(createPayload)
	createURL := client.ServiceURL("services")
	createReq, _ := http.NewRequest("POST", createURL, bytes.NewReader(createBody))
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := http.DefaultClient.Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()

	var createResult struct {
		Service struct {
			ID string `json:"id"`
		} `json:"service"`
	}
	json.NewDecoder(createResp.Body).Decode(&createResult)
	serviceID := createResult.Service.ID
	defer ensureServiceDeleted(client, serviceID)

	// Update service
	updatePayload := map[string]interface{}{
		"service": map[string]interface{}{
			"description": "Updated description",
			"enabled":     false,
		},
	}
	updateBody, _ := json.Marshal(updatePayload)
	updateURL := client.ServiceURL("services", serviceID)
	updateReq, _ := http.NewRequest("PATCH", updateURL, bytes.NewReader(updateBody))
	updateReq.Header.Set("X-Auth-Token", client.TokenID)
	updateReq.Header.Set("Content-Type", "application/json")

	updateResp, err := http.DefaultClient.Do(updateReq)
	require.NoError(t, err)
	defer updateResp.Body.Close()

	assert.Equal(t, http.StatusOK, updateResp.StatusCode)

	var updateResult struct {
		Service struct {
			Description string `json:"description"`
			Enabled     bool   `json:"enabled"`
		} `json:"service"`
	}
	json.NewDecoder(updateResp.Body).Decode(&updateResult)
	assert.Equal(t, "Updated description", updateResult.Service.Description)
	assert.False(t, updateResult.Service.Enabled)
}

// TestKeystoneDeleteService_Contract tests DELETE /v3/services/:id
func TestKeystoneDeleteService_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)
	client := setupKeystoneClient(t)

	// Create test service
	createPayload := map[string]interface{}{
		"service": map[string]interface{}{
			"type": "test-delete-type",
			"name": "test-delete",
		},
	}
	createBody, _ := json.Marshal(createPayload)
	createURL := client.ServiceURL("services")
	createReq, _ := http.NewRequest("POST", createURL, bytes.NewReader(createBody))
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := http.DefaultClient.Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()

	var createResult struct {
		Service struct {
			ID string `json:"id"`
		} `json:"service"`
	}
	json.NewDecoder(createResp.Body).Decode(&createResult)
	serviceID := createResult.Service.ID

	// Delete service
	deleteURL := client.ServiceURL("services", serviceID)
	deleteReq, _ := http.NewRequest("DELETE", deleteURL, nil)
	deleteReq.Header.Set("X-Auth-Token", client.TokenID)

	deleteResp, err := http.DefaultClient.Do(deleteReq)
	require.NoError(t, err)
	defer deleteResp.Body.Close()

	assert.Equal(t, http.StatusNoContent, deleteResp.StatusCode)

	// Verify deletion
	getReq, _ := http.NewRequest("GET", deleteURL, nil)
	getReq.Header.Set("X-Auth-Token", client.TokenID)
	getResp, _ := http.DefaultClient.Do(getReq)
	assert.Equal(t, http.StatusNotFound, getResp.StatusCode)
}

// TestKeystoneCreateEndpoint_Contract tests POST /v3/endpoints
func TestKeystoneCreateEndpoint_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)
	client := setupKeystoneClient(t)

	// Use existing service (Keystone)
	serviceID := "00000000-0000-0000-0000-000000000010"

	createPayload := map[string]interface{}{
		"endpoint": map[string]interface{}{
			"service_id": serviceID,
			"interface":  "public",
			"url":        "http://test.example.com:35357/v3",
			"region":     "TestRegion",
		},
	}
	createBody, _ := json.Marshal(createPayload)

	url := client.ServiceURL("endpoints")
	req, _ := http.NewRequest("POST", url, bytes.NewReader(createBody))
	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var result struct {
		Endpoint struct {
			ID        string `json:"id"`
			ServiceID string `json:"service_id"`
			Interface string `json:"interface"`
		} `json:"endpoint"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.NotEmpty(t, result.Endpoint.ID)
	assert.Equal(t, serviceID, result.Endpoint.ServiceID)
	defer ensureEndpointDeleted(client, result.Endpoint.ID)
}

// TestKeystoneDeleteEndpoint_Contract tests DELETE /v3/endpoints/:id
func TestKeystoneDeleteEndpoint_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)
	client := setupKeystoneClient(t)

	// Create endpoint first
	serviceID := "00000000-0000-0000-0000-000000000010"
	createPayload := map[string]interface{}{
		"endpoint": map[string]interface{}{
			"service_id": serviceID,
			"interface":  "public",
			"url":        "http://test-delete.example.com:35357/v3",
		},
	}
	createBody, _ := json.Marshal(createPayload)

	createURL := client.ServiceURL("endpoints")
	createReq, _ := http.NewRequest("POST", createURL, bytes.NewReader(createBody))
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := http.DefaultClient.Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()

	var createResult struct {
		Endpoint struct {
			ID string `json:"id"`
		} `json:"endpoint"`
	}
	json.NewDecoder(createResp.Body).Decode(&createResult)
	endpointID := createResult.Endpoint.ID

	// Delete endpoint
	deleteURL := client.ServiceURL("endpoints", endpointID)
	deleteReq, _ := http.NewRequest("DELETE", deleteURL, nil)
	deleteReq.Header.Set("X-Auth-Token", client.TokenID)

	deleteResp, err := http.DefaultClient.Do(deleteReq)
	require.NoError(t, err)
	defer deleteResp.Body.Close()

	assert.Equal(t, http.StatusNoContent, deleteResp.StatusCode)
}

// Helper to cleanup test service
func cleanupTestService(t *testing.T, client *gophercloud.ServiceClient, serviceID string) {
	t.Helper()

	url := client.ServiceURL("services", serviceID)
	req, _ := http.NewRequest("DELETE", url, nil)
	req.Header.Set("X-Auth-Token", client.TokenID)
	http.DefaultClient.Do(req)
}

// ensureServiceDeleted cleans up test service
func ensureServiceDeleted(client *gophercloud.ServiceClient, serviceID string) {
	url := client.ServiceURL("services", serviceID)
	req, _ := http.NewRequest("DELETE", url, nil)
	req.Header.Set("X-Auth-Token", client.TokenID)
	http.DefaultClient.Do(req)
}

// ensureEndpointDeleted cleans up test endpoint
func ensureEndpointDeleted(client *gophercloud.ServiceClient, endpointID string) {
	url := client.ServiceURL("endpoints", endpointID)
	req, _ := http.NewRequest("DELETE", url, nil)
	req.Header.Set("X-Auth-Token", client.TokenID)
	http.DefaultClient.Do(req)
}
