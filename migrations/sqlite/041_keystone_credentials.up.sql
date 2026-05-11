PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS credentials (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    project_id TEXT REFERENCES projects(id) ON DELETE CASCADE,
    type TEXT NOT NULL,
    blob TEXT NOT NULL,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_credentials_user ON credentials(user_id);
CREATE INDEX IF NOT EXISTS idx_credentials_project ON credentials(project_id);
CREATE INDEX IF NOT EXISTS idx_credentials_type ON credentials(type);
