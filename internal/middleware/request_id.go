package middleware

import (
	"crypto/rand"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
)

// RequestIDMiddleware reads X-Request-Id from the incoming request and propagates
// it through the context. If none is present, a new ID is generated.
// This middleware must be registered before logging and auth.
func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-Id")
		if id == "" {
			id = generateRequestID()
		}
		// Ensure the ID carries the "req-" prefix required by OpenStack spec.
		if len(id) < 4 || id[:4] != "req-" {
			id = "req-" + id
		}
		c.Set("request_id", id)
		c.Header("X-Request-Id", id)
		c.Header("X-OpenStack-Request-Id", id)
		c.Next()
	}
}

// generateRequestID creates a lightweight request identifier using a hex-encoded
// timestamp combined with 4 random bytes to keep it short and sortable.
func generateRequestID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		// Fallback: timestamp only — still unique enough per process restart.
		return fmt.Sprintf("req-%x", time.Now().UnixNano())
	}
	return fmt.Sprintf("req-%x-%x", time.Now().UnixNano(), b)
}
