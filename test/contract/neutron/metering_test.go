package neutron_test

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

// TestNeutronListMeteringLabels_Contract tests GET /v2.0/metering/metering-labels
func TestNeutronListMeteringLabels_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNeutronClient(t)

	url := client.ServiceURL("metering", "metering-labels")
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		MeteringLabels []map[string]interface{} `json:"metering_labels"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.NotNil(t, result.MeteringLabels)
}

// TestNeutronCreateMeteringLabel_Contract tests POST /v2.0/metering/metering-labels
func TestNeutronCreateMeteringLabel_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNeutronClient(t)

	// Create metering label
	payload := map[string]interface{}{
		"metering_label": map[string]interface{}{
			"name":        "test-metering-label",
			"description": "Test metering label",
		},
	}

	body, _ := json.Marshal(payload)
	url := client.ServiceURL("metering", "metering-labels")
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
		MeteringLabel map[string]interface{} `json:"metering_label"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.NotEmpty(t, result.MeteringLabel["id"])
	assert.Equal(t, "test-metering-label", result.MeteringLabel["name"])

	// Cleanup
	labelID := result.MeteringLabel["id"].(string)
	deleteURL := client.ServiceURL("metering", "metering-labels", labelID)
	deleteReq, _ := http.NewRequest("DELETE", deleteURL, nil)
	deleteReq.Header.Set("X-Auth-Token", client.TokenID)
	http.DefaultClient.Do(deleteReq)
}

// TestNeutronGetMeteringLabel_Contract tests GET /v2.0/metering/metering-labels/:id
func TestNeutronGetMeteringLabel_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNeutronClient(t)

	// Create label first
	labelID := createTestMeteringLabel(t, client, "test-label-get")

	url := client.ServiceURL("metering", "metering-labels", labelID)
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		MeteringLabel map[string]interface{} `json:"metering_label"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.Equal(t, labelID, result.MeteringLabel["id"])
	assert.Equal(t, "test-label-get", result.MeteringLabel["name"])

	// Cleanup
	cleanupTestMeteringLabel(t, client, labelID)
}

// TestNeutronDeleteMeteringLabel_Contract tests DELETE /v2.0/metering/metering-labels/:id
func TestNeutronDeleteMeteringLabel_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNeutronClient(t)

	// Create label first
	labelID := createTestMeteringLabel(t, client, "test-label-delete")

	url := client.ServiceURL("metering", "metering-labels", labelID)
	req, err := http.NewRequest("DELETE", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}

// TestNeutronListMeteringLabelRules_Contract tests GET /v2.0/metering/metering-label-rules
func TestNeutronListMeteringLabelRules_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNeutronClient(t)

	url := client.ServiceURL("metering", "metering-label-rules")
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		MeteringLabelRules []map[string]interface{} `json:"metering_label_rules"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.NotNil(t, result.MeteringLabelRules)
}

// TestNeutronCreateMeteringLabelRule_Contract tests POST /v2.0/metering/metering-label-rules
func TestNeutronCreateMeteringLabelRule_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNeutronClient(t)

	// Create label first
	labelID := createTestMeteringLabel(t, client, "test-label-for-rule")

	// Create rule
	payload := map[string]interface{}{
		"metering_label_rule": map[string]interface{}{
			"metering_label_id": labelID,
			"remote_ip_prefix":  "10.0.0.0/24",
			"direction":         "ingress",
		},
	}

	body, _ := json.Marshal(payload)
	url := client.ServiceURL("metering", "metering-label-rules")
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
		MeteringLabelRule map[string]interface{} `json:"metering_label_rule"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.NotEmpty(t, result.MeteringLabelRule["id"])
	assert.Equal(t, labelID, result.MeteringLabelRule["metering_label_id"])
	assert.Equal(t, "10.0.0.0/24", result.MeteringLabelRule["remote_ip_prefix"])

	// Cleanup
	ruleID := result.MeteringLabelRule["id"].(string)
	deleteURL := client.ServiceURL("metering", "metering-label-rules", ruleID)
	deleteReq, _ := http.NewRequest("DELETE", deleteURL, nil)
	deleteReq.Header.Set("X-Auth-Token", client.TokenID)
	http.DefaultClient.Do(deleteReq)

	cleanupTestMeteringLabel(t, client, labelID)
}

// Helper to create test metering label
func createTestMeteringLabel(t *testing.T, client *gophercloud.ServiceClient, name string) string {
	t.Helper()

	payload := map[string]interface{}{
		"metering_label": map[string]interface{}{
			"name": name,
		},
	}

	body, _ := json.Marshal(payload)
	url := client.ServiceURL("metering", "metering-labels")
	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to create metering label: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		MeteringLabel map[string]interface{} `json:"metering_label"`
	}
	json.Unmarshal(respBody, &result)

	return result.MeteringLabel["id"].(string)
}

// Helper to cleanup test metering label
func cleanupTestMeteringLabel(t *testing.T, client *gophercloud.ServiceClient, labelID string) {
	t.Helper()

	url := client.ServiceURL("metering", "metering-labels", labelID)
	req, _ := http.NewRequest("DELETE", url, nil)
	req.Header.Set("X-Auth-Token", client.TokenID)
	http.DefaultClient.Do(req)
}
