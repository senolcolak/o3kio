# O3K Architecture

## Design Philosophy

O3K is built on three core principles:

1. **API Compatibility First**: 100% OpenStack API compatibility to work seamlessly with Horizon and OpenStack CLI/SDK
2. **Synchronous Operations**: No async state machines - operations complete before API returns
3. **Fail-Fast Design**: External dependency failures return immediately (< 1 second timeouts)

## System Architecture

### High-Level Overview

```
┌─────────────────────────────────────────────────────────────┐
│                    O3K Process                        │
├─────────────────────────────────────────────────────────────┤
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │  Keystone    │  │     Nova     │  │   Neutron    │      │
│  │   :5000      │  │    :8774     │  │    :9696     │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
│  ┌──────────────┐  ┌──────────────┐                        │
│  │   Cinder     │  │    Glance    │                        │
│  │   :8776      │  │    :9292     │                        │
│  └──────────────┘  └──────────────┘                        │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐  │
│  │           Common Layer                                │  │
│  │  - Database Connection Pool                          │  │
│  │  - JWT Auth Service                                  │  │
│  │  - Middleware (Auth, Logging, CORS)                  │  │
│  └──────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
                         ↓
        ┌────────────────┼────────────────┐
        ↓                ↓                ↓
   PostgreSQL       libvirt (KVM)    Ceph (RBD)
   (State DB)      (Compute)         (Storage)
```

## Component Architecture

### 1. Keystone (Identity Service)

**Responsibilities:**
- User authentication (password-based)
- JWT token generation and validation
- Service catalog generation
- User/project/role management

**Technology:**
- JWT tokens (no token database)
- bcrypt password hashing
- PostgreSQL for user/project storage

**Key Design Decisions:**
- **JWT over Fernet**: Stateless tokens, no shared secret rotation needed
- **No token blacklist**: Tokens expire naturally based on TTL
- **Middleware-based auth**: All services share the same auth middleware

### 2. Nova (Compute Service)

**Responsibilities:**
- VM lifecycle management (create, delete, reboot, stop)
- Flavor management
- Hypervisor abstraction
- Microversion negotiation

**Technology:**
- `github.com/digitalocean/go-libvirt` - Pure Go libvirt bindings
- Direct libvirt socket communication (no CGO)
- XML-based VM definitions

**Key Design Decisions:**
- **Synchronous VM creation**: `DomainDefineXML()` + `DomainCreate()` in single API call
- **No async state machine**: VM status transitions happen immediately
- **Hypervisor mocking**: Fake hypervisor stats for Horizon compatibility

### 3. Neutron (Network Service)

**Responsibilities:**
- Virtual network management
- Subnet/CIDR allocation
- Port attachment to VMs
- Security groups (iptables-based)
- DHCP management (dnsmasq)

**Technology:**
- `github.com/vishvananda/netlink` - Linux networking
- `github.com/vishvananda/netns` - Network namespaces
- `github.com/coreos/go-iptables` - iptables rules

**Key Design Decisions:**
- **Namespace isolation**: Per-project network namespaces
- **No VXLAN in v1**: Single-node deployment uses bridges only
- **iptables security groups**: Simple, proven, no eBPF complexity in v1
- **dnsmasq for DHCP**: Standard, well-tested DHCP server

### 4. Cinder (Block Storage Service)

**Responsibilities:**
- Volume lifecycle management
- Volume attachment to VMs
- Snapshot management
- Volume types

**Technology:**
- `github.com/ceph/go-ceph` - Ceph RBD integration
- Direct RBD operations (create, delete, snapshot)

**Key Design Decisions:**
- **Ceph-only**: No local storage backend in v1
- **1-second timeout**: Fast failure if Ceph unavailable
- **Synchronous operations**: Volume creation completes before API returns

### 5. Glance (Image Service)

**Responsibilities:**
- Image metadata management
- Image upload/download
- Image storage (Ceph RBD backed)
- Public/private image visibility

**Technology:**
- `github.com/ceph/go-ceph` - Ceph RBD integration
- Streaming uploads/downloads

**Key Design Decisions:**
- **Ceph-backed storage**: Images stored as RBD snapshots
- **Metadata in PostgreSQL**: Fast queries, RBD only for data
- **Streaming transfers**: No full image buffering in memory

## Data Flow

### VM Creation Flow

```
1. User: openstack server create --flavor m1.small --image cirros test-vm
                    ↓
2. Nova API: POST /v2.1/servers
                    ↓
3. Validate token (JWT signature check)
                    ↓
4. Query PostgreSQL: Fetch flavor, image metadata
                    ↓
5. Query Neutron: Create ports, get network config
                    ↓
6. Generate libvirt XML with RBD backing
                    ↓
7. libvirt.DomainDefineXML() + DomainCreate()
                    ↓
8. Insert instance into PostgreSQL
                    ↓
9. Return HTTP 202 with instance details
```

### Authentication Flow

```
1. User: curl -X POST /v3/auth/tokens (with credentials)
                    ↓
2. Keystone: Validate username/password against PostgreSQL
                    ↓
3. bcrypt.CompareHashAndPassword()
                    ↓
4. If scoped: Fetch project and roles from PostgreSQL
                    ↓
5. Generate JWT token (HS256 signature)
                    ↓
6. Build service catalog dynamically
                    ↓
7. Return token in X-Subject-Token header + body
```

