# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Ultimate Project System

**IMPORTANT**: This project uses the Ultimate Project System (Beastmode × Spec-Kit synthesis).

### Workflow & Knowledge Hierarchy
- **L0 (Autoload)**: Read `.beastmode/BEASTMODE.md` at the start of every session
- **L1 (Phase Context)**: `.beastmode/context/{PHASE}.md` - Load at phase start
- **L2 (Domain Details)**: `.beastmode/context/{phase}/{domain}.md` - On-demand
- **L3 (Specs & Artifacts)**: `specs/NNN-*/` - Feature specifications

### Development Workflow
```
specify → plan → tasks → implement → validate → release
```
Each phase: **prime → execute → validate → checkpoint → retro**

### The Nine Articles (Constitution)
Immutable principles in `memory/constitution.md`:
1. **Library-First** - Features begin as standalone libraries
2. **CLI Interface** - All libraries expose CLI functionality
3. **Test-First** - TDD mandatory (NON-NEGOTIABLE: tests → fail → implement)
4. **Integration Testing** - Real dependencies over mocks
5. **Observability** - Everything inspectable
6. **Versioning** - Semantic versioning
7. **Simplicity** - ≤3 projects, YAGNI principles
8. **Anti-Abstraction** - Use frameworks directly
9. **Integration-First** - Contract tests before implementation

### Available Commands
- `/constitution` - Establish project principles
- `/specify [desc]` - Create feature specification
- `/plan [stack]` - Create implementation plan
- `/tasks` - Generate task list
- `/implement [--parallel]` - Execute with parallel agents (swarm mode)
- `/validate` - Run validation gates
- `/release` - Release feature

### Swarm Mode
Parallel execution enabled for:
- Research: Multiple topics simultaneously
- Implementation: Parallel-safe waves (file isolation checked)
- Retro: Context + Meta walkers together

### Persona
When working on this project, adopt the **deadpan minimalist** persona:
- Short sentences, maximum understatement
- Competent, slightly annoyed at the work (never at the user)

## Project Overview

O3K is a lightweight OpenStack implementation in Go, inspired by K3s. It provides a single ~35MB binary that implements all five core OpenStack services (Keystone, Nova, Neutron, Cinder, Glance) with 100% Terraform compatibility.

**Core Philosophy:**
- **Terraform Compatibility First**: Users can use existing Terraform OpenStack provider scripts unchanged
- **UI/CLI Compatibility**: Horizon dashboard and OpenStack CLI work identically
- **Drop-in Replacement**: Zero modifications needed to migrate from OpenStack to O3K
- **Synchronous Operations**: No message queues - operations complete before API returns
- **Fail-Fast Design**: External dependency failures return immediately (< 1 second timeouts)
- **Multi-mode Support**: Each service supports stub mode (development) and real mode (production)

## Build and Development Commands

### Quick Start with Docker Compose

```bash
# Start all services (PostgreSQL + O3K)
docker compose -f deployments/docker-compose.yml up -d

# Configure environment
source ~/.o3k-env         # Sets OS_AUTH_URL, OS_USERNAME, etc.

# Test it works
openstack token issue
```

### Building and Running

```bash
# Build the main binary
make build                    # Outputs to bin/o3k

# Run with configuration
make run                      # Builds and starts with config/o3k.yaml

# Development with hot reload (requires air)
make dev                      # Auto-restarts on code changes

# Clean build artifacts
make clean
```

### Testing

```bash
# Unit tests
make test                     # Runs all Go unit tests
go test ./internal/nova/...   # Test specific package

# Integration tests
./scripts/test-all.sh                 # Comprehensive test suite
./scripts/test-keystone.sh            # Keystone-specific tests
./test/quick_test.sh                  # Fast integration test suite
./test/integration_test.sh            # Full integration test suite

# Specific feature tests
./test/horizon_compat_test.sh         # Horizon dashboard compatibility
./test/volume_attach_test.sh          # Volume attachment workflow
./test/vxlan_multinode_test.sh        # Multi-node VXLAN networking
```

