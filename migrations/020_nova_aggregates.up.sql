-- Add host_aggregates table
CREATE TABLE IF NOT EXISTS host_aggregates (
    id SERIAL PRIMARY KEY,
    uuid UUID DEFAULT gen_random_uuid() UNIQUE NOT NULL,
    name VARCHAR(255) NOT NULL,
    availability_zone VARCHAR(255),
    metadata JSONB DEFAULT '{}',
    hosts TEXT[] DEFAULT ARRAY[]::TEXT[],
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_host_aggregates_uuid ON host_aggregates(uuid);
CREATE INDEX idx_host_aggregates_name ON host_aggregates(name);
