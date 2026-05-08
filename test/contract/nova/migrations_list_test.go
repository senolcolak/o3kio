package nova_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os/exec"
	"testing"

	"github.com/google/uuid"
	"github.com/gophercloud/gophercloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNovaListMigrations_Contract tests GET /v2.1/:project_id/os-migrations
func TestNovaListMigrations_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	url := client.ServiceURL("os-migrations")
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		Migrations []map[string]interface{} `json:"migrations"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.NotNil(t, result.Migrations)
}

// TestNovaListServerMigrations_Contract tests GET /v2.1/:project_id/servers/:id/migrations
func TestNovaListServerMigrations_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Get first server
	serverID := getTestServerID(t, client)
	if serverID == "" {
		t.Skip("No servers available for testing")
	}

	url := client.ServiceURL("servers", serverID, "migrations")
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		Migrations []map[string]interface{} `json:"migrations"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.NotNil(t, result.Migrations)
}

// TestNovaGetServerMigration_Contract tests GET /v2.1/:project_id/servers/:id/migrations/:id
func TestNovaGetServerMigration_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Create a test migration record
	serverID := getTestServerID(t, client)
	if serverID == "" {
		t.Skip("No servers available for testing")
	}

	migrationID := createTestMigration(t, serverID)

	url := client.ServiceURL("servers", serverID, "migrations", migrationID)
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		Migration map[string]interface{} `json:"migration"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.NotEmpty(t, result.Migration["id"])
	assert.Equal(t, serverID, result.Migration["server_uuid"])

	// Cleanup
	cleanupTestMigration(t, migrationID)
}

// TestNovaDeleteServerMigration_Contract tests DELETE /v2.1/:project_id/servers/:id/migrations/:id
func TestNovaDeleteServerMigration_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	serverID := getTestServerID(t, client)
	if serverID == "" {
		t.Skip("No servers available for testing")
	}

	migrationID := createTestMigration(t, serverID)

	// Test: Delete (cancel) migration
	url := client.ServiceURL("servers", serverID, "migrations", migrationID)
	req, err := http.NewRequest("DELETE", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}

// TestNovaForceCompleteMigration_Contract tests POST /v2.1/:project_id/servers/:id/migrations/:id/action
func TestNovaForceCompleteMigration_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	serverID := getTestServerID(t, client)
	if serverID == "" {
		t.Skip("No servers available for testing")
	}

	migrationID := createTestMigration(t, serverID)

	// Test: Force complete migration
	action := map[string]interface{}{
		"force_complete": nil,
	}

	body, _ := json.Marshal(action)
	url := client.ServiceURL("servers", serverID, "migrations", migrationID, "action")
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusAccepted, resp.StatusCode)

	// Cleanup
	cleanupTestMigration(t, migrationID)
}

// Helper to get test server ID
func getTestServerID(t *testing.T, client *gophercloud.ServiceClient) string {
	url := client.ServiceURL("servers", "detail")
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("X-Auth-Token", client.TokenID)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		Servers []map[string]interface{} `json:"servers"`
	}
	json.Unmarshal(respBody, &result)

	if len(result.Servers) > 0 {
		return result.Servers[0]["id"].(string)
	}

	return ""
}

// Helper to create test migration in database
func createTestMigration(t *testing.T, serverID string) string {
	t.Helper()

	migrationID := uuid.New().String()

	cmd := `docker exec o3k-postgres psql -U lightstack -d lightstack -c "
	INSERT INTO server_migrations (id, server_uuid, source_node, dest_node, status, migration_type, created_at, updated_at)
	VALUES ('` + migrationID + `', '` + serverID + `', 'node1', 'node2', 'migrating', 'live-migration', NOW(), NOW())
	ON CONFLICT DO NOTHING;"`

	exec.Command("sh", "-c", cmd).Run()

	return migrationID
}

// Helper to cleanup test migration
func cleanupTestMigration(t *testing.T, migrationID string) {
	t.Helper()

	cmd := `docker exec o3k-postgres psql -U lightstack -d lightstack -c "DELETE FROM server_migrations WHERE id='` + migrationID + `';"`
	exec.Command("sh", "-c", cmd).Run()
}
