package keystone_test

import (
	"testing"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/identity/v3/groups"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupKeystoneClient(t *testing.T) *gophercloud.ServiceClient {
	return setupClient(t)
}

func TestKeystoneGroupsCRUD_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupKeystoneClient(t)

	// Create group
	createOpts := groups.CreateOpts{
		Name:        "test-group",
		Description: "Test group for contract testing",
	}

	group, err := groups.Create(client, createOpts).Extract()
	require.NoError(t, err)
	require.NotNil(t, group)
	assert.NotEmpty(t, group.ID)
	assert.Equal(t, "test-group", group.Name)
	assert.Equal(t, "Test group for contract testing", group.Description)

	// Cleanup
	defer func() {
		err := groups.Delete(client, group.ID).ExtractErr()
		if err != nil {
			t.Logf("Cleanup failed: %v", err)
		}
	}()

	// Get group
	fetchedGroup, err := groups.Get(client, group.ID).Extract()
	require.NoError(t, err)
	assert.Equal(t, group.ID, fetchedGroup.ID)
	assert.Equal(t, group.Name, fetchedGroup.Name)

	// List groups
	allPages, err := groups.List(client, groups.ListOpts{}).AllPages()
	require.NoError(t, err)

	groupList, err := groups.ExtractGroups(allPages)
	require.NoError(t, err)
	assert.NotEmpty(t, groupList)

	// Find our group
	found := false
	for _, g := range groupList {
		if g.ID == group.ID {
			found = true
			break
		}
	}
	assert.True(t, found, "Created group should appear in list")

	// Update group
	desc := "Updated description"
	updateOpts := groups.UpdateOpts{
		Description: &desc,
	}
	updatedGroup, err := groups.Update(client, group.ID, updateOpts).Extract()
	require.NoError(t, err)
	assert.Equal(t, "Updated description", updatedGroup.Description)

	// Delete
	err = groups.Delete(client, group.ID).ExtractErr()
	require.NoError(t, err)

	// Verify deletion
	_, err = groups.Get(client, group.ID).Extract()
	assert.Error(t, err, "GET after DELETE should fail")
}
