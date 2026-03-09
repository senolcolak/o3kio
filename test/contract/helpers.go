package contract_test

import (
	"os"
	"testing"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/stretchr/testify/require"
)

// SetupOpenStackClient creates an authenticated OpenStack client for contract testing.
// It uses environment variables (OS_AUTH_URL, OS_USERNAME, etc.) to connect to O3K.
// This function follows the TDD pattern from Constitution Article III.
func SetupOpenStackClient(t *testing.T) *gophercloud.ProviderClient {
	t.Helper()

	authURL := getEnvOrDefault("OS_AUTH_URL", "http://localhost:35357/v3")
	username := getEnvOrDefault("OS_USERNAME", "admin")
	password := getEnvOrDefault("OS_PASSWORD", "secret")
	projectName := getEnvOrDefault("OS_PROJECT_NAME", "default")
	domainName := getEnvOrDefault("OS_USER_DOMAIN_NAME", "default")

	opts := gophercloud.AuthOptions{
		IdentityEndpoint: authURL,
		Username:         username,
		Password:         password,
		TenantName:       projectName,
		DomainName:       domainName,
	}

	provider, err := openstack.AuthenticatedClient(opts)
	require.NoError(t, err, "Failed to create authenticated OpenStack client")

	return provider
}

// SetupIdentityV3Client creates a Keystone v3 client for contract testing.
func SetupIdentityV3Client(t *testing.T) *gophercloud.ServiceClient {
	t.Helper()

	provider := SetupOpenStackClient(t)
	client, err := openstack.NewIdentityV3(provider, gophercloud.EndpointOpts{})
	require.NoError(t, err, "Failed to create Keystone v3 client")

	return client
}

// SetupComputeV2Client creates a Nova v2.1 client for contract testing.
func SetupComputeV2Client(t *testing.T) *gophercloud.ServiceClient {
	t.Helper()

	provider := SetupOpenStackClient(t)
	client, err := openstack.NewComputeV2(provider, gophercloud.EndpointOpts{})
	require.NoError(t, err, "Failed to create Nova v2.1 client")

	return client
}

// SetupNetworkV2Client creates a Neutron v2 client for contract testing.
func SetupNetworkV2Client(t *testing.T) *gophercloud.ServiceClient {
	t.Helper()

	provider := SetupOpenStackClient(t)
	client, err := openstack.NewNetworkV2(provider, gophercloud.EndpointOpts{})
	require.NoError(t, err, "Failed to create Neutron v2 client")

	return client
}

// SetupBlockStorageV3Client creates a Cinder v3 client for contract testing.
func SetupBlockStorageV3Client(t *testing.T) *gophercloud.ServiceClient {
	t.Helper()

	provider := SetupOpenStackClient(t)
	client, err := openstack.NewBlockStorageV3(provider, gophercloud.EndpointOpts{})
	require.NoError(t, err, "Failed to create Cinder v3 client")

	return client
}

// SetupImageV2Client creates a Glance v2 client for contract testing.
func SetupImageV2Client(t *testing.T) *gophercloud.ServiceClient {
	t.Helper()

	provider := SetupOpenStackClient(t)
	client, err := openstack.NewImageServiceV2(provider, gophercloud.EndpointOpts{})
	require.NoError(t, err, "Failed to create Glance v2 client")

	return client
}

// getEnvOrDefault returns environment variable value or default if not set.
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// SkipIfO3KNotRunning skips the test if O3K is not running.
// Use this in contract tests to avoid false failures in CI.
func SkipIfO3KNotRunning(t *testing.T) {
	t.Helper()

	authURL := getEnvOrDefault("OS_AUTH_URL", "http://localhost:35357/v3")
	opts := gophercloud.AuthOptions{
		IdentityEndpoint: authURL,
		Username:         "admin",
		Password:         "secret",
		TenantName:       "default",
		DomainName:       "default",
	}

	_, err := openstack.AuthenticatedClient(opts)
	if err != nil {
		t.Skipf("O3K not running at %s: %v", authURL, err)
	}
}
