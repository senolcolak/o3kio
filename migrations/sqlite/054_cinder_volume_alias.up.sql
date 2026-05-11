-- Add 'volume' service type alias for Cinder (Horizon compatibility)
-- Modern OpenStack uses volumev3, but Horizon still expects 'volume' type

INSERT INTO services (id, type, name, description, enabled) VALUES
    ('00000000-0000-0000-0000-000000000015', 'volume', 'cinder', 'Block Storage Service', 1)
ON CONFLICT DO NOTHING;

-- Add endpoint for 'volume' service pointing to same URL as volumev3
INSERT INTO endpoints (service_id, interface, url, region) VALUES
    ('00000000-0000-0000-0000-000000000015', 'public', 'http://localhost:8776/v3', 'RegionOne')
ON CONFLICT DO NOTHING;
