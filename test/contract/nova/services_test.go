package nova_test

import (
	"testing"

	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNovaListServices_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// List compute services
	allPages, err := services.List(client, services.ListOpts{}).AllPages()
	require.NoError(t, err)

	servicesList, err := services.ExtractServices(allPages)
	require.NoError(t, err)

	// Should have at least one service (nova-compute)
	assert.NotEmpty(t, servicesList)

	// Verify service structure
	if len(servicesList) > 0 {
		svc := servicesList[0]
		assert.NotEmpty(t, svc.Binary)
		assert.NotEmpty(t, svc.Host)
		assert.NotEmpty(t, svc.State)
		assert.NotEmpty(t, svc.Status)
	}
}
