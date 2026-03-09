-- Rollback for 010_keystone_user_enhancements.up.sql
-- Per Constitution: All migrations have rollback scripts

-- Drop indexes
DROP INDEX IF EXISTS idx_group_members_group_id;
DROP INDEX IF EXISTS idx_group_members_user_id;
DROP INDEX IF EXISTS idx_groups_domain_id;
DROP INDEX IF EXISTS idx_users_domain_id;
DROP INDEX IF EXISTS idx_users_email;

-- Drop tables
DROP TABLE IF EXISTS group_members;
DROP TABLE IF EXISTS groups;

-- Drop columns from users (note: this may cause data loss in production)
ALTER TABLE users DROP COLUMN IF EXISTS domain_id;
ALTER TABLE users DROP COLUMN IF EXISTS description;
ALTER TABLE users DROP COLUMN IF EXISTS email;
