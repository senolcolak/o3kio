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

// TestKeystoneListApplicationCredentials_Contract tests GET /v3/users/:user_id/application_credentials
func TestKeystoneListApplicationCredentials_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupKeystoneClient(t)

	userID := "00000000-0000-0000-0000-000000000001"
	url := client.ServiceURL("users", userID, "application_credentials")
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		ApplicationCredentials []map[string]interface{} `json:"application_credentials"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.NotNil(t, result.ApplicationCredentials)
}

// TestKeystoneCreateApplicationCredential_Contract tests POST /v3/users/:user_id/application_credentials
func TestKeystoneCreateApplicationCredential_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupKeystoneClient(t)

	userID := "00000000-0000-0000-0000-000000000001"
	payload := map[string]interface{}{
		"application_credential": map[string]interface{}{
			"name":        "test-app-cred",
			"description": "Test application credential",
			"roles": []map[string]interface{}{
				{"id": "00000000-0000-0000-0000-000000000003"},
			},
		},
	}

	body, _ := json.Marshal(payload)
	url := client.ServiceURL("users", userID, "application_credentials")
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
		ApplicationCredential map[string]interface{} `json:"application_credential"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.NotEmpty(t, result.ApplicationCredential["id"])
	assert.NotEmpty(t, result.ApplicationCredential["secret"])
	assert.Equal(t, "test-app-cred", result.ApplicationCredential["name"])

	// Cleanup
	credID := result.ApplicationCredential["id"].(string)
	cleanupTestApplicationCredential(t, client, userID, credID)
}

// TestKeystoneGetApplicationCredential_Contract tests GET /v3/users/:user_id/application_credentials/:id
func TestKeystoneGetApplicationCredential_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupKeystoneClient(t)

	// Create test credential
	userID := "00000000-0000-0000-0000-000000000001"
	credID := createTestApplicationCredential(t, client, userID)

	url := client.ServiceURL("users", userID, "application_credentials", credID)
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		ApplicationCredential map[string]interface{} `json:"application_credential"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.Equal(t, credID, result.ApplicationCredential["id"])

	// Cleanup
	cleanupTestApplicationCredential(t, client, userID, credID)
}

// TestKeystoneDeleteApplicationCredential_Contract tests DELETE /v3/users/:user_id/application_credentials/:id
func TestKeystoneDeleteApplicationCredential_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupKeystoneClient(t)

	// Create test credential
	userID := "00000000-0000-0000-0000-000000000001"
	credID := createTestApplicationCredential(t, client, userID)

	url := client.ServiceURL("users", userID, "application_credentials", credID)
	req, err := http.NewRequest("DELETE", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}

// TestKeystoneGetApplicationCredentialByID_Contract tests GET /v3/application_credentials/:id
func TestKeystoneGetApplicationCredentialByID_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupKeystoneClient(t)

	// Create test credential
	userID := "00000000-0000-0000-0000-000000000001"
	credID := createTestApplicationCredential(t, client, userID)

	url := client.ServiceURL("application_credentials", credID)
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		ApplicationCredential map[string]interface{} `json:"application_credential"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.Equal(t, credID, result.ApplicationCredential["id"])

	// Cleanup
	cleanupTestApplicationCredential(t, client, userID, credID)
}

// Helper to create test application credential
func createTestApplicationCredential(t *testing.T, client *gophercloud.ServiceClient, userID string) string {
	t.Helper()

	payload := map[string]interface{}{
		"application_credential": map[string]interface{}{
			"name":        "test-app-cred",
			"description": "Test application credential",
			"roles": []map[string]interface{}{
				{"id": "00000000-0000-0000-0000-000000000003"},
			},
		},
	}

	body, _ := json.Marshal(payload)
	url := client.ServiceURL("users", userID, "application_credentials")
	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to create application credential: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		ApplicationCredential map[string]interface{} `json:"application_credential"`
	}
	json.Unmarshal(respBody, &result)

	return result.ApplicationCredential["id"].(string)
}

// Helper to cleanup test application credential
func cleanupTestApplicationCredential(t *testing.T, client *gophercloud.ServiceClient, userID, credID string) {
	t.Helper()

	url := client.ServiceURL("users", userID, "application_credentials", credID)
	req, _ := http.NewRequest("DELETE", url, nil)
	req.Header.Set("X-Auth-Token", client.TokenID)
	http.DefaultClient.Do(req)
}
