package nova_test

import (
	"testing"

	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/servergroups"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNovaServerGroupsCRUD_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Create server group
	createOpts := servergroups.CreateOpts{
		Name:     "test-group",
		Policies: []string{"anti-affinity"},
	}

	group, err := servergroups.Create(client, createOpts).Extract()
	require.NoError(t, err)
	require.NotNil(t, group)
	assert.NotEmpty(t, group.ID)
	assert.Equal(t, "test-group", group.Name)
	assert.Contains(t, group.Policies, "anti-affinity")

	// Cleanup
	defer func() {
		err := servergroups.Delete(client, group.ID).ExtractErr()
		if err != nil {
			t.Logf("Cleanup failed: %v", err)
		}
	}()

	// Get server group
	fetchedGroup, err := servergroups.Get(client, group.ID).Extract()
	require.NoError(t, err)
	assert.Equal(t, group.ID, fetchedGroup.ID)
	assert.Equal(t, group.Name, fetchedGroup.Name)

	// List server groups
	allPages, err := servergroups.List(client, servergroups.ListOpts{}).AllPages()
	require.NoError(t, err)

	groupList, err := servergroups.ExtractServerGroups(allPages)
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
	assert.True(t, found, "Created server group should appear in list")

	// Delete
	err = servergroups.Delete(client, group.ID).ExtractErr()
	require.NoError(t, err)

	// Verify deletion
	_, err = servergroups.Get(client, group.ID).Extract()
	assert.Error(t, err, "GET after DELETE should fail")
}
