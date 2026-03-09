# SPEC-005: Octavia - Load Balancing as a Service

**Status**: Draft
**Version**: 1.0
**Created**: 2026-03-09
**Priority**: Medium
**Depends On**: SPEC-001 (Modular Architecture), Nova, Neutron

## Overview

Implement Octavia, OpenStack's Load Balancing as a Service (LBaaS), as an independent library-first module. Octavia provides L4/L7 load balancing using HAProxy as the data plane.

## Goals

1. **Load Balancer Management**: Create/manage load balancers (VIP, listeners, pools)
2. **Health Monitoring**: Active health checks on backend members
3. **HAProxy Backend**: Use HAProxy for load balancing (industry standard)
4. **SSL/TLS Termination**: HTTPS load balancing with certificate management
5. **OpenStack API Compatible**: 100% Octavia v2 API compatibility
6. **Library-First**: Standalone library with CLI tool

## Non-Goals

- Custom load balancing algorithms (use HAProxy built-ins)
- Layer 2 load balancing (future phase)
- Hardware load balancers (software only)
- Multi-region load balancing (future phase)

## Use Cases

### 1. HTTP Load Balancer
```
1. User creates load balancer with VIP
2. User creates HTTP listener on port 80
3. User creates pool with 3 backend servers
4. User adds health monitor (HTTP GET /)
5. HAProxy distributes traffic across healthy backends
```

### 2. HTTPS Load Balancer with TLS Termination
```
1. User uploads certificate to Barbican
2. User creates load balancer
3. User creates HTTPS listener on port 443 with certificate
4. HAProxy terminates TLS, forwards HTTP to backends
```

### 3. TCP Load Balancer (L4)
```
1. User creates load balancer
2. User creates TCP listener on port 5432 (PostgreSQL)
3. User creates pool with database replicas
4. HAProxy balances TCP connections
```

## Architecture

### Components

```
pkg/octavia/
├── service.go           # Service interface
├── loadbalancers.go     # Load balancer CRUD
├── listeners.go         # Listener management
├── pools.go             # Backend pool management
├── members.go           # Pool member operations
├── healthmonitors.go    # Health check configuration
├── l7policies.go        # L7 routing policies
├── backend/
│   ├── interface.go     # Backend interface
│   ├── stub.go          # Stub backend (development)
│   ├── haproxy.go       # HAProxy backend
│   └── namespace.go     # Network namespace management
└── cli/
    └── main.go          # CLI tool

cmd/o3k-octavia/
└── main.go              # Standalone binary

internal/octavia/
├── haproxy/
│   ├── config.go        # HAProxy config generation
│   ├── template.go      # HAProxy config templates
│   └── control.go       # HAProxy process management
└── amphora/
    └── manager.go       # Amphora VM management (future)
```

### Load Balancer Hierarchy

```
Load Balancer (VIP + network)
  ├── Listener 1 (port 80, HTTP)
  │   └── L7 Policies (URL routing)
  │       └── Pool (backend servers)
  │           ├── Member 1 (192.168.1.10:80)
  │           ├── Member 2 (192.168.1.11:80)
  │           └── Health Monitor (HTTP GET /)
  └── Listener 2 (port 443, HTTPS)
      └── Pool (backend servers)
          └── Members...
```

### Deployment Models

**Model 1: Standalone HAProxy (Initial)**
```
Single HAProxy process per load balancer
Runs in dedicated network namespace
Direct configuration + reload
Simple, fast, no VM overhead
```

**Model 2: Amphora (Future)**
```
HAProxy runs in lightweight VM (Amphora)
VM managed by Nova
Full isolation, more secure
OpenStack native approach
```

**Recommendation**: Start with Model 1 (standalone), add Model 2 later.

## Database Schema

