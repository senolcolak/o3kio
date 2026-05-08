# SQLite Migrations

SQLite-compatible versions of the PostgreSQL migrations for O3K.

## Type Mapping

| PostgreSQL | SQLite | Notes |
|---|---|---|
| `UUID` | `TEXT` | Application generates UUIDs (no `gen_random_uuid()`) |
| `JSONB` | `TEXT` | Store as JSON string; application marshals/unmarshals |
| `BOOLEAN` | `INTEGER` | `1` = true, `0` = false |
| `TEXT[]` | `TEXT` | Store as JSON array string, e.g. `'["8.8.8.8"]'` |
| `VARCHAR(n)` | `TEXT` | SQLite has no fixed-length varchar |
| `BIGINT` | `INTEGER` | SQLite INTEGER is up to 8 bytes |
| `TIMESTAMP` | `TEXT` | `DEFAULT CURRENT_TIMESTAMP` works in SQLite |
| `DEFAULT gen_random_uuid()` | removed | Application must supply UUID on INSERT |
| `DEFAULT '{}'::jsonb` | `DEFAULT '{}'` | Plain string default |
| `DEFAULT ARRAY[]::TEXT[]` | `DEFAULT '[]'` | Empty JSON array string |

## Available Migrations

| File | Description |
|---|---|
| `001_initial_schema.up.sql` | All base tables (Keystone, Nova, Neutron, Cinder, Glance) |
| `001_initial_schema.down.sql` | DROP all base tables |
| `005_add_compute_features.up.sql` | Volume attachments, instance metadata, interface attachments |
| `007_add_quotas.up.sql` | Quota table + default quota seed rows |
| `013_cinder_metadata.up.sql` | Volume and snapshot metadata tables |

Migration numbers match the PostgreSQL versions so golang-migrate tracks the same schema version across both databases.

## Foreign Keys

SQLite does not enforce foreign keys by default. Each up migration starts with:

```sql
PRAGMA foreign_keys = ON;
```

This enables enforcement for the current connection. If you use golang-migrate with a persistent SQLite connection, set this pragma once at connection open time instead.

## UUID Generation

PostgreSQL uses `DEFAULT gen_random_uuid()` to auto-assign primary keys. SQLite has no equivalent. The application must generate and supply a UUID for every INSERT. Use `github.com/google/uuid`:

```go
import "github.com/google/uuid"

id := uuid.New().String()
```

## ALTER TABLE Limitations

SQLite supports only a subset of `ALTER TABLE`:

- `ADD COLUMN` is supported (used in migration 005)
- `DROP COLUMN` is supported (SQLite 3.35+)
- `RENAME COLUMN` is supported (SQLite 3.25+)
- `ALTER COLUMN` (type changes, constraints) is **not** supported

Migration 005 uses `ALTER TABLE instances ADD COLUMN` without `IF NOT EXISTS` — this is intentional. golang-migrate tracks which migrations have run and will not re-apply them, so the column will not be added twice.

## Adding More Migrations

Convert additional PostgreSQL migrations by applying the type mapping table above. Name the file with the same number as the PostgreSQL version (e.g., `020_nova_aggregates.up.sql`) so the migration framework stays in sync.
