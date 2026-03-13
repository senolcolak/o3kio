-- Migration 050: Add port_forwardings table for Neutron floating IP port forwarding
-- Enables forwarding specific TCP/UDP ports from floating IPs to internal instances

CREATE TABLE IF NOT EXISTS port_forwardings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    floatingip_id UUID NOT NULL REFERENCES floating_ips(id) ON DELETE CASCADE,
    internal_port_id UUID NOT NULL REFERENCES ports(id) ON DELETE CASCADE,
    internal_ip_address VARCHAR(50) NOT NULL,
    external_port INT NOT NULL CHECK (external_port BETWEEN 1 AND 65535),
    internal_port INT NOT NULL CHECK (internal_port BETWEEN 1 AND 65535),
    protocol VARCHAR(10) NOT NULL CHECK (protocol IN ('tcp', 'udp')),
    status VARCHAR(50) DEFAULT 'ACTIVE',
    description TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(floatingip_id, external_port, protocol)
);

-- Indexes for query performance
CREATE INDEX idx_port_forwardings_project_id ON port_forwardings(project_id);
CREATE INDEX idx_port_forwardings_floatingip_id ON port_forwardings(floatingip_id);
CREATE INDEX idx_port_forwardings_internal_port_id ON port_forwardings(internal_port_id);
