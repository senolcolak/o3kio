-- Add Nova server groups for VM placement policies
CREATE TABLE IF NOT EXISTS server_groups (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    project_id UUID NOT NULL,
    user_id UUID,
    policies TEXT[] DEFAULT '{}',
    members UUID[] DEFAULT '{}',
    metadata JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(project_id, name)
);

CREATE INDEX idx_server_groups_project ON server_groups(project_id);
