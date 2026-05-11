PRAGMA foreign_keys = ON;

-- Add QoS policies and bandwidth limit rules
CREATE TABLE IF NOT EXISTS qos_policies (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    shared INTEGER DEFAULT 0,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS qos_bandwidth_limit_rules (
    id TEXT PRIMARY KEY,
    qos_policy_id TEXT NOT NULL REFERENCES qos_policies(id) ON DELETE CASCADE,
    max_kbps INTEGER NOT NULL,
    max_burst_kbps INTEGER,
    direction TEXT DEFAULT 'egress',
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_qos_policies_project_id ON qos_policies(project_id);
CREATE INDEX IF NOT EXISTS idx_qos_bandwidth_limit_rules_policy_id ON qos_bandwidth_limit_rules(qos_policy_id);
