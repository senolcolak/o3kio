# Phase 4: Security, External Connectivity & Production Readiness

## Executive Summary

Phase 4 focuses on transforming O3K from a feature-complete development platform into a production-ready OpenStack-compatible cloud. The emphasis is on **security, external network connectivity, operational tooling, and Horizon dashboard integration**.

## Phase 3 Review & Current State

### ✅ What We Have (Phases 0-3):
- **Identity (Keystone)**: Full v3 auth with JWT tokens, service catalog
- **Compute (Nova)**: VM lifecycle, flavors, volume attach, console access, advanced actions (suspend/shelve/resize), interface hot-plug
- **Network (Neutron)**: Networks, subnets, ports, routers, VXLAN overlay (multi-node)
- **Storage (Cinder)**: Volumes, snapshots, volume types (local/Ceph modes)
- **Images (Glance)**: Image upload/download (local/S3/Ceph backends)
- **Metadata Service**: EC2/OpenStack-compatible metadata for cloud-init

### 🔍 What's Missing for Production:
1. **Security Groups** - No firewall rules or network isolation
2. **Floating IPs** - Partially implemented but not tested/integrated
3. **SSH Key Injection** - Keypairs exist but not injected into VMs
4. **Quota Management** - No resource limits or enforcement
5. **Logging & Observability** - Basic logging only, no metrics/tracing
6. **Database Migrations** - Currently skipped, need proper migration system
7. **Horizon Integration** - Many endpoints work but not fully tested
8. **Error Handling** - Inconsistent error responses across services

### 📊 Test Coverage:
- **Phase 3 Tests**: 19/24 passing (79%)
- **Known Issues**: Volume attach timing, GetServer token validation
- **Missing Tests**: Security groups, floating IPs, quota enforcement

---

## Phase 4 Goals

### Primary Objectives:
1. ✅ **Security Groups with iptables** - Network-level firewall rules
2. ✅ **Floating IP Integration** - External network access for VMs
3. ✅ **SSH Key Injection** - Automatic key injection via cloud-init
4. ✅ **Quota Management** - Resource limits and enforcement
5. ✅ **Production Logging** - Structured logging with levels and context

### Secondary Objectives:
6. ⚠️ **Database Migration System** - Proper schema versioning
7. ⚠️ **Error Handling Standardization** - Consistent OpenStack error format
8. ⚠️ **Horizon Dashboard Testing** - Full end-to-end validation
9. ⚠️ **Performance Optimization** - Connection pooling, caching

### Stretch Goals (If Time Permits):
10. 🔮 **Live Migration** - VM migration between compute nodes
11. 🔮 **Placement API** - Resource scheduling and allocation
12. 🔮 **High Availability** - Multi-controller setup

---

## Phase 4 Feature Breakdown

### Feature 1: Security Groups with iptables
**Priority:** CRITICAL
**Estimated Effort:** 3-4 days
**OpenStack API:** Neutron v2.0 `/v2.0/security-groups`, `/v2.0/security-group-rules`

#### Implementation Details:

**Database Schema (Already exists in migrations/001):**
```sql
CREATE TABLE security_groups (
    id UUID PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    project_id UUID REFERENCES projects(id),
    description TEXT
);

CREATE TABLE security_group_rules (
    id UUID PRIMARY KEY,
    security_group_id UUID REFERENCES security_groups(id),
    direction VARCHAR(10), -- 'ingress' or 'egress'
    ethertype VARCHAR(10), -- 'IPv4' or 'IPv6'
    protocol VARCHAR(10),  -- 'tcp', 'udp', 'icmp', or protocol number
    port_range_min INTEGER,
    port_range_max INTEGER,
    remote_ip_prefix VARCHAR(50),
    remote_group_id UUID
);
```

**Neutron API Endpoints:**
- `GET /v2.0/security-groups` - List security groups
- `POST /v2.0/security-groups` - Create security group
- `GET /v2.0/security-groups/{id}` - Show security group
- `DELETE /v2.0/security-groups/{id}` - Delete security group
- `POST /v2.0/security-group-rules` - Create rule
- `DELETE /v2.0/security-group-rules/{id}` - Delete rule

