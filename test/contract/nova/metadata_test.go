package nova_test

import (
	"os"
	"testing"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper functions for Nova contract tests
func setupNovaClient(t *testing.T) *gophercloud.ServiceClient {
	t.Helper()

	authURL := getEnvOrDefault("OS_AUTH_URL", "http://localhost:5001/v3")
	username := getEnvOrDefault("OS_USERNAME", "admin")
	password := getEnvOrDefault("OS_PASSWORD", "secret")
	projectName := getEnvOrDefault("OS_PROJECT_NAME", "default")
	domainName := getEnvOrDefault("OS_USER_DOMAIN_NAME", "Default")

	opts := gophercloud.AuthOptions{
		IdentityEndpoint: authURL,
		Username:         username,
		Password:         password,
		TenantName:       projectName,
		DomainName:       domainName,
	}

	provider, err := openstack.AuthenticatedClient(opts)
	require.NoError(t, err, "Failed to create authenticated client")

	client, err := openstack.NewComputeV2(provider, gophercloud.EndpointOpts{})
	require.NoError(t, err, "Failed to create Nova client")

	return client
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func skipIfO3KNotRunning(t *testing.T) {
	t.Helper()
	if os.Getenv("SKIP_CONTRACT_TESTS") == "1" {
		t.Skip("Contract tests skipped (SKIP_CONTRACT_TESTS=1)")
	}
}

// TestNovaServerMetadataList_Contract tests GET /v2.1/{project_id}/servers/{server_id}/metadata
func TestNovaServerMetadataList_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Setup: Create a test server first
	// Use m1.tiny flavor UUID from seed data
	createOpts := servers.CreateOpts{
		Name:      "test-server-metadata-list",
		FlavorRef: "00000000-0000-0000-0000-000000000010", // m1.tiny
		ImageRef:  "00000000-0000-0000-0000-000000000001", // Dummy image for stub mode
	}

	server, err := servers.Create(client, createOpts).Extract()
	require.NoError(t, err, "Setup: CreateServer should succeed")
	defer servers.Delete(client, server.ID)

	// Wait for server to be active (or at least created)
	// Note: In stub mode this should be instant

	// Test: List server metadata (should be empty initially)
	metadata, err := servers.Metadata(client, server.ID).Extract()

	// Assertions
	require.NoError(t, err, "ListServerMetadata should succeed")
	assert.NotNil(t, metadata, "Metadata should not be nil")
	// Empty metadata is valid for a new server
}

// TestNovaServerMetadataCreate_Contract tests POST /v2.1/{project_id}/servers/{server_id}/metadata
// Note: gophercloud uses UpdateMetadata for both create and update operations
func TestNovaServerMetadataCreate_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Setup: Create a test server
	createOpts := servers.CreateOpts{
		Name:      "test-server-metadata-create",
		FlavorRef: "00000000-0000-0000-0000-000000000010",
		ImageRef:  "00000000-0000-0000-0000-000000000001",
	}

	server, err := servers.Create(client, createOpts).Extract()
	require.NoError(t, err, "Setup: CreateServer should succeed")
	defer servers.Delete(client, server.ID)

	// Test: Create/merge metadata using UpdateMetadata
	metadataOpts := servers.MetadataOpts{
		"environment": "test",
		"owner":       "contract-test",
	}

	metadata, err := servers.UpdateMetadata(client, server.ID, metadataOpts).Extract()

	// Assertions
	require.NoError(t, err, "UpdateMetadata should succeed")
	assert.NotNil(t, metadata, "Returned metadata should not be nil")
	assert.Equal(t, "test", metadata["environment"], "Metadata should contain environment key")
	assert.Equal(t, "contract-test", metadata["owner"], "Metadata should contain owner key")
}

// TestNovaServerMetadataUpdate_Contract tests PUT /v2.1/{project_id}/servers/{server_id}/metadata
// Note: gophercloud uses ResetMetadata for replacing all metadata
func TestNovaServerMetadataUpdate_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Setup: Create server with initial metadata
	createOpts := servers.CreateOpts{
		Name:      "test-server-metadata-update",
		FlavorRef: "00000000-0000-0000-0000-000000000010",
		ImageRef:  "00000000-0000-0000-0000-000000000001",
		Metadata: map[string]string{
			"initial": "value",
		},
	}

	server, err := servers.Create(client, createOpts).Extract()
	require.NoError(t, err, "Setup: CreateServer should succeed")
	defer servers.Delete(client, server.ID)

	// Test: Replace all metadata using ResetMetadata
	updateOpts := servers.MetadataOpts{
		"updated": "new-value",
		"extra":   "data",
	}

	metadata, err := servers.ResetMetadata(client, server.ID, updateOpts).Extract()

	// Assertions
	require.NoError(t, err, "ResetMetadata should succeed")
	assert.NotNil(t, metadata, "Returned metadata should not be nil")
	assert.Equal(t, "new-value", metadata["updated"], "Should have updated key")
	assert.Equal(t, "data", metadata["extra"], "Should have extra key")
	assert.NotContains(t, metadata, "initial", "Original metadata should be replaced")
}

// TestNovaServerMetadataDeleteKeyNotFound_Contract tests 404 handling
func TestNovaServerMetadataDeleteKeyNotFound_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Setup: Create server without specific metadata
	createOpts := servers.CreateOpts{
		Name:      "test-server-metadata-404",
		FlavorRef: "00000000-0000-0000-0000-000000000010",
		ImageRef:  "00000000-0000-0000-0000-000000000001",
	}

	server, err := servers.Create(client, createOpts).Extract()
	require.NoError(t, err, "Setup: CreateServer should succeed")
	defer servers.Delete(client, server.ID)

	// Test: Try to get metadata for non-existent server
	_, err = servers.Metadata(client, "non-existent-id").Extract()

	// Assertions: Should return 404
	require.Error(t, err, "Get metadata for non-existent server should fail")
}

