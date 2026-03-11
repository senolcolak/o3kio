package nova_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNovaListAggregates_Contract tests GET /v2.1/os-aggregates
func TestNovaListAggregates_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	url := client.ServiceURL("os-aggregates")
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)
	assert.Contains(t, result, "aggregates")
}

// TestNovaCreateAggregate_Contract tests POST /v2.1/os-aggregates
func TestNovaCreateAggregate_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	url := client.ServiceURL("os-aggregates")
	body := strings.NewReader(`{"aggregate": {"name": "test-aggregate", "availability_zone": "nova"}}`)
	req, err := http.NewRequest("POST", url, body)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)
	assert.Contains(t, result, "aggregate")

	// Clean up
	if agg, ok := result["aggregate"].(map[string]interface{}); ok {
		if aggID, ok := agg["id"].(string); ok {
			deleteURL := client.ServiceURL("os-aggregates", aggID)
			delReq, _ := http.NewRequest("DELETE", deleteURL, nil)
			delReq.Header.Set("X-Auth-Token", client.TokenID)
			http.DefaultClient.Do(delReq)
		}
	}
}

// TestNovaGetAggregate_Contract tests GET /v2.1/os-aggregates/:id
func TestNovaGetAggregate_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Create aggregate
	createURL := client.ServiceURL("os-aggregates")
	createBody := strings.NewReader(`{"aggregate": {"name": "test-get-aggregate", "availability_zone": "nova"}}`)
	createReq, _ := http.NewRequest("POST", createURL, createBody)
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := http.DefaultClient.Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()

	var createResult map[string]interface{}
	json.NewDecoder(createResp.Body).Decode(&createResult)
	agg := createResult["aggregate"].(map[string]interface{})
	aggID := agg["id"].(string)

	// Test: Get aggregate
	url := client.ServiceURL("os-aggregates", aggID)
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Clean up
	delReq, _ := http.NewRequest("DELETE", url, nil)
	delReq.Header.Set("X-Auth-Token", client.TokenID)
	http.DefaultClient.Do(delReq)
}

// TestNovaUpdateAggregate_Contract tests PUT /v2.1/os-aggregates/:id
func TestNovaUpdateAggregate_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Create aggregate
	createURL := client.ServiceURL("os-aggregates")
	createBody := strings.NewReader(`{"aggregate": {"name": "test-update-aggregate", "availability_zone": "nova"}}`)
	createReq, _ := http.NewRequest("POST", createURL, createBody)
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := http.DefaultClient.Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()

	var createResult map[string]interface{}
	json.NewDecoder(createResp.Body).Decode(&createResult)
	agg := createResult["aggregate"].(map[string]interface{})
	aggID := agg["id"].(string)

	// Test: Update aggregate
	url := client.ServiceURL("os-aggregates", aggID)
	body := strings.NewReader(`{"aggregate": {"name": "updated-aggregate"}}`)
	req, err := http.NewRequest("PUT", url, body)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Clean up
	delReq, _ := http.NewRequest("DELETE", url, nil)
	delReq.Header.Set("X-Auth-Token", client.TokenID)
	http.DefaultClient.Do(delReq)
}

