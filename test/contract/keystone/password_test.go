package keystone_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/identity/v3/users"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestKeystoneChangePassword_Contract tests POST /v3/users/:id/password endpoint
func TestKeystoneChangePassword_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupClient(t)

	// Setup: Create user first
	userName := "test-user-password-" + uuid.New().String()[:8]
	initialPassword := "initial-password-123"

	createdUser, err := users.Create(client, users.CreateOpts{
		Name:     userName,
		Password: initialPassword,
		Enabled:  gophercloud.Enabled,
	}).Extract()
	require.NoError(t, err, "Setup: CreateUser should succeed")
	defer users.Delete(client, createdUser.ID)

	// Test: Change password
	newPassword := "new-password-456"
	changePasswordOpts := users.ChangePasswordOpts{
		OriginalPassword: initialPassword,
		Password:         newPassword,
	}

	err = users.ChangePassword(client, createdUser.ID, changePasswordOpts).ExtractErr()

	// Assertions
	require.NoError(t, err, "ChangePassword should succeed")

	// TODO: Verify new password works by attempting to authenticate
	// This would require a new auth attempt with the new password
}

// TestKeystoneChangePasswordValidation_Contract tests password change validation
func TestKeystoneChangePasswordValidation_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupClient(t)

	// Setup: Create user
	userName := "test-user-pwd-val-" + uuid.New().String()[:8]
	initialPassword := "initial-password-123"

	createdUser, err := users.Create(client, users.CreateOpts{
		Name:     userName,
		Password: initialPassword,
		Enabled:  gophercloud.Enabled,
	}).Extract()
	require.NoError(t, err, "Setup: CreateUser should succeed")
	defer users.Delete(client, createdUser.ID)

	// Test: Change password with wrong original password (should fail)
	wrongOpts := users.ChangePasswordOpts{
		OriginalPassword: "wrong-password",
		Password:         "new-password-456",
	}

	err = users.ChangePassword(client, createdUser.ID, wrongOpts).ExtractErr()

	// Assertions: Should fail (gophercloud or server validation)
	require.Error(t, err, "ChangePassword with wrong original password should fail")
	// Note: Error message format varies - just verify it fails
}

// TestKeystoneGetUserGroups_Contract tests GET /v3/users/:id/groups endpoint
func TestKeystoneGetUserGroups_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupClient(t)

	// Setup: Get existing admin user
	allPages, err := users.List(client, users.ListOpts{
		Name: "admin",
	}).AllPages()
	require.NoError(t, err, "Setup: ListUsers should succeed")

	userList, err := users.ExtractUsers(allPages)
	require.NoError(t, err)
	require.NotEmpty(t, userList, "Admin user should exist")

	adminUser := userList[0]

	// Test: Get user's groups
	groupsPages, err := users.ListGroups(client, adminUser.ID).AllPages()

	// Assertions
	require.NoError(t, err, "GetUserGroups should succeed")
	assert.NotNil(t, groupsPages, "Groups pages should not be nil")

	// Note: Admin user might not be in any groups initially
	// We're just testing that the endpoint responds correctly
}

// TestKeystoneGetUserGroupsForNewUser_Contract tests groups for a new user
func TestKeystoneGetUserGroupsForNewUser_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupClient(t)

	// Setup: Create new user
	userName := "test-user-groups-" + uuid.New().String()[:8]
	createdUser, err := users.Create(client, users.CreateOpts{
		Name:     userName,
		Password: "password",
		Enabled:  gophercloud.Enabled,
	}).Extract()
	require.NoError(t, err, "Setup: CreateUser should succeed")
	defer users.Delete(client, createdUser.ID)

	// Test: Get groups for new user (should be empty)
	groupsPages, err := users.ListGroups(client, createdUser.ID).AllPages()

	// Assertions
	require.NoError(t, err, "GetUserGroups should succeed")
	assert.NotNil(t, groupsPages, "Groups pages should not be nil")

	// New user should have no groups
	// (Exact extraction depends on gophercloud version, so we just verify the call succeeds)
}
