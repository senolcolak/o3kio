# SPEC-004: Designate - DNS as a Service

**Status**: Draft
**Version**: 1.0
**Created**: 2026-03-09
**Priority**: Medium
**Depends On**: SPEC-001 (Modular Architecture)

## Overview

Implement Designate, OpenStack's DNS as a Service, as an independent library-first module. Designate provides multi-tenant DNS management with support for multiple backend DNS servers (BIND9, PowerDNS, etc.).

## Goals

1. **Zone Management**: Create/manage DNS zones (domains)
2. **Record Management**: CRUD operations for DNS records (A, AAAA, CNAME, MX, TXT, etc.)
3. **Backend Support**: Multiple DNS backend servers
4. **Floating IP Integration**: Automatic DNS records for Neutron floating IPs
5. **OpenStack API Compatible**: 100% Designate v2 API compatibility
6. **Library-First**: Standalone library with CLI tool

## Non-Goals

- Custom DNS protocols (use RFC standards)
- DNSSEC initially (future phase)
- Geo-based routing (future phase)
- Multi-region DNS replication (future phase)

## Use Cases

### 1. VM Auto-DNS
```
1. User creates VM with floating IP
2. User creates DNS record: vm1.example.com → floating IP
3. Designate updates backend DNS server
4. External clients resolve vm1.example.com
```

### 2. Load Balancer DNS
```
1. User creates Octavia load balancer
2. User creates DNS record: lb.example.com → LB VIP
3. Designate updates DNS
4. Clients connect via lb.example.com
```

### 3. Service Discovery
```
1. Microservices register DNS records
2. Service mesh queries DNS for discovery
3. Designate provides dynamic record updates
```

## Architecture

### Components

```
pkg/designate/
├── service.go           # Service interface
├── zones.go             # Zone (domain) management
├── recordsets.go        # Record set operations
├── pools.go             # DNS server pools
├── backend/
│   ├── interface.go     # Backend interface
│   ├── stub.go          # Stub backend (development)
│   ├── bind9.go         # BIND9 backend
│   ├── pdns.go          # PowerDNS backend
│   └── route53.go       # AWS Route53 backend (optional)
└── cli/
    └── main.go          # CLI tool

cmd/o3k-designate/
└── main.go              # Standalone binary

internal/designate/
└── sync/
    ├── poller.go        # Backend state polling
    └── neutron.go       # Neutron floating IP sync
```

### DNS Record Types Supported

| Type | Description | Example |
|------|-------------|---------|
| `A` | IPv4 address | `192.168.1.1` |
| `AAAA` | IPv6 address | `2001:db8::1` |
| `CNAME` | Canonical name | `alias.example.com` |
| `MX` | Mail exchange | `10 mail.example.com` |
| `TXT` | Text record | `"v=spf1 include:_spf.google.com"` |
| `SRV` | Service record | `10 5 5060 sip.example.com` |
| `NS` | Name server | `ns1.example.com` |
| `PTR` | Reverse DNS | `vm1.example.com` |

## Database Schema