```sql
-- Load balancers
CREATE TABLE octavia_loadbalancers (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL,
    name VARCHAR(255),
    description TEXT,
    vip_address INET,                   -- Virtual IP
    vip_port_id UUID,                   -- Neutron port ID
    vip_subnet_id UUID,                 -- Neutron subnet ID
    vip_network_id UUID,                -- Neutron network ID
    provisioning_status VARCHAR(50) DEFAULT 'PENDING_CREATE',
    operating_status VARCHAR(50) DEFAULT 'OFFLINE',
    enabled BOOLEAN DEFAULT true,
    flavor_id UUID,
    provider VARCHAR(50) DEFAULT 'haproxy',
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    INDEX idx_project_lbs (project_id, provisioning_status)
);

-- Listeners (frontends)
CREATE TABLE octavia_listeners (
    id UUID PRIMARY KEY,
    loadbalancer_id UUID REFERENCES octavia_loadbalancers(id) ON DELETE CASCADE,
    project_id UUID NOT NULL,
    name VARCHAR(255),
    description TEXT,
    protocol VARCHAR(50) NOT NULL,      -- HTTP, HTTPS, TCP, UDP
    protocol_port INTEGER NOT NULL,
    connection_limit INTEGER DEFAULT -1,
    default_pool_id UUID,
    provisioning_status VARCHAR(50) DEFAULT 'PENDING_CREATE',
    operating_status VARCHAR(50) DEFAULT 'OFFLINE',
    enabled BOOLEAN DEFAULT true,
    tls_certificate_id UUID,            -- Barbican secret ID
    sni_container_ids UUID[],           -- SNI certificates
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(loadbalancer_id, protocol_port)
);

-- Pools (backend groups)
CREATE TABLE octavia_pools (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL,
    name VARCHAR(255),
    description TEXT,
    protocol VARCHAR(50) NOT NULL,      -- HTTP, HTTPS, TCP
    lb_algorithm VARCHAR(50) DEFAULT 'ROUND_ROBIN',
    session_persistence JSONB,          -- {type: SOURCE_IP, cookie_name: ...}
    provisioning_status VARCHAR(50) DEFAULT 'PENDING_CREATE',
    operating_status VARCHAR(50) DEFAULT 'OFFLINE',
    enabled BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Pool members (backend servers)
CREATE TABLE octavia_members (
    id UUID PRIMARY KEY,
    pool_id UUID REFERENCES octavia_pools(id) ON DELETE CASCADE,
    project_id UUID NOT NULL,
    name VARCHAR(255),
    address INET NOT NULL,              -- Backend IP
    protocol_port INTEGER NOT NULL,
    weight INTEGER DEFAULT 1,
    backup BOOLEAN DEFAULT false,
    subnet_id UUID,                     -- Neutron subnet
    provisioning_status VARCHAR(50) DEFAULT 'PENDING_CREATE',
    operating_status VARCHAR(50) DEFAULT 'OFFLINE',
    enabled BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    INDEX idx_pool_members (pool_id, enabled)
);

-- Health monitors
CREATE TABLE octavia_healthmonitors (
    id UUID PRIMARY KEY,
    pool_id UUID REFERENCES octavia_pools(id) ON DELETE CASCADE UNIQUE,
    project_id UUID NOT NULL,
    name VARCHAR(255),
    type VARCHAR(50) NOT NULL,          -- HTTP, HTTPS, TCP, PING
    delay INTEGER NOT NULL,             -- Seconds between checks
    timeout INTEGER NOT NULL,
    max_retries INTEGER NOT NULL,
    http_method VARCHAR(50),            -- GET, POST, etc.
    url_path VARCHAR(255),              -- /health
    expected_codes VARCHAR(255),        -- 200,201-204
    provisioning_status VARCHAR(50) DEFAULT 'PENDING_CREATE',
    operating_status VARCHAR(50) DEFAULT 'OFFLINE',
    enabled BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- L7 policies (URL routing)
CREATE TABLE octavia_l7policies (
    id UUID PRIMARY KEY,
    listener_id UUID REFERENCES octavia_listeners(id) ON DELETE CASCADE,
    project_id UUID NOT NULL,
    name VARCHAR(255),
    description TEXT,
    action VARCHAR(50) NOT NULL,        -- REDIRECT_TO_POOL, REDIRECT_TO_URL, REJECT
    redirect_pool_id UUID REFERENCES octavia_pools(id),
    redirect_url VARCHAR(255),
    position INTEGER NOT NULL,
    provisioning_status VARCHAR(50) DEFAULT 'PENDING_CREATE',
    operating_status VARCHAR(50) DEFAULT 'OFFLINE',
    enabled BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- L7 rules (conditions for L7 policies)
CREATE TABLE octavia_l7rules (
    id UUID PRIMARY KEY,
    l7policy_id UUID REFERENCES octavia_l7policies(id) ON DELETE CASCADE,
    project_id UUID NOT NULL,
    type VARCHAR(50) NOT NULL,          -- PATH, HOST_NAME, HEADER, COOKIE
    compare_type VARCHAR(50) NOT NULL,  -- REGEX, STARTS_WITH, ENDS_WITH, CONTAINS, EQUAL_TO
    key VARCHAR(255),                   -- Header/cookie name
    value VARCHAR(255) NOT NULL,
    invert BOOLEAN DEFAULT false,
    provisioning_status VARCHAR(50) DEFAULT 'PENDING_CREATE',
    operating_status VARCHAR(50) DEFAULT 'OFFLINE',
    enabled BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Statistics
CREATE TABLE octavia_loadbalancer_stats (
    loadbalancer_id UUID REFERENCES octavia_loadbalancers(id) ON DELETE CASCADE,
    timestamp TIMESTAMP NOT NULL,
    bytes_in BIGINT DEFAULT 0,
    bytes_out BIGINT DEFAULT 0,
    active_connections INTEGER DEFAULT 0,
    total_connections BIGINT DEFAULT 0,
    request_errors BIGINT DEFAULT 0,
    PRIMARY KEY (loadbalancer_id, timestamp)
);

-- Flavors (LB sizing)
CREATE TABLE octavia_flavors (
    id UUID PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    description TEXT,
    enabled BOOLEAN DEFAULT true,
    flavor_profile_id UUID
);
```

