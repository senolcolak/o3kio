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
		adminUserID  = "00000000-0000-0000-0000-000000000001"
		projectID    = "00000000-0000-0000-0000-000000000002"
		adminRoleID  = "00000000-0000-0000-0000-000000000003"
		memberRoleID = "00000000-0000-0000-0000-000000000004"
		// defaultDomainID matches the row seeded by migration
		// 009_seed_default_domain.up.sql. Keystone auth filters user and
		// project lookups by domain_id, so the seeded admin user and
		// default project must belong to this domain or "openstack token
		// issue" returns 401 invalid credentials.
		defaultDomainID = "00000000-0000-0000-0000-000000000100"
	)

	hash, err := bcrypt.GenerateFromPassword([]byte(adminPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash admin password: %w", err)
	}

	type stmt struct {
		sql  string
		args []any
	}

	stmts := []stmt{
		// Default project — must belong to the Default domain so Keystone
		// auth's project lookup (filtered by domain_id) finds it.
		{
			`INSERT INTO projects (id, name, description, enabled, domain_id)
			 VALUES ($1, $2, $3, $4, $5) ON CONFLICT (name) DO NOTHING`,
			[]any{projectID, "default", "Default project", true, defaultDomainID},
		},
		// Admin user — must belong to the Default domain for the same reason.
		{
			`INSERT INTO users (id, name, password_hash, enabled, domain_id)
			 VALUES ($1, $2, $3, $4, $5) ON CONFLICT (name) DO NOTHING`,
			[]any{adminUserID, "admin", string(hash), true, defaultDomainID},
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

	// SCS-0103-v1 mandatory standard flavors. Mirrors migration 075 so
	// zero-config (in-code seed) installs match docker-compose installs.
	// See: https://docs.scs.community/standards/scs-0103-v1-standard-flavors/
	type scsFlavor struct {
		id      string
		name    string
		vcpus   int
		ramMB   int
		diskGB  int
		cpuType string // "shared-core" or "crowded-core"
		ssdDisk bool   // disk0-type = ssd when true
	}
	scsFlavors := []scsFlavor{
		{"00000000-0000-0000-0000-0000000005c1", "SCS-1L-1", 1, 1024, 0, "crowded-core", false},
		{"00000000-0000-0000-0000-0000000005c2", "SCS-1V-2", 1, 2048, 0, "shared-core", false},
		{"00000000-0000-0000-0000-0000000005c3", "SCS-1V-4", 1, 4096, 0, "shared-core", false},
		{"00000000-0000-0000-0000-0000000005c4", "SCS-1V-8", 1, 8192, 0, "shared-core", false},
		{"00000000-0000-0000-0000-0000000005c5", "SCS-2V-4", 2, 4096, 0, "shared-core", false},
		{"00000000-0000-0000-0000-0000000005c6", "SCS-2V-4-20s", 2, 4096, 20, "shared-core", true},
		{"00000000-0000-0000-0000-0000000005c7", "SCS-2V-8", 2, 8192, 0, "shared-core", false},
		{"00000000-0000-0000-0000-0000000005c8", "SCS-2V-16", 2, 16384, 0, "shared-core", false},
		{"00000000-0000-0000-0000-0000000005c9", "SCS-4V-8", 4, 8192, 0, "shared-core", false},
		{"00000000-0000-0000-0000-0000000005ca", "SCS-4V-16", 4, 16384, 0, "shared-core", false},
		{"00000000-0000-0000-0000-0000000005cb", "SCS-4V-16-100s", 4, 16384, 100, "shared-core", true},
		{"00000000-0000-0000-0000-0000000005cc", "SCS-4V-32", 4, 32768, 0, "shared-core", false},
		{"00000000-0000-0000-0000-0000000005cd", "SCS-8V-16", 8, 16384, 0, "shared-core", false},
		{"00000000-0000-0000-0000-0000000005ce", "SCS-8V-32", 8, 32768, 0, "shared-core", false},
		{"00000000-0000-0000-0000-0000000005cf", "SCS-16V-32", 16, 32768, 0, "shared-core", false},
	}
	for _, f := range scsFlavors {
		stmts = append(stmts, stmt{
			`INSERT INTO flavors (id, name, vcpus, ram_mb, disk_gb, is_public)
			 VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (name) DO NOTHING`,
			[]any{f.id, f.name, f.vcpus, f.ramMB, f.diskGB, true},
		})
		stmts = append(stmts, stmt{
			`INSERT INTO flavor_extra_specs (flavor_id, key, value)
			 VALUES ($1, $2, $3) ON CONFLICT (flavor_id, key) DO NOTHING`,
			[]any{f.id, "scs:cpu-type", f.cpuType},
		})
		if f.ssdDisk {
			stmts = append(stmts, stmt{
				`INSERT INTO flavor_extra_specs (flavor_id, key, value)
				 VALUES ($1, $2, $3) ON CONFLICT (flavor_id, key) DO NOTHING`,
				[]any{f.id, "scs:disk0-type", "ssd"},
			})
		}
	}

	for _, s := range stmts {
		if _, err := db.Exec(ctx, s.sql, s.args...); err != nil {
			return fmt.Errorf("seed: %w", err)
		}
	}

	return nil
}
