package neutron_test

import (
	"testing"

	"github.com/gophercloud/gophercloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPortForwardingLifecycle_Contract tests complete port forwarding lifecycle
func TestPortForwardingLifecycle_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNeutronClient(t)

	// Create test network for port forwarding
	network, subnet, port, router, floatingIP := setupPortForwardingPrerequisites(t, client)
	defer cleanupPortForwardingPrerequisites(t, client, network.ID, subnet.ID, port.ID, router.ID, floatingIP.ID)

	// 1. Create port forwarding
	createOpts := map[string]interface{}{
		"port_forwarding": map[string]interface{}{
			"internal_port_id":    port.ID,
			"internal_ip_address": "10.254.0.10",
			"external_port":       8080,
			"internal_port":       80,
			"protocol":            "tcp",
			"description":         "Web server forwarding",
		},
	}

	var createResult struct {
		PortForwarding struct {
			ID                string `json:"id"`
			ExternalPort      int    `json:"external_port"`
			InternalPort      int    `json:"internal_port"`
			Protocol          string `json:"protocol"`
			InternalIPAddress string `json:"internal_ip_address"`
			InternalPortID    string `json:"internal_port_id"`
			Description       string `json:"description"`
		} `json:"port_forwarding"`
	}

	resp, err := client.Post(
		client.ServiceURL("floatingips", floatingIP.ID, "port_forwardings"),
		createOpts,
		&createResult,
		&gophercloud.RequestOpts{
			OkCodes: []int{201},
		},
	)
	require.NoError(t, err, "Failed to create port forwarding")
	require.Equal(t, 201, resp.StatusCode)
	assert.NotEmpty(t, createResult.PortForwarding.ID)
	assert.Equal(t, 8080, createResult.PortForwarding.ExternalPort)
	assert.Equal(t, 80, createResult.PortForwarding.InternalPort)
	assert.Equal(t, "tcp", createResult.PortForwarding.Protocol)

	pfID := createResult.PortForwarding.ID

	// 2. List port forwardings
	var listResult struct {
		PortForwardings []map[string]interface{} `json:"port_forwardings"`
	}

	_, err = client.Get(
		client.ServiceURL("floatingips", floatingIP.ID, "port_forwardings"),
		&listResult,
		nil,
	)
	require.NoError(t, err, "Failed to list port forwardings")
	assert.Len(t, listResult.PortForwardings, 1)
	assert.Equal(t, pfID, listResult.PortForwardings[0]["id"])

	// 3. Get specific port forwarding
	var getResult struct {
		PortForwarding map[string]interface{} `json:"port_forwarding"`
	}

	_, err = client.Get(
		client.ServiceURL("floatingips", floatingIP.ID, "port_forwardings", pfID),
		&getResult,
		nil,
	)
	require.NoError(t, err, "Failed to get port forwarding")
	assert.Equal(t, pfID, getResult.PortForwarding["id"])
	assert.Equal(t, float64(8080), getResult.PortForwarding["external_port"])

	// 4. Update port forwarding
	updateOpts := map[string]interface{}{
		"port_forwarding": map[string]interface{}{
			"internal_port": 8080,
			"description":   "Updated web server forwarding",
		},
	}

	_, err = client.Put(
		client.ServiceURL("floatingips", floatingIP.ID, "port_forwardings", pfID),
		updateOpts,
		&getResult,
		nil,
	)
	require.NoError(t, err, "Failed to update port forwarding")
	assert.Equal(t, float64(8080), getResult.PortForwarding["internal_port"])
	assert.Equal(t, "Updated web server forwarding", getResult.PortForwarding["description"])

	// 5. Delete port forwarding
	_, err = client.Delete(
		client.ServiceURL("floatingips", floatingIP.ID, "port_forwardings", pfID),
		&gophercloud.RequestOpts{
			OkCodes: []int{204},
		},
	)
	require.NoError(t, err, "Failed to delete port forwarding")

	// Verify deletion
	_, err = client.Get(
		client.ServiceURL("floatingips", floatingIP.ID, "port_forwardings", pfID),
		nil,
		&gophercloud.RequestOpts{
			OkCodes: []int{404},
		},
	)
	assert.Error(t, err, "Port forwarding should not exist after deletion")
}

// TestPortForwardingValidation_Contract tests input validation
func TestPortForwardingValidation_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNeutronClient(t)

	network, subnet, port, router, floatingIP := setupPortForwardingPrerequisites(t, client)
	defer cleanupPortForwardingPrerequisites(t, client, network.ID, subnet.ID, port.ID, router.ID, floatingIP.ID)

	// Test invalid protocol
	createOpts := map[string]interface{}{
		"port_forwarding": map[string]interface{}{
			"internal_port_id":    port.ID,
			"internal_ip_address": "10.254.0.10",
			"external_port":       8080,
			"internal_port":       80,
			"protocol":            "icmp", // Invalid
		},
	}

	_, err := client.Post(
		client.ServiceURL("floatingips", floatingIP.ID, "port_forwardings"),
		createOpts,
		nil,
		&gophercloud.RequestOpts{
			OkCodes: []int{400},
		},
	)
	assert.Error(t, err, "Should reject invalid protocol")

	// Test invalid port range (too high)
	createOpts["port_forwarding"].(map[string]interface{})["protocol"] = "tcp"
	createOpts["port_forwarding"].(map[string]interface{})["external_port"] = 99999

	_, err = client.Post(
		client.ServiceURL("floatingips", floatingIP.ID, "port_forwardings"),
		createOpts,
		nil,
		&gophercloud.RequestOpts{
			OkCodes: []int{400},
		},
	)
	assert.Error(t, err, "Should reject invalid port range")

	// Test duplicate port forwarding
	createOpts["port_forwarding"].(map[string]interface{})["external_port"] = 8080
	client.Post(
		client.ServiceURL("floatingips", floatingIP.ID, "port_forwardings"),
		createOpts,
		nil,
		nil,
	)

	// Try to create duplicate
	_, err = client.Post(
		client.ServiceURL("floatingips", floatingIP.ID, "port_forwardings"),
		createOpts,
		nil,
		&gophercloud.RequestOpts{
			OkCodes: []int{409},
		},
	)
	assert.Error(t, err, "Should reject duplicate port forwarding")
}

