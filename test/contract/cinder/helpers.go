package cinder

import (
	"context"
	"os"
	"testing"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/blockstorage/v3/volumes"
)

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func skipIfO3KNotRunning(t *testing.T) {
	// Try to connect - if it fails, skip the test
	authURL := getEnvOrDefault("OS_AUTH_URL", "http://localhost:35357/v3")
	opts := gophercloud.AuthOptions{
		IdentityEndpoint: authURL,
		Username:         getEnvOrDefault("OS_USERNAME", "admin"),
		Password:         getEnvOrDefault("OS_PASSWORD", "secret"),
		TenantName:       getEnvOrDefault("OS_PROJECT_NAME", "default"),
		DomainName:       getEnvOrDefault("OS_USER_DOMAIN_NAME", "Default"),
	}

	provider, err := openstack.AuthenticatedClient(context.Background(), opts)
	if err != nil {
		t.Skipf("O3K not running or not accessible: %v", err)
	}

	// Try to create a Cinder client
	_, err = openstack.NewBlockStorageV3(provider, gophercloud.EndpointOpts{})
	if err != nil {
		t.Skipf("Cinder service not available: %v", err)
	}
}

func setupCinderClient(t *testing.T) *gophercloud.ServiceClient {
	authURL := getEnvOrDefault("OS_AUTH_URL", "http://localhost:35357/v3")
	opts := gophercloud.AuthOptions{
		IdentityEndpoint: authURL,
		Username:         getEnvOrDefault("OS_USERNAME", "admin"),
		Password:         getEnvOrDefault("OS_PASSWORD", "secret"),
		TenantName:       getEnvOrDefault("OS_PROJECT_NAME", "default"),
		DomainName:       getEnvOrDefault("OS_USER_DOMAIN_NAME", "Default"),
	}

	provider, err := openstack.AuthenticatedClient(context.Background(), opts)
	if err != nil {
		t.Fatalf("Failed to authenticate: %v", err)
	}

	client, err := openstack.NewBlockStorageV3(provider, gophercloud.EndpointOpts{})
	if err != nil {
		t.Fatalf("Failed to create Cinder client: %v", err)
	}

	return client
}

// Cleanup helper to ensure volumes are deleted
func ensureVolumeDeleted(client *gophercloud.ServiceClient, volumeID string) {
	volumes.Delete(context.Background(), client, volumeID, volumes.DeleteOpts{})
}
