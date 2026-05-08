# O3K — Lightweight OpenStack in a Single Binary

O3K replaces the entire OpenStack control plane with one Go binary. Like K3s did for Kubernetes — same API surface, dramatically less complexity.

```
Single binary → 5 services → 342 endpoint routes registered → PostgreSQL only
```

> **Status: Alpha.** Basic CRUD works for all services. Query filters, response schema completeness, state machine validation, and production safety features are still in progress. See [Project Status](#project-status) for honest details.

## Quick Start

```bash
cd deployments/
docker compose -f docker-compose-horizon.yml up -d

# Access Horizon: http://localhost/dashboard
# Credentials: admin / secret (domain: Default)

# Or use the CLI:
export OS_AUTH_URL=http://localhost:35357/v3
export OS_USERNAME=admin OS_PASSWORD=secret
export OS_PROJECT_NAME=default OS_USER_DOMAIN_NAME=Default OS_PROJECT_DOMAIN_NAME=Default
openstack token issue
openstack server create --flavor m1.small --image cirros --network my-net test-vm
```

## Architecture

```
┌──────────────────────────────────────────────────┐
│                  O3K Binary (~35MB)               │
│                                                  │
│  Keystone · Nova · Neutron · Cinder · Glance    │
│                                                  │
│  Shared: JWT auth, connection pool, middleware   │
└──────────────────────┬───────────────────────────┘
                       │
              ┌────────┴────────┐
              │   PostgreSQL 17 │
              └─────────────────┘
```

No RabbitMQ. No Conductor. No Scheduler daemons. One process, one database.

### Operating Modes

| Component | Development | Production |
|-----------|------------|------------|
| Compute | `stub` (fake VMs) | `real` (libvirt/KVM) |
| Networking | `stub` (no netns) | `iptables` or `ebpf` |
| Storage | `stub` or `local` | `rbd` (Ceph), `s3` (MinIO/AWS) |
| Overlay | disabled | VXLAN (multi-node) |

## Project Status

### What Works Today

| Capability | Status | Confidence |
|-----------|--------|------------|
| Basic CRUD (create/list/show/delete) for all 5 services | Working | High |
| Keystone password auth → JWT token | Working | High |
| Docker Compose single-node deployment | Working | High |
| Stub mode on macOS/Linux | Working | High |
| Unit tests (15 packages pass) | Working | High |
| Horizon login + basic resource lists | Partial | Medium |
| OpenStack CLI simple commands | Partial | Medium |
| Simple Terraform plans (create/delete) | Partial | Medium |

### What Does NOT Work Yet

| Capability | Status | Impact |
|-----------|--------|--------|
| List endpoint query filters (all services) | Not implemented | Horizon shows incomplete data, CLI filtering broken |
| Complete response schemas | ~60% of fields present | SDK nil dereferences, Terraform data source failures |
| State machine validation | Missing | Can delete attached volumes, no server state transitions |
| Multi-tenant security (RBAC on writes) | Missing | Any authenticated user can modify other users |
| Production timeouts/health checks | Missing | Vulnerable to hangs and DoS |
| Real libvirt mode (stable) | Has blocking bugs | No timeout, no mutex, will hang or crash |
| Real storage (Ceph) | Won't compile | Build tag issues in ceph_rbd.go |
| SPEC-002 auth (OAuth2, SAML, LDAP, MFA) | Not started | Only basic password auth works |
| Modular architecture (SPEC-001) | Not started | Still monolithic |

### API Surface

342 endpoint routes are registered. However, "route registered" ≠ "fully implemented":

| Service | Routes | Estimated Fidelity | Notes |
|---------|--------|-------------------|-------|
| Keystone (Identity) | 61 | ~50% | Auth works; list filters, domains, regions missing |
| Nova (Compute) | 72 | ~40% | CRUD works; actions, filters, full response fields missing |
| Neutron (Network) | 98 | ~45% | CRUD works; port binding, router:external, QoS missing |
| Cinder (Block Storage) | 73 | ~35% | CRUD works; project_id routing broken, no state machine |
| Glance (Image) | 38 | ~40% | CRUD works; tags dropped, no checksums, filters ignored |

"Fidelity" means: would a real OpenStack client (gophercloud, Terraform, Horizon) get correct behavior from this endpoint without workarounds?

### Client Compatibility (Honest Assessment)

| Client | Simple Operations | Full Workflow | Notes |
|--------|------------------|--------------|-------|
| OpenStack CLI | Works | Partial | Filtering doesn't work, some subcommands missing |
| Terraform | Basic resources | Breaks | Data sources fail, state drift from missing fields |
| Horizon | Login + lists | Partial | Network tab broken, server actions missing |
| gophercloud | Basic CRUD | Breaks | Missing response fields cause nil dereferences |

### Contract Tests

```
Unit tests: 15/15 packages passing
Contract tests: Require running server (not CI-integrated yet)
Integration tests: 20+ bash scripts (manual)
```

## Configuration

```yaml
# config/o3k.yaml
database:
  url: "postgres://o3k:secret@localhost:5432/o3k?sslmode=disable"
keystone:
  jwt_secret: ""  # MUST set via O3K_JWT_SECRET env var in production
nova:
  libvirt_mode: stub   # stub | real
neutron:
  networking_mode: stub   # stub | iptables | ebpf
cinder:
  storage_mode: local     # stub | local | rbd | s3
glance:
  storage_mode: local     # stub | local | rbd | s3
```

Environment overrides: `O3K_DB_URL`, `O3K_JWT_SECRET`, `O3K_ENV`.

Full reference: [docs/CONFIGURATION.md](docs/CONFIGURATION.md)

## Development

```bash
make build          # Build binary → bin/o3k
make test           # Run unit tests
make dev            # Hot-reload development server
make lint           # golangci-lint
./test/quick_test.sh  # Integration tests (requires running O3K)
```

### Project Structure

```
cmd/o3k/              Main binary
internal/
├── keystone/         Identity service
├── nova/             Compute service
├── neutron/          Network service
├── cinder/           Block storage
├── glance/           Image service
├── database/         DB models, migrations
├── scheduler/        Task queue, reconciler
├── tunnel/           gRPC agent tunnel
├── middleware/       Auth, logging, CORS
└── common/           Shared utilities
pkg/
├── hypervisor/       libvirt abstraction
├── networking/       netlink, VXLAN, iptables
└── storage/          RBD, S3, local backends
migrations/           62 SQL migration files
test/                 Contract + integration tests
deployments/          Docker Compose configs
docs/                 Documentation
```

## Documentation

| Topic | Guide |
|-------|-------|
| Getting started | [Deployment Guide](docs/DEPLOYMENT_GUIDE.md) |
| Architecture | [Architecture](docs/ARCHITECTURE.md) |
| Configuration | [Configuration](docs/CONFIGURATION.md) |
| Operations | [Operations](docs/OPERATIONS.md) |
| Networking | [Networking Modes](docs/NETWORKING_MODES.md) |
| Storage | [Storage Modes](docs/STORAGE_MODES.md) |
| Scaling | [Production Scaling](docs/SCALING.md) |
| API | [API Reference](docs/API.md) |
| Contributing | [Contributing](docs/CONTRIBUTING.md) |
| Troubleshooting | [Troubleshooting](docs/TROUBLESHOOTING.md) |

## Default Credentials

| Field | Value |
|-------|-------|
| User | `admin` |
| Password | `secret` |
| Project | `default` |
| Domain | `Default` |

**Change `jwt_secret` and `admin_password` in any non-local deployment.**

## Roadmap

See [docs/ROADMAP.md](docs/ROADMAP.md) for the full gap-closure plan.

**Priority order:**
1. Security fixes (RBAC, auth bypasses, timeouts)
2. Response schema completeness (make clients stop crashing)
3. Query filter implementation (make list endpoints usable)
4. State machine validation (prevent data corruption)
5. Missing critical endpoints (server actions, volume types, image import)
6. Enhanced authentication (SPEC-002)
7. Modular architecture (SPEC-001)

## License

Apache License 2.0
