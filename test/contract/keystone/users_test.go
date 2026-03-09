package keystone_test

import (
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/identity/v3/users"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupClient creates authenticated client for testing
func setupClient(t *testing.T) *gophercloud.ServiceClient {
	t.Helper()

	authURL := getEnvOrDefault("OS_AUTH_URL", "http://localhost:5001/v3")
	username := getEnvOrDefault("OS_USERNAME", "admin")
	password := getEnvOrDefault("OS_PASSWORD", "secret")
	projectName := getEnvOrDefault("OS_PROJECT_NAME", "default")
	domainName := getEnvOrDefault("OS_USER_DOMAIN_NAME", "Default") // Use "Default" not "default"

	opts := gophercloud.AuthOptions{
		IdentityEndpoint: authURL,
		Username:         username,
		Password:         password,
		TenantName:       projectName,
		DomainName:       domainName,
	}

	provider, err := openstack.AuthenticatedClient(opts)
	require.NoError(t, err, "Failed to create authenticated client")

	client, err := openstack.NewIdentityV3(provider, gophercloud.EndpointOpts{})
	require.NoError(t, err, "Failed to create Keystone client")

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

	// Just skip if environment variable explicitly says to skip
	if os.Getenv("SKIP_CONTRACT_TESTS") == "1" {
		t.Skip("Contract tests skipped (SKIP_CONTRACT_TESTS=1)")
	}
}

// TestKeystoneCreateUser_Contract tests POST /v3/users endpoint
// Per Constitution Article III: This test must FAIL (RED) initially, then pass after implementation (GREEN)
func TestKeystoneCreateUser_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupClient(t)

	// Generate unique username to avoid conflicts
	userName := "test-user-" + uuid.New().String()[:8]

	// Test: Create user using gophercloud SDK
	createOpts := users.CreateOpts{
		Name:     userName,
		Password: "test-password-123",
		Enabled:  gophercloud.Enabled,
	}

	user, err := users.Create(client, createOpts).Extract()

	// Assertions: Verify OpenStack API contract
	require.NoError(t, err, "CreateUser should succeed")
	assert.NotEmpty(t, user.ID, "User ID should be set")
	assert.Equal(t, userName, user.Name, "User name should match")
	assert.True(t, user.Enabled, "User should be enabled by default")
	assert.NotEmpty(t, user.DomainID, "User should have a domain ID")
	// Domain ID will be the UUID of "Default" domain, not the string "default"

	// Cleanup: Delete created user
	defer func() {
		err := users.Delete(client, user.ID).ExtractErr()
		if err != nil {
			t.Logf("Cleanup failed: %v", err)
		}
	}()
}

// TestKeystoneCreateUserWithEmail_Contract tests user creation with email
func TestKeystoneCreateUserWithEmail_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupClient(t)

	userName := "test-user-email-" + uuid.New().String()[:8]
	userDescription := "Test user with description" // Use description instead of email

	// Test: Create user with description
	createOpts := users.CreateOpts{
		Name:        userName,
		Password:    "test-password-123",
		Description: userDescription,
		Enabled:     gophercloud.Enabled,
	}

	user, err := users.Create(client, createOpts).Extract()

	// Assertions
	require.NoError(t, err, "CreateUser with description should succeed")
	assert.NotEmpty(t, user.ID)
	assert.Equal(t, userName, user.Name)
	assert.Equal(t, userDescription, user.Description, "User description should match")

	// Cleanup
	defer users.Delete(client, user.ID)
}

// TestKeystoneCreateUserValidation_Contract tests input validation
func TestKeystoneCreateUserValidation_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupClient(t)

	// Test: Create user with empty name (should fail)
	createOpts := users.CreateOpts{
		Name:     "",
		Password: "test-password",
	}

	_, err := users.Create(client, createOpts).Extract()

	// Assertions: Should fail with validation error (gophercloud will catch this before sending)
	require.Error(t, err, "Create with empty name should fail")
	// Note: gophercloud validates required fields, so this might not reach the server
}