## API Endpoints

### Load Balancers
- `GET /v2/lbaas/loadbalancers` - List load balancers
- `POST /v2/lbaas/loadbalancers` - Create load balancer
- `GET /v2/lbaas/loadbalancers/{id}` - Get load balancer
- `PUT /v2/lbaas/loadbalancers/{id}` - Update load balancer
- `DELETE /v2/lbaas/loadbalancers/{id}` - Delete load balancer
- `GET /v2/lbaas/loadbalancers/{id}/stats` - Get statistics
- `GET /v2/lbaas/loadbalancers/{id}/status` - Get status tree

### Listeners
- `GET /v2/lbaas/listeners` - List listeners
- `POST /v2/lbaas/listeners` - Create listener
- `GET /v2/lbaas/listeners/{id}` - Get listener
- `PUT /v2/lbaas/listeners/{id}` - Update listener
- `DELETE /v2/lbaas/listeners/{id}` - Delete listener
- `GET /v2/lbaas/listeners/{id}/stats` - Get listener stats

### Pools
- `GET /v2/lbaas/pools` - List pools
- `POST /v2/lbaas/pools` - Create pool
- `GET /v2/lbaas/pools/{id}` - Get pool
- `PUT /v2/lbaas/pools/{id}` - Update pool
- `DELETE /v2/lbaas/pools/{id}` - Delete pool

### Members
- `GET /v2/lbaas/pools/{pool_id}/members` - List members
- `POST /v2/lbaas/pools/{pool_id}/members` - Add member
- `GET /v2/lbaas/pools/{pool_id}/members/{id}` - Get member
- `PUT /v2/lbaas/pools/{pool_id}/members/{id}` - Update member
- `DELETE /v2/lbaas/pools/{pool_id}/members/{id}` - Remove member

### Health Monitors
- `GET /v2/lbaas/healthmonitors` - List health monitors
- `POST /v2/lbaas/healthmonitors` - Create health monitor
- `GET /v2/lbaas/healthmonitors/{id}` - Get health monitor
- `PUT /v2/lbaas/healthmonitors/{id}` - Update health monitor
- `DELETE /v2/lbaas/healthmonitors/{id}` - Delete health monitor

### L7 Policies
- `GET /v2/lbaas/l7policies` - List L7 policies
- `POST /v2/lbaas/l7policies` - Create L7 policy
- `GET /v2/lbaas/l7policies/{id}` - Get L7 policy
- `PUT /v2/lbaas/l7policies/{id}` - Update L7 policy
- `DELETE /v2/lbaas/l7policies/{id}` - Delete L7 policy

### L7 Rules
- `GET /v2/lbaas/l7policies/{policy_id}/rules` - List L7 rules
- `POST /v2/lbaas/l7policies/{policy_id}/rules` - Create L7 rule
- `GET /v2/lbaas/l7policies/{policy_id}/rules/{id}` - Get L7 rule
- `PUT /v2/lbaas/l7policies/{policy_id}/rules/{id}` - Update L7 rule
- `DELETE /v2/lbaas/l7policies/{policy_id}/rules/{id}` - Delete L7 rule

## HAProxy Configuration Generation

### Config Template

