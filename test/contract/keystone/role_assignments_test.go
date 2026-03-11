package keystone_test

import (
	"testing"

	"github.com/gophercloud/gophercloud/openstack/identity/v3/roles"
	"github.com/gophercloud/gophercloud/openstack/identity/v3/users"
	"github.com/gophercloud/gophercloud/openstack/identity/v3/projects"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestKeystoneAssignRoleToUser_Contract tests PUT /v3/projects/:pid/users/:uid/roles/:rid
func TestKeystoneAssignRoleToUser_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupClient(t)

	// Setup: Create user, project, and role
	user, err := users.Create(client, users.CreateOpts{
		Name:     "test-assign-user",
		Password: "password",
	}).Extract()
	require.NoError(t, err)
	defer users.Delete(client, user.ID)

	project, err := projects.Create(client, projects.CreateOpts{
		Name:     "test-assign-project",
		DomainID: "00000000-0000-0000-0000-000000000100",
	}).Extract()
	require.NoError(t, err)
	defer projects.Delete(client, project.ID)

	role, err := roles.Create(client, roles.CreateOpts{
		Name: "test-assign-role",
	}).Extract()
	require.NoError(t, err)
	defer roles.Delete(client, role.ID)

	// Test: Assign role to user on project
	err = roles.Assign(client, role.ID, roles.AssignOpts{
		UserID:    user.ID,
		ProjectID: project.ID,
	}).ExtractErr()

	// Assertions
	require.NoError(t, err, "AssignRole should succeed")

	// Verify assignment exists
	allPages, err := roles.ListAssignments(client, roles.ListAssignmentsOpts{
		UserID:         user.ID,
		ScopeProjectID: project.ID,
	}).AllPages()
	require.NoError(t, err)

	assignments, err := roles.ExtractRoleAssignments(allPages)
	require.NoError(t, err)

	found := false
	for _, assignment := range assignments {
		if assignment.Role.ID == role.ID {
			found = true
			break
		}
	}
	assert.True(t, found, "Role assignment should be found")
}

// TestKeystoneUnassignRoleFromUser_Contract tests DELETE /v3/projects/:pid/users/:uid/roles/:rid
func TestKeystoneUnassignRoleFromUser_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupClient(t)

	// Setup: Create and assign role
	user, err := users.Create(client, users.CreateOpts{
		Name:     "test-unassign-user",
		Password: "password",
	}).Extract()
	require.NoError(t, err)
	defer users.Delete(client, user.ID)

	project, err := projects.Create(client, projects.CreateOpts{
		Name:     "test-unassign-project",
		DomainID: "00000000-0000-0000-0000-000000000100",
	}).Extract()
	require.NoError(t, err)
	defer projects.Delete(client, project.ID)

	role, err := roles.Create(client, roles.CreateOpts{
		Name: "test-unassign-role",
	}).Extract()
	require.NoError(t, err)
	defer roles.Delete(client, role.ID)

	// Assign role first
	err = roles.Assign(client, role.ID, roles.AssignOpts{
		UserID:    user.ID,
		ProjectID: project.ID,
	}).ExtractErr()
	require.NoError(t, err)

	// Test: Unassign role
	err = roles.Unassign(client, role.ID, roles.UnassignOpts{
		UserID:    user.ID,
		ProjectID: project.ID,
	}).ExtractErr()

	// Assertions
	require.NoError(t, err, "UnassignRole should succeed")
}

// TestKeystoneListUserRoles_Contract tests GET /v3/projects/:pid/users/:uid/roles
func TestKeystoneListUserRoles_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupClient(t)

	// Setup: Create user, project, and assign role
	user, err := users.Create(client, users.CreateOpts{
		Name:     "test-list-roles-user",
		Password: "password",
	}).Extract()
	require.NoError(t, err)
	defer users.Delete(client, user.ID)

	project, err := projects.Create(client, projects.CreateOpts{
		Name:     "test-list-roles-project",
		DomainID: "00000000-0000-0000-0000-000000000100",
	}).Extract()
	require.NoError(t, err)
	defer projects.Delete(client, project.ID)

	role, err := roles.Create(client, roles.CreateOpts{
		Name: "test-list-roles-role",
	}).Extract()
	require.NoError(t, err)
	defer roles.Delete(client, role.ID)

	// Assign role
	err = roles.Assign(client, role.ID, roles.AssignOpts{
		UserID:    user.ID,
		ProjectID: project.ID,
	}).ExtractErr()
	require.NoError(t, err)

	// Test: List user roles on project
	allPages, err := roles.ListAssignments(client, roles.ListAssignmentsOpts{
		UserID:         user.ID,
		ScopeProjectID: project.ID,
	}).AllPages()
	require.NoError(t, err)

	assignments, err := roles.ExtractRoleAssignments(allPages)
	require.NoError(t, err)

	// Assertions
	assert.NotEmpty(t, assignments, "Should have at least one role assignment")

	found := false
	for _, assignment := range assignments {
		if assignment.Role.ID == role.ID {
			found = true
			break
		}
	}
	assert.True(t, found, "Assigned role should be in list")
}

// TestKeystoneCheckRoleAssignment_Contract tests GET /v3/projects/:pid/users/:uid/roles/:rid
func TestKeystoneCheckRoleAssignment_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupClient(t)

	// Setup: Create and assign role
	user, err := users.Create(client, users.CreateOpts{
		Name:     "test-check-user",
		Password: "password",
	}).Extract()
	require.NoError(t, err)
	defer users.Delete(client, user.ID)

	project, err := projects.Create(client, projects.CreateOpts{
		Name:     "test-check-project",
		DomainID: "00000000-0000-0000-0000-000000000100",
	}).Extract()
	require.NoError(t, err)
	defer projects.Delete(client, project.ID)

	role, err := roles.Create(client, roles.CreateOpts{
		Name: "test-check-role",
	}).Extract()
	require.NoError(t, err)
	defer roles.Delete(client, role.ID)

	// Assign role
	err = roles.Assign(client, role.ID, roles.AssignOpts{
		UserID:    user.ID,
		ProjectID: project.ID,
	}).ExtractErr()
	require.NoError(t, err)

	// Test: Check if role is assigned (gophercloud doesn't have direct check, use list)
	allPages, err := roles.ListAssignments(client, roles.ListAssignmentsOpts{
		UserID:         user.ID,
		ScopeProjectID: project.ID,
		RoleID:         role.ID,
	}).AllPages()
	require.NoError(t, err)

	assignments, err := roles.ExtractRoleAssignments(allPages)
	require.NoError(t, err)

	// Assertions
	assert.NotEmpty(t, assignments, "Role assignment should exist")
}