### Database Operations

```bash
# Start PostgreSQL (Docker)
make db-up                    # Starts postgres:17 container

# Run migrations
make migrate                  # Applies all pending migrations

# Stop PostgreSQL
make db-down                  # Stops and removes container
```

### Code Quality

```bash
# Format code
make fmt                      # Runs go fmt

# Lint code
make lint                     # Runs golangci-lint

# Install development tools
make install-tools            # Installs air, golangci-lint, migrate
```

## Architecture Overview

### Service Structure

O3K runs as a single process (`cmd/o3k/main.go`) that starts six HTTP servers on different ports:
- **Keystone** (35357): Identity service, JWT-based authentication
- **Nova** (8774): Compute service, VM lifecycle via libvirt
- **Neutron** (9696): Network service, namespace isolation via netlink
- **Cinder** (8776): Block storage, multi-backend support (local/RBD/S3)
- **Glance** (9292): Image service, multi-backend with hybrid failover
- **Metadata** (8775): EC2-compatible metadata service (no auth)

Each service is initialized in `main.go` with its configuration and shares:
- **Database connection pool**: Centralized PostgreSQL access
- **Auth middleware**: Shared JWT token validation via `keystone.AuthService`
- **Logging middleware**: Structured JSON logging

### Code Organization

```
internal/                    # Private packages
├── keystone/               # Identity - JWT auth, service catalog
├── nova/                   # Compute - VM lifecycle, flavors, keypairs
├── neutron/                # Network - networks, subnets, ports, security groups
├── cinder/                 # Block storage - volumes, snapshots
├── glance/                 # Images - metadata, multi-backend storage
├── database/               # DB models, migrations (models.go, migrate.go)
├── middleware/             # Auth, logging, CORS, recovery
├── common/                 # Config loading, error handling
├── compute/                # Node registry for multi-node coordination
└── metadata/               # EC2 metadata service

pkg/                        # Public/reusable packages
├── hypervisor/             # libvirt abstraction (stub + real modes)
├── networking/             # netlink, VXLAN, security groups
└── storage/                # Storage backends (Ceph RBD, S3, local)
```

### Key Architectural Patterns

**Service Mode Pattern**: Each service (Nova, Neutron, Cinder, Glance) supports multiple modes:
- `stub`: Returns fake data, no external dependencies (default for development)
- `real`: Full implementation with external dependencies (libvirt, Ceph, etc.)
- Mode is configured via YAML (`libvirt_mode`, `networking_mode`, `storage_mode`)
- Check mode via `libvirtMode` or `networkingMode` fields in service structs

**Multi-backend Storage Pattern** (Cinder/Glance):
- Storage modes specified as comma-separated strings: `"local"`, `"rbd"`, `"s3"`, `"local,rbd"`, `"local,s3"`, `"rbd,s3"`
- First backend is primary, subsequent are fallback
- Implemented in `pkg/storage/` with `ImageStore` and backend-specific implementations

**Synchronous Operations**:
- All API operations complete before returning (no async state machines)
- VM creation: `DomainDefineXML()` + `DomainCreate()` in single API call
- Database updates happen immediately, no queues

**JWT Authentication**:
- Tokens generated by Keystone (`internal/keystone/auth.go`)
- All services except metadata use `middleware.AuthMiddleware()`
- Token contains `user_id`, `project_id`, `roles` for authorization
- No token database - tokens are stateless (HMAC-SHA256 signed)

**Project Isolation**:
- All resources scoped by `project_id` from JWT token
- Network namespaces per project for network isolation
- Database queries auto-filter by `project_id`

## Operating Modes

### Stub Mode (Development - Default)
Safe for macOS and non-Linux systems. No external dependencies required.

**Nova stub mode**:
- Returns fake VM instances, no actual VMs created
- Check: `svc.libvirtMode == "stub"` or `svc.vmManager == nil`
- VMs tracked in database only

