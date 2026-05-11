// Package migrations embeds SQLite migration files into the binary so that
// single-binary deployments do not require the migrations/ directory at runtime.
package migrations

import "embed"

// SQLiteFS contains all SQLite migration files under sqlite/*.up.sql.
// It is passed to database.MigrateSQLiteFS instead of a filesystem path.
//
//go:embed sqlite/*.up.sql
var SQLiteFS embed.FS
