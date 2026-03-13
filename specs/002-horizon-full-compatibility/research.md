# Research: Horizon 100% Compatibility

**Feature**: OpenStack Horizon Full Compatibility
**Date**: 2026-03-13
**Plan**: [plan.md](./plan.md)

## Executive Summary

O3K currently has **91% OpenStack API coverage (308/330 endpoints)** with 19/19 existing Horizon compatibility tests passing. Research identifies **7 critical Nova server actions** requiring completion for full Horizon dashboard functionality, plus documentation enhancements.

**Key Finding**: Console access (VNC/SPICE) completed in Sprint 58-59, service catalog complete in Sprint 62-63. Primary gap is incomplete Nova server action implementations with security issue in `os-resetState`.

## 1. Horizon Critical Endpoints Analysis

### 1.1 Current Implementation Status

| Service | Implemented | Missing | Coverage | Horizon Critical |
|---------|-------------|---------|----------|------------------|
| Nova    | 57/67       | 10      | 85%      | 7 actions incomplete |
| Neutron | 64/67       | 3       | 95%      | Topology validation needed |
| Cinder  | 67/67       | 0       | 100%     | ✅ Complete |
| Glance  | 26/26       | 0       | 100%     | ✅ Complete |
| Keystone| 45/50       | 5       | 90%      | ✅ Complete (Federation low priority) |

### 1.2 Horizon API Usage Patterns

Based on codebase analysis of `/Users/I761222/git/o3k/docs/sprints/SPRINT_56-57_GAP_ANALYSIS.md`:

**Dashboard Load Sequence**:
1. POST `/v3/auth/tokens` (Keystone) - ✅ Complete
2. GET `/v2.1/servers/detail` (Nova) - ✅ Complete
3. GET `/v2.0/networks` (Neutron) - ✅ Complete
4. GET `/v3/volumes/detail` (Cinder) - ✅ Complete
5. GET `/v2/images` (Glance) - ✅ Complete
6. GET `/compute/v2.1/os-hypervisors/statistics` (Nova) - ✅ Complete

**Common User Operations**:
- Instance actions: reboot, resize, stop/start - ✅ Complete
- Console access: VNC/SPICE - ✅ Complete (Sprint 58-59)
- Volume attach/detach - ✅ Complete
- Floating IP association - ✅ Complete
- Security group management - ⚠️ Partially complete (add/remove SG to instance incomplete)

### 1.3 Missing Endpoint Priority Matrix

| Priority | Endpoint | Impact on Horizon | Implementation Effort |
|----------|----------|-------------------|----------------------|
| 🔴 HIGH  | Nova `migrate` | Instance migration dialog fails | 4 hours |
| 🔴 HIGH  | Nova `evacuate` | Host evacuation fails | 4 hours |
| 🔴 HIGH  | Nova `changePassword` | Admin password reset fails | 1 hour |
| 🔴 HIGH  | Nova `createBackup` | Backup creation fails | 3 hours |
| 🔴 HIGH  | Nova `addSecurityGroup` | SG assignment fails | 2 hours |
| 🔴 HIGH  | Nova `removeSecurityGroup` | SG removal fails | 2 hours |
| 🔴 SECURITY | Nova `os-resetState` | **Missing admin check - any user can reset!** | 30 min |
| 🟡 MEDIUM | Neutron topology validation | Network graph may not render | 2 hours |
| 🟢 LOW   | Keystone Federation | Enterprise SSO (SAML) | 40+ hours |

**Total High Priority Work**: ~19.5 hours

## 2. VNC Console Architecture

### 2.1 Current Implementation Status

✅ **COMPLETE** - Implemented in Sprint 58-59

**File**: `/Users/I761222/git/o3k/internal/nova/console.go`

**Endpoints Implemented**:
- `POST /servers/{id}/remote-consoles` - Modern endpoint (v2.6+)
- `POST /servers/{id}/action` with `os-getVNCConsole`, `os-getSPICEConsole`, `os-getRDPConsole`, `os-getSerialConsole`
- `GET /servers/{id}/os-console-output` - Serial console output

