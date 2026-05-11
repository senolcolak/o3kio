PRAGMA foreign_keys = ON;

-- Add metering labels and rules tables
CREATE TABLE IF NOT EXISTS metering_labels (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    project_id TEXT NOT NULL,
    shared INTEGER DEFAULT 0,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS metering_label_rules (
    id TEXT PRIMARY KEY,
    metering_label_id TEXT NOT NULL REFERENCES metering_labels(id) ON DELETE CASCADE,
    remote_ip_prefix TEXT NOT NULL,
    direction TEXT NOT NULL DEFAULT 'ingress',
    excluded INTEGER DEFAULT 0,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(metering_label_id, remote_ip_prefix, direction)
);

CREATE INDEX idx_metering_labels_project ON metering_labels(project_id);
CREATE INDEX idx_metering_label_rules_label ON metering_label_rules(metering_label_id);
