PRAGMA foreign_keys = ON;

-- Add Keystone groups table for user organization
CREATE TABLE IF NOT EXISTS groups (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    domain_id TEXT NOT NULL,
    description TEXT,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(name, domain_id)
);

CREATE TABLE IF NOT EXISTS group_members (
    group_id TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (group_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_groups_domain ON groups(domain_id);
CREATE INDEX IF NOT EXISTS idx_group_members_user ON group_members(user_id);
