package cinder_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCinderGetQuotaSet_Contract tests GET /v3/:project/quota-sets/:id
func TestCinderGetQuotaSet_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupCinderClient(t)

	// Use "default" project for quota lookup
	url := client.ServiceURL("quota-sets", "default")
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		QuotaSet map[string]interface{} `json:"quota_set"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.NotNil(t, result.QuotaSet)
	assert.NotNil(t, result.QuotaSet["volumes"])
	assert.NotNil(t, result.QuotaSet["snapshots"])
	assert.NotNil(t, result.QuotaSet["gigabytes"])
}

// TestCinderUpdateQuotaSet_Contract tests PUT /v3/:project/quota-sets/:id
func TestCinderUpdateQuotaSet_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupCinderClient(t)

	payload := map[string]interface{}{
		"quota_set": map[string]interface{}{
			"volumes":   20,
			"gigabytes": 2000,
		},
	}

	body, _ := json.Marshal(payload)
	url := client.ServiceURL("quota-sets", "default")
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
		QuotaSet map[string]interface{} `json:"quota_set"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.Equal(t, float64(20), result.QuotaSet["volumes"])
	assert.Equal(t, float64(2000), result.QuotaSet["gigabytes"])
}

// TestCinderDeleteQuotaSet_Contract tests DELETE /v3/:project/quota-sets/:id
func TestCinderDeleteQuotaSet_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupCinderClient(t)

	// First set custom quotas
	payload := map[string]interface{}{
		"quota_set": map[string]interface{}{
			"volumes": 5,
		},
	}
	body, _ := json.Marshal(payload)
	putURL := client.ServiceURL("quota-sets", "default")
	putReq, _ := http.NewRequest("PUT", putURL, bytes.NewReader(body))
	putReq.Header.Set("X-Auth-Token", client.TokenID)
	putReq.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(putReq)

	// Delete (reset to defaults)
	url := client.ServiceURL("quota-sets", "default")
	req, err := http.NewRequest("DELETE", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}