// setupPortForwardingPrerequisites creates network, port, router, and floating IP for testing
func setupPortForwardingPrerequisites(t *testing.T, client *gophercloud.ServiceClient) (network, subnet, port, router, floatingIP struct{ ID string }) {
	t.Helper()

	// Create external network
	extNetOpts := map[string]interface{}{
		"network": map[string]interface{}{
			"name":           "test-ext-net",
			"router:external": true,
			"admin_state_up": true,
		},
	}

	var extNetResult struct {
		Network struct {
			ID string `json:"id"`
		} `json:"network"`
	}

	_, err := client.Post(
		client.ServiceURL("networks"),
		extNetOpts,
		&extNetResult,
		nil,
	)
	require.NoError(t, err, "Failed to create external network")
	extNetID := extNetResult.Network.ID

	// Create internal network
	netOpts := map[string]interface{}{
		"network": map[string]interface{}{
			"name":           "test-pf-network",
			"admin_state_up": true,
		},
	}

	var netResult struct {
		Network struct {
			ID string `json:"id"`
		} `json:"network"`
	}

	_, err = client.Post(
		client.ServiceURL("networks"),
		netOpts,
		&netResult,
		nil,
	)
	require.NoError(t, err, "Failed to create network")
	network.ID = netResult.Network.ID

	// Create subnet
	subnetOpts := map[string]interface{}{
		"subnet": map[string]interface{}{
			"network_id": network.ID,
			"cidr":       "10.254.0.0/24",
			"ip_version": 4,
		},
	}

	var subnetResult struct {
		Subnet struct {
			ID string `json:"id"`
		} `json:"subnet"`
	}

	_, err = client.Post(
		client.ServiceURL("subnets"),
		subnetOpts,
		&subnetResult,
		nil,
	)
	require.NoError(t, err, "Failed to create subnet")
	subnet.ID = subnetResult.Subnet.ID

	// Create port
	portOpts := map[string]interface{}{
		"port": map[string]interface{}{
			"network_id": network.ID,
			"fixed_ips": []map[string]interface{}{
				{
					"subnet_id":  subnet.ID,
					"ip_address": "10.254.0.10",
				},
			},
		},
	}

	var portResult struct {
		Port struct {
			ID string `json:"id"`
		} `json:"port"`
	}

	_, err = client.Post(
		client.ServiceURL("ports"),
		portOpts,
		&portResult,
		nil,
	)
	require.NoError(t, err, "Failed to create port")
	port.ID = portResult.Port.ID

	// Create router
	routerOpts := map[string]interface{}{
		"router": map[string]interface{}{
			"name":           "test-pf-router",
			"admin_state_up": true,
			"external_gateway_info": map[string]interface{}{
				"network_id": extNetID,
			},
		},
	}

	var routerResult struct {
		Router struct {
			ID string `json:"id"`
		} `json:"router"`
	}

	_, err = client.Post(
		client.ServiceURL("routers"),
		routerOpts,
		&routerResult,
		nil,
	)
	require.NoError(t, err, "Failed to create router")
	router.ID = routerResult.Router.ID

	// Attach subnet to router
	addInterfaceOpts := map[string]interface{}{
		"subnet_id": subnet.ID,
	}

	_, err = client.Put(
		client.ServiceURL("routers", router.ID, "add_router_interface"),
		addInterfaceOpts,
		nil,
		nil,
	)
	require.NoError(t, err, "Failed to attach interface to router")

	// Create floating IP
	fipOpts := map[string]interface{}{
		"floatingip": map[string]interface{}{
			"floating_network_id": extNetID,
			"port_id":             port.ID,
		},
	}

	var fipResult struct {
		FloatingIP struct {
			ID string `json:"id"`
		} `json:"floatingip"`
	}

	_, err = client.Post(
		client.ServiceURL("floatingips"),
		fipOpts,
		&fipResult,
		nil,
	)
	require.NoError(t, err, "Failed to create floating IP")
	floatingIP.ID = fipResult.FloatingIP.ID

	return network, subnet, port, router, floatingIP
}

// cleanupPortForwardingPrerequisites deletes all test resources
func cleanupPortForwardingPrerequisites(t *testing.T, client *gophercloud.ServiceClient, networkID, subnetID, portID, routerID, floatingIPID string) {
	t.Helper()

	// Delete floating IP
	if floatingIPID != "" {
		client.Delete(client.ServiceURL("floatingips", floatingIPID), nil)
	}

	// Remove router interface
	if routerID != "" && subnetID != "" {
		removeOpts := map[string]interface{}{
			"subnet_id": subnetID,
		}
		client.Put(
			client.ServiceURL("routers", routerID, "remove_router_interface"),
			removeOpts,
			nil,
			nil,
		)
	}

	// Delete router
	if routerID != "" {
		client.Delete(client.ServiceURL("routers", routerID), nil)
	}

	// Delete port
	if portID != "" {
		client.Delete(client.ServiceURL("ports", portID), nil)
	}

	// Delete subnet
	if subnetID != "" {
		client.Delete(client.ServiceURL("subnets", subnetID), nil)
	}

	// Delete network
	if networkID != "" {
		client.Delete(client.ServiceURL("networks", networkID), nil)
	}
}
