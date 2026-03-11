-- Add QoS policies and bandwidth limit rules
CREATE TABLE IF NOT EXISTS qos_policies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    shared BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS qos_bandwidth_limit_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    qos_policy_id UUID NOT NULL REFERENCES qos_policies(id) ON DELETE CASCADE,
    max_kbps INTEGER NOT NULL,
    max_burst_kbps INTEGER,
    direction VARCHAR(16) DEFAULT 'egress',
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_qos_policies_project_id ON qos_policies(project_id);
CREATE INDEX idx_qos_bandwidth_limit_rules_policy_id ON qos_bandwidth_limit_rules(qos_policy_id);
