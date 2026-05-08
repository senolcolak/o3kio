PRAGMA foreign_keys = ON;

-- Create volume_metadata and snapshot_metadata tables
CREATE TABLE IF NOT EXISTS volume_metadata (
    id TEXT PRIMARY KEY,
    volume_id TEXT NOT NULL REFERENCES volumes(id) ON DELETE CASCADE,
    meta_key TEXT NOT NULL,
    meta_value TEXT,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(volume_id, meta_key)
);

CREATE TABLE IF NOT EXISTS snapshot_metadata (
    id TEXT PRIMARY KEY,
    snapshot_id TEXT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
    meta_key TEXT NOT NULL,
    meta_value TEXT,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(snapshot_id, meta_key)
);

CREATE INDEX IF NOT EXISTS idx_volume_metadata_volume_id ON volume_metadata(volume_id);
CREATE INDEX IF NOT EXISTS idx_snapshot_metadata_snapshot_id ON snapshot_metadata(snapshot_id);
