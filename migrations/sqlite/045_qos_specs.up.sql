CREATE TABLE IF NOT EXISTS qos_specs (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    consumer TEXT NOT NULL DEFAULT 'back-end',
    specs TEXT NOT NULL DEFAULT '{}',
    created_at TEXT DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_qos_specs_name ON qos_specs(name);
