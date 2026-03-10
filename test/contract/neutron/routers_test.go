package neutron_test

import (
	"testing"

	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/layer3/routers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNeutronRouterCRUD_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNeutronClient(t)

	// Create router
	adminStateUp := true
	createOpts := routers.CreateOpts{
		Name:         "test-router",
		AdminStateUp: &adminStateUp,
	}

	router, err := routers.Create(client, createOpts).Extract()
	require.NoError(t, err)
	require.NotNil(t, router)
	assert.NotEmpty(t, router.ID)
	assert.Equal(t, "test-router", router.Name)

	// Cleanup
	defer func() {
		err := routers.Delete(client, router.ID).ExtractErr()
		if err != nil {
			t.Logf("Cleanup failed: %v", err)
		}
	}()

	// Get router
	fetchedRouter, err := routers.Get(client, router.ID).Extract()
	require.NoError(t, err)
	assert.Equal(t, router.ID, fetchedRouter.ID)
	assert.Equal(t, router.Name, fetchedRouter.Name)

	// List routers
	allPages, err := routers.List(client, routers.ListOpts{}).AllPages()
	require.NoError(t, err)

	routerList, err := routers.ExtractRouters(allPages)
	require.NoError(t, err)
	assert.NotEmpty(t, routerList)

	// Find our router in the list
	found := false
	for _, r := range routerList {
		if r.ID == router.ID {
			found = true
			break
		}
	}
	assert.True(t, found, "Created router should appear in list")

	// Update router
	newName := "updated-router"
	updateOpts := routers.UpdateOpts{
		Name: newName,
	}
	updatedRouter, err := routers.Update(client, router.ID, updateOpts).Extract()
	require.NoError(t, err)
	assert.Equal(t, "updated-router", updatedRouter.Name)

	// Delete router (already deferred)
	err = routers.Delete(client, router.ID).ExtractErr()
	require.NoError(t, err)

	// Verify deletion
	_, err = routers.Get(client, router.ID).Extract()
	assert.Error(t, err, "GET after DELETE should fail")
}
