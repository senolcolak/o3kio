# O3K Production Readiness Assessment — v6 (Post v5-Fix Deep Audit)

**Date**: 2026-05-07
**Review Method**: 5 per-package deep-read agents, spec-grounded analysis
**Branch**: `fix/v5-critical-high-findings` (commit `34e7172`)
**Previous Score (v5)**: 6.5/10
**Current Score**: **7.5/10**

---

## Executive Summary

The v5 fix pass resolved all 22 CRITICAL+HIGH findings from the previous review. Key improvements: nested quota format works, server_usages populated, floating IP allocation transactional, marker-based pagination on all Neutron endpoints, OS-EXT-SRV-ATTR present, AuthMiddleware on compat server, microversion major version validated.

However, this deep audit reveals **6 new CRITICALs, 16 HIGHs, 22 MEDIUMs, and 23 LOWs** across all five services. The gaps fall into three categories:

1. **Spec features never implemented**: Access rules, policy engine, legacy_auth backward compat, audit logging (keystone-minimal-iam-design-v2 Week 2 entirely absent)
2. **Response shape gaps**: Create/Update responses missing fields that Get/List include; Cinder and Glance responses missing mandatory OpenStack API fields
3. **Operational safety**: Empty JWT secret accepted, unscoped tokens not rejected, console URLs hardcoded to localhost, global table locks

**What works for Terraform**: Basic CRUD for VMs, networks, volumes, security groups, app credentials, floating IPs, routers
**What breaks for Horizon**: Console (localhost URLs), volume create/update (missing fields), image owner display, SG default rules, usage tab (wrong flavor name), quota defaults (wrong format)
**What's missing from spec**: Access rules, policy engine, legacy_auth migration, audit events, unrestricted enforcement

---

## Score Justification

```
v5 assessment:                    6.5/10
After v5 fix pass (this review):  7.5/10
Next target (fix CRITICALs+HIGHs): 8.5/10
Full spec compliance target:       9.5/10
```

**Why 7.5 not 8.5**: The v5 fixes correctly resolved the issues they targeted (C1-C8, H1-H14 from v5 confirmed fixed). But deeper reading reveals a second layer of issues: Create/Update responses that are incomplete compared to their corresponding Get responses (Cinder, Neutron SG), operational concerns (localhost console URLs, empty JWT), and three entire spec features that were never implemented (access rules, policy engine, legacy_auth). The codebase has improved substantially in correctness but still has gaps that would break Horizon workflows.

---

## Findings by Severity

### CRITICAL (6) — Production blockers

| # | Package | Finding | Impact |
|---|---------|---------|--------|
| C1 | keystone | Access rules column and enforcement absent | App credentials have no fine-grained restrictions |
| C2 | keystone | legacy_auth flag missing — pre-migration credentials silently broken | All pre-existing app credentials fail bcrypt verification |
| C3 | keystone | Policy engine not implemented (spec Week 2 entirely absent) | No resource-level authorization beyond admin/non-admin |
| C4 | cinder | CreateVolume response missing volume_type, encrypted, availability_zone | Horizon volume create wizard broken |
| C5 | cinder | UpdateVolume response missing tenant_id, bootable, AZ, volume_type, encrypted, attachments | Horizon volume update UI broken |
| C6 | neutron | Bridge creation before DB insert — no rollback on failure | Leaked network bridges on host |

### HIGH (16)

| # | Package | Finding | Impact |
|---|---------|---------|--------|
| H1 | keystone | unrestricted flag stored but never enforced | Any app cred can create nested creds |
| H2 | keystone | defer roleRows.Close() inside loop — resource leak | Connection pool exhaustion |
| H3 | keystone | rows.Err() silently skipped in role subquery | Partial roles in response |
| H4 | keystone | AuthenticateApplicationCredential methods field trusts client input | Token methods forgeable |
| H5 | nova | GetServer does not query launched_at from DB | Wrong uptime display |
| H6 | nova | buildServerUsages sets flavor to instance name, not flavor name | Usage tab wrong |
| H7 | nova | Console URLs hardcoded to localhost:6080 | Console broken in Docker/multi-host |
| H8 | nova | GetQuotaSetDefaults returns flat values (not nested) | API format inconsistency |
| H9 | nova | snapshotCount never queried — always zero | Quota display wrong |
| H10 | neutron | Pagination marker is lexicographic on UUID — non-deterministic | Pages skip/duplicate items |
| H11 | neutron | provider:network_type always returns "flat" regardless of DB | VXLAN networks display wrong |
| H12 | neutron | allocateIPFromSubnet locks ALL ports table globally | Performance cliff |
| H13 | neutron | CreateSecurityGroup returns empty rules (no default egress) | SG panel empty, traffic blocked |
| H14 | neutron | getSecurityGroupRules omits absent fields instead of null | Horizon JS errors |
| H15 | cinder | volume_type and availability_zone hardcoded, never read from DB | Stale after retype |
| H16 | glance | ListImages and GetImage omit owner field | Horizon image panel broken |

### MEDIUM (22)

