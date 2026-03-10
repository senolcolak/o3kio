package nova_test

import (
	"testing"

	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/availabilityzones"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNovaListAvailabilityZones_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// List availability zones
	allPages, err := availabilityzones.List(client).AllPages()
	require.NoError(t, err)

	zones, err := availabilityzones.ExtractAvailabilityZones(allPages)
	require.NoError(t, err)

	// Should have at least one zone (default: "nova")
	assert.NotEmpty(t, zones)

	// Verify zone structure
	if len(zones) > 0 {
		zone := zones[0]
		assert.NotEmpty(t, zone.ZoneName)
		assert.NotNil(t, zone.ZoneState)
	}
}
