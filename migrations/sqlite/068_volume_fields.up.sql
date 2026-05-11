-- Add availability_zone and encrypted columns to volumes table
ALTER TABLE volumes
ADD COLUMN IF NOT EXISTS availability_zone TEXT DEFAULT 'nova';

ALTER TABLE volumes
ADD COLUMN IF NOT EXISTS encrypted INTEGER DEFAULT 0;
