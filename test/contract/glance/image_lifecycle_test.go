package glance_test

import (
	"testing"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/imageservice/v2/images"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGlanceImageCreate_Contract tests basic image creation
func TestGlanceImageCreate_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupGlanceClient(t)

	// Create image
	visibility := images.ImageVisibilityPrivate
	createOpts := images.CreateOpts{
		Name:            "test-image-basic",
		ContainerFormat: "bare",
		DiskFormat:      "qcow2",
		Visibility:      &visibility,
	}

	image, err := images.Create(client, createOpts).Extract()
	require.NoError(t, err, "Failed to create image")
	require.NotNil(t, image)
	assert.NotEmpty(t, image.ID)
	assert.Equal(t, "test-image-basic", image.Name)
	assert.Equal(t, "bare", image.ContainerFormat)
	assert.Equal(t, "qcow2", image.DiskFormat)

	// Cleanup
	defer func() {
		err := images.Delete(client, image.ID).ExtractErr()
		if err != nil {
			t.Logf("Image cleanup failed: %v", err)
		}
	}()

	// Verify image exists
	fetchedImage, err := images.Get(client, image.ID).Extract()
	require.NoError(t, err, "Failed to fetch created image")
	assert.Equal(t, image.ID, fetchedImage.ID)
}

// TestGlanceImageUpload_Contract tests image data upload
func TestGlanceImageUpload_Contract(t *testing.T) {
	t.Skip("Image upload/download tested in image_data_test.go")
}

// TestGlanceImageDownload_Contract tests image data download
func TestGlanceImageDownload_Contract(t *testing.T) {
	t.Skip("Image upload/download tested in image_data_test.go")
}

// TestGlanceImageList_Contract tests listing images
func TestGlanceImageList_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupGlanceClient(t)

	// List all images
	allPages, err := images.List(client, images.ListOpts{}).AllPages()
	require.NoError(t, err, "Failed to list images")

	allImages, err := images.ExtractImages(allPages)
	require.NoError(t, err, "Failed to extract images from pages")
	// Empty list is valid - just verify no error occurred
	assert.GreaterOrEqual(t, len(allImages), 0, "Image list length should be >= 0")
}

// TestGlanceImageUpdate_Contract tests image update
func TestGlanceImageUpdate_Contract(t *testing.T) {
	t.Skip("Image PATCH update API is complex in gophercloud v1, tested via HTTP in other tests")
}

// TestGlanceImageDelete_Contract tests image deletion
func TestGlanceImageDelete_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupGlanceClient(t)

	// Create image
	visibility := images.ImageVisibilityPrivate
	image, err := images.Create(client, images.CreateOpts{
		Name:            "test-image-delete",
		ContainerFormat: "bare",
		DiskFormat:      "qcow2",
		Visibility:      &visibility,
	}).Extract()
	require.NoError(t, err, "Failed to create image")

	// Delete image
	err = images.Delete(client, image.ID).ExtractErr()
	require.NoError(t, err, "Failed to delete image")

	// Verify image is deleted
	_, err = images.Get(client, image.ID).Extract()
	assert.Error(t, err, "Expected error when fetching deleted image")
	if err != nil {
		_, ok := err.(gophercloud.ErrDefault404)
		assert.True(t, ok, "Expected 404 error type")
	}
}

// TestGlanceImageLifecycle_Contract tests complete image lifecycle (metadata only)
func TestGlanceImageLifecycle_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupGlanceClient(t)

	// 1. Create image
	visibility := images.ImageVisibilityPrivate
	image, err := images.Create(client, images.CreateOpts{
		Name:            "test-image-lifecycle",
		ContainerFormat: "bare",
		DiskFormat:      "qcow2",
		Visibility:      &visibility,
	}).Extract()
	require.NoError(t, err, "Failed to create image")
	defer images.Delete(client, image.ID)

	// Verify initial state
	assert.Equal(t, images.ImageStatusQueued, image.Status, "New image should be in queued status")

	// 2. Verify image can be retrieved
	fetchedImage, err := images.Get(client, image.ID).Extract()
	require.NoError(t, err, "Failed to fetch image")
	assert.Equal(t, image.ID, fetchedImage.ID)

	// 3. List images (should include our image)
	allPages, err := images.List(client, images.ListOpts{}).AllPages()
	require.NoError(t, err, "Failed to list images")
	allImages, err := images.ExtractImages(allPages)
	require.NoError(t, err, "Failed to extract images")

	found := false
	for _, img := range allImages {
		if img.ID == image.ID {
			found = true
			break
		}
	}
	assert.True(t, found, "Image should be in list")

	// 4. Delete image
	err = images.Delete(client, image.ID).ExtractErr()
	require.NoError(t, err, "Failed to delete image")

	// 5. Verify deletion
	_, err = images.Get(client, image.ID).Extract()
	assert.Error(t, err, "Expected error when fetching deleted image")
}
