PRAGMA foreign_keys = ON;

-- Add email, description, and domain_id to users table
-- Part of Phase 1 Week 1-2: Keystone user CRUD implementation
-- Per Constitution: Backward compatible (ADD COLUMN only, no DROP)

ALTER TABLE users ADD COLUMN IF NOT EXISTS email TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS description TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS domain_id TEXT DEFAULT 'default';

-- Create groups table for user group management
CREATE TABLE IF NOT EXISTS groups (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    domain_id TEXT DEFAULT 'default',
    description TEXT,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(name, domain_id)
);

-- Create group_members junction table
CREATE TABLE IF NOT EXISTS group_members (
    id TEXT PRIMARY KEY,
    group_id TEXT REFERENCES groups(id) ON DELETE CASCADE,
    user_id TEXT REFERENCES users(id) ON DELETE CASCADE,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(group_id, user_id)
);

-- Create indexes for performance
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_domain_id ON users(domain_id);
CREATE INDEX IF NOT EXISTS idx_groups_domain_id ON groups(domain_id);
CREATE INDEX IF NOT EXISTS idx_group_members_user_id ON group_members(user_id);
CREATE INDEX IF NOT EXISTS idx_group_members_group_id ON group_members(group_id);
