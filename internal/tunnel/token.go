package tunnel

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// GenerateTokenHash produces an HMAC-SHA256 hex digest of nodeID signed with secret.
func GenerateTokenHash(secret, nodeID string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(nodeID))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyTokenHash reports whether hash matches the expected HMAC for secret+nodeID.
// Uses hmac.Equal to prevent timing attacks.
func VerifyTokenHash(secret, nodeID, hash string) bool {
	expected := GenerateTokenHash(secret, nodeID)
	return hmac.Equal([]byte(expected), []byte(hash))
}
