PRAGMA foreign_keys = ON;

-- Add volume type access control for private volume types
CREATE TABLE IF NOT EXISTS volume_type_access (
    volume_type_id TEXT NOT NULL REFERENCES volume_types(id) ON DELETE CASCADE,
    project_id TEXT NOT NULL,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (volume_type_id, project_id)
);

CREATE INDEX idx_volume_type_access_type ON volume_type_access(volume_type_id);
CREATE INDEX idx_volume_type_access_project ON volume_type_access(project_id);

-- Add is_public column to volume_types
ALTER TABLE volume_types ADD COLUMN IF NOT EXISTS is_public INTEGER DEFAULT 1;
