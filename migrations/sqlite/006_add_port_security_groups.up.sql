PRAGMA foreign_keys = ON;

-- Add port security groups junction table
CREATE TABLE IF NOT EXISTS port_security_groups (
    port_id TEXT NOT NULL REFERENCES ports(id) ON DELETE CASCADE,
    security_group_id TEXT NOT NULL REFERENCES security_groups(id) ON DELETE CASCADE,
    PRIMARY KEY (port_id, security_group_id)
);

CREATE INDEX idx_port_security_groups_port ON port_security_groups(port_id);
CREATE INDEX idx_port_security_groups_sg ON port_security_groups(security_group_id);
