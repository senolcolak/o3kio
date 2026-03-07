-- Add quotas table for resource limits
CREATE TABLE IF NOT EXISTS quotas (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    resource VARCHAR(50) NOT NULL,
    hard_limit INTEGER NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(project_id, resource)
);

CREATE INDEX idx_quotas_project ON quotas(project_id);
CREATE INDEX idx_quotas_resource ON quotas(resource);

-- Insert default quotas for the default project
INSERT INTO quotas (project_id, resource, hard_limit) VALUES
    ('00000000-0000-0000-0000-000000000002', 'instances', 10),
    ('00000000-0000-0000-0000-000000000002', 'cores', 20),
    ('00000000-0000-0000-0000-000000000002', 'ram', 51200),
    ('00000000-0000-0000-0000-000000000002', 'volumes', 10),
    ('00000000-0000-0000-0000-000000000002', 'gigabytes', 1000),
    ('00000000-0000-0000-0000-000000000002', 'snapshots', 10),
    ('00000000-0000-0000-0000-000000000002', 'networks', 10),
    ('00000000-0000-0000-0000-000000000002', 'subnets', 10),
    ('00000000-0000-0000-0000-000000000002', 'ports', 50),
    ('00000000-0000-0000-0000-000000000002', 'routers', 10),
    ('00000000-0000-0000-0000-000000000002', 'floatingip', 10),
    ('00000000-0000-0000-0000-000000000002', 'security_groups', 10),
    ('00000000-0000-0000-0000-000000000002', 'security_group_rules', 100)
ON CONFLICT (project_id, resource) DO NOTHING;
