package neutron_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/gophercloud/gophercloud/openstack/networking/v2/networks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNeutronCreateRBACPolicy_Contract tests POST /v2.0/rbac-policies
func TestNeutronCreateRBACPolicy_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNeutronClient(t)

	// Create a network first
	network, err := networks.Create(client, networks.CreateOpts{
		Name: "test-rbac-network",
	}).Extract()
	require.NoError(t, err)
	defer networks.Delete(client, network.ID)

	// Test: Create RBAC policy
	policy := map[string]interface{}{
		"rbac_policy": map[string]interface{}{
			"object_type":   "network",
			"object_id":     network.ID,
			"target_tenant": "550e8400-e29b-41d4-a716-446655440001", // Another project
			"action":        "access_as_shared",
		},
	}

	body, _ := json.Marshal(policy)
	url := client.ServiceURL("rbac-policies")
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
		RBACPolicy map[string]interface{} `json:"rbac_policy"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.NotEmpty(t, result.RBACPolicy["id"])
	assert.Equal(t, "network", result.RBACPolicy["object_type"])
	assert.Equal(t, network.ID, result.RBACPolicy["object_id"])
	assert.Equal(t, "access_as_shared", result.RBACPolicy["action"])

	// Cleanup
	if policyID, ok := result.RBACPolicy["id"].(string); ok {
		delURL := client.ServiceURL("rbac-policies", policyID)
		delReq, _ := http.NewRequest("DELETE", delURL, nil)
		delReq.Header.Set("X-Auth-Token", client.TokenID)
		http.DefaultClient.Do(delReq)
	}
}

// TestNeutronListRBACPolicies_Contract tests GET /v2.0/rbac-policies
func TestNeutronListRBACPolicies_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNeutronClient(t)

	// Create a network and RBAC policy
	network, err := networks.Create(client, networks.CreateOpts{
		Name: "test-list-rbac-network",
	}).Extract()
	require.NoError(t, err)
	defer networks.Delete(client, network.ID)

	policy := map[string]interface{}{
		"rbac_policy": map[string]interface{}{
			"object_type":   "network",
			"object_id":     network.ID,
			"target_tenant": "550e8400-e29b-41d4-a716-446655440001",
			"action":        "access_as_shared",
		},
	}
	policyBody, _ := json.Marshal(policy)
	createURL := client.ServiceURL("rbac-policies")
	createReq, _ := http.NewRequest("POST", createURL, bytes.NewReader(policyBody))
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatal(err)
	}
	defer createResp.Body.Close()

	// Test: List RBAC policies
	url := client.ServiceURL("rbac-policies")
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		RBACPolicies []map[string]interface{} `json:"rbac_policies"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.NotEmpty(t, result.RBACPolicies)
}

