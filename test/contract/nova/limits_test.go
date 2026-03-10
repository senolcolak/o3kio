package nova_test

import (
	"testing"

	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/limits"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNovaGetLimits_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Get limits (quotas)
	limitsResult, err := limits.Get(client, limits.GetOpts{}).Extract()
	require.NoError(t, err)

	// Verify absolute limits structure
	assert.NotNil(t, limitsResult.Absolute)
	assert.GreaterOrEqual(t, limitsResult.Absolute.MaxTotalInstances, 0)
	assert.GreaterOrEqual(t, limitsResult.Absolute.MaxTotalCores, 0)
	assert.GreaterOrEqual(t, limitsResult.Absolute.MaxTotalRAMSize, 0)
	assert.GreaterOrEqual(t, limitsResult.Absolute.TotalInstancesUsed, 0)
	assert.GreaterOrEqual(t, limitsResult.Absolute.TotalCoresUsed, 0)
	assert.GreaterOrEqual(t, limitsResult.Absolute.TotalRAMUsed, 0)
}
