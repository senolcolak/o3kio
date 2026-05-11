-- Performance indexes for frequently queried fields across all services
-- These indexes optimize list queries used by Horizon dashboard

-- Nova (Compute) indexes
CREATE INDEX IF NOT EXISTS idx_instances_status ON instances(status);
CREATE INDEX IF NOT EXISTS idx_instances_created_at ON instances(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_instances_project_status ON instances(project_id, status);
CREATE INDEX IF NOT EXISTS idx_instances_user_id ON instances(user_id);

-- Neutron (Network) indexes
CREATE INDEX IF NOT EXISTS idx_networks_status ON networks(status);
CREATE INDEX IF NOT EXISTS idx_networks_shared ON networks(shared);
CREATE INDEX IF NOT EXISTS idx_subnets_network_id ON subnets(network_id);
CREATE INDEX IF NOT EXISTS idx_ports_network_id ON ports(network_id);
CREATE INDEX IF NOT EXISTS idx_ports_device_id ON ports(device_id);
CREATE INDEX IF NOT EXISTS idx_ports_device_owner ON ports(device_owner);
CREATE INDEX IF NOT EXISTS idx_routers_project_id ON routers(project_id);

-- Cinder (Volume) indexes
CREATE INDEX IF NOT EXISTS idx_volumes_status ON volumes(status);
CREATE INDEX IF NOT EXISTS idx_volumes_created_at ON volumes(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_volumes_project_status ON volumes(project_id, status);
CREATE INDEX IF NOT EXISTS idx_snapshots_volume_id ON snapshots(volume_id);
CREATE INDEX IF NOT EXISTS idx_snapshots_project_id ON snapshots(project_id);

-- Glance (Image) indexes
CREATE INDEX IF NOT EXISTS idx_images_status ON images(status);
CREATE INDEX IF NOT EXISTS idx_images_visibility ON images(visibility);
CREATE INDEX IF NOT EXISTS idx_images_created_at ON images(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_images_project_visibility ON images(project_id, visibility);

-- Keystone (Identity) indexes
CREATE INDEX IF NOT EXISTS idx_users_enabled ON users(enabled);
CREATE INDEX IF NOT EXISTS idx_projects_enabled ON projects(enabled);
CREATE INDEX IF NOT EXISTS idx_role_assignments_user ON role_assignments(user_id);
CREATE INDEX IF NOT EXISTS idx_role_assignments_project ON role_assignments(project_id);

-- Composite indexes for common query patterns
-- These optimize queries that filter by multiple columns
CREATE INDEX IF NOT EXISTS idx_instances_project_created ON instances(project_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_volumes_project_created ON volumes(project_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_networks_project_shared ON networks(project_id, shared);

-- Horizon typically sorts by created_at descending
-- These indexes support ORDER BY created_at DESC queries
CREATE INDEX IF NOT EXISTS idx_networks_created_at ON networks(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_subnets_created_at ON subnets(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ports_created_at ON ports(created_at DESC);
