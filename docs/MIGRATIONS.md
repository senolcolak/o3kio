# Database Migration System

## Overview

O3K uses [golang-migrate](https://github.com/golang-migrate/migrate) for database schema versioning and migrations. The migration system provides a robust way to manage database schema changes across development, staging, and production environments.

## Migration Tool: `o3k-migrate`

The `o3k-migrate` CLI tool provides commands for managing database migrations.

### Installation

Build the migration tool:
```bash
go build -o o3k-migrate ./cmd/o3k-migrate
```

### Commands

#### `status` - Show Migration Status
Display the current migration version and state.

```bash
./o3k-migrate status
```

Output:
```
Database Migration Status
=========================
Current Version: 7
Status: Clean
```

#### `up` - Apply Pending Migrations
Apply all pending migrations to bring the database to the latest version.

```bash
./o3k-migrate up
```

Output:
```
✅ Migrations applied successfully
```

#### `down` - Roll Back One Migration
Roll back the most recent migration.

```bash
./o3k-migrate down
```

**Warning:** This requires confirmation and will undo one migration.

#### `goto <version>` - Migrate to Specific Version
Migrate to a specific version (can go up or down).

```bash
./o3k-migrate goto 5
```

**Use Cases:**
- Testing migration rollbacks
- Deploying specific database versions
- Debugging migration issues

#### `reset` - Drop and Recreate Database
Drop all tables and re-run all migrations from scratch.

```bash
./o3k-migrate reset
```

**⚠️ DANGER:** This will DELETE ALL DATA! Only use in development.

#### `force <version>` - Force Migration Version
Manually set the migration version without running migrations.

```bash
./o3k-migrate force 7
```

**Use Cases:**
- Fixing dirty state after failed migration
- Manual database recovery
- Syncing migration version with manually applied schema

### Options

- `-config <path>` - Path to configuration file (default: `config/o3k.yaml`)
- `-migrations <path>` - Path to migrations directory (default: `migrations`)

### Examples

```bash
# Check current migration status
./o3k-migrate status

# Apply all pending migrations
./o3k-migrate up

# Roll back one migration
./o3k-migrate down

# Migrate to version 5
./o3k-migrate goto 5

# Force version to 7 (for dirty state recovery)
./o3k-migrate force 7

# Use custom config file
./o3k-migrate -config /etc/o3k/config.yaml status
```

## Migration Files

Migration files are stored in the `migrations/` directory with the following naming convention:

```
<version>_<description>.<direction>.sql
```

- `<version>` - Sequential number (001, 002, 003, etc.)
- `<description>` - Descriptive name (e.g., `initial_schema`, `add_quotas`)
- `<direction>` - Either `up` or `down`

### Example Migration Files

```
migrations/
├── 001_initial_schema.up.sql
├── 001_initial_schema.down.sql
├── 002_seed_data.up.sql
├── 002_seed_data.down.sql
├── 003_add_routers.up.sql
├── 003_add_routers.down.sql
├── 004_add_vxlan.up.sql
├── 004_add_vxlan.down.sql
├── 005_add_compute_features.up.sql
├── 005_add_compute_features.down.sql
├── 006_add_port_security_groups.up.sql
├── 006_add_port_security_groups.down.sql
├── 007_add_quotas.up.sql
└── 007_add_quotas.down.sql
```

### Current Migrations

| Version | Description | Purpose |
|---------|-------------|---------|
| 001 | initial_schema | Core tables for all services |
| 002 | seed_data | Default admin user, project, roles |
| 003 | add_routers | Router and interface tables |
| 004 | add_vxlan | Multi-node VXLAN overlay support |
| 005 | add_compute_features | Instance interfaces, volume attachments |
| 006 | add_port_security_groups | Security group associations |
| 007 | add_quotas | Resource quota management |

## Creating New Migrations

### Manual Creation

1. Create paired up/down files:
```bash
touch migrations/008_add_feature.up.sql
touch migrations/008_add_feature.down.sql
```

2. Write the up migration (schema changes):
```sql
-- migrations/008_add_feature.up.sql
CREATE TABLE new_feature (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_new_feature_name ON new_feature(name);
```

3. Write the down migration (rollback):
```sql
-- migrations/008_add_feature.down.sql
DROP INDEX IF EXISTS idx_new_feature_name;
DROP TABLE IF EXISTS new_feature;
```

### Best Practices

1. **Always Create Paired Files**: Every `.up.sql` must have a corresponding `.down.sql`
2. **Use IF EXISTS/IF NOT EXISTS**: Make migrations idempotent where possible
3. **Test Rollbacks**: Always test that down migrations work
4. **One Logical Change Per Migration**: Don't mix unrelated schema changes
5. **Use Transactions**: Wrap migrations in transactions when possible
6. **Document Complex Changes**: Add comments explaining non-obvious changes

### Migration Guidelines

**DO:**
- Use `CREATE TABLE IF NOT EXISTS`
- Use `DROP TABLE IF EXISTS`
- Add `ON CONFLICT DO NOTHING` for seed data
- Create indexes with descriptive names
- Add foreign key constraints for referential integrity
- Include default values where appropriate

**DON'T:**
- Mix DDL and DML in the same migration (unless necessary)
- Use database-specific features without fallbacks
- Delete data without explicit user action
- Create migrations that can't be rolled back
- Use hardcoded timestamps or auto-increment IDs

## Integration with O3K

The main O3K binary automatically runs migrations on startup:

```go
// cmd/o3k/main.go
log.Println("Running database migrations...")
if err := database.MigrateUp(cfg.Database.URL, *migrationsPath); err != nil {
    log.Fatalf("Failed to run migrations: %v", err)
}
```

### Disabling Auto-Migration

For production deployments, you may want to run migrations manually:

1. Comment out the migration code in `cmd/o3k/main.go`
2. Run migrations separately before deploying:
```bash
./o3k-migrate up
systemctl restart o3k
```

## Troubleshooting

### Dirty State

**Problem:** Migration failed mid-execution, leaving database in "dirty" state.

**Solution:**
1. Check status: `./o3k-migrate status`
2. Manually fix database if needed
3. Force version: `./o3k-migrate force <version>`

### Migration Failed

**Problem:** Migration fails with SQL error.

**Diagnosis:**
1. Check migration SQL syntax
2. Verify database permissions
3. Check for conflicting schema changes

**Solution:**
1. Fix the migration file
2. Roll back: `./o3k-migrate down`
3. Re-apply: `./o3k-migrate up`

### Version Mismatch

**Problem:** Migration version doesn't match expected version.

**Solution:**
1. Check current version: `./o3k-migrate status`
2. List migration files: `ls migrations/*.up.sql`
3. Use `goto` to reach desired version: `./o3k-migrate goto <version>`

### Database Connection Failed

**Problem:** Cannot connect to database.

**Diagnosis:**
1. Check PostgreSQL is running
2. Verify connection string in config
3. Check network/firewall settings

**Solution:**
```bash
# Test connection manually
psql "postgres://lightstack:secret@localhost/lightstack?sslmode=disable"

# Check PostgreSQL status
systemctl status postgresql

# Verify config
cat config/o3k.yaml | grep -A 2 "database:"
```

## API Reference

### Go API

```go
import "github.com/cobaltcore-dev/o3k/internal/database"

// Get current migration status
info, err := database.GetMigrationStatus(dbURL, migrationsPath)
fmt.Printf("Version: %d, Dirty: %t\n", info.Version, info.Dirty)

// Apply all pending migrations
err := database.MigrateUp(dbURL, migrationsPath)

// Roll back one migration
err := database.MigrateDown(dbURL, migrationsPath)

// Migrate to specific version
err := database.MigrateToVersion(dbURL, migrationsPath, 5)

// Reset database (drop and re-create)
err := database.MigrateReset(dbURL, migrationsPath)

// Force migration version
err := database.ForceMigrationVersion(dbURL, migrationsPath, 7)

// Validate migration files
err := database.ValidateMigrations(migrationsPath)
```

## Testing

Run migration system tests:

```bash
./test/migration_test.sh
```

Tests verify:
- Migration tool binary exists
- Status command works
- All migrations have paired up/down files
- File naming conventions
- Database is at latest version
- Migration state is clean
- All commands available (down, goto, reset, force)
- Migration files are readable
- Help documentation exists

## Production Deployment

### Pre-Deployment Checklist

1. [ ] Backup database
2. [ ] Test migrations in staging
3. [ ] Review rollback plan
4. [ ] Check for long-running migrations
5. [ ] Verify data integrity after migration

### Deployment Steps

1. **Backup Database:**
```bash
pg_dump lightstack > backup_$(date +%Y%m%d_%H%M%S).sql
```

2. **Stop Application:**
```bash
systemctl stop o3k
```

3. **Run Migrations:**
```bash
./o3k-migrate status  # Check current version
./o3k-migrate up      # Apply migrations
```

4. **Verify Success:**
```bash
./o3k-migrate status  # Verify new version
```

5. **Start Application:**
```bash
systemctl start o3k
```

### Rollback Procedure

If migration causes issues:

1. **Stop Application:**
```bash
systemctl stop o3k
```

2. **Roll Back Migration:**
```bash
./o3k-migrate down
```

3. **Restore from Backup (if needed):**
```bash
psql lightstack < backup_YYYYMMDD_HHMMSS.sql
```

4. **Force Version (if dirty):**
```bash
./o3k-migrate force <version>
```

5. **Restart Application:**
```bash
systemctl start o3k
```

## Schema Versioning Table

Migrations are tracked in the `schema_migrations` table:

```sql
-- Created automatically by golang-migrate
CREATE TABLE schema_migrations (
    version bigint NOT NULL PRIMARY KEY,
    dirty boolean NOT NULL
);
```

Query migration status directly:
```sql
SELECT version, dirty FROM schema_migrations;
```

## References

- [golang-migrate Documentation](https://github.com/golang-migrate/migrate)
- [Database Migration Best Practices](https://www.postgresql.org/docs/current/ddl.html)
- [O3K Architecture Documentation](docs/ARCHITECTURE.md)
