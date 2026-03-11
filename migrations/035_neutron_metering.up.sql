-- Add metering labels and rules tables
CREATE TABLE IF NOT EXISTS metering_labels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    project_id UUID NOT NULL,
    shared BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS metering_label_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    metering_label_id UUID NOT NULL REFERENCES metering_labels(id) ON DELETE CASCADE,
    remote_ip_prefix VARCHAR(255) NOT NULL,
    direction VARCHAR(50) NOT NULL DEFAULT 'ingress',
    excluded BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(metering_label_id, remote_ip_prefix, direction)
);

CREATE INDEX idx_metering_labels_project ON metering_labels(project_id);
CREATE INDEX idx_metering_label_rules_label ON metering_label_rules(metering_label_id);
