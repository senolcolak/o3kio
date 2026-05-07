# O3K Production Readiness Assessment — v7 (Post C1+C2+C3 Spec Features)

**Date**: 2026-05-07
**Review Method**: 5 per-package deep-read agents, spec-grounded analysis
**Branch**: `main` (commit `bce4fd6`)
**Previous Score (v6)**: 7.5/10
**Current Score**: **8.0/10**

---

## Executive Summary

The three spec features (C1 access rules, C2 legacy_auth, C3 policy engine) are now implemented. Legacy auth works correctly with timing-safe comparison and transparent upgrade. The policy engine parses and evaluates rules with caching. Access rules are stored in JWT and enforced by middleware.

However, this deep audit reveals **2 CRITICALs, 14 HIGHs, 14 MEDIUMs, and 22 LOWs**. The issues fall into four categories:

1. **Spec features implemented but with correctness gaps**: unrestricted flag bypass (auth_method not in JWT), access rule `service` field ignored, policy engine operator precedence wrong, no cycle detection
2. **Response shape gaps**: Nova missing security_groups/links arrays, Cinder Create/Update responses diverge from Get
3. **Hardcoded values**: availability_zone, encrypted, console URLs, hypervisor stats
4. **Operational safety**: empty JWT secret accepted, role escalation via app credentials, cache key mismatch in Glance

**Score improvement from v6**: +0.5 (C1+C2+C3 implemented; but correctness gaps in those implementations prevent full credit)

---

## Score Justification

```
v6 assessment:                    7.5/10
After C1+C2+C3 (this review):    8.0/10
Next target (fix CRITICALs+HIGHs): 8.8/10
Full spec compliance target:       9.5/10
```

**Why 8.0 not 8.5**: The three spec features exist but have correctness issues:
- Access rules: service field silently ignored (weaker isolation than intended)
- Policy engine: and/or have equal precedence (breaks policies copied from real OpenStack)
- unrestricted: enforcement only works on same HTTP request (bypass via follow-up requests)
- Audit logging: still completely absent

The previous v6 CRITICALs (C1 access rules, C2 legacy_auth, C3 policy engine) are no longer "not implemented" — they ARE implemented. But they have implementation bugs that reduce their security value.

---

## Findings by Severity

### CRITICAL (2) — Security/Correctness Blockers

| # | Package | Finding | Impact |
|---|---------|---------|--------|
| C1 | keystone | unrestricted flag bypass — auth_method not carried in JWT, check only works on same request | Restricted app credentials can create new credentials on subsequent requests |
| C2 | glance | Cache key mismatch — GetImage uses project-scoped key, DeleteImage invalidates unscoped key | Deleted images served from cache for up to 1 hour |

### HIGH (14)

| # | Package | Finding | Impact |
|---|---------|---------|--------|
| H1 | keystone | Access rule `service` field silently ignored | Cross-service access not prevented |
| H2 | keystone | Policy engine infinite recursion on circular rule references | Server panic on malformed policies |
| H3 | keystone | App credential roles not validated against caller's roles | Privilege escalation |
| H4 | nova | Console URL uses request Host, not configured proxy | Console broken in multi-host |
| H5 | nova | Tenant usage ignores start/end params | Billing/usage reports wrong |
| H6 | nova | Server responses missing security_groups array | Horizon SG tab empty, Terraform drift |
| H7 | nova | Missing OS-EXT-SRV-ATTR:root_device_name, launch_index | Horizon extended info blank |
| H8 | nova | GetServer/ListServersDetail missing links array | Terraform canonical URL resolution fails |
| H9 | neutron | N+1 query in ListPorts (per-port SG query) | Performance cliff on large port lists |
| H10 | neutron | RemoveRouterInterface response missing required fields | gophercloud/Terraform state mismatch |
| H11 | neutron | CreateRouter namespace before DB insert, cleanup silently fails | Orphaned namespaces on failure |
| H12 | cinder | availability_zone hardcoded "nova" (FR-012) | Multi-AZ deployments broken |
| H13 | cinder | encrypted hardcoded false (FR-012) | Encrypted volumes report as unencrypted |
| H14 | middleware | Empty JWT secret accepted in production | Auth bypass if secret left blank |

### MEDIUM (14)

