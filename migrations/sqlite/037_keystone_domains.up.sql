PRAGMA foreign_keys = ON;

-- Add domains table
CREATE TABLE IF NOT EXISTS domains (
    id TEXT PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,
    description TEXT,
    enabled INTEGER DEFAULT 1,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
);

-- Seed default domain (using well-known UUID for 'default')
INSERT INTO domains (id, name, description, enabled, created_at, updated_at)
VALUES ('00000000-0000-0000-0000-000000000001', 'Default', 'Default domain', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT (name) DO NOTHING;

CREATE INDEX IF NOT EXISTS idx_domains_name ON domains(name);
CREATE INDEX IF NOT EXISTS idx_domains_enabled ON domains(enabled);
