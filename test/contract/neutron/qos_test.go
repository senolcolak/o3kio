package neutron_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNeutronListQoSPolicies_Contract tests GET /v2.0/qos/policies
func TestNeutronListQoSPolicies_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNeutronClient(t)

	url := client.ServiceURL("qos", "policies")
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
	assert.Contains(t, result, "policies")
}

// TestNeutronCreateQoSPolicy_Contract tests POST /v2.0/qos/policies
func TestNeutronCreateQoSPolicy_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNeutronClient(t)

	url := client.ServiceURL("qos", "policies")
	body := strings.NewReader(`{"policy": {"name": "test-qos-policy", "description": "Test QoS policy"}}`)
	req, err := http.NewRequest("POST", url, body)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)
	assert.Contains(t, result, "policy")

	// Clean up
	if policy, ok := result["policy"].(map[string]interface{}); ok {
		if policyID, ok := policy["id"].(string); ok {
			deleteURL := client.ServiceURL("qos", "policies", policyID)
			delReq, _ := http.NewRequest("DELETE", deleteURL, nil)
			delReq.Header.Set("X-Auth-Token", client.TokenID)
			http.DefaultClient.Do(delReq)
		}
	}
}

// TestNeutronGetQoSPolicy_Contract tests GET /v2.0/qos/policies/:id
func TestNeutronGetQoSPolicy_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNeutronClient(t)

	// Create policy
	createURL := client.ServiceURL("qos", "policies")
	createBody := strings.NewReader(`{"policy": {"name": "test-get-qos-policy"}}`)
	createReq, _ := http.NewRequest("POST", createURL, createBody)
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := http.DefaultClient.Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()

	var createResult map[string]interface{}
	json.NewDecoder(createResp.Body).Decode(&createResult)
	policy := createResult["policy"].(map[string]interface{})
	policyID := policy["id"].(string)

	// Test: Get policy
	url := client.ServiceURL("qos", "policies", policyID)
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

// TestNeutronUpdateQoSPolicy_Contract tests PUT /v2.0/qos/policies/:id
func TestNeutronUpdateQoSPolicy_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNeutronClient(t)

	// Create policy
	createURL := client.ServiceURL("qos", "policies")
	createBody := strings.NewReader(`{"policy": {"name": "test-update-qos-policy"}}`)
	createReq, _ := http.NewRequest("POST", createURL, createBody)
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := http.DefaultClient.Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()

	var createResult map[string]interface{}
	json.NewDecoder(createResp.Body).Decode(&createResult)
	policy := createResult["policy"].(map[string]interface{})
	policyID := policy["id"].(string)

	// Test: Update policy
	url := client.ServiceURL("qos", "policies", policyID)
	body := strings.NewReader(`{"policy": {"name": "updated-qos-policy", "description": "Updated description"}}`)
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

// TestNeutronDeleteQoSPolicy_Contract tests DELETE /v2.0/qos/policies/:id
func TestNeutronDeleteQoSPolicy_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNeutronClient(t)

	// Create policy
	createURL := client.ServiceURL("qos", "policies")
	createBody := strings.NewReader(`{"policy": {"name": "test-delete-qos-policy"}}`)
	createReq, _ := http.NewRequest("POST", createURL, createBody)
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := http.DefaultClient.Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()

	var createResult map[string]interface{}
	json.NewDecoder(createResp.Body).Decode(&createResult)
	policy := createResult["policy"].(map[string]interface{})
	policyID := policy["id"].(string)

	// Test: Delete policy
	url := client.ServiceURL("qos", "policies", policyID)
	req, err := http.NewRequest("DELETE", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}

