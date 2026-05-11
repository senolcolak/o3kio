-- Add instance actions tracking table
CREATE TABLE IF NOT EXISTS instance_actions (
    id TEXT PRIMARY KEY,
    instance_id TEXT NOT NULL,
    action TEXT NOT NULL,
    request_id TEXT,
    user_id TEXT,
    project_id TEXT NOT NULL,
    start_time TEXT DEFAULT CURRENT_TIMESTAMP,
    message TEXT,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_instance_actions_instance ON instance_actions(instance_id);
CREATE INDEX IF NOT EXISTS idx_instance_actions_request ON instance_actions(request_id);
