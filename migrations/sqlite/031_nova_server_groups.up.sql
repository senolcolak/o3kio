PRAGMA foreign_keys = ON;

-- Add Nova server groups for VM placement policies
CREATE TABLE IF NOT EXISTS server_groups (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    project_id TEXT NOT NULL,
    user_id TEXT,
    policies TEXT DEFAULT '{}',
    members TEXT DEFAULT '{}',
    metadata TEXT DEFAULT '{}',
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(project_id, name)
);

CREATE INDEX idx_server_groups_project ON server_groups(project_id);