**Implementation Strategy**:
```go
// Token-based noVNC proxy URL generation
func (svc *Service) GetRemoteConsole(c *gin.Context) {
    serverID := c.Param("id")

    // Generate secure token
    token := jwt.SignedString(serverID, "vnc", time.Now().Add(1*time.Hour))

    // Return noVNC URL
    url := fmt.Sprintf("http://%s:6080/vnc_auto.html?token=%s",
        svc.consoleProxyHost, token)

    c.JSON(200, gin.H{
        "remote_console": gin.H{
            "type": "novnc",
            "protocol": "vnc",
            "url": url,
        },
    })
}
```

**Horizon Compatibility**: Full compatibility. Horizon expects exact format returned.

**Deployment Note**: Requires noVNC proxy deployment (not part of O3K binary). Documentation needed.

### 2.2 noVNC Integration Pattern

**Recommended Architecture**:
```
Browser → Horizon → O3K Nova API → Token Generation
Browser → noVNC Proxy (separate container) → libvirt VNC port
```

**Token Validation Flow**:
1. O3K generates JWT token with server_id, expires_at
2. noVNC proxy validates token against O3K Keystone
3. Proxy connects to libvirt VNC socket for that server
4. WebSocket proxied to browser

**Configuration** (for documentation):
```yaml
# docker-compose.yml snippet
novnc:
  image: novnc/noVNC:latest
  ports:
    - "6080:6080"
  environment:
    - VNC_HOST=o3k-compute
```

## 3. Network Topology Data Format

### 3.1 Horizon Topology Requirements

Horizon builds topology visualization from existing endpoints (no custom endpoint needed):

**Required Data**:
- Networks: `GET /v2.0/networks` - ✅ Implemented
- Subnets: `GET /v2.0/subnets` - ✅ Implemented
- Ports: `GET /v2.0/ports` - ✅ Implemented
- Routers: `GET /v2.0/routers` - ✅ Implemented
- Router interfaces: Derived from ports with `device_owner=network:router_interface` - ✅ Implemented

**Horizon Topology Logic** (client-side JavaScript):
```javascript
// Pseudo-code from horizon/openstack_dashboard/static/topology/topology.js
function buildTopology() {
    networks = fetchNetworks()
    routers = fetchRouters()
    ports = fetchPorts()
    instances = fetchInstances()

    // Build graph
    for (network in networks) {
        // Add network node
        addNetworkNode(network)

        // Add subnets
        for (subnet in network.subnets) {
            addSubnetNode(subnet)
        }

        // Add connected instances via ports
        ports_in_network = ports.filter(p => p.network_id == network.id)
        for (port in ports_in_network) {
            if (port.device_owner == "compute:nova") {
                addInstanceNode(port.device_id)
                connectInstanceToNetwork(port.device_id, network.id)
            }
        }
    }
}
```

### 3.2 Implementation Strategy

**Decision**: No custom endpoint needed. Existing Neutron APIs provide all required data.

**Validation Required**:
1. Verify response formats match OpenStack Neutron schema
2. Test with Horizon topology visualizer
3. Ensure port `device_owner` field populated correctly

**Test Case** (for contract tests):
```go
func TestNetworkTopologyData_Contract(t *testing.T) {
    // Create: 2 networks, 1 router, 2 instances
    // Verify: Horizon can build topology from API responses
    // Expected: All nodes/edges present in visualization
}
```

## 4. Nova Server Actions Gap Analysis

### 4.1 Incomplete Implementations (CRITICAL)

**File**: `/Users/I761222/git/o3k/internal/nova/advanced_actions.go`

#### Action 1: `migrate` (30% complete)

**Current State**:
```go
func (svc *Service) MigrateServer(c *gin.Context) {
    // Validates instance exists
    // Updates task_state to "migrating"
    // Returns 202 Accepted

    // MISSING:
    // - Migration record creation
    // - Host selection logic
    // - Background migration task
    // - Status transition completion
}
```

