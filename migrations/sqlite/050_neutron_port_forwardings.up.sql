PRAGMA foreign_keys = ON;

-- Migration 050: Add port_forwardings table for Neutron floating IP port forwarding
-- Enables forwarding specific TCP/UDP ports from floating IPs to internal instances

CREATE TABLE IF NOT EXISTS port_forwardings (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    floatingip_id TEXT NOT NULL REFERENCES floating_ips(id) ON DELETE CASCADE,
    internal_port_id TEXT NOT NULL REFERENCES ports(id) ON DELETE CASCADE,
    internal_ip_address TEXT NOT NULL,
    external_port INTEGER NOT NULL CHECK (external_port BETWEEN 1 AND 65535),
    internal_port INTEGER NOT NULL CHECK (internal_port BETWEEN 1 AND 65535),
    protocol TEXT NOT NULL CHECK (protocol IN ('tcp', 'udp')),
    status TEXT DEFAULT 'ACTIVE',
    description TEXT,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(floatingip_id, external_port, protocol)
);

-- Indexes for query performance
CREATE INDEX idx_port_forwardings_project_id ON port_forwardings(project_id);
CREATE INDEX idx_port_forwardings_floatingip_id ON port_forwardings(floatingip_id);
CREATE INDEX idx_port_forwardings_internal_port_id ON port_forwardings(internal_port_id);
