-- Migration 004: VXLAN Multi-node Support
-- Adds compute node registry, VNI allocations, and FDB entries for VXLAN overlay networking

-- Compute nodes registry for multi-node coordination
CREATE TABLE IF NOT EXISTS compute_nodes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hostname VARCHAR(255) UNIQUE NOT NULL,
    tunnel_ip VARCHAR(50) NOT NULL,
    status VARCHAR(50) DEFAULT 'active', -- 'active', 'down', 'maintenance'
    capabilities JSONB DEFAULT '{}',
    last_heartbeat TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- VNI (VXLAN Network Identifier) allocations
-- VNI range: 1000-16777215 (VXLAN standard allows up to 16M VNIs)
CREATE TABLE IF NOT EXISTS network_vni_allocations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    network_id UUID REFERENCES networks(id) ON DELETE CASCADE UNIQUE,
    vni INTEGER NOT NULL UNIQUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    CHECK (vni >= 1000 AND vni <= 16777215)
);

-- FDB (Forwarding Database) entries for MAC learning
-- Maps MAC addresses to VTEP (VXLAN Tunnel Endpoint) IPs
CREATE TABLE IF NOT EXISTS vxlan_fdb_entries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    network_id UUID REFERENCES networks(id) ON DELETE CASCADE,
    mac_address VARCHAR(17) NOT NULL,
    vtep_ip VARCHAR(50) NOT NULL,
    port_id UUID REFERENCES ports(id) ON DELETE CASCADE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(network_id, mac_address)
);

-- Add VXLAN-related columns to networks table
ALTER TABLE networks ADD COLUMN IF NOT EXISTS network_type VARCHAR(50) DEFAULT 'flat';
ALTER TABLE networks ADD COLUMN IF NOT EXISTS physical_network VARCHAR(255);

-- Add MTU column to networks for VXLAN overhead handling
ALTER TABLE networks ADD COLUMN IF NOT EXISTS mtu INTEGER DEFAULT 1500;

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_compute_nodes_status ON compute_nodes(status);
CREATE INDEX IF NOT EXISTS idx_compute_nodes_heartbeat ON compute_nodes(last_heartbeat);
CREATE INDEX IF NOT EXISTS idx_vni_allocations_network ON network_vni_allocations(network_id);
CREATE INDEX IF NOT EXISTS idx_vni_allocations_vni ON network_vni_allocations(vni);
CREATE INDEX IF NOT EXISTS idx_fdb_entries_network ON vxlan_fdb_entries(network_id);
CREATE INDEX IF NOT EXISTS idx_fdb_entries_mac ON vxlan_fdb_entries(mac_address);
CREATE INDEX IF NOT EXISTS idx_fdb_entries_port ON vxlan_fdb_entries(port_id);
CREATE INDEX IF NOT EXISTS idx_fdb_entries_vtep ON vxlan_fdb_entries(vtep_ip);
CREATE INDEX IF NOT EXISTS idx_networks_type ON networks(network_type);