**Implementation Strategy**:
```go
// Required additions:
1. Create migrations table if not exists (migration_history)
2. Insert migration record with source_host, dest_host, status
3. If real mode: trigger libvirt migration
4. If stub mode: simulate with status updates
5. Update instance.host after completion
```

**Database Schema** (likely already exists):
```sql
CREATE TABLE IF NOT EXISTS migrations (
    id UUID PRIMARY KEY,
    instance_id UUID REFERENCES instances(id),
    source_host VARCHAR(255),
    dest_host VARCHAR(255),
    status VARCHAR(50), -- pending, running, completed, failed
    migration_type VARCHAR(50), -- migration, evacuation, resize
    created_at TIMESTAMP,
    updated_at TIMESTAMP
);
```

#### Action 2: `evacuate` (20% complete)

**Current State**:
```go
func (svc *Service) EvacuateServer(c *gin.Context) {
    // Updates task_state
    // Returns 200 OK

    // MISSING:
    // - Request body parsing (host, onSharedStorage, adminPass)
    // - Host validation
    // - Migration record creation
    // - adminPass handling (for rebuild scenarios)
}
```

**Request Format** (expected by Horizon):
```json
{
  "evacuate": {
    "host": "compute-node-2",  // optional
    "onSharedStorage": false,
    "adminPass": "newPassword123"  // optional
  }
}
```

**Implementation Strategy**:
```go
type EvacuateRequest struct {
    Evacuate struct {
        Host             string `json:"host"`
        OnSharedStorage  bool   `json:"onSharedStorage"`
        AdminPass        string `json:"adminPass"`
    } `json:"evacuate"`
}

// Parse request, select host if not provided, create migration record
// Return adminPass in response if generated
```

#### Action 3: `changePassword` (40% complete)

**Current State**:
```go
func (svc *Service) ChangeServerPassword(c *gin.Context) {
    // Validates instance
    // Parses adminPass from request

    // MISSING:
    // - bcrypt password hashing
    // - Database update to admin_password_hash column
}
```

**Implementation**:
```go
import "golang.org/x/crypto/bcrypt"

// Hash password
hashedPassword, err := bcrypt.GenerateFromPassword([]byte(adminPass), bcrypt.DefaultCost)
if err != nil {
    return err
}

// Update database
_, err = database.DB.Exec(ctx,
    "UPDATE instances SET admin_password_hash = $1, updated_at = NOW() WHERE id = $2",
    string(hashedPassword), serverID,
)
```

**Security Note**: Store only hash, never plaintext. Use bcrypt cost 10+ for production.

#### Action 4: `createBackup` (30% complete)

**Current State**:
```go
func (svc *Service) CreateServerBackup(c *gin.Context) {
    // Parses backup_name, backup_type, rotation

    // MISSING:
    // - Glance image creation from instance
    // - Rotation logic (delete oldest if count > rotation)
    // - Location header with new image ID
    // - Actual image_id in response
}
```

**Implementation Strategy**:
```go
// 1. Create snapshot image via Glance API
imageID, err := svc.glanceClient.CreateImage(ctx, glance.CreateImageRequest{
    Name:            backupName,
    ContainerFormat: "bare",
    DiskFormat:      "qcow2",
    Properties: map[string]string{
        "backup_type": backupType,  // daily, weekly, etc.
        "instance_uuid": serverID,
    },
})

// 2. Apply rotation (delete oldest backups if exceeded)
existingBackups := getBackupsForServer(serverID, backupType)
if len(existingBackups) >= rotation {
    // Delete oldest
    deleteImage(existingBackups[0].ID)
}

// 3. Return response with Location header
c.Header("Location", fmt.Sprintf("/v2/images/%s", imageID))
c.JSON(202, gin.H{"image_id": imageID})
```

#### Action 5-6: `addSecurityGroup` / `removeSecurityGroup` (50% complete)

