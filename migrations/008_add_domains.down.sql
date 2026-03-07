-- Remove domain support
DROP INDEX IF EXISTS idx_projects_domain_name;
DROP INDEX IF EXISTS idx_users_domain_name;
DROP INDEX IF EXISTS idx_projects_domain_id;
DROP INDEX IF EXISTS idx_users_domain_id;

ALTER TABLE projects DROP COLUMN IF EXISTS domain_id;
ALTER TABLE users DROP COLUMN IF EXISTS domain_id;

DROP TABLE IF EXISTS domains;
