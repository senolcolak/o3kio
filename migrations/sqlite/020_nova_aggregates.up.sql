-- Add host_aggregates table
CREATE TABLE IF NOT EXISTS host_aggregates (
    id INTEGER PRIMARY KEY,
    uuid TEXT UNIQUE NOT NULL,
    name TEXT NOT NULL,
    availability_zone TEXT,
    metadata TEXT DEFAULT '{}',
    hosts TEXT DEFAULT '[]',
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_host_aggregates_uuid ON host_aggregates(uuid);
CREATE INDEX idx_host_aggregates_name ON host_aggregates(name);