```sql
-- DNS Zones (domains)
CREATE TABLE designate_zones (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL,
    name VARCHAR(255) NOT NULL,         -- example.com
    email VARCHAR(255) NOT NULL,        -- hostmaster@example.com
    ttl INTEGER DEFAULT 3600,
    status VARCHAR(50) DEFAULT 'PENDING',
    serial INTEGER DEFAULT 1,           -- SOA serial
    description TEXT,
    type VARCHAR(50) DEFAULT 'PRIMARY', -- PRIMARY, SECONDARY
    masters TEXT[],                     -- For secondary zones
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(name),
    INDEX idx_project_zones (project_id)
);

-- Record sets (grouped records of same type)
CREATE TABLE designate_recordsets (
    id UUID PRIMARY KEY,
    zone_id UUID REFERENCES designate_zones(id) ON DELETE CASCADE,
    project_id UUID NOT NULL,
    name VARCHAR(255) NOT NULL,         -- www.example.com
    type VARCHAR(10) NOT NULL,          -- A, AAAA, CNAME, etc.
    ttl INTEGER,
    status VARCHAR(50) DEFAULT 'PENDING',
    description TEXT,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(zone_id, name, type),
    INDEX idx_zone_recordsets (zone_id, type)
);

-- Individual records (multiple per recordset)
CREATE TABLE designate_records (
    id UUID PRIMARY KEY,
    recordset_id UUID REFERENCES designate_recordsets(id) ON DELETE CASCADE,
    data TEXT NOT NULL,                 -- 192.168.1.1 or www.example.com
    status VARCHAR(50) DEFAULT 'PENDING',
    action VARCHAR(50),                 -- CREATE, UPDATE, DELETE
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- DNS server pools (backends)
CREATE TABLE designate_pools (
    id UUID PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    description TEXT,
    provisioner VARCHAR(50),            -- bind9, pdns, route53
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE designate_pool_attributes (
    pool_id UUID REFERENCES designate_pools(id) ON DELETE CASCADE,
    key VARCHAR(255) NOT NULL,
    value TEXT NOT NULL,
    PRIMARY KEY (pool_id, key)
);

-- Backend nameservers
CREATE TABLE designate_pool_nameservers (
    id UUID PRIMARY KEY,
    pool_id UUID REFERENCES designate_pools(id) ON DELETE CASCADE,
    host VARCHAR(255) NOT NULL,         -- 192.168.1.10
    port INTEGER DEFAULT 53,
    created_at TIMESTAMP DEFAULT NOW()
);

-- Zone transfers (for secondary zones)
CREATE TABLE designate_zone_transfers (
    id UUID PRIMARY KEY,
    zone_id UUID REFERENCES designate_zones(id) ON DELETE CASCADE,
    project_id UUID NOT NULL,
    target_project_id UUID,
    key VARCHAR(255) NOT NULL,          -- Transfer key
    status VARCHAR(50) DEFAULT 'ACTIVE',
    created_at TIMESTAMP DEFAULT NOW(),
    expires_at TIMESTAMP
);

-- Floating IP auto-DNS mappings
CREATE TABLE designate_floatingip_records (
    id UUID PRIMARY KEY,
    floatingip_id UUID NOT NULL,        -- Neutron floating IP
    recordset_id UUID REFERENCES designate_recordsets(id) ON DELETE CASCADE,
    region VARCHAR(50),
    created_at TIMESTAMP DEFAULT NOW()
);
```

## API Endpoints

### Zones
- `GET /v2/zones` - List zones (project scoped)
- `POST /v2/zones` - Create zone
- `GET /v2/zones/{id}` - Get zone
- `PATCH /v2/zones/{id}` - Update zone
- `DELETE /v2/zones/{id}` - Delete zone
- `POST /v2/zones/{id}/tasks/abandon` - Abandon zone

### Record Sets
- `GET /v2/zones/{zone_id}/recordsets` - List record sets
- `POST /v2/zones/{zone_id}/recordsets` - Create record set
- `GET /v2/zones/{zone_id}/recordsets/{id}` - Get record set
- `PUT /v2/zones/{zone_id}/recordsets/{id}` - Update record set
- `DELETE /v2/zones/{zone_id}/recordsets/{id}` - Delete record set

### Pools (Admin Only)
- `GET /v2/pools` - List pools
- `POST /v2/pools` - Create pool
- `GET /v2/pools/{id}` - Get pool
- `PATCH /v2/pools/{id}` - Update pool
- `DELETE /v2/pools/{id}` - Delete pool

### Zone Transfers
- `POST /v2/zones/{zone_id}/tasks/transfer_requests` - Create transfer
- `GET /v2/zones/tasks/transfer_requests` - List transfer requests
- `POST /v2/zones/tasks/transfer_accepts` - Accept transfer

### Floating IP Integration
- `PUT /v2/reverse/floatingips/{region}:{floatingip_id}` - Set PTR record
- `GET /v2/reverse/floatingips/{region}:{floatingip_id}` - Get PTR record
- `PATCH /v2/reverse/floatingips/{region}:{floatingip_id}` - Update PTR
- `DELETE /v2/reverse/floatingips/{region}:{floatingip_id}` - Unset PTR

## Backend Implementation

### BIND9 Backend

