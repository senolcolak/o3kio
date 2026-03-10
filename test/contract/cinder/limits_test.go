package cinder_test

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupCinderClient(t *testing.T) *gophercloud.ServiceClient {
	t.Helper()

	authURL := getEnvOrDefault("OS_AUTH_URL", "http://localhost:35357/v3")
	username := getEnvOrDefault("OS_USERNAME", "admin")
	password := getEnvOrDefault("OS_PASSWORD", "secret")
	projectName := getEnvOrDefault("OS_PROJECT_NAME", "default")
	domainName := getEnvOrDefault("OS_USER_DOMAIN_NAME", "Default")

	opts := gophercloud.AuthOptions{
		IdentityEndpoint: authURL,
		Username:         username,
		Password:         password,
		TenantName:       projectName,
		DomainName:       domainName,
	}

	provider, err := openstack.AuthenticatedClient(opts)
	require.NoError(t, err, "Failed to create authenticated client")

	client, err := openstack.NewBlockStorageV3(provider, gophercloud.EndpointOpts{})
	require.NoError(t, err, "Failed to create Cinder client")

	return client
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func skipIfO3KNotRunning(t *testing.T) {
	t.Helper()
	if os.Getenv("SKIP_CONTRACT_TESTS") == "1" {
		t.Skip("Skipping contract test (O3K not running)")
	}
}

func TestCinderGetLimits_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupCinderClient(t)

	// Get limits using direct HTTP call (gophercloud doesn't have limits package)
	url := client.ServiceURL("limits")
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var result struct {
		Limits struct {
			Rate     []interface{} `json:"rate"`
			Absolute struct {
				MaxTotalVolumes          int `json:"maxTotalVolumes"`
				MaxTotalSnapshots        int `json:"maxTotalSnapshots"`
				MaxTotalVolumeGigabytes  int `json:"maxTotalVolumeGigabytes"`
				TotalVolumesUsed         int `json:"totalVolumesUsed"`
				TotalSnapshotsUsed       int `json:"totalSnapshotsUsed"`
				TotalGigabytesUsed       int `json:"totalGigabytesUsed"`
			} `json:"absolute"`
		} `json:"limits"`
	}

	err = json.Unmarshal(body, &result)
	require.NoError(t, err)

	// Verify absolute limits
	assert.GreaterOrEqual(t, result.Limits.Absolute.MaxTotalVolumes, 0)
	assert.GreaterOrEqual(t, result.Limits.Absolute.MaxTotalSnapshots, 0)
	assert.GreaterOrEqual(t, result.Limits.Absolute.MaxTotalVolumeGigabytes, 0)
	assert.GreaterOrEqual(t, result.Limits.Absolute.TotalVolumesUsed, 0)
	assert.GreaterOrEqual(t, result.Limits.Absolute.TotalSnapshotsUsed, 0)
	assert.GreaterOrEqual(t, result.Limits.Absolute.TotalGigabytesUsed, 0)
}
