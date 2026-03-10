package nova_test

import (
	"testing"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/flavors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNovaFlavorCreate_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Create flavor
	createOpts := flavors.CreateOpts{
		Name:  "test-flavor",
		RAM:   1024,
		VCPUs: 1,
		Disk:  gophercloud.IntToPointer(10),
	}

	flavor, err := flavors.Create(client, createOpts).Extract()
	require.NoError(t, err)
	require.NotNil(t, flavor)
	assert.NotEmpty(t, flavor.ID)
	assert.Equal(t, "test-flavor", flavor.Name)
	assert.Equal(t, 1024, flavor.RAM)
	assert.Equal(t, 1, flavor.VCPUs)
	assert.Equal(t, 10, flavor.Disk)

	// Cleanup
	defer func() {
		err := flavors.Delete(client, flavor.ID).ExtractErr()
		if err != nil {
			t.Logf("Cleanup failed: %v", err)
		}
	}()

	// Get flavor
	fetchedFlavor, err := flavors.Get(client, flavor.ID).Extract()
	require.NoError(t, err)
	assert.Equal(t, flavor.ID, fetchedFlavor.ID)
	assert.Equal(t, flavor.Name, fetchedFlavor.Name)

	// List flavors (should include our new flavor)
	allPages, err := flavors.ListDetail(client, flavors.ListOpts{}).AllPages()
	require.NoError(t, err)

	flavorList, err := flavors.ExtractFlavors(allPages)
	require.NoError(t, err)
	assert.NotEmpty(t, flavorList)

	// Find our flavor
	found := false
	for _, f := range flavorList {
		if f.ID == flavor.ID {
			found = true
			break
		}
	}
	assert.True(t, found, "Created flavor should appear in list")

	// Delete
	err = flavors.Delete(client, flavor.ID).ExtractErr()
	require.NoError(t, err)

	// Verify deletion
	_, err = flavors.Get(client, flavor.ID).Extract()
	assert.Error(t, err, "GET after DELETE should fail")
}
