package benchmark

import (
	"context"
	"testing"
	"time"

	"github.com/cobaltcore-dev/o3k/pkg/cache"
)

// BenchmarkCacheGet measures Redis cache read performance
func BenchmarkCacheGet(b *testing.B) {
	ctx := context.Background()
	c, err := cache.NewCache(cache.Config{
		RedisURL:   "redis://localhost:6379/0",
		Enabled:    true,
		KeyPrefix:  "o3k:bench:",
		DefaultTTL: 1 * time.Hour,
	})
	if err != nil {
		b.Skipf("Redis not available: %v", err)
	}

	// Pre-populate cache with test data
	testData := map[string]string{
		"id":     "test-flavor-123",
		"name":   "m1.small",
		"vcpus":  "1",
		"ram":    "2048",
		"disk":   "20",
	}
	c.Set(ctx, "flavor:test-flavor-123", testData, 1*time.Hour)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			var result map[string]string
			c.Get(ctx, "flavor:test-flavor-123", &result)
		}
	})
}

// BenchmarkCacheSet measures Redis cache write performance
func BenchmarkCacheSet(b *testing.B) {
	ctx := context.Background()
	c, err := cache.NewCache(cache.Config{
		RedisURL:   "redis://localhost:6379/0",
		Enabled:    true,
		KeyPrefix:  "o3k:bench:",
		DefaultTTL: 1 * time.Hour,
	})
	if err != nil {
		b.Skipf("Redis not available: %v", err)
	}

	testData := map[string]string{
		"id":     "test-flavor-456",
		"name":   "m1.medium",
		"vcpus":  "2",
		"ram":    "4096",
		"disk":   "40",
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			c.Set(ctx, "flavor:bench-"+string(rune(i)), testData, 1*time.Hour)
			i++
		}
	})
}

// BenchmarkCacheMiss measures cache miss + fallback query performance
func BenchmarkCacheMiss(b *testing.B) {
	ctx := context.Background()
	c, err := cache.NewCache(cache.Config{
		RedisURL:   "redis://localhost:6379/0",
		Enabled:    true,
		KeyPrefix:  "o3k:bench:",
		DefaultTTL: 1 * time.Hour,
	})
	if err != nil {
		b.Skipf("Redis not available: %v", err)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			var result map[string]string
			// Intentionally miss cache every time
			err := c.Get(ctx, "nonexistent-key-"+string(rune(i)), &result)
			_ = err // Expected to be cache miss
			i++
		}
	})
}

// BenchmarkCacheHitVsMiss compares hit vs miss performance
func BenchmarkCacheHitVsMiss(b *testing.B) {
	ctx := context.Background()
	c, err := cache.NewCache(cache.Config{
		RedisURL:   "redis://localhost:6379/0",
		Enabled:    true,
		KeyPrefix:  "o3k:bench:",
		DefaultTTL: 1 * time.Hour,
	})
	if err != nil {
		b.Skipf("Redis not available: %v", err)
	}

	// Populate 1000 items
	for i := 0; i < 1000; i++ {
		c.Set(ctx, "item:"+string(rune(i)), map[string]int{"value": i}, 1*time.Hour)
	}

	b.Run("CacheHit", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				var result map[string]int
				c.Get(ctx, "item:"+string(rune(i%1000)), &result)
				i++
			}
		})
	})

	b.Run("CacheMiss", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				var result map[string]int
				c.Get(ctx, "missing:"+string(rune(i)), &result)
				i++
			}
		})
	})
}
