# Comprehensive Review Report — O3K

**Date**: 2026-04-27
**Scope**: Full codebase (10 packages, all Go files)
**Method**: Wave 0 per-package deep review (10 parallel agents)
**Findings**: 75 total (18 CRITICAL, 25 HIGH, 20 MEDIUM, 12 LOW)

---

## Executive Summary

The O3K codebase has working API surfaces and passes its contract tests, but contains serious correctness and security issues across all packages. The most urgent problems are: unauthenticated admin endpoints in Keystone (complete privilege escalation), XML injection in libvirt domain definitions, data races in the gRPC tunnel, and multiple paths to data corruption in Neutron/Cinder.

---

## CRITICAL Findings (18)

### 1. Keystone: Unauthenticated Admin Endpoints
**File**: `internal/keystone/handlers.go` (all management routes)
**Issue**: Project/User/Role/Domain CRUD endpoints have no AuthMiddleware. Any anonymous HTTP request can create admin users, delete projects, escalate roles.
**Fix**: Add `middleware.AuthMiddleware(authService)` to all management route groups.

### 2. Keystone: Plaintext Secret in "secret_hash" Column
**File**: `internal/keystone/application_credentials.go:130`
**Issue**: Application credential secret stored verbatim in column named `secret_hash`. Misleading name and no protection if DB is compromised.
**Fix**: bcrypt-hash the secret before storage; compare with `bcrypt.CompareHashAndPassword` on auth.

### 3. Keystone: SQL Placeholder Overflow at argCount >= 10
**File**: `internal/keystone/domains.go:167-198`, `credentials.go`, `neutron/trunks.go`
**Issue**: `string(rune('0' + argCount))` produces `':'` at 10, `';'` at 11, etc. Invalid SQL placeholders.
**Fix**: Use `fmt.Sprintf("$%d", argCount)` or `strconv.Itoa(argCount)`.

### 4. Keystone: rand.Read Error Ignored
**File**: `internal/keystone/application_credentials.go`
**Issue**: `rand.Read(b)` return value unchecked. On entropy exhaustion, secret is all zeros.
**Fix**: Check error; fail the request if random generation fails.

### 5. Tunnel: Concurrent stream.Send Race
**File**: `internal/tunnel/client.go:101-121`
**Issue**: Heartbeat goroutine and executeTask goroutines call `stream.Send()` without synchronization. gRPC streams are not concurrency-safe for Send.
**Fix**: Add `sendMu sync.Mutex` to AgentClient; wrap all Send calls.

### 6. Tunnel: Inflight Accounting Mismatch
**File**: `internal/tunnel/dispatch.go:20-47` vs `server.go:172-207`
**Issue**: `Dispatcher.Dispatch` never calls `TryAcquireInflight` but `AgentStream` always calls `ReleaseInflight` on task completion. Counter goes negative.
**Fix**: Call `TryAcquireInflight` in Dispatch before sending, or remove Release from the completion path.

### 7. Scheduler: Reconciler Missing Transaction
**File**: `internal/scheduler/reconciler.go:46`
**Issue**: `FOR UPDATE SKIP LOCKED` used without `BeginTx`. The row lock is released immediately after the query returns, so two reconcilers can process the same stalled task.
**Fix**: Wrap reconciler loop body in `database.WithTx(ctx, func(tx) { ... })`.

### 8. Scheduler: Retry Threshold Inconsistency
**File**: `internal/scheduler/worker.go:148` (retries>=2), `reconciler.go:66` (retries>=3), schema CHECK (retries<=3)
**Issue**: Worker fails tasks at 2 retries, reconciler at 3, schema allows 3. Tasks that hit 2 retries are permanently failed by worker before reconciler's threshold.
**Fix**: Define single `const maxTaskRetries = 3`; use everywhere.

### 9. Nova: GetFlavorExtraSpecs Data Loss
**File**: `internal/nova/flavors.go:136`
**Issue**: `rows.Scan(&key, &value)` succeeds but the map assignment `extraSpecs[key] = value` is missing. Returns empty map always.
**Fix**: Add `extraSpecs[key] = value` after Scan.

### 10. Nova: ListServersDetail NULL Panic
**File**: `internal/nova/handlers.go:698-704`
**Issue**: NULL-able columns (`image_id`, `key_name`, `availability_zone`, `terminated_at`) scanned into plain `string`/`time.Time`. NULL causes panic.
**Fix**: Use `*string` / `*time.Time` or `sql.NullString`/`sql.NullTime`.

