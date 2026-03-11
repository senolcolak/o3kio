CREATE TABLE IF NOT EXISTS cinder_quotas (
    project_id UUID NOT NULL,
    resource VARCHAR(255) NOT NULL,
    "limit" INTEGER NOT NULL,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    PRIMARY KEY(project_id, resource)
);

CREATE INDEX IF NOT EXISTS idx_cinder_quotas_project ON cinder_quotas(project_id);