**Current State**:
```go
func (svc *Service) AddSecurityGroup(c *gin.Context) {
    // Validates instance and security group exist
    // Returns 202 Accepted

    // MISSING:
    // - INSERT into server_security_groups table
    // - Duplicate check
    // - Neutron port security group updates (for real mode)
}

func (svc *Service) RemoveSecurityGroup(c *gin.Context) {
    // Validates SG exists
    // Returns 202 Accepted

    // MISSING:
    // - DELETE from server_security_groups table
    // - Association existence check
    // - Last SG protection (prevent removing all SGs)
    // - Neutron port updates
}
```

**Database Schema** (likely exists):
```sql
CREATE TABLE IF NOT EXISTS server_security_groups (
    server_id UUID REFERENCES instances(id) ON DELETE CASCADE,
    security_group_id UUID REFERENCES security_groups(id) ON DELETE CASCADE,
    PRIMARY KEY (server_id, security_group_id)
);
```

**Implementation**:
```go
// addSecurityGroup
func (svc *Service) AddSecurityGroup(c *gin.Context) {
    // 1. Check duplicate
    var exists bool
    err := database.DB.QueryRow(ctx,
        "SELECT EXISTS(SELECT 1 FROM server_security_groups WHERE server_id=$1 AND security_group_id=$2)",
        serverID, sgID,
    ).Scan(&exists)

    if exists {
        c.JSON(409, gin.H{"error": "Security group already associated"})
        return
    }

    // 2. Insert association
    _, err = database.DB.Exec(ctx,
        "INSERT INTO server_security_groups (server_id, security_group_id) VALUES ($1, $2)",
        serverID, sgID,
    )

    // 3. Update Neutron ports (real mode)
    if svc.networkingMode == "real" {
        updatePortSecurityGroups(serverID, sgID, "add")
    }

    c.JSON(202, gin.H{})
}

// removeSecurityGroup
func (svc *Service) RemoveSecurityGroup(c *gin.Context) {
    // 1. Check association exists
    var count int
    err := database.DB.QueryRow(ctx,
        "SELECT COUNT(*) FROM server_security_groups WHERE server_id=$1",
        serverID,
    ).Scan(&count)

    if count <= 1 {
        c.JSON(400, gin.H{"error": "Cannot remove last security group"})
        return
    }

    // 2. Delete association
    _, err = database.DB.Exec(ctx,
        "DELETE FROM server_security_groups WHERE server_id=$1 AND security_group_id=$2",
        serverID, sgID,
    )

    // 3. Update Neutron ports
    if svc.networkingMode == "real" {
        updatePortSecurityGroups(serverID, sgID, "remove")
    }

    c.JSON(202, gin.H{})
}
```

#### Action 7: `os-resetState` (70% complete) - **SECURITY ISSUE**

**Current State**:
```go
func (svc *Service) ResetServerState(c *gin.Context) {
    // Parses state from request
    // Updates instance status in database
    // Returns 202 Accepted

    // SECURITY ISSUE:
    // No admin role check! ANY user can reset instance state!
}
```

**Fix Required**:
```go
func (svc *Service) ResetServerState(c *gin.Context) {
    // CRITICAL: Add admin role check
    roles := c.GetStringSlice("roles")
    if !contains(roles, "admin") {
        c.JSON(403, gin.H{"error": "Admin role required for state reset"})
        return
    }

    // Existing logic...
}
```

**Pattern from Cinder** (reference implementation):
```go
// /Users/I761222/git/o3k/internal/cinder/volumes.go:ResetVolumeStatus
roles := c.GetStringSlice("roles")
isAdmin := false
for _, role := range roles {
    if role == "admin" {
        isAdmin = true
        break
    }
}
if !isAdmin {
    c.JSON(http.StatusForbidden, gin.H{"error": "Admin role required"})
    return
}
```

### 4.2 Implementation Priority