```go
func (h *HAProxyBackend) generateConfig(lb *LoadBalancer) string {
    var buf bytes.Buffer

    // Global section
    buf.WriteString("global\n")
    buf.WriteString("    daemon\n")
    buf.WriteString("    maxconn 4096\n")
    buf.WriteString("    stats socket /var/run/haproxy.sock mode 600 level admin\n")
    buf.WriteString("    log /dev/log local0\n\n")

    // Defaults
    buf.WriteString("defaults\n")
    buf.WriteString("    log global\n")
    buf.WriteString("    mode http\n")
    buf.WriteString("    option httplog\n")
    buf.WriteString("    option dontlognull\n")
    buf.WriteString("    timeout connect 5000\n")
    buf.WriteString("    timeout client 50000\n")
    buf.WriteString("    timeout server 50000\n\n")

    // Frontends (listeners)
    for _, listener := range lb.Listeners {
        buf.WriteString(fmt.Sprintf("frontend %s\n", listener.ID))
        buf.WriteString(fmt.Sprintf("    bind %s:%d", lb.VIPAddress, listener.ProtocolPort))

        // SSL certificate
        if listener.Protocol == "HTTPS" && listener.TLSCertificateID != "" {
            certPath := h.getCertificatePath(listener.TLSCertificateID)
            buf.WriteString(fmt.Sprintf(" ssl crt %s", certPath))
        }
        buf.WriteString("\n")

        // Mode
        if listener.Protocol == "TCP" {
            buf.WriteString("    mode tcp\n")
        } else {
            buf.WriteString("    mode http\n")
        }

        // L7 policies
        for _, policy := range listener.L7Policies {
            for _, rule := range policy.Rules {
                acl := h.generateACL(rule)
                buf.WriteString(fmt.Sprintf("    acl %s %s\n", rule.ID, acl))
            }
            action := h.generateAction(policy)
            buf.WriteString(fmt.Sprintf("    %s\n", action))
        }

        // Default backend
        if listener.DefaultPoolID != "" {
            buf.WriteString(fmt.Sprintf("    default_backend %s\n\n", listener.DefaultPoolID))
        }
    }

    // Backends (pools)
    for _, pool := range lb.Pools {
        buf.WriteString(fmt.Sprintf("backend %s\n", pool.ID))
        buf.WriteString(fmt.Sprintf("    balance %s\n", h.mapAlgorithm(pool.LBAlgorithm)))

        // Session persistence
        if pool.SessionPersistence != nil {
            buf.WriteString(h.generatePersistence(pool.SessionPersistence))
        }

        // Health check
        if pool.HealthMonitor != nil {
            buf.WriteString(h.generateHealthCheck(pool.HealthMonitor))
        }

        // Members
        for _, member := range pool.Members {
            if !member.Enabled {
                continue
            }
            buf.WriteString(fmt.Sprintf("    server %s %s:%d weight %d",
                member.ID, member.Address, member.ProtocolPort, member.Weight))
            if member.Backup {
                buf.WriteString(" backup")
            }
            if pool.HealthMonitor != nil {
                buf.WriteString(" check")
            }
            buf.WriteString("\n")
        }
        buf.WriteString("\n")
    }

    // Stats endpoint
    buf.WriteString("listen stats\n")
    buf.WriteString("    bind :9000\n")
    buf.WriteString("    mode http\n")
    buf.WriteString("    stats enable\n")
    buf.WriteString("    stats uri /stats\n")
    buf.WriteString("    stats refresh 10s\n")

    return buf.String()
}
```

### Example Generated Config

```haproxy
global
    daemon
    maxconn 4096
    stats socket /var/run/haproxy.sock mode 600 level admin
    log /dev/log local0

defaults
    log global
    mode http
    option httplog
    timeout connect 5000
    timeout client 50000
    timeout server 50000

frontend listener-uuid-1
    bind 192.168.1.100:80
    mode http
    default_backend pool-uuid-1

backend pool-uuid-1
    balance roundrobin
    option httpchk GET /health
    http-check expect status 200
    server member-1 192.168.1.10:8080 weight 1 check inter 5s fall 3 rise 2
    server member-2 192.168.1.11:8080 weight 1 check inter 5s fall 3 rise 2
    server member-3 192.168.1.12:8080 weight 1 check inter 5s fall 3 rise 2

listen stats
    bind :9000
    mode http
    stats enable
    stats uri /stats
```

## Network Namespace Implementation

Each load balancer runs in dedicated namespace:

