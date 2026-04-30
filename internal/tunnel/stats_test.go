package tunnel_test

import (
	"testing"

	"github.com/cobaltcore-dev/o3k/internal/tunnel"
	"github.com/stretchr/testify/assert"
)

func TestCollectHostStats(t *testing.T) {
	stats := tunnel.CollectHostStats("stub")
	assert.Greater(t, stats.VCPUTotal, int64(0))
	assert.Equal(t, int64(8192), stats.RAMTotalMB)
	assert.Equal(t, int64(100), stats.DiskTotalGB)
}