| # | Package | Finding |
|---|---------|---------|
| M1 | keystone | Policy parser: and/or equal precedence (breaks standard boolean logic) |
| M2 | keystone | Tokenizer silently drops unrecognized tokens |
| M3 | keystone | Cache key non-deterministic for nested maps |
| M4 | nova | ListServersDetail ORDER BY missing id tiebreaker |
| M5 | nova | Hypervisor statistics hardcoded constants |
| M6 | nova | ForceDeleteInstanceAction bypasses policy engine |
| M7 | nova | UpdateServer returns user_id = projectID (wrong field) |
| M8 | neutron | allocateFloatingIP commits before row insert (TOCTOU) |
| M9 | neutron | DeleteNetwork deletes bridge before DB row |
| M10 | neutron | CreateSecurityGroup default rule insert errors discarded |
| M11 | neutron | ListSecurityGroupRules no pagination |
| M12 | cinder | Pagination uses non-unique created_at (no id tiebreaker) |
| M13 | glance | protected field hardcoded false, not persisted |
| M14 | middleware | RequireProjectScope not in global middleware chain |

### LOW (22)

Not listed individually — mostly: rows.Err() unchecked in edge paths, fmt.Println instead of structured logger, test type mismatches, dead code, cursor tie issues, goroutine leak in tests.

---

## Spec Coverage Analysis

### keystone-minimal-iam-design-v2

| Requirement | v6 Status | v7 Status |
|-------------|-----------|-----------|
| App credential CRUD | WORKING | WORKING |
| App credential authentication | WORKING | WORKING |
| Bcrypt cost 12 | WORKING | WORKING |
| Token revocation (DB-persisted) | WORKING | WORKING |
| Service catalog in token | WORKING | WORKING |
| Access rules (path/method/service) | NOT IMPLEMENTED | PARTIAL (service field ignored) |
| unrestricted flag enforcement | NOT IMPLEMENTED | BROKEN (same-request only) |
| legacy_auth backward compat | NOT IMPLEMENTED | WORKING |
| Policy engine (RBAC evaluation) | NOT IMPLEMENTED | PARTIAL (precedence wrong, no cycle detection) |
| Audit logging (auth events) | NOT IMPLEMENTED | NOT IMPLEMENTED |

**Coverage: ~65%** (up from 45% in v6)

### SPEC-002 (Horizon Full Compatibility)

| Requirement | v6 Status | v7 Status |
|-------------|-----------|-----------|
| FR-001: Token format with catalog | WORKING | WORKING |
| FR-003: Microversion headers | WORKING | WORKING |
| FR-005: Server extended attributes | PARTIAL | PARTIAL (root_device_name, launch_index missing) |
| FR-006: Marker-based pagination | PARTIAL | PARTIAL (id tiebreaker missing in several endpoints) |
| FR-009: Network provider attributes | BROKEN | WORKING (reads from DB now) |
| FR-010: Security group rules in response | PARTIAL | WORKING (Create returns default rules) |
| FR-011: Router interface response | PARTIAL | PARTIAL (Remove still missing fields) |
| FR-012: Volume detail attributes | BROKEN | PARTIAL (volume_type from DB; AZ/encrypted still hardcoded) |
| FR-014: Quota set format | WORKING | WORKING |
| FR-015: Tenant usage with servers | PARTIAL | PARTIAL (flavor correct; time range ignored) |
| FR-020: Hypervisor statistics | PARTIAL | PARTIAL (hardcoded values) |

**Coverage: ~60%** (up from 55% in v6)

---

## What Still Breaks

| Scenario | What Happens | Root Cause |
|----------|--------------|------------|
| Horizon opens VNC console | Wrong URL returned | H4: uses request Host not proxy config |
| Horizon instance detail | Missing SG tab, links | H6, H8: fields absent from response |
| Restricted app cred creates nested cred | Succeeds (should fail) | C1: auth_method not in JWT |
| Admin stores circular policy rules | Server panic | H2: no cycle detection |
| Member creates app cred with admin role | Succeeds (should fail) | H3: roles not validated |
| Delete image then GET | Returns stale data for 1h | C2: cache key mismatch |
| Multi-AZ volume operations | Wrong AZ reported | H12: hardcoded "nova" |
| Encrypted volume display | Shows "not encrypted" | H13: hardcoded false |
| Operator deploys with blank JWT secret | All tokens cross-verifiable | H14: empty string accepted |
| Horizon Usage tab with date filter | Ignores date range | H5: params not applied |

