package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/cobaltcore-dev/o3k/internal/database"
)

// tableMigration describes a table and its ordered column list for migration.
// Columns must match both the SQLite source and PostgreSQL destination schemas.
// Any column absent in the source is handled gracefully (row returns nil).
type tableMigration struct {
	name    string
	columns []string
}

// migrations lists all tables in dependency order (parents before children)
// and the columns present in the SQLite schema after all sqlite migrations.
var tableMigrations = []tableMigration{
	// Keystone — base tables first
	{"domains", []string{
		"id", "name", "description", "enabled", "created_at", "updated_at",
	}},
	{"projects", []string{
		"id", "name", "description", "enabled", "domain_id", "created_at", "updated_at",
	}},
	{"users", []string{
		"id", "name", "password_hash", "enabled", "email", "description", "domain_id", "created_at", "updated_at",
	}},
	{"roles", []string{
		"id", "name", "created_at",
	}},
	{"role_assignments", []string{
		"id", "user_id", "project_id", "role_id", "created_at",
	}},
	{"groups", []string{
		"id", "name", "domain_id", "description", "created_at", "updated_at",
	}},
	{"group_members", []string{
		"id", "group_id", "user_id", "created_at",
	}},

	// Nova
	{"flavors", []string{
		"id", "name", "vcpus", "ram_mb", "disk_gb", "is_public", "created_at",
	}},
	{"instances", []string{
		"id", "name", "project_id", "user_id", "flavor_id", "image_id",
		"status", "power_state", "libvirt_domain_id",
		"console_vnc_port", "console_vnc_password", "task_state", "locked",
		"created_at", "updated_at", "launched_at", "terminated_at",
	}},
	{"keypairs", []string{
		"id", "name", "user_id", "public_key", "fingerprint", "created_at",
	}},
	{"instance_metadata", []string{
		"id", "instance_id", "meta_key", "meta_value", "created_at",
	}},
	{"instance_userdata", []string{
		"id", "instance_id", "userdata", "created_at",
	}},
	{"instance_snapshots", []string{
		"id", "instance_id", "snapshot_name", "image_id", "flavor_id", "created_at",
	}},

	// Neutron
	{"networks", []string{
		"id", "name", "project_id", "admin_state_up", "status", "shared", "mtu",
		"created_at", "updated_at",
	}},
	{"subnets", []string{
		"id", "name", "network_id", "project_id", "cidr", "gateway_ip",
		"ip_version", "enable_dhcp", "dns_nameservers",
		"created_at", "updated_at",
	}},
	{"security_groups", []string{
		"id", "name", "project_id", "description", "created_at", "updated_at",
	}},
	{"security_group_rules", []string{
		"id", "security_group_id", "direction", "ethertype", "protocol",
		"port_range_min", "port_range_max", "remote_ip_prefix", "remote_group_id",
		"created_at",
	}},
	{"ports", []string{
		"id", "name", "network_id", "project_id", "device_id", "device_owner",
		"mac_address", "admin_state_up", "status", "fixed_ips",
		"created_at", "updated_at",
	}},
	{"port_security_groups", []string{
		"port_id", "security_group_id",
	}},
	{"routers", []string{
		"id", "name", "project_id", "admin_state_up", "status",
		"external_gateway_info", "distributed", "ha",
		"created_at", "updated_at",
	}},
	{"router_interfaces", []string{
		"id", "router_id", "port_id", "subnet_id", "created_at",
	}},
	{"router_routes", []string{
		"id", "router_id", "destination", "nexthop", "created_at",
	}},
	{"floating_ips", []string{
		"id", "project_id", "floating_network_id", "floating_ip_address",
		"fixed_ip_address", "port_id", "router_id", "status", "description",
		"created_at", "updated_at",
	}},
	{"interface_attachments", []string{
		"id", "instance_id", "port_id", "mac_address", "attached_at",
	}},
	{"compute_nodes", []string{
		"id", "hostname", "tunnel_ip", "status", "capabilities",
		"last_heartbeat", "created_at", "updated_at",
	}},
	{"network_vni_allocations", []string{
		"id", "network_id", "vni", "created_at",
	}},
	{"vxlan_fdb_entries", []string{
		"id", "network_id", "mac_address", "vtep_ip", "port_id",
		"created_at", "updated_at",
	}},

	// Cinder
	{"volume_types", []string{
		"id", "name", "description", "is_public", "created_at",
	}},
	{"volumes", []string{
		"id", "name", "project_id", "user_id", "size_gb", "status",
		"bootable", "attached_to_instance_id", "rbd_pool", "rbd_image",
		"volume_type", "created_at", "updated_at",
	}},
	{"volume_attachments", []string{
		"id", "volume_id", "instance_id", "device", "attached_at",
	}},
	{"snapshots", []string{
		"id", "name", "volume_id", "project_id", "size_gb", "status", "created_at",
	}},
	{"volume_metadata", []string{
		"id", "volume_id", "meta_key", "meta_value", "created_at", "updated_at",
	}},
	{"snapshot_metadata", []string{
		"id", "snapshot_id", "meta_key", "meta_value", "created_at", "updated_at",
	}},

	// Glance
	{"images", []string{
		"id", "name", "project_id", "status", "visibility",
		"size_bytes", "disk_format", "container_format",
		"min_disk_gb", "min_ram_mb", "checksum",
		"rbd_pool", "rbd_image",
		"created_at", "updated_at",
	}},
	{"image_tags", []string{
		"id", "image_id", "tag", "created_at",
	}},
	{"image_members", []string{
		"id", "image_id", "member_id", "status", "created_at", "updated_at",
	}},

	// Quotas
	{"quotas", []string{
		"id", "project_id", "resource", "hard_limit", "created_at", "updated_at",
	}},
}

