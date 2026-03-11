CREATE TABLE IF NOT EXISTS services (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    enabled BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(type, name)
);

CREATE TABLE IF NOT EXISTS endpoints (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    service_id UUID NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    interface VARCHAR(50) NOT NULL,
    url TEXT NOT NULL,
    region VARCHAR(255),
    enabled BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_endpoints_service ON endpoints(service_id);
CREATE INDEX IF NOT EXISTS idx_endpoints_interface ON endpoints(interface);

-- Seed default services
INSERT INTO services (id, type, name, description, enabled) VALUES
    ('00000000-0000-0000-0000-000000000010', 'identity', 'keystone', 'Identity Service', true),
    ('00000000-0000-0000-0000-000000000011', 'compute', 'nova', 'Compute Service', true),
    ('00000000-0000-0000-0000-000000000012', 'network', 'neutron', 'Network Service', true),
    ('00000000-0000-0000-0000-000000000013', 'volumev3', 'cinderv3', 'Block Storage Service v3', true),
    ('00000000-0000-0000-0000-000000000014', 'image', 'glance', 'Image Service', true)
ON CONFLICT DO NOTHING;

-- Seed default endpoints
INSERT INTO endpoints (service_id, interface, url, region) VALUES
    ('00000000-0000-0000-0000-000000000010', 'public', 'http://localhost:35357/v3', 'RegionOne'),
    ('00000000-0000-0000-0000-000000000011', 'public', 'http://localhost:8774/v2.1', 'RegionOne'),
    ('00000000-0000-0000-0000-000000000012', 'public', 'http://localhost:9696/v2.0', 'RegionOne'),
    ('00000000-0000-0000-0000-000000000013', 'public', 'http://localhost:8776/v3', 'RegionOne'),
    ('00000000-0000-0000-0000-000000000014', 'public', 'http://localhost:9292/v2', 'RegionOne')
ON CONFLICT DO NOTHING;
