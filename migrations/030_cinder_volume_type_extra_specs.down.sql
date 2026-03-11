-- Rollback extra_specs column
ALTER TABLE volume_types DROP COLUMN IF EXISTS extra_specs;