### 11. Neutron: Duplicate IP Allocation
**File**: `internal/neutron/ports.go:527`
**Issue**: IP allocation uses `10 + time.Now().Unix()%240` — same IP within same second. No uniqueness check against existing allocations.
**Fix**: Query allocated IPs for subnet, find first available in CIDR range.

### 12. Neutron: MTU Response Mismatch
**File**: `internal/neutron/network.go:508`
**Issue**: Response hardcodes MTU 1500 even when database stores 1450 for VXLAN networks. Clients configure wrong MTU, causing silent packet drops.
**Fix**: Return MTU from database row, not hardcoded value.

### 13. Cinder: Non-Atomic Volume Transfer
**File**: `internal/cinder/transfers.go:231`
**Issue**: AcceptVolumeTransfer does two UPDATE statements without a transaction. Crash between them leaves volume in limbo (transfer deleted but project_id unchanged).
**Fix**: Wrap in `database.WithTx`.

### 14. Cinder: Name-vs-UUID Ceph Deletion
**File**: `internal/cinder/volumes.go:551`
**Issue**: Raw URL parameter passed to `rbd remove`. If user passes volume name instead of UUID, wrong RBD image is deleted.
**Fix**: Look up volume by ID from DB first; use stored RBD image name.

### 15. Hypervisor: XML Injection
**File**: `pkg/hypervisor/xml_template.go:48`
**Issue**: `fmt.Sprintf` interpolates user-controlled strings (VM name, disk path, network ID) directly into libvirt XML without escaping. Attacker-controlled VM names can inject arbitrary XML elements.
**Fix**: Add `xmlEscape(s string) string` that escapes `<>&"'`; apply to all interpolated values.

### 16. Hypervisor: Resume Cold-Boots Instead of Restoring
**File**: `pkg/hypervisor/libvirt.go:561`
**Issue**: `resumeVMReal` calls `DomainCreate` (cold boot) instead of `DomainManagedSaveRemove` + `DomainCreate` or `DomainCreateWithFlags`. RAM state from ManagedSave is lost.
**Fix**: Use `DomainHasManagedSaveImage`; if true, use `DomainCreateWithFlags(DOMAIN_START_FROM_MANAGED_SAVE)`.

### 17. Networking: Namespace Prefix Match
**File**: `pkg/networking/netns.go:128`
**Issue**: `NamespaceExists` uses `strings.HasPrefix`. Namespace "net-abc" matches when checking for "net-ab". Could delete wrong namespace.
**Fix**: Use exact match (`== name`) or match with trailing delimiter.

### 18. Networking: Panic on Short IDs
**File**: `pkg/networking/security_groups.go:77` and 5+ other locations
**Issue**: `securityGroupID[:8]` without length check. Short/empty IDs cause runtime panic.
**Fix**: Add `if len(id) < 8 { return error }` guard.

---

## HIGH Findings (25)

| # | Package | File:Line | Issue |
|---|---------|-----------|-------|
| 1 | tunnel | task.go constants | Task type mismatch: "create_vm" vs "VM_CREATE" in executor switch |
| 2 | scheduler | hub_adapter.go:23 | PickAgent ignores agentID parameter (always random) |
| 3 | nova | advanced_actions.go:46 | power_state 4 for SUSPENDED (OpenStack spec says 7) |
| 4 | nova | interface_attach.go:337 | CIDR slicing assumes /24 (breaks on /16, /28, etc.) |
| 5 | nova | handlers.go:894-910 | DeleteServer never deletes port DB records (orphaned) |
| 6 | neutron | ports.go:924 | ListSecurityGroupRules missing project_id filter (cross-tenant) |
| 7 | neutron | floatingip.go:441 | Hardcoded "10.0.1.10" when fixed_ip missing |
| 8 | cinder | volumes.go:589,680,729 | Bare type assertions in VolumeAction (panic on bad input) |
| 9 | cinder | volumes.go:628 | os-extend allows shrinking (no size comparison) |
| 10 | hypervisor | libvirt.go:65-70 | libvirtURI parameter silently ignored |
| 11 | hypervisor | libvirt.go:111 | No mutex on libvirt operations (data race) |
| 12 | networking | security_groups.go:260-279 | INPUT/OUTPUT chains instead of FORWARD (rules never hit) |
| 13 | networking | dhcp.go:123-149 | Unsynchronized concurrent map access |
| 14 | database | tx.go:17 | WithTx panics when MockDB.BeginTx returns (nil, nil) |
| 15 | database | migrate.go:232 | name[len(name)-6:] panics on short filenames |
| 16 | database | db.go:81 | Close() doesn't nil out DB reference |
| 17 | keystone | handlers.go:237-242 | Token revocation returns 204 but is no-op |
| 18 | compat | checker.go:36 | ListenAddr field stored but never used |
| 19 | nova | handlers.go:445 | CreateServer ignores DB insert error |
| 20 | neutron | router.go:312 | AddRouterInterface doesn't verify subnet belongs to project |
| 21 | cinder | backups.go:89 | Backup restore overwrites volume without snapshot |
| 22 | glance | handlers.go:201 | Image download doesn't set Content-Length |
| 23 | nova | handlers.go:312 | Flavor delete succeeds even when VMs reference it |
| 24 | neutron | vxlan_coordinator.go:67 | 1-second poll with no jitter (thundering herd) |
| 25 | tunnel | server.go:89 | Agent removal doesn't cancel pending tasks |

