package keystone_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/gophercloud/gophercloud/openstack/identity/v3/roles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestKeystoneCreateRole_Contract tests POST /v3/roles endpoint
func TestKeystoneCreateRole_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupClient(t)

	// Generate unique role name
	roleName := "test-role-" + uuid.New().String()[:8]

	// Test: Create role using gophercloud SDK
	createOpts := roles.CreateOpts{
		Name:     roleName,
		DomainID: "00000000-0000-0000-0000-000000000100", // Default domain
	}

	role, err := roles.Create(client, createOpts).Extract()

	// Assertions: Verify OpenStack API contract
	require.NoError(t, err, "CreateRole should succeed")
	assert.NotEmpty(t, role.ID, "Role ID should be set")
	assert.Equal(t, roleName, role.Name, "Role name should match")

	// Cleanup
	defer func() {
		err := roles.Delete(client, role.ID).ExtractErr()
		if err != nil {
			t.Logf("Cleanup failed: %v", err)
		}
	}()
}

// TestKeystoneUpdateRole_Contract tests PATCH /v3/roles/:id endpoint
func TestKeystoneUpdateRole_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupClient(t)

	// Setup: Create role first
	roleName := "test-role-update-" + uuid.New().String()[:8]
	createdRole, err := roles.Create(client, roles.CreateOpts{
		Name: roleName,
	}).Extract()
	require.NoError(t, err, "Setup: CreateRole should succeed")
	defer roles.Delete(client, createdRole.ID)

	// Test: Update role name
	newName := "updated-" + roleName
	updateOpts := roles.UpdateOpts{
		Name: newName,
	}

	updatedRole, err := roles.Update(client, createdRole.ID, updateOpts).Extract()

	// Assertions
	require.NoError(t, err, "UpdateRole should succeed")
	assert.Equal(t, createdRole.ID, updatedRole.ID, "Role ID should remain the same")
	assert.Equal(t, newName, updatedRole.Name, "Name should be updated")
}

// TestKeystoneUpdateRoleName_Contract tests updating role name
func TestKeystoneUpdateRoleName_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupClient(t)

	// Setup: Create role
	originalName := "test-role-rename-" + uuid.New().String()[:8]
	createdRole, err := roles.Create(client, roles.CreateOpts{
		Name: originalName,
	}).Extract()
	require.NoError(t, err)
	defer roles.Delete(client, createdRole.ID)

	// Test: Update role name
	newName := "renamed-role-" + uuid.New().String()[:8]
	updateOpts := roles.UpdateOpts{
		Name: newName,
	}

	updatedRole, err := roles.Update(client, createdRole.ID, updateOpts).Extract()

	// Assertions
	require.NoError(t, err)
	assert.Equal(t, createdRole.ID, updatedRole.ID)
	assert.Equal(t, newName, updatedRole.Name, "Name should be updated")
}

// TestKeystoneDeleteRole_Contract tests DELETE /v3/roles/:id endpoint
func TestKeystoneDeleteRole_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupClient(t)

	// Setup: Create role
	roleName := "test-role-delete-" + uuid.New().String()[:8]
	createdRole, err := roles.Create(client, roles.CreateOpts{
		Name: roleName,
	}).Extract()
	require.NoError(t, err)

	// Test: Delete role
	err = roles.Delete(client, createdRole.ID).ExtractErr()

	// Assertions
	require.NoError(t, err, "DeleteRole should succeed")

	// Verify deletion: GET should return 404
	_, err = roles.Get(client, createdRole.ID).Extract()
	assert.Error(t, err, "GET after DELETE should fail with 404")
	assert.Contains(t, err.Error(), "404", "Should be 404 Not Found")
}

// TestKeystoneDeleteNonExistentRole_Contract tests 404 handling
func TestKeystoneDeleteNonExistentRole_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupClient(t)

	// Test: Delete non-existent role
	fakeRoleID := uuid.New().String()
	err := roles.Delete(client, fakeRoleID).ExtractErr()

	// Assertions: Should return 404
	require.Error(t, err, "Delete non-existent role should fail")
	assert.Contains(t, err.Error(), "404", "Should be 404 Not Found")
}

// TestKeystoneRoleCRUD_Lifecycle tests full CRUD lifecycle
func TestKeystoneRoleCRUD_Lifecycle(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupClient(t)

	// CREATE
	roleName := "test-role-lifecycle-" + uuid.New().String()[:8]
	createOpts := roles.CreateOpts{
		Name: roleName,
	}

	role, err := roles.Create(client, createOpts).Extract()
	require.NoError(t, err)
	assert.NotEmpty(t, role.ID)

	// READ
	fetchedRole, err := roles.Get(client, role.ID).Extract()
	require.NoError(t, err)
	assert.Equal(t, role.ID, fetchedRole.ID)
	assert.Equal(t, roleName, fetchedRole.Name)

	// UPDATE
	newName := "updated-" + roleName
	updateOpts := roles.UpdateOpts{
		Name: newName,
	}
	updatedRole, err := roles.Update(client, role.ID, updateOpts).Extract()
	require.NoError(t, err)
	assert.Equal(t, newName, updatedRole.Name)

	// DELETE
	err = roles.Delete(client, role.ID).ExtractErr()
	require.NoError(t, err)

	// Verify deletion
	_, err = roles.Get(client, role.ID).Extract()
	assert.Error(t, err)
}