// TestNeutronGetRBACPolicy_Contract tests GET /v2.0/rbac-policies/:id
func TestNeutronGetRBACPolicy_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNeutronClient(t)

	// Create network and RBAC policy
	network, err := networks.Create(client, networks.CreateOpts{
		Name: "test-get-rbac-network",
	}).Extract()
	require.NoError(t, err)
	defer networks.Delete(client, network.ID)

	policy := map[string]interface{}{
		"rbac_policy": map[string]interface{}{
			"object_type":   "network",
			"object_id":     network.ID,
			"target_tenant": "550e8400-e29b-41d4-a716-446655440001",
			"action":        "access_as_shared",
		},
	}
	policyBody, _ := json.Marshal(policy)
	createURL := client.ServiceURL("rbac-policies")
	createReq, _ := http.NewRequest("POST", createURL, bytes.NewReader(policyBody))
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatal(err)
	}
	defer createResp.Body.Close()

	createBody, _ := io.ReadAll(createResp.Body)
	var createResult struct {
		RBACPolicy map[string]interface{} `json:"rbac_policy"`
	}
	json.Unmarshal(createBody, &createResult)
	policyID := createResult.RBACPolicy["id"].(string)

	// Test: Get RBAC policy
	url := client.ServiceURL("rbac-policies", policyID)
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		RBACPolicy map[string]interface{} `json:"rbac_policy"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.Equal(t, policyID, result.RBACPolicy["id"])
	assert.Equal(t, "network", result.RBACPolicy["object_type"])

	// Cleanup
	delReq, _ := http.NewRequest("DELETE", url, nil)
	delReq.Header.Set("X-Auth-Token", client.TokenID)
	http.DefaultClient.Do(delReq)
}

// TestNeutronUpdateRBACPolicy_Contract tests PUT /v2.0/rbac-policies/:id
func TestNeutronUpdateRBACPolicy_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNeutronClient(t)

	// Create network and RBAC policy
	network, err := networks.Create(client, networks.CreateOpts{
		Name: "test-update-rbac-network",
	}).Extract()
	require.NoError(t, err)
	defer networks.Delete(client, network.ID)

	policy := map[string]interface{}{
		"rbac_policy": map[string]interface{}{
			"object_type":   "network",
			"object_id":     network.ID,
			"target_tenant": "550e8400-e29b-41d4-a716-446655440001",
			"action":        "access_as_shared",
		},
	}
	policyBody, _ := json.Marshal(policy)
	createURL := client.ServiceURL("rbac-policies")
	createReq, _ := http.NewRequest("POST", createURL, bytes.NewReader(policyBody))
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatal(err)
	}
	defer createResp.Body.Close()

	createBody, _ := io.ReadAll(createResp.Body)
	var createResult struct {
		RBACPolicy map[string]interface{} `json:"rbac_policy"`
	}
	json.Unmarshal(createBody, &createResult)
	policyID := createResult.RBACPolicy["id"].(string)

	// Test: Update RBAC policy (change target tenant)
	updatePolicy := map[string]interface{}{
		"rbac_policy": map[string]interface{}{
			"target_tenant": "550e8400-e29b-41d4-a716-446655440002",
		},
	}
	updateBody, _ := json.Marshal(updatePolicy)
	url := client.ServiceURL("rbac-policies", policyID)
	req, err := http.NewRequest("PUT", url, bytes.NewReader(updateBody))
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		RBACPolicy map[string]interface{} `json:"rbac_policy"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.Equal(t, "550e8400-e29b-41d4-a716-446655440002", result.RBACPolicy["target_tenant"])

	// Cleanup
	delReq, _ := http.NewRequest("DELETE", url, nil)
	delReq.Header.Set("X-Auth-Token", client.TokenID)
	http.DefaultClient.Do(delReq)
}

// TestNeutronDeleteRBACPolicy_Contract tests DELETE /v2.0/rbac-policies/:id
func TestNeutronDeleteRBACPolicy_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNeutronClient(t)

	// Create network and RBAC policy
	network, err := networks.Create(client, networks.CreateOpts{
		Name: "test-delete-rbac-network",
	}).Extract()
	require.NoError(t, err)
	defer networks.Delete(client, network.ID)

	policy := map[string]interface{}{
		"rbac_policy": map[string]interface{}{
			"object_type":   "network",
			"object_id":     network.ID,
			"target_tenant": "550e8400-e29b-41d4-a716-446655440001",
			"action":        "access_as_shared",
		},
	}
	policyBody, _ := json.Marshal(policy)
	createURL := client.ServiceURL("rbac-policies")
	createReq, _ := http.NewRequest("POST", createURL, bytes.NewReader(policyBody))
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatal(err)
	}
	defer createResp.Body.Close()

	createBody, _ := io.ReadAll(createResp.Body)
	var createResult struct {
		RBACPolicy map[string]interface{} `json:"rbac_policy"`
	}
	json.Unmarshal(createBody, &createResult)
	policyID := createResult.RBACPolicy["id"].(string)

	// Test: Delete RBAC policy
	url := client.ServiceURL("rbac-policies", policyID)
	req, err := http.NewRequest("DELETE", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}