```go
func (h *HAProxyBackend) CreateLoadBalancer(lb *LoadBalancer) error {
    // 1. Create network namespace
    nsName := fmt.Sprintf("lb-%s", lb.ID)
    if err := h.createNamespace(nsName); err != nil {
        return fmt.Errorf("failed to create namespace: %w", err)
    }

    // 2. Create veth pair and move to namespace
    vethHost := fmt.Sprintf("lb-%s-host", lb.ID[:8])
    vethNS := fmt.Sprintf("lb-%s-ns", lb.ID[:8])
    if err := h.createVethPair(vethHost, vethNS, nsName); err != nil {
        return fmt.Errorf("failed to create veth pair: %w", err)
    }

    // 3. Configure VIP in namespace
    if err := h.configureVIP(nsName, vethNS, lb.VIPAddress); err != nil {
        return fmt.Errorf("failed to configure VIP: %w", err)
    }

    // 4. Generate HAProxy config
    config := h.generateConfig(lb)
    configPath := filepath.Join("/var/lib/octavia", lb.ID, "haproxy.cfg")
    if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
        return fmt.Errorf("failed to write config: %w", err)
    }

    // 5. Start HAProxy in namespace
    ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
    defer cancel()

    cmd := exec.CommandContext(ctx, "ip", "netns", "exec", nsName,
        "haproxy", "-f", configPath, "-D")
    if err := cmd.Run(); err != nil {
        return fmt.Errorf("HAProxy start failed (fail-early): %w", err)
    }

    return nil
}
```

## Fail-Early Strategy

```go
func (s *Service) CreateMember(ctx context.Context, member *Member) error {
    // 1. Validate member
    if err := member.Validate(); err != nil {
        return fmt.Errorf("validation failed: %w", err)
    }

    // 2. Store in database (PENDING_CREATE)
    member.ProvisioningStatus = "PENDING_CREATE"
    if err := s.db.InsertMember(member); err != nil {
        return fmt.Errorf("database insert failed: %w", err)
    }

    // 3. Regenerate HAProxy config
    lb := s.getLoadBalancerForMember(member)
    config := s.backend.GenerateConfig(lb)

    // 4. Reload HAProxy (with timeout)
    ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
    defer cancel()

    if err := s.backend.ReloadConfig(ctx, lb.ID, config); err != nil {
        // Mark as ERROR, don't hide failure
        s.db.UpdateMemberStatus(member.ID, "ERROR")
        return fmt.Errorf("HAProxy reload failed (fail-early): %w", err)
    }

    // 5. Mark as ACTIVE
    s.db.UpdateMemberStatus(member.ID, "ACTIVE")
    return nil
}
```

If HAProxy fails to reload:
- Member creation fails immediately with 503 Service Unavailable
- Previous configuration remains active (no downtime)
- Operator alerted immediately

## Barbican Integration (TLS Certificates)

```go
func (h *HAProxyBackend) getCertificatePath(certificateID string) string {
    // 1. Fetch certificate from Barbican
    barbicanClient := h.getBarbicanClient()
    certSecret := barbicanClient.GetSecret(certificateID)

    // Certificate container has: certificate, private_key, intermediates
    certPEM := certSecret.Payload

    // 2. Write to HAProxy cert directory
    certPath := filepath.Join("/var/lib/octavia/certs", certificateID+".pem")
    if err := os.WriteFile(certPath, []byte(certPEM), 0600); err != nil {
        log.Errorf("Failed to write certificate: %v", err)
        return ""
    }

    return certPath
}
```

## Health Monitoring

```go
func (s *Service) monitorHealth(ctx context.Context) {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            s.updateMemberStatuses()
        }
    }
}

func (s *Service) updateMemberStatuses() {
    // Query HAProxy stats socket for each LB
    lbs := s.db.ListActiveLoadBalancers()

    for _, lb := range lbs {
        stats := s.backend.GetStats(lb.ID)

        for _, memberStats := range stats.Members {
            status := "ONLINE"
            if memberStats.CheckStatus == "DOWN" {
                status = "ERROR"
            } else if memberStats.CheckStatus == "MAINT" {
                status = "OFFLINE"
            }

            s.db.UpdateMemberOperatingStatus(memberStats.ID, status)
        }
    }
}
```

## CLI Tool

```bash
# Create load balancer
o3k-octavia-cli lb create --name web-lb --vip-subnet-id <subnet-uuid>

# Create listener
o3k-octavia-cli listener create --name http-listener --protocol HTTP \
  --protocol-port 80 --loadbalancer-id <lb-uuid>

# Create pool
o3k-octavia-cli pool create --name backend-pool --lb-algorithm ROUND_ROBIN \
  --listener-id <listener-uuid> --protocol HTTP

# Add members
o3k-octavia-cli member create backend-pool --address 192.168.1.10 --protocol-port 8080
o3k-octavia-cli member create backend-pool --address 192.168.1.11 --protocol-port 8080

# Create health monitor
o3k-octavia-cli healthmonitor create --type HTTP --delay 5 --timeout 3 \
  --max-retries 3 --url-path /health --pool-id <pool-uuid>

# Show stats
o3k-octavia-cli lb stats <lb-uuid>
```

