PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS neutron_agents (
    id TEXT PRIMARY KEY,
    agent_type TEXT NOT NULL,
    "binary" TEXT NOT NULL,
    host TEXT NOT NULL,
    description TEXT,
    admin_state_up INTEGER DEFAULT 1,
    alive INTEGER DEFAULT 1,
    started_at TEXT DEFAULT CURRENT_TIMESTAMP,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    configurations TEXT DEFAULT '{}',
    UNIQUE(agent_type, host, "binary")
);

CREATE TABLE IF NOT EXISTS router_l3_agents (
    router_id TEXT NOT NULL REFERENCES routers(id) ON DELETE CASCADE,
    agent_id TEXT NOT NULL REFERENCES neutron_agents(id) ON DELETE CASCADE,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY(router_id, agent_id)
);

CREATE INDEX IF NOT EXISTS idx_neutron_agents_type ON neutron_agents(agent_type);
CREATE INDEX IF NOT EXISTS idx_neutron_agents_host ON neutron_agents(host);
CREATE INDEX IF NOT EXISTS idx_router_l3_agents_router ON router_l3_agents(router_id);
CREATE INDEX IF NOT EXISTS idx_router_l3_agents_agent ON router_l3_agents(agent_id);