```go
type BIND9Backend struct {
    confPath    string // /etc/bind/named.conf.local
    zoneDir     string // /var/lib/bind/zones
    rndcPath    string // /usr/sbin/rndc
    timeout     time.Duration
}

func (b *BIND9Backend) CreateZone(zone *Zone) error {
    // 1. Generate zone file
    zoneFile := b.generateZoneFile(zone)
    err := os.WriteFile(filepath.Join(b.zoneDir, zone.Name), zoneFile, 0644)
    if err != nil {
        return fmt.Errorf("failed to write zone file: %w", err)
    }

    // 2. Update named.conf.local
    b.appendZoneConfig(zone.Name)

    // 3. Reload BIND9
    ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
    defer cancel()

    cmd := exec.CommandContext(ctx, b.rndcPath, "reload")
    if err := cmd.Run(); err != nil {
        return fmt.Errorf("BIND9 reload failed (fail-early): %w", err)
    }

    return nil
}

func (b *BIND9Backend) generateZoneFile(zone *Zone) []byte {
    var buf bytes.Buffer

    // SOA record
    fmt.Fprintf(&buf, "$ORIGIN %s.\n", zone.Name)
    fmt.Fprintf(&buf, "$TTL %d\n", zone.TTL)
    fmt.Fprintf(&buf, "@ IN SOA ns1.%s. %s. (\n", zone.Name, zone.Email)
    fmt.Fprintf(&buf, "    %d ; serial\n", zone.Serial)
    fmt.Fprintf(&buf, "    3600 ; refresh\n")
    fmt.Fprintf(&buf, "    600 ; retry\n")
    fmt.Fprintf(&buf, "    86400 ; expire\n")
    fmt.Fprintf(&buf, "    3600 ; minimum\n")
    fmt.Fprintf(&buf, ")\n")

    // NS records
    fmt.Fprintf(&buf, "@ IN NS ns1.%s.\n", zone.Name)

    // Other records from database
    recordsets := b.getRecordSets(zone.ID)
    for _, rs := range recordsets {
        for _, record := range rs.Records {
            fmt.Fprintf(&buf, "%s %d IN %s %s\n",
                rs.Name, rs.TTL, rs.Type, record.Data)
        }
    }

    return buf.Bytes()
}
```

### PowerDNS Backend

```go
type PowerDNSBackend struct {
    apiURL   string // http://localhost:8081/api/v1
    apiKey   string
    client   *http.Client
    timeout  time.Duration
}

func (p *PowerDNSBackend) CreateZone(zone *Zone) error {
    ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
    defer cancel()

    payload := map[string]interface{}{
        "name": zone.Name + ".",
        "kind": "Native",
        "soa_edit_api": "INCEPTION-INCREMENT",
        "nameservers": []string{"ns1." + zone.Name + "."},
    }

    body, _ := json.Marshal(payload)
    req, _ := http.NewRequestWithContext(ctx, "POST",
        p.apiURL+"/servers/localhost/zones", bytes.NewReader(body))
    req.Header.Set("X-API-Key", p.apiKey)
    req.Header.Set("Content-Type", "application/json")

    resp, err := p.client.Do(req)
    if err != nil {
        return fmt.Errorf("PowerDNS API failed (fail-early): %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != 201 {
        return fmt.Errorf("PowerDNS returned %d", resp.StatusCode)
    }

    return nil
}
```

### Stub Backend (Development)

```go
type StubBackend struct{}

func (s *StubBackend) CreateZone(zone *Zone) error {
    log.Printf("STUB: Would create zone %s", zone.Name)
    return nil
}

func (s *StubBackend) CreateRecordSet(recordset *RecordSet) error {
    log.Printf("STUB: Would create recordset %s IN %s",
        recordset.Name, recordset.Type)
    return nil
}
```

## Fail-Early Strategy

All backend operations have 1-second timeout:

```go
func (s *Service) CreateRecordSet(ctx context.Context, rs *RecordSet) error {
    // 1. Validate input
    if err := rs.Validate(); err != nil {
        return fmt.Errorf("validation failed: %w", err)
    }

    // 2. Store in database (PENDING)
    rs.Status = "PENDING"
    if err := s.db.InsertRecordSet(rs); err != nil {
        return fmt.Errorf("database insert failed: %w", err)
    }

    // 3. Push to backend DNS (with timeout)
    ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
    defer cancel()

    if err := s.backend.CreateRecordSet(ctx, rs); err != nil {
        // Mark as ERROR, don't hide failure
        s.db.UpdateRecordSetStatus(rs.ID, "ERROR")
        return fmt.Errorf("backend failed (fail-early): %w", err)
    }

    // 4. Mark as ACTIVE
    s.db.UpdateRecordSetStatus(rs.ID, "ACTIVE")
    return nil
}
```

If BIND9 is down:
- Zone creation fails immediately with 503 Service Unavailable
- Record creation fails immediately with 503 Service Unavailable
- No queuing, no background workers, operator alerted immediately

## Neutron Integration

### Automatic Floating IP Records

```go
// Watch Neutron floating IP events
func (s *Service) WatchFloatingIPs(ctx context.Context) {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            s.syncFloatingIPs()
        }
    }
}

func (s *Service) syncFloatingIPs() {
    // Query Neutron for floating IPs with DNS names
    neutronClient := s.getNeutronClient()
    floatingIPs := neutronClient.ListFloatingIPs(ListOpts{
        Fields: []string{"id", "floating_ip_address", "dns_name", "dns_domain"},
    })

    for _, fip := range floatingIPs {
        if fip.DNSName == "" || fip.DNSDomain == "" {
            continue
        }

        // Create or update DNS record
        fqdn := fip.DNSName + "." + fip.DNSDomain
        s.createOrUpdateARecord(fqdn, fip.FloatingIPAddress, fip.ID)
    }
}
```