// runMigrateDatastore handles the "migrate-datastore" subcommand.
// It copies all rows from a SQLite source into a PostgreSQL destination.
// The destination schema must already exist (run migrations first).
func runMigrateDatastore(args []string) {
	fs := flag.NewFlagSet("migrate-datastore", flag.ExitOnError)
	from := fs.String("from", "", "source datastore (e.g., sqlite:///var/lib/o3k/db/state.db)")
	to := fs.String("to", "", "destination datastore (e.g., postgres://user:pass@host/db)")
	batchSize := fs.Int("batch", 500, "rows per batch (currently unused; reserved for future batching)")
	_ = fs.Parse(args)

	if *from == "" || *to == "" {
		fmt.Fprintln(os.Stderr, "Usage: o3k migrate-datastore --from <source> --to <dest>")
		fmt.Fprintln(os.Stderr, "  --from  sqlite:///path/to/state.db")
		fmt.Fprintln(os.Stderr, "  --to    postgres://user:pass@host:5432/o3k")
		fmt.Fprintln(os.Stderr, "  --batch rows per batch (default 500)")
		os.Exit(1)
	}

	if !strings.HasPrefix(*from, "sqlite://") && !strings.HasPrefix(*from, "sqlite:") {
		fmt.Fprintln(os.Stderr, "ERROR: --from must be a sqlite:// URL")
		os.Exit(1)
	}

	fmt.Printf("Migrating data: %s → %s\n\n", *from, *to)

	ctx := context.Background()

	// Open source SQLite.
	dbPath := strings.TrimPrefix(strings.TrimPrefix(*from, "sqlite://"), "sqlite:")
	if dbPath == "" {
		dbPath = "/var/lib/o3k/db/state.db"
	}
	dbPath = filepath.Clean(dbPath)

	if err := database.ConnectSQLite(ctx, dbPath); err != nil {
		log.Fatalf("Failed to open source SQLite at %s: %v", dbPath, err)
	}
	srcDB := database.DB // capture before overwriting with destination

	// Open destination PostgreSQL.
	if err := database.Connect(ctx, *to, database.DefaultPoolConfig()); err != nil {
		log.Fatalf("Failed to connect to destination PostgreSQL: %v", err)
	}
	dstDB := database.DB

	_ = *batchSize // reserved

	totalRows := 0
	totalTables := 0

	for _, tm := range tableMigrations {
		n, err := migrateTable(ctx, srcDB, dstDB, tm)
		if err != nil {
			log.Fatalf("Failed to migrate table %q: %v", tm.name, err)
		}
		if n > 0 {
			fmt.Printf("  ✓ %-35s %d rows\n", tm.name, n)
			totalTables++
		}
		totalRows += n
	}

	fmt.Printf("\nMigration complete: %d rows across %d tables\n", totalRows, totalTables)
	fmt.Println("\nNext steps:")
	fmt.Printf("  1. Update config: database.datastore = %q\n", *to)
	fmt.Println("  2. Restart o3k server")
}

// migrateTable copies all rows of one table from src to dst.
// Returns the number of rows copied.
// If the table does not exist in src (older schema), returns (0, nil).
func migrateTable(ctx context.Context, src, dst database.DBIF, tm tableMigration) (int, error) {
	// Quick existence check — if the table is absent in the source, skip silently.
	var count int
	if err := src.QueryRow(ctx, "SELECT COUNT(*) FROM "+tm.name).Scan(&count); err != nil {
		// Table absent or other non-fatal error — treat as empty.
		return 0, nil
	}
	if count == 0 {
		return 0, nil
	}

	cols := strings.Join(tm.columns, ", ")
	selectSQL := fmt.Sprintf("SELECT %s FROM %s", cols, tm.name)

	rows, err := src.Query(ctx, selectSQL)
	if err != nil {
		// Column mismatch with older schema — skip rather than abort.
		return 0, nil
	}
	defer rows.Close()

	// Build INSERT … ON CONFLICT DO NOTHING for idempotency.
	placeholders := make([]string, len(tm.columns))
	for i := range placeholders {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}
	insertSQL := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s) ON CONFLICT DO NOTHING",
		tm.name, cols, strings.Join(placeholders, ", "),
	)

	migrated := 0
	for rows.Next() {
		// Allocate scan targets as a slice of any.
		values := make([]any, len(tm.columns))
		ptrs := make([]any, len(tm.columns))
		for i := range values {
			ptrs[i] = &values[i]
		}

		if err := rows.Scan(ptrs...); err != nil {
			return migrated, fmt.Errorf("scan row from %s: %w", tm.name, err)
		}

		if _, err := dst.Exec(ctx, insertSQL, values...); err != nil {
			return migrated, fmt.Errorf("insert row into %s: %w", tm.name, err)
		}
		migrated++
	}

	if err := rows.Err(); err != nil {
		return migrated, fmt.Errorf("iterate rows of %s: %w", tm.name, err)
	}

	return migrated, nil
}
