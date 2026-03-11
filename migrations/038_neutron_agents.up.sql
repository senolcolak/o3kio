CREATE TABLE IF NOT EXISTS neutron_agents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_type VARCHAR(255) NOT NULL,
    "binary" VARCHAR(255) NOT NULL,
    host VARCHAR(255) NOT NULL,
    description TEXT,
    admin_state_up BOOLEAN DEFAULT TRUE,
    alive BOOLEAN DEFAULT TRUE,
    started_at TIMESTAMP DEFAULT NOW(),
    created_at TIMESTAMP DEFAULT NOW(),
    configurations JSONB DEFAULT '{}'::jsonb,
    UNIQUE(agent_type, host, "binary")
);

CREATE TABLE IF NOT EXISTS router_l3_agents (
    router_id UUID NOT NULL REFERENCES routers(id) ON DELETE CASCADE,
    agent_id UUID NOT NULL REFERENCES neutron_agents(id) ON DELETE CASCADE,
    created_at TIMESTAMP DEFAULT NOW(),
    PRIMARY KEY(router_id, agent_id)
);

CREATE INDEX IF NOT EXISTS idx_neutron_agents_type ON neutron_agents(agent_type);
CREATE INDEX IF NOT EXISTS idx_neutron_agents_host ON neutron_agents(host);
CREATE INDEX IF NOT EXISTS idx_router_l3_agents_router ON router_l3_agents(router_id);
CREATE INDEX IF NOT EXISTS idx_router_l3_agents_agent ON router_l3_agents(agent_id);
