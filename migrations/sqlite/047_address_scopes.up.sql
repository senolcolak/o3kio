PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS address_scopes (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    ip_version INTEGER NOT NULL,
    shared INTEGER NOT NULL DEFAULT 0,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_address_scopes_project_id ON address_scopes(project_id);
CREATE INDEX IF NOT EXISTS idx_address_scopes_ip_version ON address_scopes(ip_version);