### Configuration

```yaml
designate:
  neutron_integration:
    enabled: true
    sync_interval: 30s
    auto_ptr: true  # Automatically create PTR records
```

## CLI Tool

```bash
# Create zone
o3k-designate-cli zone create --name example.com --email admin@example.com

# List zones
o3k-designate-cli zone list

# Create A record
o3k-designate-cli recordset create example.com --name www --type A --records 192.168.1.10

# Create CNAME
o3k-designate-cli recordset create example.com --name blog --type CNAME --records www.example.com

# Update record
o3k-designate-cli recordset update example.com www --records 192.168.1.20

# Delete record
o3k-designate-cli recordset delete example.com www

# Set PTR for floating IP
o3k-designate-cli floatingip set RegionOne:fip-uuid --ptrdname vm1.example.com
```

## Testing Strategy

### Unit Tests
- Zone file generation
- Record validation
- Backend interface mocking
- TTL calculations
- Serial number incrementing

### Integration Tests
```bash
#!/bin/bash
# test/integration/designate_test.sh

# Start Designate with BIND9 backend
docker run -d --name bind9 ubuntu/bind9
o3k-designate --config test-config.yaml &

# Create zone
ZONE_ID=$(openstack zone create --email admin@test.com test.com. -f value -c id)

# Create A record
openstack recordset create $ZONE_ID www --type A --records 192.168.1.10

# Verify DNS resolution
sleep 2  # Allow propagation
IP=$(dig @localhost www.test.com +short)
assert_equals "$IP" "192.168.1.10"

# Delete record
openstack recordset delete $ZONE_ID www

# Verify deletion
IP=$(dig @localhost www.test.com +short)
assert_empty "$IP"
```

### Contract Tests
```go
func TestDesignateOpenStackCompatibility(t *testing.T) {
    // Use python-designateclient
    client := NewDesignateClient("http://localhost:9001")

    // Create zone
    zone := client.CreateZone(ZoneCreateRequest{
        Name: "test.com.",
        Email: "admin@test.com",
    })
    assert.NotEmpty(t, zone.ID)

    // Create recordset
    rs := client.CreateRecordSet(zone.ID, RecordSetCreateRequest{
        Name: "www.test.com.",
        Type: "A",
        Records: []string{"192.168.1.10"},
    })
    assert.Equal(t, "ACTIVE", rs.Status)
}
```

## Migration Path

### Phase 1: Core Zone Management (Weeks 1-2)
- Database schema
- Zone CRUD API
- Stub backend
- CLI tool
- Unit tests

### Phase 2: BIND9 Backend (Week 3)
- BIND9 integration
- Zone file generation
- rndc control
- Integration tests

### Phase 3: Record Management (Week 4)
- RecordSet CRUD API
- A, AAAA, CNAME, TXT support
- TTL management
- DNS validation

### Phase 4: Neutron Integration (Week 5)
- Floating IP sync
- Automatic record creation
- PTR record support
- End-to-end tests

### Phase 5: Advanced Features (Week 6)
- PowerDNS backend
- Zone transfers
- Pool management
- MX, SRV, NS records

## Success Criteria

- [ ] python-designateclient works with all endpoints
- [ ] BIND9 backend functional
- [ ] DNS resolution works (dig/nslookup)
- [ ] Neutron floating IP auto-DNS works
- [ ] CLI tool works for common operations
- [ ] Fail-early: BIND9 failures return < 1s
- [ ] Contract tests pass
- [ ] OpenStack CLI integration works
- [ ] Zone transfers work
- [ ] Documentation complete

## Security Considerations

1. **Zone Ownership**: Strict project_id validation
2. **TSIG Keys**: Support for BIND9 TSIG authentication
3. **Rate Limiting**: Prevent DNS amplification attacks
4. **Validation**: Sanitize all DNS names (prevent injection)
5. **Wildcard Records**: Controlled wildcard support
6. **Zone Transfers**: Authenticated zone transfers only

## Performance Targets

- Zone creation: < 1s (including BIND9 reload)
- Record creation: < 500ms
- Record listing: < 100ms (1000 records)
- DNS resolution: < 50ms (via BIND9)
- Backend operation timeout: 1s (fail-early)

## References

- OpenStack Designate API v2
- RFC 1035 (DNS Domain Names)
- RFC 2136 (DNS Dynamic Updates)
- BIND9 Administrator Reference Manual
- PowerDNS API Documentation
