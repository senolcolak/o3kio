package glance_test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/gophercloud/gophercloud/openstack/imageservice/v2/images"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGlanceListCachedImages_Contract tests GET /v2/cache/images
func TestGlanceListCachedImages_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupGlanceClient(t)

	url := client.ServiceURL("cache", "images")
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		CachedImages []map[string]interface{} `json:"cached_images"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.NotNil(t, result.CachedImages)
}

// TestGlancePrefetchImage_Contract tests PUT /v2/cache/images/:id
func TestGlancePrefetchImage_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupGlanceClient(t)

	// Create a test image first
	visibility := images.ImageVisibilityPrivate
	createOpts := images.CreateOpts{
		Name:            "cache-test-image",
		ContainerFormat: "bare",
		DiskFormat:      "qcow2",
		Visibility:      &visibility,
	}

	image, err := images.Create(client, createOpts).Extract()
	require.NoError(t, err)
	defer images.Delete(client, image.ID)

	// Prefetch the image into cache
	url := client.ServiceURL("cache", "images", image.ID)
	req, err := http.NewRequest("PUT", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusAccepted, resp.StatusCode)
}

// TestGlanceDeleteCachedImage_Contract tests DELETE /v2/cache/images/:id
func TestGlanceDeleteCachedImage_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupGlanceClient(t)

	// Create and prefetch an image
	visibility := images.ImageVisibilityPrivate
	createOpts := images.CreateOpts{
		Name:            "cache-delete-test",
		ContainerFormat: "bare",
		DiskFormat:      "qcow2",
		Visibility:      &visibility,
	}

	image, err := images.Create(client, createOpts).Extract()
	require.NoError(t, err)
	defer images.Delete(client, image.ID)

	// Prefetch first
	prefetchURL := client.ServiceURL("cache", "images", image.ID)
	prefetchReq, _ := http.NewRequest("PUT", prefetchURL, nil)
	prefetchReq.Header.Set("X-Auth-Token", client.TokenID)
	http.DefaultClient.Do(prefetchReq)

	// Delete from cache
	url := client.ServiceURL("cache", "images", image.ID)
	req, err := http.NewRequest("DELETE", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}

// TestGlanceClearCache_Contract tests DELETE /v2/cache/images (clear all)
func TestGlanceClearCache_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupGlanceClient(t)

	url := client.ServiceURL("cache", "images")
	req, err := http.NewRequest("DELETE", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}
