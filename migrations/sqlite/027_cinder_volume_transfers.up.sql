PRAGMA foreign_keys = ON;

-- Add Cinder volume transfers table for ownership changes
CREATE TABLE IF NOT EXISTS volume_transfers (
    id TEXT PRIMARY KEY,
    volume_id TEXT NOT NULL REFERENCES volumes(id) ON DELETE CASCADE,
    name TEXT,
    source_project_id TEXT NOT NULL,
    destination_project_id TEXT,
    auth_key TEXT NOT NULL,
    accepted INTEGER DEFAULT 0,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_volume_transfers_volume ON volume_transfers(volume_id);
CREATE INDEX IF NOT EXISTS idx_volume_transfers_source ON volume_transfers(source_project_id);
CREATE INDEX IF NOT EXISTS idx_volume_transfers_accepted ON volume_transfers(accepted);