// TestNeutronListBandwidthLimitRules_Contract tests GET /v2.0/qos/policies/:id/bandwidth_limit_rules
func TestNeutronListBandwidthLimitRules_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNeutronClient(t)

	// Create policy
	createURL := client.ServiceURL("qos", "policies")
	createBody := strings.NewReader(`{"policy": {"name": "test-bw-rules-policy"}}`)
	createReq, _ := http.NewRequest("POST", createURL, createBody)
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := http.DefaultClient.Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()

	var createResult map[string]interface{}
	json.NewDecoder(createResp.Body).Decode(&createResult)
	policy := createResult["policy"].(map[string]interface{})
	policyID := policy["id"].(string)

	// Test: List bandwidth limit rules
	url := client.ServiceURL("qos", "policies", policyID, "bandwidth_limit_rules")
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Clean up
	delURL := client.ServiceURL("qos", "policies", policyID)
	delReq, _ := http.NewRequest("DELETE", delURL, nil)
	delReq.Header.Set("X-Auth-Token", client.TokenID)
	http.DefaultClient.Do(delReq)
}

// TestNeutronCreateBandwidthLimitRule_Contract tests POST /v2.0/qos/policies/:id/bandwidth_limit_rules
func TestNeutronCreateBandwidthLimitRule_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNeutronClient(t)

	// Create policy
	createURL := client.ServiceURL("qos", "policies")
	createBody := strings.NewReader(`{"policy": {"name": "test-create-bw-rule-policy"}}`)
	createReq, _ := http.NewRequest("POST", createURL, createBody)
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := http.DefaultClient.Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()

	var createResult map[string]interface{}
	json.NewDecoder(createResp.Body).Decode(&createResult)
	policy := createResult["policy"].(map[string]interface{})
	policyID := policy["id"].(string)

	// Test: Create bandwidth limit rule
	url := client.ServiceURL("qos", "policies", policyID, "bandwidth_limit_rules")
	body := strings.NewReader(`{"bandwidth_limit_rule": {"max_kbps": 10000, "max_burst_kbps": 1000}}`)
	req, err := http.NewRequest("POST", url, body)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	// Clean up
	delURL := client.ServiceURL("qos", "policies", policyID)
	delReq, _ := http.NewRequest("DELETE", delURL, nil)
	delReq.Header.Set("X-Auth-Token", client.TokenID)
	http.DefaultClient.Do(delReq)
}

// TestNeutronGetBandwidthLimitRule_Contract tests GET /v2.0/qos/policies/:id/bandwidth_limit_rules/:rule_id
func TestNeutronGetBandwidthLimitRule_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNeutronClient(t)

	// Create policy
	createURL := client.ServiceURL("qos", "policies")
	createBody := strings.NewReader(`{"policy": {"name": "test-get-bw-rule-policy"}}`)
	createReq, _ := http.NewRequest("POST", createURL, createBody)
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := http.DefaultClient.Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()

	var createResult map[string]interface{}
	json.NewDecoder(createResp.Body).Decode(&createResult)
	policy := createResult["policy"].(map[string]interface{})
	policyID := policy["id"].(string)

	// Create rule
	createRuleURL := client.ServiceURL("qos", "policies", policyID, "bandwidth_limit_rules")
	createRuleBody := strings.NewReader(`{"bandwidth_limit_rule": {"max_kbps": 10000, "max_burst_kbps": 1000}}`)
	createRuleReq, _ := http.NewRequest("POST", createRuleURL, createRuleBody)
	createRuleReq.Header.Set("X-Auth-Token", client.TokenID)
	createRuleReq.Header.Set("Content-Type", "application/json")

	createRuleResp, err := http.DefaultClient.Do(createRuleReq)
	require.NoError(t, err)
	defer createRuleResp.Body.Close()

	var createRuleResult map[string]interface{}
	json.NewDecoder(createRuleResp.Body).Decode(&createRuleResult)
	rule := createRuleResult["bandwidth_limit_rule"].(map[string]interface{})
	ruleID := rule["id"].(string)

	// Test: Get rule
	url := client.ServiceURL("qos", "policies", policyID, "bandwidth_limit_rules", ruleID)
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Clean up
	delURL := client.ServiceURL("qos", "policies", policyID)
	delReq, _ := http.NewRequest("DELETE", delURL, nil)
	delReq.Header.Set("X-Auth-Token", client.TokenID)
	http.DefaultClient.Do(delReq)
}

