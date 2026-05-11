PRAGMA foreign_keys = ON;

-- Neutron L3 Router tables

-- Routers table
CREATE TABLE IF NOT EXISTS routers (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    project_id TEXT REFERENCES projects(id) ON DELETE CASCADE,
    admin_state_up INTEGER DEFAULT 1,
    status TEXT DEFAULT 'ACTIVE',
    external_gateway_info TEXT, -- {"network_id": "...", "enable_snat": true, "external_fixed_ips": [...]}
    distributed INTEGER DEFAULT 0,
    ha INTEGER DEFAULT 0,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
);

-- Router interfaces (ports attached to routers)
CREATE TABLE IF NOT EXISTS router_interfaces (
    id TEXT PRIMARY KEY,
    router_id TEXT REFERENCES routers(id) ON DELETE CASCADE,
    port_id TEXT REFERENCES ports(id) ON DELETE CASCADE,
    subnet_id TEXT REFERENCES subnets(id) ON DELETE CASCADE,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(router_id, subnet_id)
);

-- Floating IPs table
CREATE TABLE IF NOT EXISTS floating_ips (
    id TEXT PRIMARY KEY,
    project_id TEXT REFERENCES projects(id) ON DELETE CASCADE,
    floating_network_id TEXT REFERENCES networks(id) ON DELETE CASCADE,
    floating_ip_address TEXT NOT NULL,
    fixed_ip_address TEXT,
    port_id TEXT REFERENCES ports(id) ON DELETE SET NULL,
    router_id TEXT REFERENCES routers(id) ON DELETE SET NULL,
    status TEXT DEFAULT 'DOWN',
    description TEXT,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(floating_ip_address)
);

-- Router static routes
CREATE TABLE IF NOT EXISTS router_routes (
    id TEXT PRIMARY KEY,
    router_id TEXT REFERENCES routers(id) ON DELETE CASCADE,
    destination TEXT NOT NULL, -- CIDR format
    nexthop TEXT NOT NULL,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(router_id, destination)
);

-- Indexes for performance
CREATE INDEX idx_routers_project_id ON routers(project_id);
CREATE INDEX idx_router_interfaces_router_id ON router_interfaces(router_id);
CREATE INDEX idx_router_interfaces_port_id ON router_interfaces(port_id);
CREATE INDEX idx_floating_ips_project_id ON floating_ips(project_id);
CREATE INDEX idx_floating_ips_port_id ON floating_ips(port_id);
CREATE INDEX idx_floating_ips_router_id ON floating_ips(router_id);
CREATE INDEX idx_floating_ips_network_id ON floating_ips(floating_network_id);
CREATE INDEX idx_router_routes_router_id ON router_routes(router_id);
