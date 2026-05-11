PRAGMA foreign_keys = ON;

-- Create groups table for Keystone group management
CREATE TABLE IF NOT EXISTS groups (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    domain_id TEXT NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    description TEXT,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(name, domain_id)
);

-- Create group_users junction table for group membership
CREATE TABLE IF NOT EXISTS group_users (
    group_id TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY (group_id, user_id)
);

-- Indexes for faster queries
CREATE INDEX IF NOT EXISTS idx_groups_domain_id ON groups(domain_id);
CREATE INDEX IF NOT EXISTS idx_group_users_group_id ON group_users(group_id);
CREATE INDEX IF NOT EXISTS idx_group_users_user_id ON group_users(user_id);