---

## What Now Works (Confirmed v7 Improvements Over v6)

1. **Access rules stored in JWT and enforced by middleware** (path + method matching works)
2. **Legacy auth dual-path with transparent bcrypt upgrade** (timing-safe)
3. **Policy engine evaluates role/user_id/project_id/rule references** (basic rules work)
4. **Policy CRUD API with DB persistence and cache refresh**
5. **Network provider:network_type reads from DB** (was hardcoded "flat")
6. **CreateSecurityGroup returns default egress rules** (was empty)
7. **Bridge creation after DB insert** in CreateNetwork (was before)
8. **allocateIPFromSubnet no longer locks all ports** (scoped to subnet)
9. **Floating IP allocation transactional** (serializable isolation)
10. **Glance owner field present** in image responses

---

## Remediation Priority

### Phase 1: Fix CRITICALs (2-3 hours)
1. **C1**: Add `is_app_credential` + `unrestricted` to JWT claims; check in CreateAppCred — 1.5h
2. **C2**: Fix Glance DeleteImage cache key to include projectID — 10 min

### Phase 2: Fix Security HIGHs (3-4 hours)
1. **H1**: Add service matching to EnforceAccessRules middleware — 30 min
2. **H2**: Add visited-set cycle detection to policy evaluate — 30 min
3. **H3**: Validate requested roles against caller's actual roles — 1h
4. **H14**: Reject empty JWT secret in LoadConfig — 10 min

### Phase 3: Fix Remaining HIGHs (4-5 hours)
1. **H4-H8**: Nova response completeness (console URL from config, security_groups, links, ext attrs) — 3h
2. **H9-H11**: Neutron N+1, RemoveRouterInterface, CreateRouter ordering — 2h
3. **H12-H13**: Cinder AZ/encrypted from DB — 1h

### Phase 4: Fix MEDIUMs (4-6 hours)
- M1-M14: Operator precedence, pagination tiebreakers, policy bypass, etc. — 4-6h

### Effort Summary

| Category | Effort | Impact on Score |
|----------|--------|----------------|
| Fix CRITICALs (C1-C2) | 2-3 hours | 8.0 → 8.2 |
| Fix security HIGHs (H1-H3, H14) | 3-4 hours | 8.2 → 8.5 |
| Fix remaining HIGHs (H4-H13) | 4-5 hours | 8.5 → 8.8 |
| Fix MEDIUMs (M1-M14) | 4-6 hours | 8.8 → 9.0 |
| Implement audit logging | 4-6 hours | 9.0 → 9.2 |
| Full spec compliance (contract tests, remaining FRs) | 15-20 hours | 9.2 → 9.5 |

**Total to reach 9.0/10**: ~18-22 hours (all CRITICALs + HIGHs + MEDIUMs)
**Total to reach 9.5/10**: ~40-50 hours (+ audit logging + contract tests + remaining FR gaps)

---

## Honest Assessment: What C1+C2+C3 Actually Delivered

| Feature | Intended | Actual |
|---------|----------|--------|
| Access rules | Per-service path/method restriction | Path/method only — service field ignored |
| Legacy auth | Seamless upgrade from old credentials | Working correctly |
| Policy engine | oslo.policy-compatible rule evaluation | Works for simple rules; precedence wrong for complex rules; crashes on cycles |
| unrestricted enforcement | Prevent credential escalation | Only works within same HTTP request (bypass trivial) |

**Honest verdict**: Legacy auth is solid. Access rules and policy engine are 80% there but have correctness gaps that reduce their security value. The unrestricted enforcement is effectively broken and needs a JWT-level fix.

---

## Build Status

- `go build ./...` — PASS
- `go vet ./...` — PASS (warnings in test/ only)
- `go test ./internal/...` — PASS (all packages)

---

## Methodology

1. Dispatched 5 per-package Wave 0 agents reading ALL code in each package group
2. Each agent reviewed against three design specifications
3. Every finding verified by reading actual code
4. Compared response shapes (Create vs Get vs List) for completeness
5. Checked for hardcoded values that should be DB/config-backed
6. Verified security features (access rules, policy, unrestricted) end-to-end
