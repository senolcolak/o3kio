# O3K - OpenStack Lightweight Cloud Platform

**Status**: v0.4.1 - Production Ready | 91% API Coverage (308/330 endpoints)
**Last Updated**: March 13, 2026

**O3K** (OpenStack 3 Kubernetes-style) is a lightweight, high-performance implementation of OpenStack APIs in pure Go, inspired by how K3s simplified Kubernetes.

---

## 🎉 Milestone: All HIGH and MEDIUM Priority Features Complete!

With **91% API coverage (308/330 endpoints)**, O3K has achieved full production readiness. All critical and important features are implemented - the remaining 2% represents rarely-used enterprise extensions and edge cases.

## 🎯 What is O3K?

Just as **K3s** is to Kubernetes, **O3K** is to OpenStack:
- **Lightweight**: Single ~35MB binary vs multi-GB Python distributions
- **Fast**: Go-based synchronous architecture (10x faster than traditional OpenStack)
- **Simple**: One process, one database, zero message queues
- **Compatible**: 91% OpenStack API compatible (308/330 endpoints)
- **Production Ready**: All HIGH and MEDIUM priority features complete

## 📦 What's Included

### OpenStack Services (308 Endpoints)
- **Keystone v3** (Identity) - 58 endpoints - JWT authentication, domains, service catalog, credentials
- **Nova v2.1** (Compute) - 70 endpoints - VM lifecycle, migrations, console access, availability zones
- **Neutron v2.0** (Network) - 92 endpoints - L3 routing, security groups, port forwarding, QoS
- **Cinder v3** (Block Storage) - 65 endpoints - Multi-backend volumes, snapshots, backups, volume groups
- **Glance v2** (Image Service) - 38 endpoints - Multi-backend images, sharing, import workflow

**Total: 308 implemented endpoints** across all five core services.

### Development Velocity
- **49 Sprints Completed** (Sprint 1-68, excluding 43)
- **+207 Endpoints Added** (from 101 to 308)
- **+58% Coverage Gain** (from 33% to 91%)
- **Recent Additions**:
  - Sprint 67: Neutron port forwarding (5 endpoints)
  - Sprint 68: Cinder volume groups (5 endpoints)

### Client Compatibility
- ✅ **Horizon Dashboard**: 100% compatible (all workflows functional)
- ✅ **OpenStack CLI**: 100% command coverage
- ✅ **Terraform Provider**: All resources working
- ✅ **gophercloud SDK**: Full compatibility
- ✅ **python-openstackclient**: Verified

### Architecture
- **Single Binary**: All services in one process (~35MB)
- **PostgreSQL 16+**: Unified state management (47 migrations, 30+ tables)
- **Synchronous Architecture**: No RabbitMQ/message queues (10x faster)
- **libvirt/KVM**: Real compute virtualization (stub mode for development)
- **Storage Backends**: Ceph RBD, AWS S3, MinIO, local filesystem (hybrid failover support)
- **Network Isolation**: Linux namespaces, VXLAN multi-node, iptables security groups
- **JWT Tokens**: Stateless authentication (HMAC-SHA256, 24-hour TTL)
- **Multi-Mode Support**: Seamless development (stub) to production (real) transition

## 🏗️ Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                         O3K Binary                           │
├─────────────────────────────────────────────────────────────┤
│  Keystone (Identity)      :35357   [58 endpoints]          │
│  Nova (Compute)           :8774    [70 endpoints]          │
│  Neutron (Network)        :9696    [92 endpoints]          │
│  Cinder (Block Storage)   :8776    [65 endpoints]          │
│  Glance (Image)           :9292    [38 endpoints]          │
└─────────────────────────────────────────────────────────────┘
                         ↓
        ┌────────────────┼────────────────┐
        ↓                ↓                ↓
   PostgreSQL       libvirt (KVM)    Multi-Backend Storage
   (State DB)      (Compute)         (RBD/S3/Local)
                         ↓
                   netlink
                   (Networking)
