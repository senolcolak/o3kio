PRAGMA foreign_keys = ON;

-- Add Neutron trunk ports for VLAN tagging
CREATE TABLE IF NOT EXISTS trunks (
    id TEXT PRIMARY KEY,
    name TEXT,
    description TEXT,
    project_id TEXT NOT NULL,
    port_id TEXT NOT NULL REFERENCES ports(id) ON DELETE CASCADE,
    admin_state_up INTEGER DEFAULT 1,
    status TEXT DEFAULT 'ACTIVE',
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(port_id)
);

CREATE TABLE IF NOT EXISTS trunk_subports (
    trunk_id TEXT NOT NULL REFERENCES trunks(id) ON DELETE CASCADE,
    port_id TEXT NOT NULL REFERENCES ports(id) ON DELETE CASCADE,
    segmentation_type TEXT NOT NULL,
    segmentation_id INTEGER NOT NULL,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (trunk_id, port_id)
);

CREATE INDEX idx_trunks_project ON trunks(project_id);
CREATE INDEX idx_trunk_subports_trunk ON trunk_subports(trunk_id);
