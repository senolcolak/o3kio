package keystone_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getTokenForTest(t *testing.T) string {
	authReq := map[string]interface{}{
		"auth": map[string]interface{}{
			"identity": map[string]interface{}{
				"methods": []string{"password"},
				"password": map[string]interface{}{
					"user": map[string]interface{}{
						"name":     "admin",
						"password": "secret",
						"domain": map[string]interface{}{
							"name": "Default",
						},
					},
				},
			},
			"scope": map[string]interface{}{
				"project": map[string]interface{}{
					"name": "default",
					"domain": map[string]interface{}{
						"name": "Default",
					},
				},
			},
		},
	}

	body, _ := json.Marshal(authReq)
	resp, err := http.Post("http://localhost:35357/v3/auth/tokens", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	token := resp.Header.Get("X-Subject-Token")
	require.NotEmpty(t, token, "Token should be in X-Subject-Token header")
	return token
}

// TestKeystoneCreateApplicationCredential_Contract tests POST /v3/users/:id/application_credentials
func TestKeystoneCreateApplicationCredential_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	token := getTokenForTest(t)
	userID := "00000000-0000-0000-0000-000000000001" // Default admin user

	credential := map[string]interface{}{
		"application_credential": map[string]interface{}{
			"name":        "test-app-credential",
			"description": "Test credential for contract testing",
		},
	}

	body, _ := json.Marshal(credential)
	req, err := http.NewRequest("POST", "http://localhost:35357/v3/users/"+userID+"/application_credentials", bytes.NewReader(body))
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", token)
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
	assert.Equal(t, "test-app-credential", result.ApplicationCredential["name"])

	// Cleanup
	if credID, ok := result.ApplicationCredential["id"].(string); ok {
		delReq, _ := http.NewRequest("DELETE", "http://localhost:35357/v3/users/"+userID+"/application_credentials/"+credID, nil)
		delReq.Header.Set("X-Auth-Token", token)
		http.DefaultClient.Do(delReq)
	}
}

// TestKeystoneListApplicationCredentials_Contract tests GET /v3/users/:user_id/application_credentials
func TestKeystoneListApplicationCredentials_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	token := getTokenForTest(t)
	userID := "00000000-0000-0000-0000-000000000001"

	// Create a test credential first
	credential := map[string]interface{}{
		"application_credential": map[string]interface{}{
			"name": "test-list-credential",
		},
	}
	credBody, _ := json.Marshal(credential)
	createReq, _ := http.NewRequest("POST", "http://localhost:35357/v3/users/"+userID+"/application_credentials", bytes.NewReader(credBody))
	createReq.Header.Set("X-Auth-Token", token)
	createReq.Header.Set("Content-Type", "application/json")
	createResp, _ := http.DefaultClient.Do(createReq)
	defer createResp.Body.Close()

	// Test: List credentials
	req, err := http.NewRequest("GET", "http://localhost:35357/v3/users/"+userID+"/application_credentials", nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", token)

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

	assert.NotEmpty(t, result.ApplicationCredentials)
}

// TestKeystoneGetApplicationCredential_Contract tests GET /v3/users/:user_id/application_credentials/:id
func TestKeystoneGetApplicationCredential_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	token := getTokenForTest(t)
	userID := "00000000-0000-0000-0000-000000000001"

	// Create a test credential
	credential := map[string]interface{}{
		"application_credential": map[string]interface{}{
			"name": "test-get-credential",
		},
	}
	credBody, _ := json.Marshal(credential)
	createReq, _ := http.NewRequest("POST", "http://localhost:35357/v3/users/"+userID+"/application_credentials", bytes.NewReader(credBody))
	createReq.Header.Set("X-Auth-Token", token)
	createReq.Header.Set("Content-Type", "application/json")
	createResp, _ := http.DefaultClient.Do(createReq)
	defer createResp.Body.Close()

	createBody, _ := io.ReadAll(createResp.Body)
	var createResult struct {
		ApplicationCredential map[string]interface{} `json:"application_credential"`
	}
	json.Unmarshal(createBody, &createResult)
	credID := createResult.ApplicationCredential["id"].(string)

	// Test: Get credential
	req, err := http.NewRequest("GET", "http://localhost:35357/v3/users/"+userID+"/application_credentials/"+credID, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", token)

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
	assert.Equal(t, "test-get-credential", result.ApplicationCredential["name"])
	assert.Nil(t, result.ApplicationCredential["secret"]) // Secret not returned on GET

	// Cleanup
	delReq, _ := http.NewRequest("DELETE", "http://localhost:35357/v3/users/"+userID+"/application_credentials/"+credID, nil)
	delReq.Header.Set("X-Auth-Token", token)
	http.DefaultClient.Do(delReq)
}

// TestKeystoneDeleteApplicationCredential_Contract tests DELETE /v3/users/:user_id/application_credentials/:id
func TestKeystoneDeleteApplicationCredential_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	token := getTokenForTest(t)
	userID := "00000000-0000-0000-0000-000000000001"

	// Create a test credential
	credential := map[string]interface{}{
		"application_credential": map[string]interface{}{
			"name": "test-delete-credential",
		},
	}
	credBody, _ := json.Marshal(credential)
	createReq, _ := http.NewRequest("POST", "http://localhost:35357/v3/users/"+userID+"/application_credentials", bytes.NewReader(credBody))
	createReq.Header.Set("X-Auth-Token", token)
	createReq.Header.Set("Content-Type", "application/json")
	createResp, _ := http.DefaultClient.Do(createReq)
	defer createResp.Body.Close()

	createBody, _ := io.ReadAll(createResp.Body)
	var createResult struct {
		ApplicationCredential map[string]interface{} `json:"application_credential"`
	}
	json.Unmarshal(createBody, &createResult)
	credID := createResult.ApplicationCredential["id"].(string)

	// Test: Delete credential
	req, err := http.NewRequest("DELETE", "http://localhost:35357/v3/users/"+userID+"/application_credentials/"+credID, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", token)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}
