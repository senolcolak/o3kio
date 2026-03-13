-- Migration 051: Add properties JSONB column to images table
-- Enables storing backup metadata and custom image properties

ALTER TABLE images ADD COLUMN IF NOT EXISTS properties JSONB DEFAULT '{}'::jsonb;

-- Add index for JSONB property searches
CREATE INDEX IF NOT EXISTS idx_images_properties ON images USING GIN (properties);

-- Add comment
COMMENT ON COLUMN images.properties IS 'Custom image properties and backup metadata (backup_type, instance_uuid, etc.)';
