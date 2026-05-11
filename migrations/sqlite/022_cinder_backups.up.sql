PRAGMA foreign_keys = ON;

-- Add volume backups table
CREATE TABLE IF NOT EXISTS volume_backups (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    volume_id TEXT NOT NULL REFERENCES volumes(id) ON DELETE CASCADE,
    name TEXT,
    description TEXT,
    status TEXT DEFAULT 'creating',
    size_gb INTEGER NOT NULL,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_volume_backups_project_id ON volume_backups(project_id);
CREATE INDEX IF NOT EXISTS idx_volume_backups_volume_id ON volume_backups(volume_id);
