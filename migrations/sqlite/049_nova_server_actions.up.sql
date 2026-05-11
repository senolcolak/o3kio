PRAGMA foreign_keys = ON;

-- Migration 049: Nova Server Actions Enhancement
-- Adds support for password management and security group associations

-- Add admin password hash storage for changePassword action
ALTER TABLE instances ADD COLUMN IF NOT EXISTS admin_password_hash TEXT;

-- Create server security groups association table
-- Used by addSecurityGroup/removeSecurityGroup actions
CREATE TABLE IF NOT EXISTS server_security_groups (
    server_id TEXT NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    security_group_id TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (server_id, security_group_id)
);

CREATE INDEX IF NOT EXISTS idx_server_security_groups_server
    ON server_security_groups(server_id);
CREATE INDEX IF NOT EXISTS idx_server_security_groups_sg
    ON server_security_groups(security_group_id);