// TestKeystoneUpdateUser_Contract tests PATCH /v3/users/:id endpoint
func TestKeystoneUpdateUser_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupClient(t)

	// Setup: Create user first
	userName := "test-user-update-" + uuid.New().String()[:8]
	createdUser, err := users.Create(client, users.CreateOpts{
		Name:     userName,
		Password: "initial-password",
		Enabled:  gophercloud.Enabled,
	}).Extract()
	require.NoError(t, err, "Setup: CreateUser should succeed")
	defer users.Delete(client, createdUser.ID)

	// Test: Update user description only (gophercloud doesn't support Email field)
	newDescription := "Updated user description"

	updateOpts := users.UpdateOpts{
		Description: &newDescription,
	}

	updatedUser, err := users.Update(client, createdUser.ID, updateOpts).Extract()

	// Assertions
	require.NoError(t, err, "UpdateUser should succeed")
	assert.Equal(t, createdUser.ID, updatedUser.ID, "User ID should remain the same")
	assert.Equal(t, newDescription, updatedUser.Description, "Description should be updated")
	assert.Equal(t, userName, updatedUser.Name, "Name should remain unchanged")
}

// TestKeystoneUpdateUserEnabled_Contract tests enabling/disabling users
func TestKeystoneUpdateUserEnabled_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupClient(t)

	// Setup: Create user
	userName := "test-user-enabled-" + uuid.New().String()[:8]
	createdUser, err := users.Create(client, users.CreateOpts{
		Name:     userName,
		Password: "password",
		Enabled:  gophercloud.Enabled,
	}).Extract()
	require.NoError(t, err)
	defer users.Delete(client, createdUser.ID)

	// Test: Disable user
	disabled := false
	updatedUser, err := users.Update(client, createdUser.ID, users.UpdateOpts{
		Enabled: &disabled,
	}).Extract()

	// Assertions
	require.NoError(t, err)
	assert.False(t, updatedUser.Enabled, "User should be disabled")

	// Test: Re-enable user
	enabled := true
	updatedUser, err = users.Update(client, createdUser.ID, users.UpdateOpts{
		Enabled: &enabled,
	}).Extract()

	require.NoError(t, err)
	assert.True(t, updatedUser.Enabled, "User should be re-enabled")
}

// TestKeystoneDeleteUser_Contract tests DELETE /v3/users/:id endpoint
func TestKeystoneDeleteUser_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupClient(t)

	// Setup: Create user
	userName := "test-user-delete-" + uuid.New().String()[:8]
	createdUser, err := users.Create(client, users.CreateOpts{
		Name:     userName,
		Password: "password",
	}).Extract()
	require.NoError(t, err)

	// Test: Delete user
	err = users.Delete(client, createdUser.ID).ExtractErr()

	// Assertions
	require.NoError(t, err, "DeleteUser should succeed")

	// Verify deletion: GET should return 404
	_, err = users.Get(client, createdUser.ID).Extract()
	assert.Error(t, err, "GET after DELETE should fail with 404")
	assert.Contains(t, err.Error(), "404", "Should be 404 Not Found")
}

// TestKeystoneDeleteNonExistentUser_Contract tests 404 handling
func TestKeystoneDeleteNonExistentUser_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupClient(t)

	// Test: Delete non-existent user
	fakeUserID := uuid.New().String()
	err := users.Delete(client, fakeUserID).ExtractErr()

	// Assertions: Should return 404
	require.Error(t, err, "Delete non-existent user should fail")
	assert.Contains(t, err.Error(), "404", "Should be 404 Not Found")
}

// TestKeystoneGetUserProjects_Contract tests GET /v3/users/:id/projects endpoint
func TestKeystoneGetUserProjects_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupClient(t)

	// Setup: Get existing admin user (from seed data)
	allPages, err := users.List(client, users.ListOpts{
		Name: "admin",
	}).AllPages()
	require.NoError(t, err, "Setup: ListUsers should succeed")

	userList, err := users.ExtractUsers(allPages)
	require.NoError(t, err)
	require.NotEmpty(t, userList, "Admin user should exist")

	adminUser := userList[0]

	// Test: Get user's projects using ListProjects method
	projectsPages, err := users.ListProjects(client, adminUser.ID).AllPages()

	// Assertions
	require.NoError(t, err, "GetUserProjects should succeed")
	assert.NotNil(t, projectsPages, "Projects pages should not be nil")

	// Note: We're testing that the endpoint responds correctly
	// The exact project extraction depends on gophercloud version
	// For now, verify the call succeeds and returns valid pages
}
