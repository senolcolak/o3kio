-- Add Glance tasks table for async operations
CREATE TABLE IF NOT EXISTS image_tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type VARCHAR(50) NOT NULL,
    status VARCHAR(50) DEFAULT 'pending',
    input JSONB,
    result JSONB,
    message TEXT,
    owner VARCHAR(255),
    expires_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_image_tasks_status ON image_tasks(status);
CREATE INDEX idx_image_tasks_owner ON image_tasks(owner);
CREATE INDEX idx_image_tasks_type ON image_tasks(type);
