package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rs/zerolog/log"
)

// QueryLogger wraps database queries with logging and performance monitoring
type QueryLogger struct {
	SlowQueryThreshold time.Duration
}

// NewQueryLogger creates a new query logger
func NewQueryLogger(slowQueryThreshold time.Duration) *QueryLogger {
	return &QueryLogger{
		SlowQueryThreshold: slowQueryThreshold,
	}
}

// Query executes a query and logs performance metrics
func (ql *QueryLogger) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	start := time.Now()
	rows, err := DB.Query(ctx, sql, args...)
	duration := time.Since(start)

	if duration > ql.SlowQueryThreshold {
		log.Warn().
			Dur("duration", duration).
			Str("query", sql).
			Interface("args", args).
			Msg("Slow query detected")
	}

	return rows, err
}

// QueryRow executes a single-row query and logs performance metrics
func (ql *QueryLogger) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	start := time.Now()
	row := DB.QueryRow(ctx, sql, args...)
	duration := time.Since(start)

	if duration > ql.SlowQueryThreshold {
		log.Warn().
			Dur("duration", duration).
			Str("query", sql).
			Interface("args", args).
			Msg("Slow query detected")
	}

	return row
}

// Exec executes a command and logs performance metrics
func (ql *QueryLogger) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	start := time.Now()
	tag, err := DB.Exec(ctx, sql, args...)
	duration := time.Since(start)

	if duration > ql.SlowQueryThreshold {
		log.Warn().
			Dur("duration", duration).
			Str("query", sql).
			Interface("args", args).
			Msg("Slow query detected")
	}

	return tag, err
}

// QueryAnalyzer provides utilities for analyzing query performance
type QueryAnalyzer struct{}

// NewQueryAnalyzer creates a new query analyzer
func NewQueryAnalyzer() *QueryAnalyzer {
	return &QueryAnalyzer{}
}

// Explain runs EXPLAIN ANALYZE on a query and returns the plan
func (qa *QueryAnalyzer) Explain(ctx context.Context, sql string, args ...interface{}) (string, error) {
	explainSQL := fmt.Sprintf("EXPLAIN ANALYZE %s", sql)

	rows, err := DB.Query(ctx, explainSQL, args...)
	if err != nil {
		return "", fmt.Errorf("failed to explain query: %w", err)
	}
	defer rows.Close()

	var plan string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			return "", err
		}
		plan += line + "\n"
	}

	return plan, nil
}

// GetQueryStats returns statistics about the connection pool
func GetQueryStats() map[string]interface{} {
	if DB == nil {
		return nil
	}

	stat := DB.Stat()
	return map[string]interface{}{
		"total_conns":      stat.TotalConns(),
		"acquired_conns":   stat.AcquiredConns(),
		"idle_conns":       stat.IdleConns(),
		"max_conns":        stat.MaxConns(),
		"acquire_count":    stat.AcquireCount(),
		"acquire_duration": stat.AcquireDuration().String(),
		"canceled_acquire": stat.CanceledAcquireCount(),
		"constructing":     stat.ConstructingConns(),
		"empty_acquire":    stat.EmptyAcquireCount(),
		"max_idle_destroy": stat.MaxIdleDestroyCount(),
		"max_lifetime":     stat.MaxLifetimeDestroyCount(),
	}
}

// IndexSuggestion represents a suggested database index
type IndexSuggestion struct {
	Table   string
	Columns []string
	Reason  string
}

// Common index suggestions based on query patterns
var CommonIndexSuggestions = []IndexSuggestion{
	{
		Table:   "instances",
		Columns: []string{"project_id", "status"},
		Reason:  "Frequently filtered by project and status",
	},
	{
		Table:   "volumes",
		Columns: []string{"project_id", "status"},
		Reason:  "Frequently filtered by project and status",
	},
	{
		Table:   "networks",
		Columns: []string{"project_id"},
		Reason:  "Frequently filtered by project",
	},
	{
		Table:   "ports",
		Columns: []string{"network_id"},
		Reason:  "Frequently joined with networks",
	},
	{
		Table:   "security_group_rules",
		Columns: []string{"security_group_id"},
		Reason:  "Frequently joined with security groups",
	},
	{
		Table:   "floating_ips",
		Columns: []string{"port_id"},
		Reason:  "Frequently looked up by port",
	},
	{
		Table:   "images",
		Columns: []string{"project_id", "visibility"},
		Reason:  "Frequently filtered by project and visibility",
	},
}

// CheckMissingIndexes analyzes tables and suggests missing indexes
func (qa *QueryAnalyzer) CheckMissingIndexes(ctx context.Context) ([]IndexSuggestion, error) {
	var suggestions []IndexSuggestion

	for _, suggestion := range CommonIndexSuggestions {
		// Check if index exists
		query := `
			SELECT EXISTS (
				SELECT 1
				FROM pg_indexes
				WHERE tablename = $1
				AND indexdef LIKE '%' || $2 || '%'
			)
		`

		columnList := fmt.Sprintf("(%s)", suggestion.Columns[0])
		var exists bool
		err := DB.QueryRow(ctx, query, suggestion.Table, columnList).Scan(&exists)
		if err != nil {
			continue
		}

		if !exists {
			suggestions = append(suggestions, suggestion)
		}
	}

	return suggestions, nil
}