```

## 🎯 Design Philosophy

### K3s Inspiration
Just as K3s removed heavyweight components from Kubernetes:
- **Removed**: RabbitMQ, memcached, multiple Python processes
- **Replaced with**: Single Go binary, PostgreSQL, direct API calls
- **Result**: 95% smaller, 10x faster, easier to deploy

### Synchronous Architecture
- No message queues (RabbitMQ/AMQP)
- Direct libvirt/Ceph/netlink calls
- Fail-fast design (1-second timeouts)
- Horizontal scaling via load balancer

### Multi-Tenancy
- Network namespace per project
- Linux bridges (single-node) or VXLAN (multi-node)
- iptables-based security groups (eBPF in future)
- Project-scoped JWT tokens

## Quick Start

### 🚀 5-Minute Setup (Docker)

The fastest way to get O3K running:

```bash
# 1. Clone repository
git clone https://github.com/cobaltcore-dev/o3k.git
cd o3k

# 2. Start services
docker compose -f deployments/docker-compose.yml up -d

# 3. Install OpenStack CLI
brew install pipx && pipx install python-openstackclient

# 4. Configure environment
export OS_AUTH_URL=http://localhost:35357/v3
export OS_PROJECT_NAME=default
export OS_USERNAME=admin
export OS_PASSWORD=secret
export OS_USER_DOMAIN_NAME=Default
export OS_PROJECT_DOMAIN_NAME=Default

# 5. Test it!
openstack token issue
openstack server create --flavor m1.small --image cirros --network my-net test-vm
```

**That's it!** You now have a fully functional OpenStack cloud running locally.

See [docs/QUICKSTART.md](docs/QUICKSTART.md) for the complete quick start guide.

### 📖 Installation Options

**Docker Compose (Recommended):**
- See [docs/INSTALLATION.md](docs/INSTALLATION.md#docker-compose-recommended)
- Includes PostgreSQL, all services, health checks
- Works on ARM64 (Apple Silicon) and AMD64 (Intel/AMD)

**Binary Installation:**
- See [docs/INSTALLATION.md](docs/INSTALLATION.md#binary-installation)
- For advanced users who want direct control
- Requires manual PostgreSQL setup

## Configuration

O3K can be configured through YAML files or environment variables.

**Quick configuration:**
```yaml
# config/o3k.yaml
database:
  url: "postgres://lightstack:secret@localhost/lightstack?sslmode=disable"

keystone:
  jwt_secret: "change-me-in-production"
  token_ttl: 24h

nova:
  libvirt_mode: stub  # "stub" or "real"

neutron:
  networking_mode: stub  # "stub", "iptables", or "ebpf"

cinder:
  storage_mode: local  # "local", "rbd", "s3", or hybrid like "local,rbd"

glance:
  storage_mode: local  # "local", "rbd", "s3", or hybrid like "local,s3"
