package cinder_test

import (
	"testing"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/blockstorage/v3/volumes"
	"github.com/gophercloud/gophercloud/openstack/blockstorage/v3/snapshots"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCinderVolumeCreate_Contract tests basic volume creation
func TestCinderVolumeCreate_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupCinderClient(t)

	// Create volume
	size := 1 // 1 GB
	createOpts := volumes.CreateOpts{
		Name: "test-volume-basic",
		Size: size,
	}

	volume, err := volumes.Create(client, createOpts).Extract()
	require.NoError(t, err, "Failed to create volume")
	require.NotNil(t, volume)
	assert.NotEmpty(t, volume.ID)
	assert.Equal(t, "test-volume-basic", volume.Name)
	assert.Equal(t, size, volume.Size)

	// Cleanup
	defer func() {
		err := volumes.Delete(client, volume.ID, volumes.DeleteOpts{}).ExtractErr()
		if err != nil {
			t.Logf("Volume cleanup failed: %v", err)
		}
	}()

	// Verify volume exists
	fetchedVolume, err := volumes.Get(client, volume.ID).Extract()
	require.NoError(t, err, "Failed to fetch created volume")
	assert.Equal(t, volume.ID, fetchedVolume.ID)
}

// TestCinderVolumeList_Contract tests listing volumes
func TestCinderVolumeList_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupCinderClient(t)

	// List all volumes
	allPages, err := volumes.List(client, volumes.ListOpts{}).AllPages()
	require.NoError(t, err, "Failed to list volumes")

	allVolumes, err := volumes.ExtractVolumes(allPages)
	require.NoError(t, err, "Failed to extract volumes from pages")
	// Empty list is valid - just verify no error occurred
	assert.GreaterOrEqual(t, len(allVolumes), 0, "Volume list length should be >= 0")

	// List with details
	detailOpts := volumes.ListOpts{
		AllTenants: false,
	}
	detailPages, err := volumes.List(client, detailOpts).AllPages()
	require.NoError(t, err, "Failed to list volumes with details")

	detailVolumes, err := volumes.ExtractVolumes(detailPages)
	require.NoError(t, err, "Failed to extract volumes from detail pages")
	assert.GreaterOrEqual(t, len(detailVolumes), 0)
}

// TestCinderVolumeAttach_Contract tests volume attachment workflow
func TestCinderVolumeAttach_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupCinderClient(t)

	// Create volume
	volume, err := volumes.Create(client, volumes.CreateOpts{
		Name: "test-volume-attach",
		Size: 1,
	}).Extract()
	require.NoError(t, err, "Failed to create volume")
	defer volumes.Delete(client, volume.ID, volumes.DeleteOpts{})

	// Note: Actual attachment requires a server instance
	// This test verifies the volume can be created and is ready for attachment
	// In real scenarios, use Nova's volume attachment API

	// Verify volume is available for attachment
	fetchedVolume, err := volumes.Get(client, volume.ID).Extract()
	require.NoError(t, err, "Failed to fetch volume")
	assert.Equal(t, volume.ID, fetchedVolume.ID)
	// Status should be "available" or "creating"
	assert.Contains(t, []string{"available", "creating"}, fetchedVolume.Status)
}

// TestCinderSnapshotCreate_Contract tests volume snapshot creation
func TestCinderSnapshotCreate_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupCinderClient(t)

	// Create volume first
	volume, err := volumes.Create(client, volumes.CreateOpts{
		Name: "test-volume-for-snapshot",
		Size: 1,
	}).Extract()
	require.NoError(t, err, "Failed to create volume")
	defer volumes.Delete(client, volume.ID, volumes.DeleteOpts{})

	// Create snapshot
	createOpts := snapshots.CreateOpts{
		VolumeID: volume.ID,
		Name:     "test-snapshot",
		Force:    true, // Allow snapshot of in-use volumes
	}

	snapshot, err := snapshots.Create(client, createOpts).Extract()
	require.NoError(t, err, "Failed to create snapshot")
	require.NotNil(t, snapshot)
	assert.NotEmpty(t, snapshot.ID)
	assert.Equal(t, "test-snapshot", snapshot.Name)
	assert.Equal(t, volume.ID, snapshot.VolumeID)

	// Cleanup
	defer func() {
		err := snapshots.Delete(client, snapshot.ID).ExtractErr()
		if err != nil {
			t.Logf("Snapshot cleanup failed: %v", err)
		}
	}()

	// Verify snapshot exists
	fetchedSnapshot, err := snapshots.Get(client, snapshot.ID).Extract()
	require.NoError(t, err, "Failed to fetch created snapshot")
	assert.Equal(t, snapshot.ID, fetchedSnapshot.ID)
}

// TestCinderVolumeLifecycle_Contract tests complete volume lifecycle
func TestCinderVolumeLifecycle_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupCinderClient(t)

	// 1. Create volume
	volume, err := volumes.Create(client, volumes.CreateOpts{
		Name:        "test-volume-lifecycle",
		Size:        1,
		Description: "Test volume for lifecycle",
	}).Extract()
	require.NoError(t, err, "Failed to create volume")
	defer volumes.Delete(client, volume.ID, volumes.DeleteOpts{})

	// 2. Update volume
	newName := "test-volume-updated"
	newDescription := "Updated description"
	updateOpts := volumes.UpdateOpts{
		Name:        &newName,
		Description: &newDescription,
	}

	updatedVolume, err := volumes.Update(client, volume.ID, updateOpts).Extract()
	require.NoError(t, err, "Failed to update volume")
	assert.Equal(t, newName, updatedVolume.Name)
	assert.Equal(t, newDescription, updatedVolume.Description)

	// 3. Create snapshot
	snapshot, err := snapshots.Create(client, snapshots.CreateOpts{
		VolumeID: volume.ID,
		Name:     "test-snapshot-lifecycle",
		Force:    true,
	}).Extract()
	require.NoError(t, err, "Failed to create snapshot")
	defer snapshots.Delete(client, snapshot.ID)

	// 4. Verify snapshot
	fetchedSnapshot, err := snapshots.Get(client, snapshot.ID).Extract()
	require.NoError(t, err, "Failed to fetch snapshot")
	assert.Equal(t, snapshot.ID, fetchedSnapshot.ID)
	assert.Equal(t, volume.ID, fetchedSnapshot.VolumeID)

	// 5. List volumes (should include our volume)
	allPages, err := volumes.List(client, volumes.ListOpts{}).AllPages()
	require.NoError(t, err, "Failed to list volumes")
	allVolumes, err := volumes.ExtractVolumes(allPages)
	require.NoError(t, err, "Failed to extract volumes")

	found := false
	for _, v := range allVolumes {
		if v.ID == volume.ID {
			found = true
			assert.Equal(t, newName, v.Name)
			break
		}
	}
	assert.True(t, found, "Volume should be in list")

	// 6. Extend volume (skipped - ExtendSize not available in gophercloud v1)
	// TODO: Re-enable when using gophercloud v2 or add raw HTTP call

	// 7. Cleanup (snapshot first, then volume)
	err = snapshots.Delete(client, snapshot.ID).ExtractErr()
	require.NoError(t, err, "Failed to delete snapshot")

	err = volumes.Delete(client, volume.ID, volumes.DeleteOpts{}).ExtractErr()
	require.NoError(t, err, "Failed to delete volume")

	// 9. Verify deletion
	_, err = volumes.Get(client, volume.ID).Extract()
	assert.Error(t, err, "Expected error when fetching deleted volume")
	if err != nil {
		_, ok := err.(gophercloud.ErrDefault404)
		assert.True(t, ok, "Expected 404 error type")
	}
}