1. **Fix os-resetState admin check** (30 min) - SECURITY ISSUE
2. **Implement changePassword** (1 hour) - Common user operation
3. **Implement addSecurityGroup** (2 hours) - Horizon UI frequently used
4. **Implement removeSecurityGroup** (2 hours) - Paired with add
5. **Implement createBackup** (3 hours) - Backup creation workflow
6. **Implement migrate** (4 hours) - Live migration feature
7. **Implement evacuate** (4 hours) - Disaster recovery

**Total Effort**: ~19.5 hours (~2.5 days)

## 5. Service Catalog Configuration

### 5.1 Current Implementation

✅ **COMPLETE** - Implemented in Sprint 62-63

**File**: `/Users/I761222/git/o3k/internal/keystone/service_catalog.go`

**Database Tables**:
- `services`: Service definitions (compute, network, volume, image, identity)
- `endpoints`: Endpoint URLs per service (public, internal, admin interfaces)

**Token Response Format**:
```json
{
  "token": {
    "catalog": [
      {
        "type": "compute",
        "name": "nova",
        "endpoints": [
          {
            "interface": "public",
            "region": "RegionOne",
            "url": "http://o3k-host:8774/v2.1"
          }
        ]
      }
    ]
  }
}
```

### 5.2 Horizon Configuration Requirements

**Horizon local_settings.py**:
```python
OPENSTACK_HOST = "o3k-host"
OPENSTACK_KEYSTONE_URL = "http://%s:35357/v3" % OPENSTACK_HOST

# Important: Use v3 for modern Horizon
OPENSTACK_API_VERSIONS = {
    "identity": 3,
    "image": 2,
    "volume": 3,
}

# Token TTL must match O3K configuration
TOKEN_TIMEOUT_MARGIN = 3600  # 1 hour
```

**Required Service Catalog Entries**:
- `identity` (Keystone): http://o3k-host:35357/v3
- `compute` (Nova): http://o3k-host:8774/v2.1
- `network` (Neutron): http://o3k-host:9696/v2.0
- `volumev3` (Cinder): http://o3k-host:8776/v3
- `image` (Glance): http://o3k-host:9292/v2

**Validation**: Verify all services present in token response, URLs accessible from Horizon container.

## 6. Best Practices for Horizon Deployment

### 6.1 Docker Compose Configuration

**Recommended Setup**:
```yaml
version: '3.8'
services:
  o3k:
    image: o3k:latest
    ports:
      - "35357:35357"  # Keystone
      - "8774:8774"    # Nova
      - "9696:9696"    # Neutron
      - "8776:8776"    # Cinder
      - "9292:9292"    # Glance
    environment:
      - DATABASE_URL=postgresql://...
      - JWT_SECRET=change-in-production
    volumes:
      - ./config/o3k.yaml:/etc/o3k/o3k.yaml

  postgres:
    image: postgres:16
    environment:
      - POSTGRES_DB=o3k
      - POSTGRES_USER=o3k
      - POSTGRES_PASSWORD=secret

  horizon:
    image: kolla/ubuntu-horizon:zed
    ports:
      - "80:80"
    environment:
      - OPENSTACK_HOST=o3k
    volumes:
      - ./horizon/local_settings.py:/etc/openstack-dashboard/local_settings.py
    depends_on:
      - o3k

  novnc:
    image: novnc/noVNC:latest
    ports:
      - "6080:6080"
    environment:
      - VNC_HOST=o3k
```

### 6.2 CORS Configuration

**O3K Configuration** (config/o3k.yaml):
```yaml
cors:
  enabled: true
  allowed_origins:
    - "http://localhost"
    - "http://horizon"
  allowed_methods:
    - GET
    - POST
    - PUT
    - PATCH
    - DELETE
  allowed_headers:
    - "*"
  expose_headers:
    - "X-Subject-Token"
```

**Why CORS Needed**: Horizon makes XHR requests from browser to O3K APIs on different ports/domains.

### 6.3 Token TTL Tuning

**Recommended Settings**:
- Development: 24 hours (86400 seconds)
- Production: 4 hours (14400 seconds)
- Minimum: 1 hour to avoid frequent re-authentication

