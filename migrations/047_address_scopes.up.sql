CREATE TABLE IF NOT EXISTS address_scopes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    ip_version INTEGER NOT NULL,
    shared BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_address_scopes_project_id ON address_scopes(project_id);
CREATE INDEX IF NOT EXISTS idx_address_scopes_ip_version ON address_scopes(ip_version);
