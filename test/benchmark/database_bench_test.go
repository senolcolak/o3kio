package benchmark

import (
	"context"
	"testing"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/database"
)

// BenchmarkDatabaseQuery measures single query performance
func BenchmarkDatabaseQuery(b *testing.B) {
	ctx := context.Background()

	// Connect to test database
	poolConfig := &database.PoolConfig{
		MaxConns:          50,
		MinConns:          2,
		MaxConnLifetime:   1 * time.Hour,
		MaxConnIdleTime:   10 * time.Minute,
		HealthCheckPeriod: 30 * time.Second,
	}

	err := database.Connect(ctx, "postgres://lightstack:secret@localhost/lightstack?sslmode=disable", poolConfig)
	if err != nil {
		b.Skipf("Database not available: %v", err)
	}
	defer database.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			var count int
			database.DB.QueryRow(ctx, "SELECT COUNT(*) FROM flavors").Scan(&count)
		}
	})
}

// BenchmarkDatabaseQueryRow measures QueryRow performance with optimization
func BenchmarkDatabaseQueryRow(b *testing.B) {
	ctx := context.Background()

	poolConfig := &database.PoolConfig{
		MaxConns:          50,
		MinConns:          2,
		MaxConnLifetime:   1 * time.Hour,
		MaxConnIdleTime:   10 * time.Minute,
		HealthCheckPeriod: 30 * time.Second,
	}

	err := database.Connect(ctx, "postgres://lightstack:secret@localhost/lightstack?sslmode=disable", poolConfig)
	if err != nil {
		b.Skipf("Database not available: %v", err)
	}
	defer database.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			var id, name string
			database.DB.QueryRow(ctx,
				"SELECT id, name FROM flavors LIMIT 1",
			).Scan(&id, &name)
		}
	})
}

// BenchmarkDatabaseJoinQuery measures JOIN query performance
func BenchmarkDatabaseJoinQuery(b *testing.B) {
	ctx := context.Background()

	poolConfig := &database.PoolConfig{
		MaxConns:          50,
		MinConns:          2,
		MaxConnLifetime:   1 * time.Hour,
		MaxConnIdleTime:   10 * time.Minute,
		HealthCheckPeriod: 30 * time.Second,
	}

	err := database.Connect(ctx, "postgres://lightstack:secret@localhost/lightstack?sslmode=disable", poolConfig)
	if err != nil {
		b.Skipf("Database not available: %v", err)
	}
	defer database.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			rows, _ := database.DB.Query(ctx, `
				SELECT s.id, s.name, s.network_id, n.name as network_name
				FROM subnets s
				JOIN networks n ON s.network_id = n.id
				LIMIT 10
			`)
			if rows != nil {
				rows.Close()
			}
		}
	})
}

// BenchmarkConnectionPoolUtilization measures concurrent query throughput
func BenchmarkConnectionPoolUtilization(b *testing.B) {
	ctx := context.Background()

	poolConfig := &database.PoolConfig{
		MaxConns:          50,
		MinConns:          2,
		MaxConnLifetime:   1 * time.Hour,
		MaxConnIdleTime:   10 * time.Minute,
		HealthCheckPeriod: 30 * time.Second,
	}

	err := database.Connect(ctx, "postgres://lightstack:secret@localhost/lightstack?sslmode=disable", poolConfig)
	if err != nil {
		b.Skipf("Database not available: %v", err)
	}
	defer database.Close()

	b.Run("LowConcurrency", func(b *testing.B) {
		b.SetParallelism(10) // 10 goroutines
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				var count int
				database.DB.QueryRow(ctx, "SELECT COUNT(*) FROM images").Scan(&count)
			}
		})
	})

	b.Run("HighConcurrency", func(b *testing.B) {
		b.SetParallelism(100) // 100 goroutines
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				var count int
				database.DB.QueryRow(ctx, "SELECT COUNT(*) FROM images").Scan(&count)
			}
		})
	})
}

// BenchmarkDatabaseInsert measures write performance
func BenchmarkDatabaseInsert(b *testing.B) {
	ctx := context.Background()

	poolConfig := &database.PoolConfig{
		MaxConns:          50,
		MinConns:          2,
		MaxConnLifetime:   1 * time.Hour,
		MaxConnIdleTime:   10 * time.Minute,
		HealthCheckPeriod: 30 * time.Second,
	}

	err := database.Connect(ctx, "postgres://lightstack:secret@localhost/lightstack?sslmode=disable", poolConfig)
	if err != nil {
		b.Skipf("Database not available: %v", err)
	}
	defer database.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Insert test flavor (will fail on duplicate, that's ok for benchmark)
		database.DB.Exec(ctx, `
			INSERT INTO flavors (id, name, vcpus, ram_mb, disk_gb, is_public)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (id) DO NOTHING
		`, "bench-"+string(rune(i)), "bench.flavor", 1, 1024, 10, true)
	}
}
