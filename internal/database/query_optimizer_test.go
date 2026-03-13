package database

import (
	"testing"
	"time"
)

func TestDefaultPoolConfig(t *testing.T) {
	cfg := DefaultPoolConfig()

	if cfg.MaxConns != 20 {
		t.Errorf("Expected MaxConns=20, got %d", cfg.MaxConns)
	}

	if cfg.MinConns != 2 {
		t.Errorf("Expected MinConns=2, got %d", cfg.MinConns)
	}

	if cfg.MaxConnLifetime != 1*time.Hour {
		t.Errorf("Expected MaxConnLifetime=1h, got %v", cfg.MaxConnLifetime)
	}

	if cfg.MaxConnIdleTime != 15*time.Minute {
		t.Errorf("Expected MaxConnIdleTime=15m, got %v", cfg.MaxConnIdleTime)
	}

	if cfg.HealthCheckPeriod != 1*time.Minute {
		t.Errorf("Expected HealthCheckPeriod=1m, got %v", cfg.HealthCheckPeriod)
	}
}

func TestNewQueryLogger(t *testing.T) {
	threshold := 100 * time.Millisecond
	ql := NewQueryLogger(threshold)

	if ql.SlowQueryThreshold != threshold {
		t.Errorf("Expected threshold=%v, got %v", threshold, ql.SlowQueryThreshold)
	}
}

func TestNewQueryAnalyzer(t *testing.T) {
	qa := NewQueryAnalyzer()

	if qa == nil {
		t.Error("Expected non-nil QueryAnalyzer")
	}
}

func TestGetQueryStats_NoConnection(t *testing.T) {
	// Save original DB
	originalDB := DB
	defer func() { DB = originalDB }()

	// Set DB to nil
	DB = nil

	stats := GetQueryStats()
	if stats != nil {
		t.Error("Expected nil stats when DB is nil")
	}
}

func TestIndexSuggestions(t *testing.T) {
	if len(CommonIndexSuggestions) == 0 {
		t.Error("Expected some common index suggestions")
	}

	// Check that suggestions have required fields
	for i, suggestion := range CommonIndexSuggestions {
		if suggestion.Table == "" {
			t.Errorf("Suggestion %d missing table name", i)
		}

		if len(suggestion.Columns) == 0 {
			t.Errorf("Suggestion %d missing columns", i)
		}

		if suggestion.Reason == "" {
			t.Errorf("Suggestion %d missing reason", i)
		}
	}
}

func TestCommonIndexSuggestions_Coverage(t *testing.T) {
	// Verify we have suggestions for key tables
	expectedTables := []string{
		"instances",
		"volumes",
		"networks",
		"ports",
		"security_group_rules",
		"floating_ips",
		"images",
	}

	suggestionMap := make(map[string]bool)
	for _, suggestion := range CommonIndexSuggestions {
		suggestionMap[suggestion.Table] = true
	}

	for _, table := range expectedTables {
		if !suggestionMap[table] {
			t.Errorf("Missing index suggestion for table: %s", table)
		}
	}
}