**iptables Integration:**
- Use `github.com/coreos/go-iptables` library
- Create chains per security group: `o3k-sg-<group_id>`
- Apply rules to TAP devices in network namespaces
- Default policy: DROP all, explicit ACCEPT rules
- Handle RELATED,ESTABLISHED connections

**Example iptables Rules:**
```bash
# Create chain for security group
iptables -N o3k-sg-default

# Allow SSH (rule from security_group_rules)
iptables -A o3k-sg-default -p tcp --dport 22 -j ACCEPT

# Allow established connections
iptables -A o3k-sg-default -m state --state RELATED,ESTABLISHED -j ACCEPT

# Apply to port (TAP device)
iptables -A FORWARD -i tap-<port_id> -j o3k-sg-default
```

**Files to Create/Modify:**
- `internal/neutron/security_groups.go` - CRUD handlers
- `pkg/networking/iptables.go` - iptables abstraction
- `test/security_groups_test.sh` - Comprehensive test

**Default Security Group:**
- Auto-create "default" security group per project
- Rules: Allow all egress, deny all ingress

---

### Feature 2: Floating IP Integration & NAT
**Priority:** HIGH
**Estimated Effort:** 2-3 days
**OpenStack API:** Neutron v2.0 `/v2.0/floatingips`

#### Current State:
- ✅ Database schema exists
- ✅ CRUD handlers implemented (`internal/neutron/floatingip.go`)
- ❌ NAT configuration NOT implemented
- ❌ Router integration NOT tested
- ❌ External network NOT configured

#### What's Needed:

**1. External Network Setup:**
```go
// Create external network with special flag
CreateNetwork(name: "external", external: true, shared: true)
// This network represents the physical network with public IPs
```

**2. NAT Configuration with iptables:**
```bash
# SNAT for VM egress traffic (uses router gateway IP)
iptables -t nat -A POSTROUTING -s <vm_private_ip> -o eth0 -j SNAT --to-source <floating_ip>

# DNAT for ingress traffic to VM
iptables -t nat -A PREROUTING -d <floating_ip> -j DNAT --to-destination <vm_private_ip>

# Forward rules
iptables -A FORWARD -d <vm_private_ip> -j ACCEPT
iptables -A FORWARD -s <vm_private_ip> -j ACCEPT
```

**3. Router Gateway Integration:**
- Associate router with external network
- Allocate floating IPs from external network pool
- Configure SNAT for router (default gateway for all VMs)

**Files to Modify:**
- `internal/neutron/floatingip.go` - Add NAT configuration
- `internal/neutron/router.go` - Add gateway attachment
- `pkg/networking/nat.go` - NEW: NAT/SNAT/DNAT helpers
- `test/floatingip_test.sh` - NEW: Full floating IP test

**Test Scenario:**
1. Create external network (10.0.0.0/24)
2. Create private network (192.168.1.0/24)
3. Create router, attach to both networks
4. Create VM on private network
5. Allocate floating IP from external network
6. Associate floating IP with VM's port
7. Verify VM can reach external network (egress)
8. Verify external hosts can reach VM (ingress via floating IP)

---

### Feature 3: SSH Key Injection via cloud-init
**Priority:** MEDIUM
**Estimated Effort:** 1-2 days
**OpenStack API:** Nova v2.1 `/v2.1/os-keypairs`

#### Current State:
- ✅ Keypair CRUD stubs exist (`internal/nova/handlers.go`)
- ❌ Not stored in database
- ❌ Not injected into VMs
- ❌ Not passed to metadata service

#### Implementation:

**1. Database Schema:**
```sql
CREATE TABLE IF NOT EXISTS keypairs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id),
    name VARCHAR(255) NOT NULL,
    public_key TEXT NOT NULL,
    fingerprint VARCHAR(255),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, name)
);
```

**2. Nova Keypair API:**
```go
// POST /v2.1/os-keypairs
type CreateKeypairRequest struct {
    Keypair struct {
        Name      string `json:"name" binding:"required"`
        PublicKey string `json:"public_key"` // Optional, generate if not provided
    } `json:"keypair"`
}

// Generate SSH key pair if not provided
func generateSSHKeyPair() (publicKey, privateKey string, err error) {
    // Use crypto/rsa to generate 2048-bit key
    // Return OpenSSH format public key
}
```