| # | Package | Finding |
|---|---------|---------|
| M1 | keystone | IsTokenRevoked uses context.Background() — no timeout |
| M2 | keystone | CreateApplicationCredential role IDs not validated against user's roles |
| M3 | keystone | Audit logging absent (no keystone_auth_events table) |
| M4 | keystone | handlers_test.go — single trivial test, no auth flow coverage |
| M5 | keystone | GetDomain maps all DB errors to 404 |
| M6 | nova | ListFlavorsDetail marker pagination timestamp collision |
| M7 | nova | GetServer response missing links array |
| M8 | nova | UpdateServer sets user_id = projectID (data corruption) |
| M9 | nova | Microversion middleware missing Vary on OpenStack-API-Version |
| M10 | nova | CheckQuota ignores errors from usage queries |
| M11 | nova | buildServerUsages ignores start/stop time range params |
| M12 | nova | GetServer missing security_groups array |
| M13 | neutron | AddRouterInterface not atomic — port+router_interface can partially succeed |
| M14 | neutron | ListPorts N+1 query for security groups |
| M15 | neutron | ListSubnets returns caller's project_id as tenant_id for shared subnets |
| M16 | neutron | RemoveRouterInterface response missing network_id, tenant_id |
| M17 | neutron | allocateFloatingIP commits TX then inserts outside TX — race window |
| M18 | neutron | GetSubnet returns caller's project_id as tenant_id |
| M19 | neutron | UpdateSecurityGroup response omits created_at, updated_at |
| M20 | neutron | DeleteSecurityGroup doesn't check if group still in use |
| M21 | glance | GetImage cache invalidation uses wrong key format |
| M22 | common | Empty JWT secret accepted (only "change-me-in-production" checked) |

### LOW (23)

| # | Package | Finding |
|---|---------|---------|
| L1 | keystone | CleanExpiredRevocations has no caller — memory leak |
| L2 | keystone | BuildServiceCatalog returns inconsistent map ordering |
| L3 | keystone | Contract tests missing for spec requirements 1-6 |
| L4 | nova | ListHypervisors id is integer, not UUID (mv >= 2.53) |
| L5 | nova | CreateServer response doesn't include full detail shape |
| L6 | nova | aggregates GetAggregate uses DB round-trip for json.Unmarshal |
| L7 | nova | DeleteServer doesn't check rows.Err() after port unbind loop |
| L8 | nova | ListFlavors (brief) has no pagination |
| L9 | nova | N+1 quota queries (7 individual QueryRow calls) |
| L10 | neutron | UpdateNetwork ignores DB no-rows — silent success |
| L11 | neutron | ListSecurityGroupRules has no pagination |
| L12 | neutron | json.Unmarshal errors silently ignored in multiple places |
| L13 | neutron | rows.Err() not checked in GetPort security groups query |
| L14 | neutron | CreateTrunk calls time.Now() multiple times |
| L15 | neutron | GetTrunk/UpdateTrunk don't check subPortRows.Err() |
| L16 | neutron | ListNetworkIPAvailabilities uses hardcoded /24 estimate |
| L17 | neutron | trunks.go subPortRows leaked on error path |
| L18 | neutron | vxlan_coordinator allocateVNI linear scan without lock |
| L19 | cinder | ListSnapshots has no marker-based pagination |
| L20 | cinder | DeleteVolume doesn't check non-ErrNoRows DB errors |
| L21 | glance | StageImageData buffers entire image in memory |
| L22 | glance | GetTask leaks internal error details |
| L23 | common | NewInternalServerError uses computeFault for all services |

---

## Spec Coverage Analysis

### keystone-minimal-iam-design-v2

| Requirement | Status |
|-------------|--------|
| App credential CRUD | WORKING |
| App credential authentication | WORKING |
| Bcrypt cost 12 | WORKING |
| Token revocation (DB-persisted) | WORKING |
| Service catalog in token | WORKING |
| Access rules (path/method/service) | NOT IMPLEMENTED |
| unrestricted flag enforcement | NOT IMPLEMENTED |
| legacy_auth backward compat | NOT IMPLEMENTED |
| Policy engine (RBAC evaluation) | NOT IMPLEMENTED |
| Audit logging (auth events) | NOT IMPLEMENTED |
| appcreds/ subpackage | NOT IMPLEMENTED |
| policy/ subpackage | NOT IMPLEMENTED |
| tokens/ subpackage | NOT IMPLEMENTED |

**Coverage: ~45%** (down from 60% claimed in v5 — honest reassessment; stored-but-not-enforced does not count)

### SPEC-002 (Horizon Full Compatibility)

| Requirement | Status |
|-------------|--------|
| FR-001: Token format with catalog | WORKING |
| FR-003: Microversion headers | WORKING (no behavior gating by version) |
| FR-005: Server extended attributes | PARTIAL (OS-EXT-SRV-ATTR:host present; root_device_name, launch_index missing) |
| FR-006: Marker-based pagination | PARTIAL (Nova working; Neutron UUID-lexicographic is non-deterministic; Cinder timestamp-only) |
| FR-009: Network provider attributes | BROKEN (hardcoded "flat") |
| FR-010: Security group rules in response | PARTIAL (Get/List have rules; Create returns empty) |
| FR-011: Router interface response | PARTIAL (Add has fields; Remove missing network_id, tenant_id) |
| FR-012: Volume detail attributes | BROKEN (volume_type, encrypted, AZ hardcoded not from DB) |
| FR-014: Quota set format | WORKING (nested format correct; defaults endpoint flat — inconsistency) |
| FR-015: Tenant usage with servers | PARTIAL (populated but flavor field wrong, no time filter) |
| FR-020: Hypervisor statistics | PARTIAL (running_vms from DB; other values hardcoded) |