## Testing Strategy

### Unit Tests
- HAProxy config generation
- ACL generation
- Health check configuration
- L7 policy parsing
- Certificate handling

### Integration Tests
```bash
#!/bin/bash
# test/integration/octavia_test.sh

# Start HAProxy backend
o3k-octavia --config test-config.yaml &

# Create load balancer
LB_ID=$(openstack loadbalancer create --name test-lb --vip-subnet-id $SUBNET_ID -f value -c id)

# Wait for ACTIVE
wait_for_active loadbalancer $LB_ID

# Create listener
LISTENER_ID=$(openstack loadbalancer listener create --name http --protocol HTTP \
  --protocol-port 80 $LB_ID -f value -c id)

# Create pool
POOL_ID=$(openstack loadbalancer pool create --name backend --lb-algorithm ROUND_ROBIN \
  --listener $LISTENER_ID --protocol HTTP -f value -c id)

# Add members (assume test backends running on 192.168.1.10-12)
openstack loadbalancer member create --address 192.168.1.10 --protocol-port 8080 $POOL_ID
openstack loadbalancer member create --address 192.168.1.11 --protocol-port 8080 $POOL_ID

# Test load balancing
VIP=$(openstack loadbalancer show $LB_ID -f value -c vip_address)
for i in {1..10}; do
  curl -s http://$VIP/ | grep "Server:"
done

# Verify round-robin distribution
```

### Contract Tests
```go
func TestOctaviaOpenStackCompatibility(t *testing.T) {
    client := NewOctaviaClient("http://localhost:9876")

    // Create load balancer
    lb := client.CreateLoadBalancer(LBCreateRequest{
        Name: "test-lb",
        VIPSubnetID: subnetID,
    })
    assert.NotEmpty(t, lb.ID)
    assert.Equal(t, "PENDING_CREATE", lb.ProvisioningStatus)

    // Wait for ACTIVE
    waitForStatus(t, client, lb.ID, "ACTIVE")
}
```

## Migration Path

### Phase 1: Core Load Balancer (Weeks 1-2)
- Database schema
- Load balancer CRUD API
- Stub backend
- CLI tool
- Unit tests

### Phase 2: HAProxy Backend (Week 3)
- HAProxy config generation
- Namespace management
- Process management
- Integration tests

### Phase 3: Listeners & Pools (Week 4)
- Listener API
- Pool API
- Member API
- Config regeneration

### Phase 4: Health Monitoring (Week 5)
- Health monitor API
- HAProxy stats socket
- Status updates
- Member health tracking

### Phase 5: Advanced Features (Week 6)
- TLS termination (Barbican integration)
- L7 policies
- Session persistence
- Statistics API

## Success Criteria

- [ ] python-octaviaclient works with all endpoints
- [ ] HAProxy backend functional
- [ ] Load balancing works (verified with curl)
- [ ] Health monitoring detects failed backends
- [ ] TLS termination works with Barbican
- [ ] L7 routing works (path/host-based)
- [ ] CLI tool works for common operations
- [ ] Fail-early: HAProxy failures return < 1s
- [ ] Contract tests pass
- [ ] OpenStack CLI integration works
- [ ] Horizon dashboard shows load balancers
- [ ] Documentation complete

## Security Considerations

1. **VIP Isolation**: Load balancers in separate namespaces
2. **Certificate Security**: TLS certificates stored in Barbican
3. **HAProxy Hardening**: Disable unused features
4. **Stats Security**: Stats endpoint not publicly accessible
5. **Rate Limiting**: Protect API from abuse
6. **Access Control**: Project-scoped load balancers

## Performance Targets

- Load balancer creation: < 2s (including namespace setup)
- Member add: < 1s (config reload)
- HAProxy reload: < 500ms (graceful, no dropped connections)
- Health check update: < 10s (configurable interval)
- Backend operation timeout: 1s (fail-early)
- Connection throughput: > 10,000 req/s (per LB)

## References

- OpenStack Octavia API v2
- HAProxy Configuration Manual
- RFC 7540 (HTTP/2)
- Linux Network Namespaces
- PKCS#12 (Certificate Containers)
