package neutron_test

import (
	"testing"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/networks"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/subnets"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/ports"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/layer3/floatingips"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNeutronNetworkCreate_Contract tests basic network creation
func TestNeutronNetworkCreate_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNeutronClient(t)

	// Create network
	createOpts := networks.CreateOpts{
		Name:         "test-network-basic",
		AdminStateUp: gophercloud.Enabled,
	}

	network, err := networks.Create(client, createOpts).Extract()
	require.NoError(t, err, "Failed to create network")
	require.NotNil(t, network)
	assert.NotEmpty(t, network.ID)
	assert.Equal(t, "test-network-basic", network.Name)
	assert.True(t, network.AdminStateUp)

	// Cleanup
	defer func() {
		err := networks.Delete(client, network.ID).ExtractErr()
		if err != nil {
			t.Logf("Cleanup failed: %v", err)
		}
	}()

	// Verify network exists
	fetchedNetwork, err := networks.Get(client, network.ID).Extract()
	require.NoError(t, err, "Failed to fetch created network")
	assert.Equal(t, network.ID, fetchedNetwork.ID)
}

// TestNeutronSubnetCreate_Contract tests subnet creation
func TestNeutronSubnetCreate_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNeutronClient(t)

	// Create network first
	network, err := networks.Create(client, networks.CreateOpts{
		Name: "test-network-for-subnet",
	}).Extract()
	require.NoError(t, err, "Failed to create network")
	defer networks.Delete(client, network.ID)

	// Create subnet
	ipVersion := 4
	cidr := "192.168.100.0/24"
	createOpts := subnets.CreateOpts{
		NetworkID: network.ID,
		Name:      "test-subnet",
		IPVersion: gophercloud.IPVersion(ipVersion),
		CIDR:      cidr,
	}

	subnet, err := subnets.Create(client, createOpts).Extract()
	require.NoError(t, err, "Failed to create subnet")
	require.NotNil(t, subnet)
	assert.NotEmpty(t, subnet.ID)
	assert.Equal(t, "test-subnet", subnet.Name)
	assert.Equal(t, cidr, subnet.CIDR)
	assert.Equal(t, ipVersion, subnet.IPVersion)

	// Cleanup
	defer func() {
		err := subnets.Delete(client, subnet.ID).ExtractErr()
		if err != nil {
			t.Logf("Subnet cleanup failed: %v", err)
		}
	}()

	// Verify subnet exists
	fetchedSubnet, err := subnets.Get(client, subnet.ID).Extract()
	require.NoError(t, err, "Failed to fetch created subnet")
	assert.Equal(t, subnet.ID, fetchedSubnet.ID)
}

// TestNeutronPortCreate_Contract tests port creation
func TestNeutronPortCreate_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNeutronClient(t)

	// Create network
	network, err := networks.Create(client, networks.CreateOpts{
		Name: "test-network-for-port",
	}).Extract()
	require.NoError(t, err, "Failed to create network")
	defer networks.Delete(client, network.ID)

	// Create port
	createOpts := ports.CreateOpts{
		NetworkID:    network.ID,
		Name:         "test-port",
		AdminStateUp: gophercloud.Enabled,
	}

	port, err := ports.Create(client, createOpts).Extract()
	require.NoError(t, err, "Failed to create port")
	require.NotNil(t, port)
	assert.NotEmpty(t, port.ID)
	assert.Equal(t, "test-port", port.Name)
	assert.Equal(t, network.ID, port.NetworkID)
	assert.True(t, port.AdminStateUp)

	// Cleanup
	defer func() {
		err := ports.Delete(client, port.ID).ExtractErr()
		if err != nil {
			t.Logf("Port cleanup failed: %v", err)
		}
	}()

	// Verify port exists
	fetchedPort, err := ports.Get(client, port.ID).Extract()
	require.NoError(t, err, "Failed to fetch created port")
	assert.Equal(t, port.ID, fetchedPort.ID)
}