---

## MEDIUM Findings (20)

| # | Package | Issue Summary |
|---|---------|---------------|
| 1 | hypervisor | VNC binds 0.0.0.0 with no password |
| 2 | compat | isProviderError unreachable dead code |
| 3 | nova | Microversion header parsing ignores malformed values |
| 4 | neutron | Security group rule priority not enforced |
| 5 | cinder | Volume type extra_specs not validated |
| 6 | keystone | Service catalog endpoint URLs not validated |
| 7 | tunnel | Heartbeat interval hardcoded (not configurable) |
| 8 | scheduler | No exponential backoff on retry |
| 9 | database | Migration file ordering depends on filename sort |
| 10 | networking | VXLAN VNI allocation has no upper bound check |
| 11 | nova | Console URL returns placeholder in stub mode |
| 12 | neutron | Subnet CIDR overlap not checked on create |
| 13 | cinder | Snapshot parent_id not verified on create |
| 14 | glance | Image size not updated after upload |
| 15 | keystone | Password policy not enforced (any length accepted) |
| 16 | nova | Server metadata key length not limited |
| 17 | neutron | Port MAC uniqueness not enforced per-network |
| 18 | tunnel | mTLS cert rotation not implemented |
| 19 | scheduler | Task timeout not enforced (tasks can run forever) |
| 20 | database | Connection pool exhaustion not monitored |

---

## LOW Findings (12)

| # | Issue Summary |
|---|---------------|
| 1 | fmt.Printf used instead of structured logging (20+ locations) |
| 2 | Missing godoc on exported functions |
| 3 | Inconsistent error message formatting |
| 4 | Test coverage below 50% in nova, cinder packages |
| 5 | TODO comments without issue tracking references |
| 6 | Magic numbers in multiple packages |
| 7 | Unused function parameters in stub implementations |
| 8 | Import ordering inconsistent |
| 9 | Context not propagated through some DB calls |
| 10 | Missing defer rows.Close() in 3 query functions |
| 11 | Hardcoded port numbers in tests |
| 12 | No graceful shutdown signal handling |

---

## Cross-Package Patterns

1. **iptables chain safety bypass**: Security groups on INPUT/OUTPUT instead of FORWARD — VM traffic never hits the rules
2. **Unvalidated string slicing**: `id[:8]`, `id[:11]` throughout without length checks
3. **fmt.Printf instead of structured logging**: 20+ locations across all packages
4. **Missing transaction wrapping**: Multiple multi-step DB operations without atomic guarantees
5. **Bare type assertions**: Throughout Cinder/Keystone/Nova action handlers
6. **string(rune('0'+N)) placeholder bug**: In keystone/domains.go, keystone/credentials.go, neutron/trunks.go

---

## Fix Priority

| Priority | Count | Action |
|----------|-------|--------|
| P0 (CRITICAL security) | 4 | Keystone auth + XML injection — fix immediately |
| P1 (CRITICAL data) | 8 | Races, corruption, panics — fix this sprint |
| P2 (CRITICAL logic) | 6 | Wrong behavior — fix this sprint |
| P3 (HIGH) | 25 | Fix next sprint |
| P4 (MEDIUM) | 20 | Track in backlog |
| P5 (LOW) | 12 | Address opportunistically |

---

## Deferred Findings

None. All findings will be fixed on the `fix/comprehensive-review` branch.
