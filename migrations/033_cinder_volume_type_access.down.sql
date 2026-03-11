-- Rollback volume type access
ALTER TABLE volume_types DROP COLUMN IF EXISTS is_public;
DROP TABLE IF EXISTS volume_type_access;
