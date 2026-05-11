-- Add volume_type column to volumes table
ALTER TABLE volumes ADD COLUMN IF NOT EXISTS volume_type TEXT;
