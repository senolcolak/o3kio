PRAGMA foreign_keys = ON;

-- Extend server_groups (created in migration 017) with columns that were
-- omitted from the initial schema.
-- SQLite ADD COLUMN does not support IF NOT EXISTS; the migration runner
-- guarantees each migration runs at most once, so these will not be
-- re-executed on an existing database.
ALTER TABLE server_groups ADD COLUMN user_id TEXT;
ALTER TABLE server_groups ADD COLUMN metadata TEXT DEFAULT '{}';

-- Add unique constraint on (project_id, name).
-- SQLite does not support ADD CONSTRAINT; use a unique index instead.
CREATE UNIQUE INDEX IF NOT EXISTS idx_server_groups_project_name ON server_groups(project_id, name);
