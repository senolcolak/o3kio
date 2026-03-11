-- Add Neutron trunk ports for VLAN tagging
CREATE TABLE IF NOT EXISTS trunks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255),
    description TEXT,
    project_id UUID NOT NULL,
    port_id UUID NOT NULL REFERENCES ports(id) ON DELETE CASCADE,
    admin_state_up BOOLEAN DEFAULT TRUE,
    status VARCHAR(50) DEFAULT 'ACTIVE',
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(port_id)
);

CREATE TABLE IF NOT EXISTS trunk_subports (
    trunk_id UUID NOT NULL REFERENCES trunks(id) ON DELETE CASCADE,
    port_id UUID NOT NULL REFERENCES ports(id) ON DELETE CASCADE,
    segmentation_type VARCHAR(50) NOT NULL,
    segmentation_id INTEGER NOT NULL,
    created_at TIMESTAMP DEFAULT NOW(),
    PRIMARY KEY (trunk_id, port_id)
);

CREATE INDEX idx_trunks_project ON trunks(project_id);
CREATE INDEX idx_trunk_subports_trunk ON trunk_subports(trunk_id);
