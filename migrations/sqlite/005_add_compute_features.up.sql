PRAGMA foreign_keys = ON;

-- Migration 005: Add volume attachments and advanced compute features

-- Volume attachments table
CREATE TABLE IF NOT EXISTS volume_attachments (
    id TEXT PRIMARY KEY,
    volume_id TEXT NOT NULL REFERENCES volumes(id) ON DELETE CASCADE,
    instance_id TEXT NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    device TEXT NOT NULL, -- e.g., /dev/vdb, /dev/vdc
    attached_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(volume_id), -- One volume can only be attached to one instance
    UNIQUE(instance_id, device) -- One device per instance
);

CREATE INDEX IF NOT EXISTS idx_volume_attachments_volume ON volume_attachments(volume_id);
CREATE INDEX IF NOT EXISTS idx_volume_attachments_instance ON volume_attachments(instance_id);

-- Add console fields to instances table
ALTER TABLE instances ADD COLUMN console_vnc_port INTEGER;
ALTER TABLE instances ADD COLUMN console_vnc_password TEXT;

-- Instance metadata table (for cloud-init)
CREATE TABLE IF NOT EXISTS instance_metadata (
    id TEXT PRIMARY KEY,
    instance_id TEXT NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    meta_key TEXT NOT NULL,
    meta_value TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(instance_id, meta_key)
);

CREATE INDEX IF NOT EXISTS idx_instance_metadata_instance ON instance_metadata(instance_id);

-- Instance user data table (for cloud-init)
CREATE TABLE IF NOT EXISTS instance_userdata (
    id TEXT PRIMARY KEY,
    instance_id TEXT NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    userdata TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(instance_id)
);

CREATE INDEX IF NOT EXISTS idx_instance_userdata_instance ON instance_userdata(instance_id);

-- Add task_state for tracking async operations
ALTER TABLE instances ADD COLUMN task_state TEXT;

-- Instance snapshots table (for shelve operation)
CREATE TABLE IF NOT EXISTS instance_snapshots (
    id TEXT PRIMARY KEY,
    instance_id TEXT NOT NULL,
    snapshot_name TEXT NOT NULL,
    image_id TEXT,
    flavor_id TEXT NOT NULL REFERENCES flavors(id),
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(instance_id)
);

CREATE INDEX IF NOT EXISTS idx_instance_snapshots_instance ON instance_snapshots(instance_id);

-- Network interface attachments table
CREATE TABLE IF NOT EXISTS interface_attachments (
    id TEXT PRIMARY KEY,
    instance_id TEXT NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    port_id TEXT NOT NULL REFERENCES ports(id) ON DELETE CASCADE,
    mac_address TEXT NOT NULL,
    attached_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(instance_id, port_id)
);

CREATE INDEX IF NOT EXISTS idx_interface_attachments_instance ON interface_attachments(instance_id);
CREATE INDEX IF NOT EXISTS idx_interface_attachments_port ON interface_attachments(port_id);