// TestNovaDeleteAggregate_Contract tests DELETE /v2.1/os-aggregates/:id
func TestNovaDeleteAggregate_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Create aggregate
	createURL := client.ServiceURL("os-aggregates")
	createBody := strings.NewReader(`{"aggregate": {"name": "test-delete-aggregate", "availability_zone": "nova"}}`)
	createReq, _ := http.NewRequest("POST", createURL, createBody)
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := http.DefaultClient.Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()

	var createResult map[string]interface{}
	json.NewDecoder(createResp.Body).Decode(&createResult)
	agg := createResult["aggregate"].(map[string]interface{})
	aggID := agg["id"].(string)

	// Test: Delete aggregate
	url := client.ServiceURL("os-aggregates", aggID)
	req, err := http.NewRequest("DELETE", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestNovaAddHostToAggregate_Contract tests POST /v2.1/os-aggregates/:id/action (add_host)
func TestNovaAddHostToAggregate_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Create aggregate
	createURL := client.ServiceURL("os-aggregates")
	createBody := strings.NewReader(`{"aggregate": {"name": "test-addhost-aggregate", "availability_zone": "nova"}}`)
	createReq, _ := http.NewRequest("POST", createURL, createBody)
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := http.DefaultClient.Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()

	var createResult map[string]interface{}
	json.NewDecoder(createResp.Body).Decode(&createResult)
	agg := createResult["aggregate"].(map[string]interface{})
	aggID := agg["id"].(string)

	// Test: Add host to aggregate
	url := client.ServiceURL("os-aggregates", aggID, "action")
	body := strings.NewReader(`{"add_host": {"host": "compute-01"}}`)
	req, err := http.NewRequest("POST", url, body)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Clean up
	delURL := client.ServiceURL("os-aggregates", aggID)
	delReq, _ := http.NewRequest("DELETE", delURL, nil)
	delReq.Header.Set("X-Auth-Token", client.TokenID)
	http.DefaultClient.Do(delReq)
}

// TestNovaRemoveHostFromAggregate_Contract tests POST /v2.1/os-aggregates/:id/action (remove_host)
func TestNovaRemoveHostFromAggregate_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Create aggregate and add host
	createURL := client.ServiceURL("os-aggregates")
	createBody := strings.NewReader(`{"aggregate": {"name": "test-removehost-aggregate", "availability_zone": "nova"}}`)
	createReq, _ := http.NewRequest("POST", createURL, createBody)
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := http.DefaultClient.Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()

	var createResult map[string]interface{}
	json.NewDecoder(createResp.Body).Decode(&createResult)
	agg := createResult["aggregate"].(map[string]interface{})
	aggID := agg["id"].(string)

	// Add host first
	addURL := client.ServiceURL("os-aggregates", aggID, "action")
	addBody := strings.NewReader(`{"add_host": {"host": "compute-01"}}`)
	addReq, _ := http.NewRequest("POST", addURL, addBody)
	addReq.Header.Set("X-Auth-Token", client.TokenID)
	addReq.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(addReq)

	// Test: Remove host from aggregate
	url := client.ServiceURL("os-aggregates", aggID, "action")
	body := strings.NewReader(`{"remove_host": {"host": "compute-01"}}`)
	req, err := http.NewRequest("POST", url, body)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Clean up
	delURL := client.ServiceURL("os-aggregates", aggID)
	delReq, _ := http.NewRequest("DELETE", delURL, nil)
	delReq.Header.Set("X-Auth-Token", client.TokenID)
	http.DefaultClient.Do(delReq)
}

// TestNovaSetAggregateMetadata_Contract tests POST /v2.1/os-aggregates/:id/action (set_metadata)
func TestNovaSetAggregateMetadata_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Create aggregate
	createURL := client.ServiceURL("os-aggregates")
	createBody := strings.NewReader(`{"aggregate": {"name": "test-metadata-aggregate", "availability_zone": "nova"}}`)
	createReq, _ := http.NewRequest("POST", createURL, createBody)
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := http.DefaultClient.Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()

	var createResult map[string]interface{}
	json.NewDecoder(createResp.Body).Decode(&createResult)
	agg := createResult["aggregate"].(map[string]interface{})
	aggID := agg["id"].(string)

	// Test: Set aggregate metadata
	url := client.ServiceURL("os-aggregates", aggID, "action")
	body := strings.NewReader(`{"set_metadata": {"metadata": {"key1": "value1", "key2": "value2"}}}`)
	req, err := http.NewRequest("POST", url, body)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Clean up
	delURL := client.ServiceURL("os-aggregates", aggID)
	delReq, _ := http.NewRequest("DELETE", delURL, nil)
	delReq.Header.Set("X-Auth-Token", client.TokenID)
	http.DefaultClient.Do(delReq)
}
