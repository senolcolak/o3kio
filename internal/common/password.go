package common

import (
	"crypto/rand"
	"math/big"
)

const alphanumeric = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// GeneratePassword returns a cryptographically random alphanumeric string
// with uniform distribution (no modulo bias).
func GeneratePassword(length int) string {
	max := big.NewInt(int64(len(alphanumeric)))
	b := make([]byte, length)
	for i := range b {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			panic("crypto/rand.Int failed: " + err.Error())
		}
		b[i] = alphanumeric[n.Int64()]
	}
	return string(b)
}