## Database Schema

### Core Tables

- **users**: User credentials and metadata
- **projects**: Project (tenant) metadata
- **roles**: Role definitions (admin, member, reader)
- **role_assignments**: User-project-role mappings
- **instances**: VM instances
- **flavors**: VM flavor definitions
- **networks**: Virtual networks
- **subnets**: IP subnets
- **ports**: Network ports (attached to VMs)
- **volumes**: Cinder volumes
- **images**: Glance image metadata

### Indexes

Key indexes for performance:
- `instances(project_id)` - Fast per-project filtering
- `ports(device_id)` - Fast port lookup by VM
- `role_assignments(user_id, project_id)` - Fast role checks

## Security

### Authentication

- **Password hashing**: bcrypt (cost factor 10)
- **Token signing**: HS256 (HMAC-SHA256)
- **Token TTL**: 24 hours default (configurable)

### Authorization

- **Middleware-based**: All protected endpoints check X-Auth-Token
- **Role-based access**: RequireRole() middleware for privileged ops
- **Project scoping**: Automatic filtering by project_id from token

### Network Security

- **Namespace isolation**: Projects cannot see each other's networks
- **Security groups**: Per-VM iptables rules
- **Default deny**: All traffic blocked unless explicitly allowed

## Performance Characteristics

### Target Metrics

- **API latency**: < 10ms for read operations
- **VM creation**: < 2 seconds (depends on libvirt)
- **Volume creation**: < 1 second (fail-fast if Ceph down)
- **Token validation**: < 1ms (JWT signature check)

### Connection Pooling

- **PostgreSQL**: 20 connections default (configurable)
- **libvirt**: Connection pooling with max 10 connections
- **Ceph**: Connection per operation (stateless)

### Concurrency

- **Goroutines per request**: Each HTTP request runs in separate goroutine
- **Database queries**: All queries use context for cancellation
- **Netlink operations**: Mutex per namespace to prevent races

## Deployment Models

### Single-Node (v1)

```
┌────────────────────────────────┐
│       O3K Host          │
│                                │
│  ┌──────────────────────────┐ │
│  │  O3K Binary       │ │
│  │  (All services)          │ │
│  └──────────────────────────┘ │
│                                │
│  ┌──────────────────────────┐ │
│  │  PostgreSQL              │ │
│  └──────────────────────────┘ │
│                                │
│  ┌──────────────────────────┐ │
│  │  libvirt + KVM           │ │
│  └──────────────────────────┘ │
└────────────────────────────────┘
          ↓
    Ceph Cluster
```

### Multi-Node (v2 - Future)

```
┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐
│  Control Node   │  │  Compute Node 1 │  │  Compute Node 2 │
│                 │  │                 │  │                 │
│  - Keystone     │  │  - Nova Agent   │  │  - Nova Agent   │
│  - Nova API     │  │  - libvirt      │  │  - libvirt      │
│  - Neutron API  │  │  - Neutron Agnt │  │  - Neutron Agnt │
│  - Cinder API   │  │  - VXLAN        │  │  - VXLAN        │
│  - Glance API   │  │                 │  │                 │
│  - PostgreSQL   │  │                 │  │                 │
└─────────────────┘  └─────────────────┘  └─────────────────┘
          ↓                  ↓                    ↓
                      Ceph Cluster
```

## Observability (Future)

### Metrics (Prometheus)

- API request latency (p50, p95, p99)
- Database connection pool stats
- libvirt operation latency
- Ceph operation latency
- Active VMs/volumes/networks count

### Logging (Structured JSON)

```json
{
  "timestamp": "2024-03-06T12:34:56Z",
  "level": "info",
  "service": "nova",
  "method": "POST",
  "path": "/v2.1/servers",
  "status": 202,
  "duration_ms": 1842,
  "user_id": "abc123",
  "project_id": "def456"
}
```

### Tracing (OpenTelemetry)

- Distributed traces across services
- Span per database query
- Span per libvirt/Ceph operation

## Failure Modes

### PostgreSQL Down

- **Impact**: All API operations fail
- **Response**: HTTP 503 Service Unavailable
- **Mitigation**: Database replication in production

### Ceph Down

- **Impact**: Volume/image operations fail
- **Response**: HTTP 503 within 1 second (fast fail)
- **Mitigation**: Ceph cluster redundancy

### libvirt Down

- **Impact**: VM operations fail
- **Response**: HTTP 503
- **Mitigation**: Multi-node deployment with live migration

## Future Enhancements

### v2.0 (Multi-node)

- VXLAN overlay networks
- Floating IPs (external network access)
- Live migration
- Placement API (resource scheduling)
- eBPF security groups (kernel-space filtering)

### v3.0 (Production)

- High availability (multi-master)
- Heat (orchestration)
- Swift (object storage)
- Ceilometer (telemetry)
- Multi-region support

## References

- [OpenStack API Reference](https://docs.openstack.org/api-ref/)
- [Keystone v3 API](https://docs.openstack.org/api-ref/identity/v3/)
- [Nova v2.1 API](https://docs.openstack.org/api-ref/compute/)
- [Neutron v2.0 API](https://docs.openstack.org/api-ref/network/v2/)
