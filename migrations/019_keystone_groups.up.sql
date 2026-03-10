-- Create groups table for Keystone group management
CREATE TABLE IF NOT EXISTS groups (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    domain_id UUID NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    description TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(name, domain_id)
);

-- Create group_users junction table for group membership
CREATE TABLE IF NOT EXISTS group_users (
    group_id UUID NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY (group_id, user_id)
);

-- Indexes for faster queries
CREATE INDEX idx_groups_domain_id ON groups(domain_id);
CREATE INDEX idx_group_users_group_id ON group_users(group_id);
CREATE INDEX idx_group_users_user_id ON group_users(user_id);
