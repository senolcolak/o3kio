package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var DB DBIF

var pool *pgxpool.Pool

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

	p, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Test connection
	if err := p.Ping(ctx); err != nil {
		p.Close()
		return fmt.Errorf("failed to ping database: %w", err)
	}

	DB = NewPgxAdapter(p)
	pool = p
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

// ConnectSQLite opens a SQLite database at dbPath and sets DB.
func ConnectSQLite(ctx context.Context, dbPath string) error {
	adapter, err := NewSQLiteAdapter(dbPath)
	if err != nil {
		return fmt.Errorf("connect sqlite: %w", err)
	}
	DB = adapter
	return nil
}

// BackendType returns "sqlite" or "postgres" based on the active DB connection.
func BackendType() string {
	if _, ok := DB.(*SQLiteAdapter); ok {
		return "sqlite"
	}
	return "postgres"
}

// Close closes the database connection pool (postgres) or SQLite file.
func Close() {
	if pool != nil {
		pool.Close()
	}
	if a, ok := DB.(*SQLiteAdapter); ok {
		a.Close()
	}
}

func Stats() *pgxpool.Stat {
	if pool == nil {
		return nil
	}
	return pool.Stat()
}

func HealthCheck(ctx context.Context) error {
	if pool == nil {
		return fmt.Errorf("database connection not initialized")
	}
	return pool.Ping(ctx)
}
