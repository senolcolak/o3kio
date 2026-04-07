package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// QueryLogger wraps database operations with execution time logging
type QueryLogger struct {
	pool             *pgxpool.Pool
	slowQueryThreshold time.Duration // Log queries slower than this
}

// NewQueryLogger creates a query logger with slow query threshold
func NewQueryLogger(pool *pgxpool.Pool, slowQueryThreshold time.Duration) *QueryLogger {
	return &QueryLogger{
		pool:             pool,
		slowQueryThreshold: slowQueryThreshold,
	}
}

// Query executes a query with timing
func (ql *QueryLogger) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	start := time.Now()
	rows, err := ql.pool.Query(ctx, sql, args...)
	duration := time.Since(start)

	if duration > ql.slowQueryThreshold {
		log.Warn().Str("sql", sql).Dur("duration", duration).Interface("args", args).Msg("slow query")
	}

	return rows, err
}

// QueryRow executes a single-row query with timing
func (ql *QueryLogger) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	start := time.Now()
	row := ql.pool.QueryRow(ctx, sql, args...)
	duration := time.Since(start)

	if duration > ql.slowQueryThreshold {
		log.Warn().Str("sql", sql).Dur("duration", duration).Interface("args", args).Msg("slow query")
	}

	return row
}

// Exec executes a command with timing
func (ql *QueryLogger) Exec(ctx context.Context, sql string, args ...interface{}) error {
	start := time.Now()
	_, err := ql.pool.Exec(ctx, sql, args...)
	duration := time.Since(start)

	if duration > ql.slowQueryThreshold {
		log.Warn().Str("sql", sql).Dur("duration", duration).Interface("args", args).Msg("slow query")
	}

	return err
}

// QueryStats holds query performance statistics
type QueryStats struct {
	TotalQueries   int64
	SlowQueries    int64
	TotalDuration  time.Duration
	AverageDuration time.Duration
	SlowestQuery   string
	SlowestDuration time.Duration
}

// QueryStatsCollector collects query statistics
type QueryStatsCollector struct {
	stats            QueryStats
	slowQueryThreshold time.Duration
}

// NewQueryStatsCollector creates a new query statistics collector
func NewQueryStatsCollector(slowQueryThreshold time.Duration) *QueryStatsCollector {
	return &QueryStatsCollector{
		slowQueryThreshold: slowQueryThreshold,
	}
}

// RecordQuery records a query execution
func (qsc *QueryStatsCollector) RecordQuery(sql string, duration time.Duration) {
	qsc.stats.TotalQueries++
	qsc.stats.TotalDuration += duration

	if duration > qsc.slowQueryThreshold {
		qsc.stats.SlowQueries++
	}

	if duration > qsc.stats.SlowestDuration {
		qsc.stats.SlowestQuery = sql
		qsc.stats.SlowestDuration = duration
	}

	qsc.stats.AverageDuration = time.Duration(int64(qsc.stats.TotalDuration) / qsc.stats.TotalQueries)
}

// GetStats returns current statistics
func (qsc *QueryStatsCollector) GetStats() QueryStats {
	return qsc.stats
}

// ResetStats resets all statistics
func (qsc *QueryStatsCollector) ResetStats() {
	qsc.stats = QueryStats{}
}

// PrintStats prints formatted statistics
func (qsc *QueryStatsCollector) PrintStats() {
	fmt.Printf("\n=== Query Performance Statistics ===\n")
	fmt.Printf("Total Queries:    %d\n", qsc.stats.TotalQueries)
	fmt.Printf("Slow Queries:     %d (%.2f%%)\n",
		qsc.stats.SlowQueries,
		float64(qsc.stats.SlowQueries)/float64(qsc.stats.TotalQueries)*100)
	fmt.Printf("Total Duration:   %v\n", qsc.stats.TotalDuration)
	fmt.Printf("Average Duration: %v\n", qsc.stats.AverageDuration)
	fmt.Printf("Slowest Query:    %s\n", qsc.stats.SlowestQuery)
	fmt.Printf("Slowest Duration: %v\n", qsc.stats.SlowestDuration)
	fmt.Printf("===================================\n\n")
}