**3. Metadata Service Integration:**
- Modify `internal/metadata/service.go` to query keypairs table
- Return in `public_keys` field of metadata

**4. VM Creation Flow:**
```go
// When creating server with key_name:
POST /v2.1/servers
{
    "server": {
        "name": "test-vm",
        "flavorRef": "...",
        "key_name": "my-key"  // <-- Reference keypair
    }
}

// 1. Lookup keypair by user_id + name
// 2. Store key reference in instance_metadata
// 3. cloud-init will fetch from metadata service and inject
```

**Files to Create/Modify:**
- `internal/nova/keypairs.go` - NEW: Full keypair implementation
- `internal/metadata/service.go` - Modify to include SSH keys
- `test/keypair_test.sh` - NEW: Keypair creation and injection test

**Test Scenario:**
1. Create keypair via API (auto-generate or import)
2. Create VM with `key_name` parameter
3. Query metadata service as VM (X-Instance-ID header)
4. Verify public key appears in metadata
5. Boot real VM with cloud-init, verify SSH works

---

### Feature 4: Quota Management & Enforcement
**Priority:** MEDIUM
**Estimated Effort:** 2 days
**OpenStack API:** Nova v2.1 `/v2.1/os-quota-sets`

#### Implementation:

**Database Schema:**
```sql
CREATE TABLE IF NOT EXISTS quotas (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id),
    resource VARCHAR(50) NOT NULL, -- 'instances', 'cores', 'ram', 'volumes', etc.
    limit INTEGER NOT NULL,
    UNIQUE(project_id, resource)
);

-- Default quotas (inserted on project creation)
-- instances: 10
-- cores: 20
-- ram: 51200 (50 GB)
-- volumes: 10
-- volume_gigabytes: 1000
-- floatingip: 10
-- networks: 10
```

**Nova Quota API:**
```go
// GET /v2.1/os-quota-sets/{project_id}
func GetQuotas(c *gin.Context) {
    // Return current limits and usage
    return {
        "quota_set": {
            "instances": 10,
            "cores": 20,
            "ram": 51200,
            "instances_used": 3,
            "cores_used": 6,
            "ram_used": 8192
        }
    }
}

// PUT /v2.1/os-quota-sets/{project_id}
func UpdateQuotas(c *gin.Context) {
    // Admin-only: Update quota limits
}
```

**Quota Enforcement:**
- Check quota BEFORE creating resource
- Return HTTP 413 (Quota Exceeded) if limit reached
- Increment usage atomically in transaction

**Example Enforcement:**
```go
func (svc *Service) CreateServer(c *gin.Context) {
    projectID := c.GetString("project_id")

    // Check instance quota
    var instanceCount, instanceLimit int
    err := database.DB.QueryRow(ctx,
        "SELECT COUNT(*), (SELECT limit FROM quotas WHERE project_id = $1 AND resource = 'instances') FROM instances WHERE project_id = $1",
        projectID,
    ).Scan(&instanceCount, &instanceLimit)

    if instanceCount >= instanceLimit {
        c.JSON(413, gin.H{"error": "Quota exceeded for instances"})
        return
    }

    // Proceed with creation...
}
```

**Files to Create/Modify:**
- `internal/nova/quotas.go` - NEW: Quota CRUD and enforcement
- `internal/neutron/quotas.go` - NEW: Network quotas
- `internal/cinder/quotas.go` - NEW: Volume quotas
- `internal/middleware/quota.go` - NEW: Quota enforcement middleware
- `test/quota_test.sh` - NEW: Quota enforcement test

---

### Feature 5: Production Logging & Observability
**Priority:** MEDIUM
**Estimated Effort:** 1-2 days

#### Current State:
- ✅ Basic gin logging middleware
- ❌ No structured logging
- ❌ No log levels (DEBUG/INFO/WARN/ERROR)
- ❌ No request tracing
- ❌ No metrics/monitoring

#### Implementation:

**1. Structured Logging with zerolog:**
```go
import "github.com/rs/zerolog/log"

// Replace all fmt.Printf with structured logs
log.Info().
    Str("instance_id", instanceID).
    Str("action", "create").
    Msg("Creating VM instance")

log.Error().
    Err(err).
    Str("instance_id", instanceID).
    Msg("Failed to create VM")
```