**O3K Configuration**:
```yaml
keystone:
  jwt_secret: "production-secret-key-here"
  token_ttl: 14400  # 4 hours
```

**Horizon Session Timeout** (local_settings.py):
```python
SESSION_TIMEOUT = 14400  # Match O3K token TTL
```

### 6.4 Troubleshooting Common Issues

| Error | Cause | Solution |
|-------|-------|----------|
| "Unable to retrieve version information" | Service catalog missing service type | Verify all 5 services registered in Keystone |
| "CORS policy: No 'Access-Control-Allow-Origin'" | CORS not configured | Enable CORS in O3K config, add Horizon origin |
| "Token has expired" | Token TTL too short | Increase token_ttl in O3K, match SESSION_TIMEOUT in Horizon |
| "Console not available" | noVNC proxy not running | Deploy noVNC container, verify port 6080 accessible |
| "Network topology not loading" | Missing Neutron endpoints | Verify router/network/subnet APIs return data |

## 7. Testing Strategy

### 7.1 Contract Tests (gophercloud)

**New Tests Required**:
1. `test/contract/nova/server_actions_test.go`
   - Test migrate, evacuate, changePassword, createBackup
   - Test addSecurityGroup, removeSecurityGroup
   - Test os-resetState with admin/non-admin users

2. `test/contract/neutron/topology_test.go`
   - Verify network/subnet/port response formats
   - Test router interface data

### 7.2 Integration Tests (bash + OpenStack CLI)

**Enhanced Horizon Compatibility Test** (`test/horizon_compat_test.sh`):
```bash
# Existing: 19 tests (all passing)
# Add:
- Test instance migration workflow
- Test backup creation and rotation
- Test security group add/remove
- Test admin password change
- Test console URL generation
- Test state reset (admin only)
```

**New Performance Test** (`test/horizon_load_test.sh`):
```bash
# Create 100+ resources
- 50 instances
- 20 networks
- 30 volumes
- 10 images

# Measure Horizon dashboard load time
# Target: < 2 seconds
```

### 7.3 Manual Horizon Testing

**Test Checklist** (for quickstart.md):
1. [ ] Login to Horizon dashboard
2. [ ] View Overview panel (hypervisor stats)
3. [ ] List instances, view details
4. [ ] Launch new instance (full wizard)
5. [ ] Open VNC console (noVNC popup)
6. [ ] Attach/detach volume
7. [ ] Associate/disassociate floating IP
8. [ ] Add/remove security group
9. [ ] Create backup
10. [ ] View network topology
11. [ ] Perform instance actions (reboot, resize, stop/start)
12. [ ] Verify quota display

## 8. Documentation Requirements

### 8.1 New Documentation Files

1. **docs/HORIZON_INTEGRATION.md**
   - Architecture diagram (Horizon → O3K services)
   - Deployment guide (Docker Compose)
   - Configuration reference
   - Troubleshooting section

2. **docs/KEYSTONE_AUTH_FLOW.md**
   - JWT token generation flow
   - Service catalog construction
   - Multi-tenancy isolation
   - Token validation middleware

3. **docs/API_COVERAGE.md** (update existing)
   - Endpoint coverage matrix (308/330 → 330/330)
   - Microversion support status
   - Known limitations

### 8.2 Quickstart Guide Structure

**quickstart.md Contents**:
```markdown
# Quick Start: Horizon with O3K

## Prerequisites
- Docker & Docker Compose
- 4GB RAM minimum
- 10GB disk space

## Step 1: Deploy O3K
[docker-compose.yml snippet]

## Step 2: Configure Horizon
[local_settings.py configuration]

## Step 3: Verify Services
[Health check commands]

## Step 4: Access Dashboard
http://localhost/dashboard

## Troubleshooting
[Common issues and fixes]
```

## 9. Success Criteria Validation

### 9.1 Measurable Outcomes

