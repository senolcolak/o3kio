PRAGMA foreign_keys = ON;

-- Add server migrations tracking table
CREATE TABLE IF NOT EXISTS server_migrations (
    id TEXT PRIMARY KEY,
    server_uuid TEXT NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    source_node TEXT,
    dest_node TEXT,
    old_flavor_id TEXT,
    new_flavor_id TEXT,
    status TEXT DEFAULT 'queued',
    migration_type TEXT DEFAULT 'migration',
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_server_migrations_server ON server_migrations(server_uuid);
CREATE INDEX idx_server_migrations_status ON server_migrations(status);
