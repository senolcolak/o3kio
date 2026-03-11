-- Add volume backups table
CREATE TABLE IF NOT EXISTS volume_backups (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL,
    volume_id UUID NOT NULL REFERENCES volumes(id) ON DELETE CASCADE,
    name VARCHAR(255),
    description TEXT,
    status VARCHAR(50) DEFAULT 'creating',
    size_gb INTEGER NOT NULL,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_volume_backups_project_id ON volume_backups(project_id);
CREATE INDEX idx_volume_backups_volume_id ON volume_backups(volume_id);