| Success Criteria (from spec) | Validation Method | Target | Status |
|-------------------------------|-------------------|--------|--------|
| SC-001: Horizon deploys without modifications | Integration test | Pass | ✅ Currently true |
| SC-002: 34+ tests passing (19 existing + 15 new) | `make test-horizon` | 34/34 | ⚠️ 19/34 (need 15 more) |
| SC-003: Instance lifecycle < 2 minutes | Performance test | < 120s | ⚠️ Need test |
| SC-004: Dashboard loads < 2s with 100+ resources | Browser DevTools | < 2s | ⚠️ Need test |
| SC-005: Zero JavaScript errors | Browser console | 0 errors | ⚠️ Need validation |
| SC-006: Topology renders with 10+ networks | Manual test | Renders | ⚠️ Need validation |
| SC-007: VNC console opens < 3s | Manual test | < 3s | ✅ Complete Sprint 58-59 |
| SC-012: 95%+ endpoint coverage | API analysis | 95% | ⚠️ Currently 91% |

### 9.2 Implementation Roadmap

**Sprint 56-57** (HIGH PRIORITY):
- Complete 7 Nova server actions
- Fix os-resetState security issue
- Add 8 contract tests

**Sprint 58** (MEDIUM PRIORITY):
- Performance testing with 100+ resources
- Manual Horizon validation
- Database query optimization (if needed)

**Sprint 59** (LOW PRIORITY):
- Documentation completion
- Quickstart guide
- API coverage report update

**Estimated Total**: 3 sprints (~3 weeks)

## 10. Decisions & Rationale

### Decision 1: No Custom Topology Endpoint

**Rationale**: Horizon builds topology client-side from existing Neutron APIs. Custom endpoint would add complexity without benefit. Existing endpoints (networks, subnets, ports, routers) provide all required data.

**Alternatives Considered**:
- Server-side topology aggregation endpoint
- GraphQL-style query for topology data

**Rejected Because**: Adds abstraction layer (violates Constitution Article VIII: Anti-Abstraction), increases maintenance burden, no clear benefit over existing APIs.

### Decision 2: Console Tokens via JWT (Not Database)

**Rationale**: Console tokens use existing JWT infrastructure. No new database table needed, tokens are stateless and time-limited.

**Alternatives Considered**:
- Database table for console sessions
- Redis cache for token storage

**Rejected Because**: Database adds latency and persistence overhead. Redis adds external dependency. JWT aligns with existing O3K token design.

### Decision 3: Server Actions Implementation Priority

**Rationale**: Prioritize by Horizon UI frequency of use and security impact.

**Priority Order**:
1. Security fix (os-resetState) - CRITICAL
2. Common operations (changePassword, SG add/remove) - HIGH
3. Admin operations (migrate, evacuate, backup) - MEDIUM

**Alternatives Considered**:
- Alphabetical order
- Complexity-based (easiest first)

**Rejected Because**: Security must be first. User-facing operations before admin-only.

### Decision 4: Test-First for New Actions

**Rationale**: Constitution Article III (Test-First) is NON-NEGOTIABLE.

**Approach**:
1. Write contract test for action (must fail)
2. Implement action
3. Verify test passes

**Alternatives Considered**:
- Implement first, test later
- Manual testing only

**Rejected Because**: Violates constitution, risks regression, manual testing is not repeatable.

## 11. Open Questions

None. All unknowns from plan Technical Context have been resolved.

## Appendix: File References

- **Nova Actions**: `/Users/I761222/git/o3k/internal/nova/advanced_actions.go`
- **Console Implementation**: `/Users/I761222/git/o3k/internal/nova/console.go`
- **Service Catalog**: `/Users/I761222/git/o3k/internal/keystone/service_catalog.go`
- **Gap Analysis**: `/Users/I761222/git/o3k/docs/sprints/SPRINT_56-57_GAP_ANALYSIS.md`
- **Remaining Work**: `/Users/I761222/git/o3k/REMAINING_WORK.md`
- **Horizon Tests**: `/Users/I761222/git/o3k/test/horizon_compat_test.sh`
