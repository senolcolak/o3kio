-- Add availability_zone and encrypted columns to volumes table
ALTER TABLE volumes
ADD COLUMN IF NOT EXISTS availability_zone TEXT DEFAULT 'nova',
ADD COLUMN IF NOT EXISTS encrypted BOOLEAN DEFAULT false;