**Neutron stub mode**:
- Returns network objects without creating namespaces/bridges
- No iptables rules or actual networking

**Cinder/Glance stub mode**:
- Tracks volumes/images in database only
- No actual storage operations

### Real Mode (Production)
Requires Linux with appropriate dependencies.

**Nova real mode** (`libvirt_mode: real`):
- Requires libvirt + KVM on Linux
- Creates actual VMs via `github.com/digitalocean/go-libvirt`
- XML templates in `pkg/hypervisor/xml_template.go`

**Neutron real mode** (`networking_mode: iptables` or `ebpf`):
- Requires Linux with network namespaces
- Uses `github.com/vishvananda/netlink` for namespace/bridge creation
- iptables via `github.com/coreos/go-iptables` for security groups

**Cinder/Glance real mode** (`storage_mode: local`, `rbd`, `s3`, or hybrid):
- `local`: Host filesystem storage
- `rbd`: Ceph RBD via `github.com/ceph/go-ceph`
- `s3`: AWS S3/MinIO via `github.com/aws/aws-sdk-go-v2`

## Database Schema

PostgreSQL with 15 tables. Key tables:

- **users**: User credentials (bcrypt hashed passwords)
- **projects**: Projects/tenants
- **roles**, **role_assignments**: RBAC
- **instances**: VM instances (linked to flavors, images)
- **flavors**: VM flavor definitions
- **networks**, **subnets**, **ports**: Network topology
- **volumes**: Block storage volumes
- **images**: Image metadata (data in storage backend)
- **keypairs**: SSH public keys
- **compute_nodes**: Multi-node registry for VXLAN coordination

Migrations in `migrations/` directory, applied via `golang-migrate/migrate`.

## Testing Guidelines

### Unit Tests
- Located alongside source files as `*_test.go`
- Test database operations with mock connections or test DB
- Test mode detection logic (stub vs real)

### Integration Tests
All integration tests are bash scripts in `test/` directory that:
1. Start O3K in background
2. Use OpenStack CLI (`openstack` command) to test workflows
3. Verify responses with `jq` for JSON parsing
4. Clean up resources

**Common test patterns:**
```bash
# Authenticate
openstack token issue

# Test resource lifecycle
openstack server create --flavor m1.small --image cirros test-vm
openstack server show test-vm -f json | jq -r '.status'
openstack server delete test-vm
```

### Running Tests
- Integration tests require O3K to be running: `docker compose up -d` or `make run`
- Tests use environment variables from `.env` or `~/.o3k-env`
- Quick validation: `./test/quick_test.sh` (runs in ~30 seconds)

### Test-First Development (Constitution Article III)
When implementing new features:
1. **Write tests first** - Create `*_test.go` files
2. **Get approval** - Review test strategy
3. **Confirm RED** - Tests must fail initially
4. **Implement** - Write code to make tests pass (GREEN)
5. **Refactor** - Clean up while keeping tests green

This is **NON-NEGOTIABLE** per the project constitution.

## Configuration

Configuration loaded from `config/o3k.yaml` (or via `--config` flag).

**Critical settings:**
- `database.url`: PostgreSQL connection string
- `keystone.jwt_secret`: MUST change in production (security critical)
- `nova.libvirt_mode`: `stub` (dev) or `real` (prod)
- `neutron.networking_mode`: `stub`, `iptables`, or `ebpf`
- `cinder.storage_mode`: `stub`, `local`, `rbd`, `s3`, or hybrid (comma-separated)
- `glance.storage_mode`: `stub`, `local`, `rbd`, `s3`, or hybrid (comma-separated)

**Multi-node VXLAN** (advanced):
- Set `neutron.vxlan_enabled: true`
- Configure `compute.node_id` and `compute.tunnel_ip`
- Enables cross-node VM networking via VXLAN overlay

### Default Credentials
Seed data creates:
- **User**: `admin`
- **Password**: `secret`
- **Project**: `default`

*Change these in production environments.*

