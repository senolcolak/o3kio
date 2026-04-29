package tunnel_test

import (
	"testing"

	"github.com/cobaltcore-dev/o3k/internal/tunnel"
	"github.com/stretchr/testify/assert"
)

func TestTokenGenerate(t *testing.T) {
	secret := "my-tunnel-secret"
	nodeID := "node-abc-123"

	hash := tunnel.GenerateTokenHash(secret, nodeID)
	assert.NotEmpty(t, hash)
	assert.True(t, tunnel.VerifyTokenHash(secret, nodeID, hash))
	assert.False(t, tunnel.VerifyTokenHash("wrong-secret", nodeID, hash))
}

func TestTokenGenerateDeterministic(t *testing.T) {
	secret := "my-tunnel-secret"
	nodeID := "node-abc-123"

	hash1 := tunnel.GenerateTokenHash(secret, nodeID)
	hash2 := tunnel.GenerateTokenHash(secret, nodeID)
	assert.Equal(t, hash1, hash2)
}
