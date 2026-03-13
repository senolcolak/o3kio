-- Migration 051: Rollback

ALTER TABLE images DROP COLUMN IF EXISTS properties;
DROP INDEX IF EXISTS idx_images_properties;
