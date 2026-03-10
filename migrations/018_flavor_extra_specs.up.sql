-- Create flavor_extra_specs table for flavor metadata
CREATE TABLE IF NOT EXISTS flavor_extra_specs (
    flavor_id UUID NOT NULL REFERENCES flavors(id) ON DELETE CASCADE,
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    PRIMARY KEY (flavor_id, key)
);

-- Index for faster flavor-based queries
CREATE INDEX idx_flavor_extra_specs_flavor_id ON flavor_extra_specs(flavor_id);
