-- Migration 069: Add protected column to images table
ALTER TABLE images ADD COLUMN IF NOT EXISTS protected BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN images.protected IS 'When true, image cannot be deleted';
