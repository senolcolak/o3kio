PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS volume_groups (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name TEXT,
    description TEXT,
    status TEXT NOT NULL DEFAULT 'creating',
    group_type TEXT NOT NULL,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_volume_groups_project_id ON volume_groups(project_id);
CREATE INDEX IF NOT EXISTS idx_volume_groups_status ON volume_groups(status);
