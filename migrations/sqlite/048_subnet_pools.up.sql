PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS subnet_pools (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    prefixes TEXT NOT NULL,
    min_prefixlen INTEGER NOT NULL DEFAULT 8,
    max_prefixlen INTEGER NOT NULL DEFAULT 32,
    default_prefixlen INTEGER,
    shared INTEGER NOT NULL DEFAULT 0,
    is_default INTEGER NOT NULL DEFAULT 0,
    ip_version INTEGER NOT NULL DEFAULT 4,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_subnet_pools_project_id ON subnet_pools(project_id);
CREATE INDEX IF NOT EXISTS idx_subnet_pools_shared ON subnet_pools(shared);
CREATE INDEX IF NOT EXISTS idx_subnet_pools_is_default ON subnet_pools(is_default);