**Coverage: ~55%** (up from 45% in v5)

### SPEC-000 (Compliance Addendum)

| Requirement | Status |
|-------------|--------|
| Contract test suite | PARTIAL (basic CRUD only, no spec-requirement tests) |
| Terraform test suite | NOT STARTED |
| CLI test suite | Partial (bash scripts) |
| Schema validation | NOT STARTED |
| Zero failures policy | Cannot evaluate |

**Coverage: ~15%** (up from 10%)

---

## What Now Works (After v5 Fixes — Verified)

- C1: AuthMiddleware on compat/embedded.go (VERIFIED)
- C2: OS-EXT-SRV-ATTR:host present in GetServer/ListServersDetail
- C3: GetQuotaSet returns nested {in_use, limit, reserved} format
- C4: server_usages populated from DB
- C5+C6: allocateFloatingIP uses serializable TX + subnet filter
- C7: ListNetworks returns project_id from DB as tenant_id
- C8: allocateNextIP uses net.ParseCIDR (not /24 assumption)
- H1-H14 from v5: All verified fixed (rows.Err(), defer Close, microversion validation, marker pagination, SG rules in response, etc.)

---

## What Still Breaks

| Scenario | What Happens | Root Cause |
|----------|--------------|------------|
| Horizon opens VNC console | Connection refused | H7: localhost:6080 |
| Horizon creates volume | Missing columns in result | C4: CreateVolume response incomplete |
| Horizon renames volume | Detail page blanks out | C5: UpdateVolume response incomplete |
| Horizon image panel | Owner column blank | H16: owner field missing |
| Horizon Usage tab | Wrong flavor names shown | H6: instance name used as flavor |
| Horizon SG after create | Empty rules panel | H13: no default egress rules |
| Horizon quota defaults | API format error | H8: flat format vs nested |
| VXLAN networks in topology | Shows as "flat" | H11: hardcoded network type |
| App credential with access rules | Rules not enforced | C1: access_rules absent |
| Upgrade with existing app creds | All creds fail | C2: no legacy_auth path |
| Admin resets user password on Terraform | Works | (Fixed in v5) |

---

## Remediation Priority

### Phase 1: Fix CRITICALs (4-6 hours)
1. **C4+C5**: Complete Cinder Create/UpdateVolume response shapes — 1h
2. **C6**: Reorder bridge creation after DB insert in CreateNetwork — 30 min
3. **C1+C2+C3**: Access rules + legacy_auth + policy engine — these are LARGE features, not quick fixes. Estimate: 15-20 hours for access rules + legacy_auth, 30-40 hours for policy engine.

### Phase 2: Fix HIGHs (6-8 hours)
1. **H5-H9**: Nova response fixes (launched_at, flavor name, snapshot count, defaults format) — 2h
2. **H7**: Console URL from config/request host — 30 min
3. **H10-H14**: Neutron pagination/response fixes — 3h
4. **H15-H16**: Cinder/Glance field fixes — 1h
5. **H1-H4**: Keystone enforcement fixes — 2h

### Phase 3: Fix MEDIUMs (4-6 hours)
- M1-M22: Various correctness and safety fixes — 4-6h

### Honest Assessment of Remaining Effort

| Category | Effort | Impact on Score |
|----------|--------|----------------|
| Fix all HIGHs (H1-H16) | 8-10 hours | 7.5 → 8.0 |
| Fix all MEDIUMs (M1-M22) | 4-6 hours | 8.0 → 8.5 |
| Fix Cinder CRITICALs (C4-C6) | 2 hours | Removes 3 production blockers |
| Implement access rules + legacy_auth (C1+C2) | 15-20 hours | Spec compliance 45% → 70% |
| Implement policy engine (C3) | 30-40 hours | Spec compliance 70% → 90% |

**Total to reach 8.5/10**: ~20 hours (C4-C6 + all HIGHs + all MEDIUMs)
**Total to reach 9.5/10**: ~70-80 hours (+ access rules + policy engine + contract tests)

---

## Methodology

1. Dispatched 5 per-package Wave 0 agents reading ALL code in each package group
2. Each agent reviewed against three design specifications (keystone-minimal-iam-design-v2, SPEC-002, SPEC-000)
3. Every finding verified by reading actual code — no assumptions
4. Previous v5 fixes (C1-C8, H1-H14) confirmed resolved
5. Score reflects honest spec coverage + operational readiness

---

## Build Status

- `go build ./...` — PASS
- `go vet ./...` — PASS
- `go test ./internal/...` — PASS (all packages)
- Contract tests — PARTIAL (basic CRUD only)
