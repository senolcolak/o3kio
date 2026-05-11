-- Add description column to volumes and snapshots tables
ALTER TABLE volumes ADD COLUMN IF NOT EXISTS description TEXT;
ALTER TABLE snapshots ADD COLUMN IF NOT EXISTS description TEXT;