// TestNeutronFloatingIPCreate_Contract tests floating IP creation
func TestNeutronFloatingIPCreate_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNeutronClient(t)

	// Create external network
	extNetwork, err := networks.Create(client, networks.CreateOpts{
		Name:         "test-external-network",
		AdminStateUp: gophercloud.Enabled,
	}).Extract()
	require.NoError(t, err, "Failed to create external network")
	defer networks.Delete(client, extNetwork.ID)

	// Create floating IP
	createOpts := floatingips.CreateOpts{
		FloatingNetworkID: extNetwork.ID,
		Description:       "Test floating IP",
	}

	fip, err := floatingips.Create(client, createOpts).Extract()
	require.NoError(t, err, "Failed to create floating IP")
	require.NotNil(t, fip)
	assert.NotEmpty(t, fip.ID)
	assert.Equal(t, extNetwork.ID, fip.FloatingNetworkID)
	assert.NotEmpty(t, fip.FloatingIP)

	// Cleanup
	defer func() {
		err := floatingips.Delete(client, fip.ID).ExtractErr()
		if err != nil {
			t.Logf("Floating IP cleanup failed: %v", err)
		}
	}()

	// Verify floating IP exists
	fetchedFIP, err := floatingips.Get(client, fip.ID).Extract()
	require.NoError(t, err, "Failed to fetch created floating IP")
	assert.Equal(t, fip.ID, fetchedFIP.ID)
	assert.Equal(t, fip.FloatingIP, fetchedFIP.FloatingIP)
}

// TestNeutronNetworkLifecycle_Contract tests complete network resource lifecycle
func TestNeutronNetworkLifecycle_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNeutronClient(t)

	// 1. Create network
	network, err := networks.Create(client, networks.CreateOpts{
		Name:         "test-network-lifecycle",
		AdminStateUp: gophercloud.Enabled,
	}).Extract()
	require.NoError(t, err, "Failed to create network")
	defer networks.Delete(client, network.ID)

	// 2. Create subnet
	subnet, err := subnets.Create(client, subnets.CreateOpts{
		NetworkID: network.ID,
		Name:      "test-subnet-lifecycle",
		IPVersion: gophercloud.IPv4,
		CIDR:      "10.0.1.0/24",
	}).Extract()
	require.NoError(t, err, "Failed to create subnet")
	defer subnets.Delete(client, subnet.ID)

	// 3. Create port
	port, err := ports.Create(client, ports.CreateOpts{
		NetworkID: network.ID,
		Name:      "test-port-lifecycle",
	}).Extract()
	require.NoError(t, err, "Failed to create port")
	defer ports.Delete(client, port.ID)

	// 4. Verify all resources exist
	fetchedNetwork, err := networks.Get(client, network.ID).Extract()
	require.NoError(t, err, "Failed to fetch network")
	assert.Equal(t, network.ID, fetchedNetwork.ID)

	fetchedSubnet, err := subnets.Get(client, subnet.ID).Extract()
	require.NoError(t, err, "Failed to fetch subnet")
	assert.Equal(t, subnet.ID, fetchedSubnet.ID)

	fetchedPort, err := ports.Get(client, port.ID).Extract()
	require.NoError(t, err, "Failed to fetch port")
	assert.Equal(t, port.ID, fetchedPort.ID)

	// 5. Update network
	newName := "test-network-updated"
	updatedNetwork, err := networks.Update(client, network.ID, networks.UpdateOpts{
		Name: &newName,
	}).Extract()
	require.NoError(t, err, "Failed to update network")
	assert.Equal(t, newName, updatedNetwork.Name)

	// 6. List networks (should include our network)
	allPages, err := networks.List(client, networks.ListOpts{}).AllPages()
	require.NoError(t, err, "Failed to list networks")
	allNetworks, err := networks.ExtractNetworks(allPages)
	require.NoError(t, err, "Failed to extract networks")

	found := false
	for _, n := range allNetworks {
		if n.ID == network.ID {
			found = true
			break
		}
	}
	assert.True(t, found, "Network should be in list")

	// 7. Cleanup (port -> subnet -> network order)
	err = ports.Delete(client, port.ID).ExtractErr()
	require.NoError(t, err, "Failed to delete port")

	err = subnets.Delete(client, subnet.ID).ExtractErr()
	require.NoError(t, err, "Failed to delete subnet")

	err = networks.Delete(client, network.ID).ExtractErr()
	require.NoError(t, err, "Failed to delete network")
}
