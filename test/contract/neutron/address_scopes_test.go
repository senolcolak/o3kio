package neutron_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/gophercloud/gophercloud"

	"github.com/stretchr/testify/require"
)

// TestNeutronListAddressScopes_Contract tests GET /v2.0/address-scopes
func TestNeutronListAddressScopes_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNeutronClient(t)

	url := client.ServiceURL("address-scopes")
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		AddressScopes []map[string]interface{} `json:"address_scopes"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.NotNil(t, result.AddressScopes)
}

// TestNeutronCreateAddressScope_Contract tests POST /v2.0/address-scopes
func TestNeutronCreateAddressScope_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNeutronClient(t)

	payload := map[string]interface{}{
		"address_scope": map[string]interface{}{
			"name":       "test-address-scope",
			"ip_version": 4,
		},
	}

	body, _ := json.Marshal(payload)
	url := client.ServiceURL("address-scopes")
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
		AddressScope map[string]interface{} `json:"address_scope"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.NotEmpty(t, result.AddressScope["id"])
	assert.Equal(t, "test-address-scope", result.AddressScope["name"])
	assert.Equal(t, float64(4), result.AddressScope["ip_version"])

	// Cleanup
	scopeID := result.AddressScope["id"].(string)
	cleanupTestAddressScope(t, client, scopeID)
}

// TestNeutronGetAddressScope_Contract tests GET /v2.0/address-scopes/:id
func TestNeutronGetAddressScope_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNeutronClient(t)

	// Create test address scope
	scopeID := createTestAddressScope(t, client)

	url := client.ServiceURL("address-scopes", scopeID)
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		AddressScope map[string]interface{} `json:"address_scope"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.Equal(t, scopeID, result.AddressScope["id"])

	// Cleanup
	cleanupTestAddressScope(t, client, scopeID)
}

// TestNeutronUpdateAddressScope_Contract tests PUT /v2.0/address-scopes/:id
func TestNeutronUpdateAddressScope_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNeutronClient(t)

	// Create test address scope
	scopeID := createTestAddressScope(t, client)

	payload := map[string]interface{}{
		"address_scope": map[string]interface{}{
			"name": "updated-address-scope",
		},
	}

	body, _ := json.Marshal(payload)
	url := client.ServiceURL("address-scopes", scopeID)
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
		AddressScope map[string]interface{} `json:"address_scope"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.Equal(t, "updated-address-scope", result.AddressScope["name"])

	// Cleanup
	cleanupTestAddressScope(t, client, scopeID)
}

// TestNeutronDeleteAddressScope_Contract tests DELETE /v2.0/address-scopes/:id
func TestNeutronDeleteAddressScope_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNeutronClient(t)

	// Create test address scope
	scopeID := createTestAddressScope(t, client)

	url := client.ServiceURL("address-scopes", scopeID)
	req, err := http.NewRequest("DELETE", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}

// Helper to create test address scope
func createTestAddressScope(t *testing.T, client *gophercloud.ServiceClient) string {
	t.Helper()

	payload := map[string]interface{}{
		"address_scope": map[string]interface{}{
			"name":       "test-scope",
			"ip_version": 4,
		},
	}

	body, _ := json.Marshal(payload)
	url := client.ServiceURL("address-scopes")
	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to create address scope: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		AddressScope map[string]interface{} `json:"address_scope"`
	}
	json.Unmarshal(respBody, &result)

	return result.AddressScope["id"].(string)
}

// Helper to cleanup test address scope
func cleanupTestAddressScope(t *testing.T, client *gophercloud.ServiceClient, scopeID string) {
	t.Helper()

	url := client.ServiceURL("address-scopes", scopeID)
	req, _ := http.NewRequest("DELETE", url, nil)
	req.Header.Set("X-Auth-Token", client.TokenID)
	http.DefaultClient.Do(req)
}
