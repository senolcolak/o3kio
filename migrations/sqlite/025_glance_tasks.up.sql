-- Add Glance tasks table for async operations
CREATE TABLE IF NOT EXISTS image_tasks (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    status TEXT DEFAULT 'pending',
    input TEXT,
    result TEXT,
    message TEXT,
    owner TEXT,
    expires_at TEXT,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_image_tasks_status ON image_tasks(status);
CREATE INDEX IF NOT EXISTS idx_image_tasks_owner ON image_tasks(owner);
CREATE INDEX IF NOT EXISTS idx_image_tasks_type ON image_tasks(type);
