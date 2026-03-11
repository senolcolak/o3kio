-- Add extra_specs column to volume_types
ALTER TABLE volume_types ADD COLUMN IF NOT EXISTS extra_specs JSONB DEFAULT '{}'::jsonb;
