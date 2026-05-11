-- Add project_id column for multi-tenancy support
ALTER TABLE qos_specs ADD COLUMN IF NOT EXISTS project_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000002';

-- Drop old unique constraint on name
-- SQLite does not support DROP CONSTRAINT; the unique index must be dropped instead
DROP INDEX IF EXISTS qos_specs_name_key;

-- Add new unique constraint on (name, project_id)
CREATE UNIQUE INDEX IF NOT EXISTS qos_specs_name_project_unique ON qos_specs (name, project_id);

-- Add index on project_id for filtering
CREATE INDEX IF NOT EXISTS idx_qos_specs_project_id ON qos_specs(project_id);

-- Add updated_at column
ALTER TABLE qos_specs ADD COLUMN IF NOT EXISTS updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP;
