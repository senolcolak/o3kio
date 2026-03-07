-- Rollback Migration 004: VXLAN Multi-node Support

-- Drop network columns
ALTER TABLE networks DROP COLUMN IF EXISTS mtu;
ALTER TABLE networks DROP COLUMN IF EXISTS physical_network;
ALTER TABLE networks DROP COLUMN IF EXISTS network_type;

-- Drop indexes
DROP INDEX IF EXISTS idx_networks_type;
DROP INDEX IF EXISTS idx_fdb_entries_vtep;
DROP INDEX IF EXISTS idx_fdb_entries_port;
DROP INDEX IF EXISTS idx_fdb_entries_mac;
DROP INDEX IF EXISTS idx_fdb_entries_network;
DROP INDEX IF EXISTS idx_vni_allocations_vni;
DROP INDEX IF EXISTS idx_vni_allocations_network;
DROP INDEX IF EXISTS idx_compute_nodes_heartbeat;
DROP INDEX IF EXISTS idx_compute_nodes_status;

-- Drop tables
DROP TABLE IF EXISTS vxlan_fdb_entries;
DROP TABLE IF EXISTS network_vni_allocations;
DROP TABLE IF EXISTS compute_nodes;
