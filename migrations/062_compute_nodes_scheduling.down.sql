ALTER TABLE compute_nodes DROP COLUMN IF EXISTS total_vcpu;
ALTER TABLE compute_nodes DROP COLUMN IF EXISTS total_ram_mb;
ALTER TABLE compute_nodes DROP COLUMN IF EXISTS total_disk_gb;
ALTER TABLE compute_nodes DROP COLUMN IF EXISTS reserved_vcpu;
ALTER TABLE compute_nodes DROP COLUMN IF EXISTS reserved_ram_mb;
ALTER TABLE compute_nodes DROP COLUMN IF EXISTS reserved_disk_gb;
ALTER TABLE compute_nodes DROP COLUMN IF EXISTS stats_updated_at;
ALTER TABLE compute_nodes DROP COLUMN IF EXISTS agent_stream_server_id;
