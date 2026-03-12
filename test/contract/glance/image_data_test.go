package glance_test

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/gophercloud/gophercloud/openstack/imageservice/v2/images"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGlanceUploadImageData_Contract tests PUT /v2/images/:id/file
func TestGlanceUploadImageData_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupGlanceClient(t)

	// Create an image first
	image, err := images.Create(client, images.CreateOpts{
		Name:            "test-upload-image",
		ContainerFormat: "bare",
		DiskFormat:      "raw",
	}).Extract()
	require.NoError(t, err)
	defer images.Delete(client, image.ID)

	// Upload image data
	imageData := []byte("fake image data content for testing")
	url := client.ServiceURL("images", image.ID, "file")
	req, err := http.NewRequest("PUT", url, bytes.NewReader(imageData))
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	// Verify image status changed to active
	updatedImage, err := images.Get(client, image.ID).Extract()
	require.NoError(t, err)
	assert.Equal(t, images.ImageStatusActive, updatedImage.Status)
}

// TestGlanceDownloadImageData_Contract tests GET /v2/images/:id/file
func TestGlanceDownloadImageData_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupGlanceClient(t)

	// Create an image and upload data
	image, err := images.Create(client, images.CreateOpts{
		Name:            "test-download-image",
		ContainerFormat: "bare",
		DiskFormat:      "raw",
	}).Extract()
	require.NoError(t, err)
	defer images.Delete(client, image.ID)

	// Upload image data
	imageData := []byte("fake image data for download test")
	uploadURL := client.ServiceURL("images", image.ID, "file")
	uploadReq, _ := http.NewRequest("PUT", uploadURL, bytes.NewReader(imageData))
	uploadReq.Header.Set("X-Auth-Token", client.TokenID)
	uploadReq.Header.Set("Content-Type", "application/octet-stream")
	uploadResp, _ := http.DefaultClient.Do(uploadReq)
	uploadResp.Body.Close()

	// Download image data
	url := client.ServiceURL("images", image.ID, "file")
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify downloaded data
	downloadedData, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, imageData, downloadedData)
}

// TestGlanceDownloadNonExistentImage_Contract tests GET /v2/images/:id/file for non-existent image
func TestGlanceDownloadNonExistentImage_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupGlanceClient(t)

	// Try to download non-existent image
	url := client.ServiceURL("images", "00000000-0000-0000-0000-999999999999", "file")
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}
