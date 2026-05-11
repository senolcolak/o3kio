PRAGMA foreign_keys = ON;

-- Add flavor access table for tenant/project-specific flavor visibility
CREATE TABLE IF NOT EXISTS flavor_access (
    flavor_id TEXT NOT NULL REFERENCES flavors(id) ON DELETE CASCADE,
    project_id TEXT NOT NULL,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (flavor_id, project_id)
);

CREATE INDEX IF NOT EXISTS idx_flavor_access_project ON flavor_access(project_id);
