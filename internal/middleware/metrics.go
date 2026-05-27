package middleware

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

// metricsStore accumulates per-route counters without external dependencies.
// The key is "METHOD /path STATUS", e.g. "GET /v2/servers 200".
type metricsStore struct {
	mu       sync.RWMutex
	counters map[string]*atomic.Int64
	// Latency buckets (ms): each entry tracks the count of requests whose
	// duration fell within that upper bound.
	latencyBuckets []int64 // upper bounds in ms: 10, 50, 100, 500, 1000, +Inf
	latencyCounts  []atomic.Int64
}

var store = &metricsStore{
	counters:       make(map[string]*atomic.Int64),
	latencyBuckets: []int64{10, 50, 100, 500, 1000},
}

func init() {
	// latencyCounts has len(latencyBuckets)+1 slots: one per bucket + overflow.
	store.latencyCounts = make([]atomic.Int64, len(store.latencyBuckets)+1)
}

// MetricsMiddleware records request counts and latency for every handled route.
// It should be added after RequestIDMiddleware but before business-logic
// middleware so it captures the full request lifecycle.
func MetricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		ms := time.Since(start).Milliseconds()
		status := c.Writer.Status()
		key := fmt.Sprintf("%s %s %d", c.Request.Method, c.FullPath(), status)

		store.mu.RLock()
		ctr, exists := store.counters[key]
		store.mu.RUnlock()

		if !exists {
			store.mu.Lock()
			if ctr, exists = store.counters[key]; !exists {
				ctr = new(atomic.Int64)
				store.counters[key] = ctr
			}
			store.mu.Unlock()
		}
		ctr.Add(1)

		// Record latency bucket.
		idx := len(store.latencyBuckets) // default: overflow bucket
		for i, bound := range store.latencyBuckets {
			if ms <= bound {
				idx = i
				break
			}
		}
		store.latencyCounts[idx].Add(1)
	}
}

// RegisterMetricsRoute adds GET /metrics to a router with no authentication.
// Output is plain text in a simple key=value format.
func RegisterMetricsRoute(r *gin.Engine) {
	r.GET("/metrics", metricsHandler)
}

func metricsHandler(c *gin.Context) {
	store.mu.RLock()
	snap := make(map[string]int64, len(store.counters))
	for k, v := range store.counters {
		snap[k] = v.Load()
	}
	store.mu.RUnlock()

	w := c.Writer
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	fmt.Fprintln(w, "# o3k request counters (method path status_code count)")
	for key, count := range snap {
		fmt.Fprintf(w, "o3k_requests_total{route=%q} %d\n", key, count)
	}

	fmt.Fprintln(w, "# o3k latency distribution (upper_bound_ms count)")
	bounds := make([]int64, 0, len(store.latencyBuckets)+1)
	bounds = append(bounds, store.latencyBuckets...)
	bounds = append(bounds, -1) // -1 represents +Inf
	for i, bound := range bounds {
		label := fmt.Sprintf("%d", bound)
		if bound == -1 {
			label = "+Inf"
		}
		fmt.Fprintf(w, "o3k_request_duration_ms_bucket{le=%q} %d\n", label, store.latencyCounts[i].Load())
	}
}
