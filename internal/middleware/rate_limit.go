package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// RateLimiter implements a simple sliding-window IP-based rate limiter.
type RateLimiter struct {
	mu       sync.Mutex
	requests map[string][]time.Time
	limit    int
	window   time.Duration
}

// NewRateLimiter creates a RateLimiter that allows at most limit requests per window per key.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}
}

// Allow reports whether a new request from key is within the rate limit.
// It removes expired timestamps and appends the current time if allowed.
func (rl *RateLimiter) Allow(key string) bool {
	now := time.Now()
	cutoff := now.Add(-rl.window)

	rl.mu.Lock()
	defer rl.mu.Unlock()

	times := rl.requests[key]

	// Evict timestamps outside the window.
	valid := times[:0]
	for _, t := range times {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	// If all timestamps expired, remove the map key to prevent unbounded growth.
	if len(valid) == 0 {
		delete(rl.requests, key)
		rl.requests[key] = []time.Time{now}
		return true
	}

	if len(valid) >= rl.limit {
		rl.requests[key] = valid
		return false
	}

	rl.requests[key] = append(valid, now)
	return true
}

// RateLimitMiddleware returns a gin.HandlerFunc that rejects requests exceeding the limit.
func RateLimitMiddleware(limiter *RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		if !limiter.Allow(ip) {
			c.Header("Retry-After", "60")
			c.JSON(http.StatusTooManyRequests, gin.H{"overLimit": gin.H{
				"code":    http.StatusTooManyRequests,
				"message": "Too many requests",
			}})
			c.Abort()
			return
		}
		c.Next()
	}
}
