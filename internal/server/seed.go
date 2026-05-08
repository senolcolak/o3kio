package server

import (
	"context"
	"fmt"

	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// SeedDefaults creates the admin user, default project, and admin/member roles
// if they don't already exist. Idempotent — safe to call on every startup.
//
// Uses fixed UUIDs so that repeated calls never duplicate rows and foreign
// keys remain stable across restarts.
func SeedDefaults(ctx context.Context, db database.DBIF, adminPassword string) error {
	// Check if admin user already exists — if so, nothing to do.
	var exists int
	err := db.QueryRow(ctx, "SELECT 1 FROM users WHERE name = $1", "admin").Scan(&exists)
	if err == nil {
		return nil // Already seeded.
	}
	if err != database.ErrNoRows {
		return fmt.Errorf("check existing seed: %w", err)
	}

	const (
		projectID    = "00000000-0000-0000-0000-000000000002"
		adminRoleID  = "00000000-0000-0000-0000-000000000003"
		memberRoleID = "00000000-0000-0000-0000-000000000004"
	)

	adminUserID := uuid.New().String()

	hash, err := bcrypt.GenerateFromPassword([]byte(adminPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash admin password: %w", err)
	}

	type stmt struct {
		sql  string
		args []any
	}

	stmts := []stmt{
		// Default project
		{
			`INSERT INTO projects (id, name, description, enabled)
			 VALUES ($1, $2, $3, $4) ON CONFLICT (name) DO NOTHING`,
			[]any{projectID, "default", "Default project", true},
		},
		// Admin user
		{
			`INSERT INTO users (id, name, password_hash, enabled)
			 VALUES ($1, $2, $3, $4) ON CONFLICT (name) DO NOTHING`,
			[]any{adminUserID, "admin", string(hash), true},
		},
		// Roles
		{
			`INSERT INTO roles (id, name) VALUES ($1, $2) ON CONFLICT (name) DO NOTHING`,
			[]any{adminRoleID, "admin"},
		},
		{
			`INSERT INTO roles (id, name) VALUES ($1, $2) ON CONFLICT (name) DO NOTHING`,
			[]any{memberRoleID, "member"},
		},
		// Assign admin role to admin user on default project.
		// role_assignments has a composite unique on (user_id, project_id, role_id)
		// and a required primary key id column.
		{
			`INSERT INTO role_assignments (id, user_id, project_id, role_id)
			 VALUES ($1, $2, $3, $4) ON CONFLICT (user_id, project_id, role_id) DO NOTHING`,
			[]any{uuid.New().String(), adminUserID, projectID, adminRoleID},
		},
		// Default flavors
		{
			`INSERT INTO flavors (id, name, vcpus, ram_mb, disk_gb, is_public)
			 VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (name) DO NOTHING`,
			[]any{"00000000-0000-0000-0000-000000000010", "m1.tiny", 1, 512, 1, true},
		},
		{
			`INSERT INTO flavors (id, name, vcpus, ram_mb, disk_gb, is_public)
			 VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (name) DO NOTHING`,
			[]any{"00000000-0000-0000-0000-000000000011", "m1.small", 1, 2048, 20, true},
		},
		{
			`INSERT INTO flavors (id, name, vcpus, ram_mb, disk_gb, is_public)
			 VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (name) DO NOTHING`,
			[]any{"00000000-0000-0000-0000-000000000012", "m1.medium", 2, 4096, 40, true},
		},
		{
			`INSERT INTO flavors (id, name, vcpus, ram_mb, disk_gb, is_public)
			 VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (name) DO NOTHING`,
			[]any{"00000000-0000-0000-0000-000000000013", "m1.large", 4, 8192, 80, true},
		},
		{
			`INSERT INTO flavors (id, name, vcpus, ram_mb, disk_gb, is_public)
			 VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (name) DO NOTHING`,
			[]any{"00000000-0000-0000-0000-000000000014", "m1.xlarge", 8, 16384, 160, true},
		},
	}

	for _, s := range stmts {
		if _, err := db.Exec(ctx, s.sql, s.args...); err != nil {
			return fmt.Errorf("seed: %w", err)
		}
	}

	return nil
}
