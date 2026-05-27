# O3K — Lightweight OpenStack in a Single Binary

O3K is a single Go binary that exposes the OpenStack control-plane HTTP
APIs (Keystone, Nova, Neutron, Cinder, Glance, Placement, Metadata).
The goal is K3s-style packaging for OpenStack: one process, no message
queue, no separate scheduler / conductor daemons, SQLite by default.

```
Single binary → 7 services → 342 endpoint routes → SQLite default (PostgreSQL optional)
```

> **Status: Alpha. Not production-ready.** Basic CRUD works for all
> services and the binary boots zero-config. API fidelity against real
> OpenStack clients is roughly 70-80% per service: query filters,
> response-schema fields, and state-machine validation are all
> incomplete. Real-mode hypervisor, networking, and storage code paths
> exist and have been exercised on developer machines but have not been
> hardened or audited. See [Project Status](#project-status) for the
> honest gap list and [SECURITY.md](SECURITY.md) for the threat model.

## Quick Start

### Zero Config

```bash
# Download and run — no config required
./o3k

# On first start, O3K:
# - creates a SQLite database at ~/.local/share/o3k/db/state.db
# - runs all 74 migrations (embedded in the binary)
# - starts every service in stub mode
# - generates a JWT secret and an admin password, and prints the
#   admin password ONCE to stderr (capture it now or set
#   O3K_ADMIN_PASSWORD beforehand)
# - exposes /healthz, /readyz, and /metrics on each service
```

### With PostgreSQL

```bash
./o3k --datastore postgres --db-url "postgres://user:pass@localhost/o3k"
```

### With Custom Ports

```bash
./o3k --port 5000  # Keystone: 5357, Nova: 5774, etc.
```

### Database Options

O3K supports two database backends:

| Backend | Use Case | Config |
|---------|----------|--------|
| SQLite (default) | Development, single-node, edge | `./o3k` or `./o3k --datastore sqlite` |
| PostgreSQL | Production, multi-replica | `./o3k --datastore postgres --db-url "postgres://..."` |

SQLite mode embeds all 74 migrations in the binary — no external files needed. Data is stored at `$O3K_DATA_DIR/db/state.db` (default: `~/.local/share/o3k/`).

To migrate from SQLite to PostgreSQL:
```bash
./o3k-migrate --from sqlite:///path/to/state.db --to "postgres://user:pass@host/db"
```

### Docker Compose (with Horizon)

```bash
cd deployments/
docker compose -f docker-compose-horizon.yml up -d

# Horizon: http://localhost/dashboard
# The first time O3K starts it generates a random admin password and
# prints it to the container's stderr. Grab it with:
#   docker compose -f docker-compose-horizon.yml logs o3k | grep -A1 'admin password'
# To pin a password instead, set O3K_ADMIN_PASSWORD in the environment.

# CLI usage (after recovering the printed password):
export OS_AUTH_URL=http://localhost:35357/v3
export OS_USERNAME=admin OS_PASSWORD='<the printed password>'
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
│  Placement · Metadata                            │
│                                                  │
│  Shared: JWT auth, connection pool, middleware   │
└──────────────────────┬───────────────────────────┘
                       │
          ┌────────────┼────────────┐
          │  SQLite    │  PostgreSQL │
          │  (default) │  (optional) │
          └────────────┴────────────┘
```

No RabbitMQ. No Conductor. No Scheduler daemons. One process, one database.

### Operating Modes

| Component | Stub (default) | Real |
|-----------|----------------|------|
| Compute | fake VMs in-process | libvirt/KVM (Linux only, exercised on developer machines, not production-hardened) |
| Networking | no namespaces or iptables | iptables or eBPF (Linux + `CAP_NET_ADMIN`) |
| Storage | in-memory or local files | RBD (Ceph) or S3 (MinIO/AWS); hybrid modes supported |
| Overlay | disabled | VXLAN multi-node (experimental) |

## Project Status

External readiness audits have scored O3K around 4.5/10 (45%) against
production OpenStack workloads. Phase 1 trust-cleanup work has lifted
that toward roughly 6/10 for evaluation use, but production targets
(stable real-mode operation under load, full RBAC, contract-test
parity with Devstack) remain ahead. We track concrete gaps below
rather than picking a single number.

### What Works Today

| Capability | Status | Confidence |
|-----------|--------|------------|
| Basic CRUD (create/list/show/delete) for all 5 services | Working | High |
| Keystone password auth → JWT token | Working | High |
| Zero-config single binary (`./o3k`) | Working | High |
| Docker Compose single-node deployment | Working | High |
| Stub mode on macOS/Linux | Working | High |
| Health endpoints (/healthz, /readyz) | Working | High |
| Metrics endpoint (/metrics) | Working | High |
| Rate limiting on token creation | Working | High |
| RBAC policy middleware | Working | High |
| OpenTelemetry tracing | Working | Medium |
| Horizon login + basic resource lists | Working | Medium |
| OpenStack CLI simple commands | Working | Medium |
| Simple Terraform plans (create/delete) | Working | Medium |
| Most Terraform `openstack_*` resources | Working | Medium |

### What Does NOT Work Yet

| Capability | Status | Impact |
|-----------|--------|--------|
| Full RBAC policy files (policy.json) | Partial | Admin-only operations rely on role check, not full policy evaluation |
| OpenTelemetry OTLP collector | Partial | Stdout exporter works; OTLP endpoint is optional config |
| Real libvirt mode (stable) | Partial | Works but limited production testing |
| Real storage (Ceph) | Partial | Build tag cleanup in progress |
| SPEC-002 auth (OAuth2, SAML, LDAP) | Not started | Only password auth works |
| Modular architecture (SPEC-001) | Not started | Still monolithic |
| Remaining ~25% API response fields | In progress | Some Terraform data sources may fail |

### API Surface

342 endpoint routes registered. Estimated fidelity per service against
real OpenStack clients:

| Service | Routes | Estimated Fidelity | Notes |
|---------|--------|-------------------|-------|
| Keystone (Identity) | 61 | ~75% | Regions added; federation/SAML missing |
| Nova (Compute) | 72 | ~75% | CRUD + actions work; some response fields missing |
| Neutron (Network) | 98 | ~78% | Extensions added; DVR/SFC missing |
| Cinder (Block Storage) | 73 | ~72% | AZs added; race conditions fixed |
| Glance (Image) | 38 | ~70% | Core workflow solid; metadefs advanced missing |

"Fidelity" here means: what fraction of real-OpenStack behaviour does
a given client (gophercloud, Terraform, Horizon) get correct without
workarounds? These numbers are internal estimates, not measured pass
rates against an upstream conformance suite.

### Client Compatibility

| Client | Simple Operations | Full Workflow | Notes |
|--------|------------------|--------------|-------|
| OpenStack CLI | Works | Works | Most commands functional |
| Terraform | Works | Mostly works | Main resources tested; some data sources may fail |
| Horizon | Works | Works | Login, compute, network, storage functional |
| gophercloud | Basic CRUD | Breaks | Missing response fields cause nil dereferences |

### Contract Tests

```
Unit tests: blocking in CI
Contract tests: blocking in CI (require Docker Compose stack; pass-rate gate at 85%)
Integration tests: 20+ bash scripts (manual)
Vulnerability scan: govulncheck blocking in CI
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
migrations/           74 SQL migration files
test/                 Contract + integration tests
deployments/          Docker Compose configs
docs/                 Documentation
```

## Documentation

| Topic | Guide |
|-------|-------|
| Getting started | [Deployment Guide](docs/DEPLOYMENT.md) |
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

The seed migration creates an `admin` user in the `default` project /
`Default` domain. The password is **not** hard-coded:

| Source | Behaviour |
|--------|-----------|
| Zero-config (`./o3k`) | Bootstrap generates a random password and prints it once to stderr |
| `O3K_ADMIN_PASSWORD` env var set | That value is used verbatim |
| Neither | A random password is generated and printed once to stderr; capture it before it scrolls |

In any deployment beyond local development you must:

- set a strong `O3K_ADMIN_PASSWORD` (or rotate the generated one
  immediately via `openstack user set`)
- set `O3K_JWT_SECRET` to a unique value of at least 32 bytes
- terminate TLS in front of O3K
- review [SECURITY.md](SECURITY.md) for the full threat model

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
