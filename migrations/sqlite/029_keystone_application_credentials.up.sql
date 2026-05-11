PRAGMA foreign_keys = ON;

-- Add Keystone application credentials for non-expiring authentication
CREATE TABLE IF NOT EXISTS application_credentials (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    project_id TEXT,
    secret_hash TEXT NOT NULL,
    description TEXT,
    unrestricted INTEGER DEFAULT 0,
    expires_at TEXT,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, name)
);

CREATE INDEX IF NOT EXISTS idx_application_credentials_user ON application_credentials(user_id);
CREATE INDEX IF NOT EXISTS idx_application_credentials_project ON application_credentials(project_id);