## Common Patterns and Idioms

### Commit Messages
Follow conventional commits format:
```
<type>(<scope>): <description>

[optional body]
```

**Types**: `feat`, `fix`, `refactor`, `docs`, `test`, `chore`

**Examples**:
- `feat(glance): add S3 backend support for image storage`
- `fix(nova): correct power_state in server detail response`
- `refactor(cinder): simplify volume attachment logic`

### Error Handling
```go
// Always wrap errors with context
if err != nil {
    return fmt.Errorf("failed to create volume: %w", err)
}

// Use helper functions for HTTP errors (internal/common/error_helpers.go)
return common.HandleError(c, err)
```

### Database Queries
```go
// Always use context and prepared statements
ctx := c.Request.Context()
err := database.DB.QueryRow(ctx,
    "SELECT id FROM instances WHERE project_id = $1",
    projectID,
).Scan(&instanceID)
```

### Mode Detection
```go
// Nova: Check libvirtMode or vmManager
if svc.libvirtMode == "stub" || svc.vmManager == nil {
    // Stub mode: return fake data
}

// Neutron: Check networkingMode
if svc.networkingMode == "stub" {
    // Stub mode: skip netlink operations
}
```

### Microversion Negotiation (Nova)
Nova supports OpenStack microversions. Check request header:
```go
requestedVersion := c.GetHeader("OpenStack-API-Version")
if requestedVersion == "" {
    requestedVersion = c.GetHeader("X-OpenStack-Nova-API-Version")
}
```

## Important Notes

### Security Considerations
- Never commit production JWT secrets
- Database passwords should use environment variables in production
- Token TTL default is 24h (configurable)

### Backwards Compatibility
- All API endpoints must maintain OpenStack API compatibility
- Breaking API changes require OpenStack microversion bumps
- Horizon dashboard compatibility is tested in `test/horizon_compat_test.sh`

### Performance
- Database connection pool default: 20 connections
- libvirt timeouts: 1 second for fail-fast
- Ceph timeouts: 1 second for fail-fast

### Platform Support
- **macOS**: Stub mode only (no libvirt/KVM)
- **Linux**: All modes supported
- **Windows**: Not tested, likely stub mode only

## Troubleshooting Development Issues

**"Failed to connect to libvirt"**: Use stub mode on macOS or ensure libvirt is running on Linux
**"Network namespace not found"**: Requires root/sudo on Linux, use stub mode otherwise
**"Database connection failed"**: Check PostgreSQL is running and connection string is correct
**"Token validation failed"**: Ensure `jwt_secret` matches between Keystone and other services

## Active Technologies
- Go 1.26 + Gin (HTTP framework), pgx (PostgreSQL driver), gophercloud (contract testing), go-libvirt (hypervisor), netlink (networking) (002-horizon-full-compatibility)
- PostgreSQL 17 (primary database) (002-horizon-full-compatibility)

## Recent Changes
- 002-horizon-full-compatibility: Added Go 1.26 + Gin (HTTP framework), pgx (PostgreSQL driver), gophercloud (contract testing), go-libvirt (hypervisor), netlink (networking)

## Skill routing

When the user's request matches an available skill, ALWAYS invoke it using the Skill
tool as your FIRST action. Do NOT answer directly, do NOT use other tools first.
The skill has specialized workflows that produce better results than ad-hoc answers.

Key routing rules:
- Product ideas, "is this worth building", brainstorming → invoke office-hours
- Bugs, errors, "why is this broken", 500 errors → invoke investigate
- Ship, deploy, push, create PR → invoke ship
- QA, test the site, find bugs → invoke qa
- Code review, check my diff → invoke review
- Update docs after shipping → invoke document-release
- Weekly retro → invoke retro
- Design system, brand → invoke design-consultation
- Visual audit, design polish → invoke design-review
- Architecture review → invoke plan-eng-review
- Save progress, checkpoint, resume → invoke checkpoint
- Code quality, health check → invoke health
