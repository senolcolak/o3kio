package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}

	conn, err := pgx.Connect(context.Background(), dbURL)
	if err != nil {
		return fmt.Errorf("unable to connect to database: %w", err)
	}
	defer func() {
		if err := conn.Close(context.Background()); err != nil {
			fmt.Fprintf(os.Stderr, "Error closing connection: %v\n", err)
		}
	}()

	migration := `
-- Neutron L3 Router tables

-- Routers table
CREATE TABLE IF NOT EXISTS routers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    project_id UUID REFERENCES projects(id) ON DELETE CASCADE,
    admin_state_up BOOLEAN DEFAULT true,
    status VARCHAR(50) DEFAULT 'ACTIVE',
    external_gateway_info JSONB,
    distributed BOOLEAN DEFAULT false,
    ha BOOLEAN DEFAULT false,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Router interfaces
CREATE TABLE IF NOT EXISTS router_interfaces (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    router_id UUID REFERENCES routers(id) ON DELETE CASCADE,
    port_id UUID REFERENCES ports(id) ON DELETE CASCADE,
    subnet_id UUID REFERENCES subnets(id) ON DELETE CASCADE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(router_id, subnet_id)
);

-- Floating IPs table
CREATE TABLE IF NOT EXISTS floating_ips (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID REFERENCES projects(id) ON DELETE CASCADE,
    floating_network_id UUID REFERENCES networks(id) ON DELETE CASCADE,
    floating_ip_address VARCHAR(50) NOT NULL,
    fixed_ip_address VARCHAR(50),
    port_id UUID REFERENCES ports(id) ON DELETE SET NULL,
    router_id UUID REFERENCES routers(id) ON DELETE SET NULL,
    status VARCHAR(50) DEFAULT 'DOWN',
    description TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(floating_ip_address)
);

-- Router static routes
CREATE TABLE IF NOT EXISTS router_routes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    router_id UUID REFERENCES routers(id) ON DELETE CASCADE,
    destination VARCHAR(50) NOT NULL,
    nexthop VARCHAR(50) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(router_id, destination)
);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_routers_project_id ON routers(project_id);
CREATE INDEX IF NOT EXISTS idx_router_interfaces_router_id ON router_interfaces(router_id);
CREATE INDEX IF NOT EXISTS idx_router_interfaces_port_id ON router_interfaces(port_id);
CREATE INDEX IF NOT EXISTS idx_floating_ips_project_id ON floating_ips(project_id);
CREATE INDEX IF NOT EXISTS idx_floating_ips_port_id ON floating_ips(port_id);
CREATE INDEX IF NOT EXISTS idx_floating_ips_router_id ON floating_ips(router_id);
CREATE INDEX IF NOT EXISTS idx_floating_ips_network_id ON floating_ips(floating_network_id);
CREATE INDEX IF NOT EXISTS idx_router_routes_router_id ON router_routes(router_id);
`

	fmt.Println("Applying L3 router migration...")
	_, err = conn.Exec(context.Background(), migration)
	if err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	fmt.Println("✓ Migration applied successfully!")
	fmt.Println("✓ Created tables: routers, router_interfaces, floating_ips, router_routes")
	fmt.Println("✓ Created 8 indexes")
	return nil
}
