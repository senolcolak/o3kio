-- Rollback for Migration 049: Nova Server Actions Enhancement

-- Remove server security groups association table
DROP INDEX IF EXISTS idx_server_security_groups_sg;
DROP INDEX IF EXISTS idx_server_security_groups_server;
DROP TABLE IF EXISTS server_security_groups;

-- Remove admin password hash column
ALTER TABLE instances DROP COLUMN IF EXISTS admin_password_hash;
