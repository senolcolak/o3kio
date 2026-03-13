package nova_test

import (
	"testing"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/flavors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function (duplicated to avoid import cycle - could be moved to common file)
func getAuthenticatedProvider() (*gophercloud.ProviderClient, error) {
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

	return openstack.AuthenticatedClient(opts)
}

// TestNovaFlavorInvalidInput_Contract tests error handling for invalid flavor creation
func TestNovaFlavorInvalidInput_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Test 1: Missing name
	createOpts := flavors.CreateOpts{
		Name:  "", // Invalid: empty name
		RAM:   1024,
		VCPUs: 1,
		Disk:  gophercloud.IntToPointer(10),
	}

	_, err := flavors.Create(client, createOpts).Extract()
	assert.Error(t, err, "Creating flavor with empty name should fail")

	// Test 2: Negative RAM
	createOpts = flavors.CreateOpts{
		Name:  "invalid-flavor",
		RAM:   -1024, // Invalid: negative
		VCPUs: 1,
		Disk:  gophercloud.IntToPointer(10),
	}

	_, err = flavors.Create(client, createOpts).Extract()
	assert.Error(t, err, "Creating flavor with negative RAM should fail")

	// Test 3: Zero VCPUs
	createOpts = flavors.CreateOpts{
		Name:  "invalid-flavor",
		RAM:   1024,
		VCPUs: 0, // Invalid: zero
		Disk:  gophercloud.IntToPointer(10),
	}

	_, err = flavors.Create(client, createOpts).Extract()
	assert.Error(t, err, "Creating flavor with zero VCPUs should fail")

	// Test 4: Negative disk
	createOpts = flavors.CreateOpts{
		Name:  "invalid-flavor",
		RAM:   1024,
		VCPUs: 1,
		Disk:  gophercloud.IntToPointer(-10), // Invalid: negative
	}

	_, err = flavors.Create(client, createOpts).Extract()
	assert.Error(t, err, "Creating flavor with negative disk should fail")
}

// TestNovaFlavorNotFound_Contract tests 404 handling
func TestNovaFlavorNotFound_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Test 1: Get non-existent flavor
	fakeID := "00000000-0000-0000-0000-999999999999"
	_, err := flavors.Get(client, fakeID).Extract()
	assert.Error(t, err, "GET non-existent flavor should fail")
	assert.Contains(t, err.Error(), "404", "Should be 404 error")

	// Test 2: Delete non-existent flavor
	err = flavors.Delete(client, fakeID).ExtractErr()
	assert.Error(t, err, "DELETE non-existent flavor should fail")
	assert.Contains(t, err.Error(), "404", "Should be 404 error")
}

// TestNovaFlavorDuplicateName_Contract tests conflict handling
func TestNovaFlavorDuplicateName_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Create first flavor
	createOpts := flavors.CreateOpts{
		Name:  "duplicate-test-flavor",
		RAM:   1024,
		VCPUs: 1,
		Disk:  gophercloud.IntToPointer(10),
	}

	flavor1, err := flavors.Create(client, createOpts).Extract()
	require.NoError(t, err)
	require.NotNil(t, flavor1)

	defer func() {
		flavors.Delete(client, flavor1.ID)
	}()

	// Attempt to create duplicate
	flavor2, err := flavors.Create(client, createOpts).Extract()

	// OpenStack may allow duplicates or reject them - check behavior
	if err != nil {
		// If rejected, should be 409 Conflict
		assert.Contains(t, err.Error(), "409", "Duplicate name should return 409 Conflict")
	} else {
		// If allowed, cleanup second flavor
		defer flavors.Delete(client, flavor2.ID)
		t.Logf("Note: O3K allows duplicate flavor names (flavor1=%s, flavor2=%s)", flavor1.ID, flavor2.ID)
	}
}

// TestNovaFlavorUnauthorized_Contract tests auth failure handling
func TestNovaFlavorUnauthorized_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	// Create client with invalid token
	provider, err := getAuthenticatedProvider()
	require.NoError(t, err)

	client, err := openstack.NewComputeV2(provider, gophercloud.EndpointOpts{})
	require.NoError(t, err)

	// Override with invalid token
	client.TokenID = "invalid-token-12345"

	// Attempt operation with bad token
	_, err = flavors.ListDetail(client, flavors.ListOpts{}).AllPages()
	assert.Error(t, err, "Operation with invalid token should fail")
	assert.Contains(t, err.Error(), "401", "Should be 401 Unauthorized")
}

// TestNovaFlavorAccessControl_Contract tests admin-only operations
func TestNovaFlavorAccessControl_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	// This test assumes flavor creation is admin-only
	// Would need non-admin user setup to test properly
	t.Skip("Requires non-admin user setup for RBAC testing")
}

// TestNovaFlavorListPagination_Contract tests pagination handling
func TestNovaFlavorListPagination_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Create multiple flavors for pagination testing
	flavorIDs := []string{}
	for i := 0; i < 5; i++ {
		createOpts := flavors.CreateOpts{
			Name:  "pagination-test-flavor-" + string(rune('a'+i)),
			RAM:   1024,
			VCPUs: 1,
			Disk:  gophercloud.IntToPointer(10),
		}

		flavor, err := flavors.Create(client, createOpts).Extract()
		if err != nil {
			t.Logf("Failed to create test flavor: %v", err)
			continue
		}
		flavorIDs = append(flavorIDs, flavor.ID)
	}

	defer func() {
		for _, id := range flavorIDs {
			flavors.Delete(client, id)
		}
	}()

	// Test pagination with limit
	page1, err := flavors.ListDetail(client, flavors.ListOpts{
		Limit: 2,
	}).AllPages()
	assert.NoError(t, err)

	list1, err := flavors.ExtractFlavors(page1)
	assert.NoError(t, err)
	assert.NotEmpty(t, list1, "Should return at least some flavors with limit")

	// Test marker-based pagination (if supported)
	if len(list1) > 0 {
		page2, err := flavors.ListDetail(client, flavors.ListOpts{
			Marker: list1[len(list1)-1].ID,
		}).AllPages()
		assert.NoError(t, err)

		list2, err := flavors.ExtractFlavors(page2)
		assert.NoError(t, err)

		// Verify marker-based results are different from first page
		if len(list2) > 0 {
			assert.NotEqual(t, list1[0].ID, list2[0].ID, "Marker pagination should return different results")
		}
	}
}

// TestNovaFlavorInvalidID_Contract tests invalid UUID handling
func TestNovaFlavorInvalidID_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Test 1: Malformed UUID
	invalidIDs := []string{
		"not-a-uuid",
		"12345",
		"xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
		"",
	}

	for _, invalidID := range invalidIDs {
		_, err := flavors.Get(client, invalidID).Extract()
		assert.Error(t, err, "GET with invalid ID '%s' should fail", invalidID)
	}
}
