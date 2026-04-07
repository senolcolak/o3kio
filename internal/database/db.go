package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var DB *pgxpool.Pool

// PoolConfig contains database connection pool configuration
type PoolConfig struct {
	MaxConns          int32         // Maximum number of connections in the pool
	MinConns          int32         // Minimum number of idle connections
	MaxConnLifetime   time.Duration // Maximum lifetime of a connection
	MaxConnIdleTime   time.Duration // Maximum idle time before closing connection
	HealthCheckPeriod time.Duration // How often to check connection health
}

// DefaultPoolConfig returns sensible defaults for connection pooling
func DefaultPoolConfig() *PoolConfig {
	return &PoolConfig{
		MaxConns:          20,               // Default from config
		MinConns:          2,                // Keep 2 connections warm
		MaxConnLifetime:   1 * time.Hour,    // Recycle connections hourly
		MaxConnIdleTime:   15 * time.Minute, // Close idle connections after 15 min
		HealthCheckPeriod: 1 * time.Minute,  // Check health every minute
	}
}

// Connect establishes database connection pool with optimized settings
func Connect(ctx context.Context, connString string, poolConfig *PoolConfig) error {
	config, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return fmt.Errorf("failed to parse database URL: %w", err)
	}

	// Apply pool configuration
	if poolConfig == nil {
		poolConfig = DefaultPoolConfig()
	}

	config.MaxConns = poolConfig.MaxConns
	config.MinConns = poolConfig.MinConns
	config.MaxConnLifetime = poolConfig.MaxConnLifetime
	config.MaxConnIdleTime = poolConfig.MaxConnIdleTime
	config.HealthCheckPeriod = poolConfig.HealthCheckPeriod

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Test connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return fmt.Errorf("failed to ping database: %w", err)
	}

	DB = pool
	return nil
}

// ConnectSimple establishes database connection pool with simple configuration (backwards compatible)
func ConnectSimple(ctx context.Context, connString string, maxConns int) error {
	return Connect(ctx, connString, &PoolConfig{
		MaxConns:          int32(maxConns),
		MinConns:          2,
		MaxConnLifetime:   1 * time.Hour,
		MaxConnIdleTime:   15 * time.Minute,
		HealthCheckPeriod: 1 * time.Minute,
	})
}

// Close closes the database connection pool
func Close() {
	if DB != nil {
		DB.Close()
	}
}

// Stats returns current connection pool statistics
func Stats() *pgxpool.Stat {
	if DB == nil {
		return nil
	}
	return DB.Stat()
}

// HealthCheck verifies database connectivity
func HealthCheck(ctx context.Context) error {
	if DB == nil {
		return fmt.Errorf("database connection not initialized")
	}
	return DB.Ping(ctx)
}