// TestNeutronUpdateBandwidthLimitRule_Contract tests PUT /v2.0/qos/policies/:id/bandwidth_limit_rules/:rule_id
func TestNeutronUpdateBandwidthLimitRule_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNeutronClient(t)

	// Create policy
	createURL := client.ServiceURL("qos", "policies")
	createBody := strings.NewReader(`{"policy": {"name": "test-update-bw-rule-policy"}}`)
	createReq, _ := http.NewRequest("POST", createURL, createBody)
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := http.DefaultClient.Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()

	var createResult map[string]interface{}
	json.NewDecoder(createResp.Body).Decode(&createResult)
	policy := createResult["policy"].(map[string]interface{})
	policyID := policy["id"].(string)

	// Create rule
	createRuleURL := client.ServiceURL("qos", "policies", policyID, "bandwidth_limit_rules")
	createRuleBody := strings.NewReader(`{"bandwidth_limit_rule": {"max_kbps": 10000, "max_burst_kbps": 1000}}`)
	createRuleReq, _ := http.NewRequest("POST", createRuleURL, createRuleBody)
	createRuleReq.Header.Set("X-Auth-Token", client.TokenID)
	createRuleReq.Header.Set("Content-Type", "application/json")

	createRuleResp, err := http.DefaultClient.Do(createRuleReq)
	require.NoError(t, err)
	defer createRuleResp.Body.Close()

	var createRuleResult map[string]interface{}
	json.NewDecoder(createRuleResp.Body).Decode(&createRuleResult)
	rule := createRuleResult["bandwidth_limit_rule"].(map[string]interface{})
	ruleID := rule["id"].(string)

	// Test: Update rule
	url := client.ServiceURL("qos", "policies", policyID, "bandwidth_limit_rules", ruleID)
	body := strings.NewReader(`{"bandwidth_limit_rule": {"max_kbps": 20000, "max_burst_kbps": 2000}}`)
	req, err := http.NewRequest("PUT", url, body)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Clean up
	delURL := client.ServiceURL("qos", "policies", policyID)
	delReq, _ := http.NewRequest("DELETE", delURL, nil)
	delReq.Header.Set("X-Auth-Token", client.TokenID)
	http.DefaultClient.Do(delReq)
}

// TestNeutronDeleteBandwidthLimitRule_Contract tests DELETE /v2.0/qos/policies/:id/bandwidth_limit_rules/:rule_id
func TestNeutronDeleteBandwidthLimitRule_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNeutronClient(t)

	// Create policy
	createURL := client.ServiceURL("qos", "policies")
	createBody := strings.NewReader(`{"policy": {"name": "test-delete-bw-rule-policy"}}`)
	createReq, _ := http.NewRequest("POST", createURL, createBody)
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := http.DefaultClient.Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()

	var createResult map[string]interface{}
	json.NewDecoder(createResp.Body).Decode(&createResult)
	policy := createResult["policy"].(map[string]interface{})
	policyID := policy["id"].(string)

	// Create rule
	createRuleURL := client.ServiceURL("qos", "policies", policyID, "bandwidth_limit_rules")
	createRuleBody := strings.NewReader(`{"bandwidth_limit_rule": {"max_kbps": 10000, "max_burst_kbps": 1000}}`)
	createRuleReq, _ := http.NewRequest("POST", createRuleURL, createRuleBody)
	createRuleReq.Header.Set("X-Auth-Token", client.TokenID)
	createRuleReq.Header.Set("Content-Type", "application/json")

	createRuleResp, err := http.DefaultClient.Do(createRuleReq)
	require.NoError(t, err)
	defer createRuleResp.Body.Close()

	var createRuleResult map[string]interface{}
	json.NewDecoder(createRuleResp.Body).Decode(&createRuleResult)
	rule := createRuleResult["bandwidth_limit_rule"].(map[string]interface{})
	ruleID := rule["id"].(string)

	// Test: Delete rule
	url := client.ServiceURL("qos", "policies", policyID, "bandwidth_limit_rules", ruleID)
	req, err := http.NewRequest("DELETE", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	// Clean up policy
	delURL := client.ServiceURL("qos", "policies", policyID)
	delReq, _ := http.NewRequest("DELETE", delURL, nil)
	delReq.Header.Set("X-Auth-Token", client.TokenID)
	http.DefaultClient.Do(delReq)
}
