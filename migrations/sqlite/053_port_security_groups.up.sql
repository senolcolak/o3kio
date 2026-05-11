PRAGMA foreign_keys = ON;

-- Port-Security Group association table
-- Enables security groups to be assigned to ports (OpenStack standard)
CREATE TABLE IF NOT EXISTS port_security_groups (
    port_id TEXT REFERENCES ports(id) ON DELETE CASCADE,
    security_group_id TEXT REFERENCES security_groups(id) ON DELETE CASCADE,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (port_id, security_group_id)
);

CREATE INDEX idx_port_security_groups_port_id ON port_security_groups(port_id);
CREATE INDEX idx_port_security_groups_security_group_id ON port_security_groups(security_group_id);

-- Add default security group to existing ports (if any)
-- This ensures existing ports get basic firewall rules
INSERT INTO port_security_groups (port_id, security_group_id)
SELECT p.id, sg.id
FROM ports p
CROSS JOIN security_groups sg
WHERE sg.name = 'default'
  AND sg.project_id = p.project_id
  AND NOT EXISTS (
    SELECT 1 FROM port_security_groups psg
    WHERE psg.port_id = p.id
  );
