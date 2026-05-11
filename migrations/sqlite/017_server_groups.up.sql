PRAGMA foreign_keys = ON;

-- Create server_groups table for Nova server group management
CREATE TABLE IF NOT EXISTS server_groups (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    policies TEXT NOT NULL,
    members TEXT NOT NULL DEFAULT '[]',
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Index for faster project-based queries
CREATE INDEX idx_server_groups_project_id ON server_groups(project_id);