**2. Request Tracing:**
```go
// Middleware to add request_id to context
func TracingMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        requestID := uuid.New().String()
        c.Set("request_id", requestID)

        log := log.With().Str("request_id", requestID).Logger()
        c.Set("logger", &log)

        c.Next()
    }
}
```

**3. Configuration:**
```yaml
logging:
  level: info  # debug, info, warn, error
  format: json # json or console
  output: stdout # stdout or file path
```

**Files to Modify:**
- `internal/middleware/logging.go` - Enhance with structured logging
- `internal/common/config.go` - Add logging configuration
- All service files - Replace fmt.Printf with log.*

---

### Feature 6: Database Migration System
**Priority:** LOW (but important)
**Estimated Effort:** 1 day

#### Current State:
- ✅ Migration files exist (migrations/*.sql)
- ❌ Migration runner commented out
- ❌ Migrations manually applied, not tracked
- ❌ No rollback capability

#### Implementation:

**Use golang-migrate library:**
```go
import "github.com/golang-migrate/migrate/v4"

func RunMigrations(dbURL, migrationsPath string) error {
    m, err := migrate.New(
        fmt.Sprintf("file://%s", migrationsPath),
        dbURL,
    )
    if err != nil {
        return err
    }

    if err := m.Up(); err != nil && err != migrate.ErrNoChange {
        return err
    }

    version, dirty, err := m.Version()
    log.Info().
        Uint("version", version).
        Bool("dirty", dirty).
        Msg("Database migration complete")

    return nil
}
```

**Enable in main.go:**
```go
// Uncomment migration runner
if err := database.RunMigrations(cfg.Database.URL, *migrationsPath); err != nil {
    log.Fatal().Err(err).Msg("Failed to run migrations")
}
```

**Add Migration Commands:**
```bash
# Create new migration
o3k migrate create add_column_to_instances

# Apply migrations
o3k migrate up

# Rollback
o3k migrate down 1
```

---

### Feature 7: Horizon Dashboard Integration Testing
**Priority:** LOW
**Estimated Effort:** 2-3 days

#### Test Matrix:

**Authentication:**
- ✅ Login with username/password
- ✅ Token-based session management
- ✅ Project selection

**Compute (Nova):**
- ✅ List instances
- ⚠️ Launch instance (needs full test)
- ⚠️ Instance actions (reboot/suspend/shelve)
- ❌ Console access (VNC proxy integration)
- ❌ Volume attach from UI

**Network (Neutron):**
- ✅ List networks
- ✅ Create network
- ❌ Security groups UI
- ❌ Floating IP association UI

**Storage (Cinder):**
- ✅ List volumes
- ✅ Create volume
- ❌ Volume snapshots
- ❌ Volume types

**Setup Horizon:**
```bash
# Install Horizon
pip install openstack-dashboard

# Configure local_settings.py
OPENSTACK_KEYSTONE_URL = "http://localhost:35357/v3"
OPENSTACK_API_VERSIONS = {
    "identity": 3,
    "compute": 2,
}

# Run Horizon
python manage.py runserver 0.0.0.0:8080
```

**Test Checklist:**
- [ ] Login works without errors
- [ ] All tabs load (Compute, Network, Volumes)
- [ ] Instance launch succeeds
- [ ] Security group creation works
- [ ] Floating IP allocation works
- [ ] Volume attachment works

---

## Implementation Order (Recommended)

### Week 1: Security & External Connectivity
**Days 1-2:** Security Groups with iptables
**Days 3-4:** Floating IP NAT integration
**Day 5:** SSH key injection

### Week 2: Production Readiness
**Days 6-7:** Quota management
**Day 8:** Production logging
**Day 9:** Database migrations
**Day 10:** Horizon integration testing

---

## Success Criteria for Phase 4

### Must Have (P0):
- ✅ Security groups with iptables CRUD and enforcement
- ✅ Floating IPs with working NAT
- ✅ SSH keys automatically injected into VMs
- ✅ Quota limits enforced across all services
- ✅ Structured logging with request tracing

### Should Have (P1):
- ✅ Database migrations working properly
- ✅ Consistent error handling (OpenStack error format)
- ✅ Basic Horizon integration (login + instance launch)

### Nice to Have (P2):
- ⚠️ Full Horizon testing (all tabs working)
- ⚠️ Performance benchmarks
- ⚠️ Load testing

---

## Test Suite Additions

### New Test Scripts:
1. `test/security_groups_test.sh` - Security group CRUD and rule enforcement
2. `test/floatingip_test.sh` - Floating IP allocation and NAT
3. `test/keypair_test.sh` - SSH key creation and injection
4. `test/quota_test.sh` - Quota enforcement (creation blocked when exceeded)
5. `test/horizon_integration_test.sh` - Automated Horizon UI tests (using Selenium/Playwright)

### Updated Test Scripts:
- `test/phase4_test_suite.sh` - Comprehensive Phase 4 test runner

---

## Risk Assessment

### High Risk:
1. **iptables Complexity** - Network namespace isolation + iptables rules can be fragile
   - Mitigation: Extensive testing, fallback to permissive mode on failure

2. **NAT Configuration** - Floating IP NAT requires kernel routing configuration
   - Mitigation: Test in isolated environment first, document prerequisites

3. **Horizon Compatibility** - Horizon may have unexpected API requirements
   - Mitigation: Test incrementally, check Horizon logs for errors

### Medium Risk:
4. **Quota Race Conditions** - Concurrent requests may bypass quota checks
   - Mitigation: Use database transactions with locks

5. **Performance Impact** - iptables rule count may affect packet processing
   - Mitigation: Optimize rule ordering, use ipsets for large rule sets

---

## Dependencies

### External Libraries (New):
```go
require (
    github.com/coreos/go-iptables v0.8.0          // Already in go.mod
    github.com/rs/zerolog v1.33.0                  // NEW: Structured logging
    github.com/golang-migrate/migrate/v4 v4.18.1   // Already in go.mod
    golang.org/x/crypto v0.31.0                    // NEW: SSH key generation
)
```

### System Requirements:
- **iptables** installed and configured
- **ip netns** support (kernel namespaces)
- **Kernel IP forwarding** enabled (`net.ipv4.ip_forward=1`)
- **NAT support** in kernel (`nf_nat`, `iptable_nat` modules)

---

## Documentation Needs

### Admin Documentation:
- Security group rule syntax and examples
- Floating IP pool configuration
- Quota tuning and monitoring
- iptables troubleshooting guide

### Developer Documentation:
- API endpoint reference (auto-generated from code)
- Database schema documentation
- Testing guide for new features

---

## Post-Phase 4 State

After Phase 4 completion, O3K will be:

✅ **Production-Ready:**
- Security groups provide network isolation
- Floating IPs enable external connectivity
- Quotas prevent resource exhaustion
- Structured logging enables troubleshooting

✅ **Horizon-Compatible:**
- All major Horizon workflows functional
- Instance launch/manage works end-to-end
- Network configuration via UI

✅ **Operationally Mature:**
- Database migrations automated
- Consistent error handling
- Request tracing for debugging

🚀 **Ready for Phase 5:** Multi-node HA, Live Migration, Placement API

---

## Appendix: OpenStack API Coverage

### After Phase 4:

**Keystone (Identity):** ~90% coverage
- ✅ Auth tokens, projects, users, roles
- ✅ Service catalog
- ❌ Federation (SAML/OIDC)

**Nova (Compute):** ~85% coverage
- ✅ VM lifecycle, flavors, actions
- ✅ Volume attach, console, metadata
- ✅ Interface hot-plug
- ❌ Live migration
- ❌ Placement API

**Neutron (Network):** ~80% coverage
- ✅ Networks, subnets, ports, routers
- ✅ VXLAN overlay
- ✅ Security groups (Phase 4)
- ✅ Floating IPs (Phase 4)
- ❌ LBaaS (load balancers)
- ❌ VPNaaS

**Cinder (Storage):** ~70% coverage
- ✅ Volumes, snapshots, types
- ✅ Volume attach/detach
- ❌ Volume migration
- ❌ Backup/restore

**Glance (Images):** ~60% coverage
- ✅ Image upload/download
- ✅ Multi-backend (local/S3/Ceph)
- ❌ Image sharing/visibility
- ❌ Image import (glance-direct)

---

**Total Estimated Effort:** 10-12 days (2 weeks)
**Phase 4 Status:** PLANNING COMPLETE - READY TO START
