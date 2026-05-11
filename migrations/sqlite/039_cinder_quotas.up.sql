PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS cinder_quotas (
    project_id TEXT NOT NULL,
    resource TEXT NOT NULL,
    "limit" INTEGER NOT NULL,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY(project_id, resource)
);

CREATE INDEX IF NOT EXISTS idx_cinder_quotas_project ON cinder_quotas(project_id);