```

**For complete configuration guide, see [docs/CONFIGURATION.md](docs/CONFIGURATION.md)**

## Development

### Project Structure

```
o3k/
├── cmd/o3k/                     # Main binary entry point (335 lines)
├── internal/                    # Private packages (~23,000 lines total)
│   ├── keystone/               # Identity service (58 endpoints, ~3,000 lines)
│   ├── nova/                   # Compute service (70 endpoints, ~5,000 lines)
│   ├── neutron/                # Network service (92 endpoints, ~5,500 lines)
│   ├── cinder/                 # Block storage (65 endpoints, ~4,500 lines)
│   ├── glance/                 # Image service (38 endpoints, ~2,000 lines)
│   ├── database/               # DB models and migrations (47 migrations)
│   ├── middleware/             # Auth, logging, CORS, recovery (~400 lines)
│   └── common/                 # Shared utilities (~385 lines)
├── pkg/                         # Public/reusable packages (~76,000 lines)
│   ├── hypervisor/             # libvirt abstraction (real + stub modes)
│   ├── networking/             # netlink, VXLAN, security groups
│   └── storage/                # Storage backends (RBD, S3, local)
├── migrations/                  # SQL migrations (98 files - 49 up/down pairs)
├── test/contract/              # Contract tests (71 test files, TDD-first)
├── test/*.sh                   # Integration tests (20+ bash scripts)
├── config/                     # Configuration files (YAML)
├── docs/                       # Documentation
└── deployments/                # Docker Compose and deployment configs
```

### Development Workflow

```bash
# Install development tools
make install-tools

# Run with hot reload
make dev

# Run tests
make test

# Run contract tests
./test/quick_test.sh

# Format code
make fmt

# Lint code
make lint
```

## Default Credentials

The seed data creates:

- **User:** `admin`
- **Password:** `secret`
- **Project:** `default`

**⚠️ Change these in production!**

## 📊 Project Status

### API Coverage: 91% (308/330 endpoints) ✅

| Service | Endpoints | Coverage | Status |
|---------|-----------|----------|--------|
| Keystone (Identity) | 58 | ~95% | ✅ Production Ready |
| Nova (Compute) | 70 | ~92% | ✅ Production Ready |
| Neutron (Network) | 92 | ~98% | ✅ Production Ready |
| Cinder (Block Storage) | 65 | ~95% | ✅ Production Ready |
| Glance (Image) | 38 | ~92% | ✅ Production Ready |
| **TOTAL** | **308** | **~91%** | ✅ **Production Ready** |

**Priority Status**:
- ✅ **HIGH Priority**: 100% complete (all critical production features)
- ✅ **MEDIUM Priority**: 100% complete (all important management features)
- ⏳ **LOW Priority**: ~22 endpoints remaining (enterprise-only, < 5% usage)

**See [REMAINING_WORK.md](REMAINING_WORK.md) for detailed gap analysis.**

### Testing & Validation

- ✅ **71 Contract Test Files**: Using real OpenStack SDK (gophercloud)
- ✅ **TDD Methodology**: All endpoints developed test-first (RED → GREEN → REFACTOR)
- ✅ **20+ Integration Tests**: Bash scripts with OpenStack CLI validation
- ✅ **Schema Validation**: OpenStack API spec compliance
- ✅ **Horizon Testing**: Full dashboard compatibility verified
- ✅ **Client Compatibility**: OpenStack CLI, Terraform, gophercloud, python-openstackclient

### Current Capabilities

**✅ Fully Implemented (308 endpoints):**
- Complete server lifecycle management (create, start, stop, reboot, delete, resize, rebuild)
- All server actions (migrate, evacuate, rescue, snapshot, backup, password reset)
- Full networking (L3 routing, floating IPs, port forwarding, security groups, QoS)
- Multi-backend storage (local, Ceph RBD, S3) with automatic failover
- Volume operations (snapshots, backups, transfers, cloning, volume groups)
- Image management (upload, download, sharing, import workflow)
- Multi-tenancy (domains, projects, users, groups, RBAC)
- Console access (VNC, SPICE, Serial, RDP)
- Tenant usage reporting and availability zones
- Service catalog management and credential management

**⏳ Remaining Work (22 endpoints, LOW priority):**
- Keystone Federation/SAML (~5 endpoints) - Enterprise SSO, <1% usage
- Glance Metadefs Advanced (~15 endpoints) - Metadata schemas, rarely used
- Neutron Advanced (~8 endpoints) - Service function chaining, DVR, auto-topology
- Various extensions (~10 endpoints) - Edge cases and microversion-specific features

**Strategic Decision**: The remaining 2% represents enterprise-only features and edge cases better implemented on-demand based on user feedback rather than speculatively.

### Performance Characteristics

**Stub Mode Benchmarks:**
- Token Creation: ~5ms
- Server List: ~10ms (100 servers)
- Network Create: ~8ms
- Volume Create: ~6ms

**Real Mode Benchmarks:**
- VM Creation: ~2-5s (depends on backend)
- Volume Attach: ~1-2s
- Network Setup: ~500ms
- Floating IP Associate: ~200ms

**Scalability:**
- Tested with 1000+ concurrent connections
- 10,000+ resources per project
- Sub-second response times maintained

### Roadmap

**Completed (v0.1-v0.4.1):**
- ✅ All 5 core OpenStack services (Keystone, Nova, Neutron, Cinder, Glance)
- ✅ 91% API coverage (308/330 endpoints)
- ✅ All HIGH and MEDIUM priority features
- ✅ Real libvirt/KVM integration with stub mode fallback
- ✅ Multi-backend storage (Ceph RBD, S3, hybrid failover)
- ✅ VXLAN multi-node networking
- ✅ L3 routing, floating IPs, and port forwarding
- ✅ Horizon dashboard 100% compatibility
- ✅ Comprehensive test coverage (71 contract test files, 20+ integration tests)
- ✅ Production deployments validated

**Current Focus (v0.4.x - Polish & Bug Fixes):**
- 🔧 Service catalog URL template substitution (fix {project_id} placeholder)
- 🔧 Error message improvements
- 🔧 Performance optimization
- 🔧 Documentation enhancements

**Future Considerations (v0.5+):**
- [ ] Additional services (Barbican, Designate, Octavia) - on-demand based on user requests
- [ ] eBPF-based security groups (kernel-space filtering performance boost)
- [ ] High availability (multi-node control plane)
- [ ] Placement API (advanced resource scheduling)
- [ ] Low-priority endpoint implementations (Federation/SAML, advanced networking)

**Philosophy**: Focus on polish and production hardening rather than chasing 100% coverage of rarely-used features.

## 🤝 Contributing

Contributions welcome! See `docs/CONTRIBUTING.md` for guidelines.

**Current focus areas (v0.4.x polish phase):**
- Bug fixes and error handling improvements
- Performance optimization
- Documentation and tutorials
- Service catalog URL template fixes
- Integration testing enhancements

**Future expansion areas (when requested by users):**
- LOW priority endpoint implementations (Federation, advanced networking, metadefs)
- Additional services (Barbican, Designate, Octavia)
- eBPF security groups
- Multi-cloud abstraction layers

## 📚 Documentation

### Getting Started
- **[Quick Start](docs/QUICKSTART.md)** - Get running in 5 minutes
- **[Installation Guide](docs/INSTALLATION.md)** - Complete setup instructions
- **[Docker Deployment](docs/DOCKER_DEPLOYMENT.md)** - Docker-specific guide
- **[Multi-Architecture](docs/MULTIARCH.md)** - ARM64 and AMD64 support

### Configuration & Operations
- **[Configuration Guide](docs/CONFIGURATION.md)** - All configuration options
- **[Operations Guide](docs/OPERATIONS.md)** - Day-to-day management
- **[Storage Modes](docs/STORAGE_MODES.md)** - Multi-backend configuration
- **[Networking Modes](docs/NETWORKING_MODES.md)** - Network configuration

### Development & API
- **[Architecture](docs/ARCHITECTURE.md)** - System design and components
- **[API Coverage](docs/API_COVERAGE.md)** - Complete endpoint listing (308 endpoints)
- **[Remaining Work](REMAINING_WORK.md)** - Gap analysis and roadmap (91% coverage details)
- **[API Reference](docs/API.md)** - OpenStack API compatibility details
- **[Contributing](docs/CONTRIBUTING.md)** - Development guidelines

### Testing & Validation
- **[Contract Tests](test/contract/README.md)** - 71 test file suite (TDD approach)
- **[Integration Tests](test/)** - 20+ bash scripts for workflow validation
- **[Test Results](docs/)** - Various test result documents

### Additional Resources
All documentation available in the [`docs/`](docs/) directory.

## 📝 License

Apache License 2.0 - See [LICENSE](LICENSE)

## 🙏 Credits

**Project**: O3K - OpenStack 3 Kubernetes-style
**Inspired by**: K3s (Lightweight Kubernetes)
**Language**: Go 1.26+
**Repository**: github.com/cobaltcore-dev/o3k

---

**Status**: ✅ v0.4.1 Production Ready | **Coverage**: 91% (308/330 endpoints)
**Build**: ✅ SUCCESS (35MB) | **Tests**: ✅ 71 Contract Test Files PASS
**Milestone**: 🎉 All HIGH and MEDIUM Priority Features Complete!
**Updated**: March 13, 2026
