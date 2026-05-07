# O3K Production Readiness Assessment — v4 (Spec Gap Analysis)

**Date**: 2026-05-07
**Review Method**: 5 parallel agents (4 per-package + 1 security), spec-grounded analysis
**Branch**: `fix/production-readiness-security-data` (commit 552aa92)
**Previous Claimed Score**: 7.5/10
**Revised Score**: **5.5/10**

---

## Executive Summary

The previous assessment (7.5/10) was optimistic. When measured against the three design specifications (SPEC-000, SPEC-002, keystone-minimal-iam-design-v2), the actual production readiness is **5.5/10**.

The primary feature of the IAM spec (application credential authentication) is not implemented. Token revocation loses state on restart. Horizon will break on every instance page. Neutron pagination is still OFFSET-based. Multiple security issues remain unresolved.

**Honest assessment**: O3K works for basic Terraform workflows (create/delete VMs, networks, volumes) but fails for:
- CI/CD automation (no app credentials)
- Horizon dashboard (missing server attributes, broken catalog)
- Multi-instance deployment (in-memory revocation, no durable quota)
- Security audit (JWT default secret, host header injection, IDOR in user listing)

---

## Score Revision Rationale

| Previous Claim | Reality | Evidence |
|----------------|---------|----------|
| "Token revocation functional" | Process-local only; restart = all revocations lost | `auth.go:574-585` — `IsTokenRevoked` only checks sync.Map, never queries DB |
| "App credential secrets properly hashed" | True, but auth method not implemented | `handlers.go:201-212` — dispatch returns 400 for app_credential method |
| "Marker-based pagination" | Only Nova ListServers; Neutron still OFFSET | `neutron/ports.go:221`, `network.go:515` |
| "Error format compliant" | True — this fix was verified | `internal/common/errors.go` + test passes |

---

## Findings by Severity

### CRITICAL (6) — Must fix before any production deployment

| # | Finding | Spec Source | File | Impact |
|---|---------|-------------|------|--------|
| C1 | App credential auth method not implemented | keystone-iam-v2 §3.1 | `keystone/handlers.go:201-212` | Terraform service accounts, CI/CD cannot authenticate |
| C2 | `application_credential_roles` table missing | keystone-iam-v2 §4.1 | `keystone/application_credentials.go:82,194,288,395` | All 4 app credential handlers crash at runtime |
| C3 | Token revocation in-memory only | keystone-iam-v2 §2.3 | `keystone/auth.go:574-585` | Revoked tokens regain validity on restart or in multi-instance |
| C4 | Hardcoded DB credentials in committed config | Security baseline | `config/o3k.yaml:2,14` | Credential exposure to anyone with repo access |
| C5 | GetServer missing 6/7 OS-EXT-* attributes | SPEC-002 FR-005 | `nova/handlers.go:870` | Horizon instance detail panel broken |
| C6 | ListServersDetail missing OS-EXT-* attributes | SPEC-002 FR-005 | `nova/handlers.go:711` | Horizon instances table broken |

### HIGH (11) — Blocks specific integrations

| # | Finding | Spec Source | File | Impact |
|---|---------|-------------|------|--------|
| H1 | ValidateToken missing catalog, project, methods, expires_at | SPEC-002 FR-001 | `keystone/handlers.go:246-256` | Horizon service discovery broken |
| H2 | Token roles carry name as "id" field | keystone-iam-v2 §2.1 | `keystone/auth.go:342-344` | Policy engines and role-based UI break |
| H3 | Password hashing bcrypt cost 10, not spec-required 12 | keystone-iam-v2 §5.2 | `keystone/auth.go:605` | Weaker brute-force resistance |
| H4 | JWT secret falls back to insecure default without aborting | Security baseline | `internal/common/config.go:138` | Full auth bypass if deployed with default |
| H5 | Host header reflected without sanitization in catalog | OWASP | `cmd/o3k/main.go:431`, `internal/common/baseurl.go` | SSRF/open redirect |
| H6 | Nova microversion headers missing on all endpoints | SPEC-002 FR-003 | `nova/handlers.go` (all) | Horizon cannot negotiate features |
| H7 | ListServersDetail OFFSET pagination (only ListServers fixed) | SPEC-002 FR-006 | `nova/handlers.go:679` | Pagination unstable under writes |
| H8 | Neutron network response missing provider + router:external | SPEC-002 FR-009 | `neutron/network.go:499-596` | Horizon topology broken |
| H9 | Neutron pagination OFFSET-based; ListSecurityGroups has NONE | SPEC-002 FR-006 | `neutron/ports.go:221`, `network.go:515` | DoS via unbounded queries |
| H10 | IP allocation TOCTOU race (no transaction/lock) | Data integrity | `neutron/ports.go:536-583` | Duplicate IP assignment |
| H11 | ListNetworks returns caller's project_id as tenant_id | Data correctness | `neutron/network.go:578-596` | Wrong ownership for shared networks |

### MEDIUM (19) — Functional but incorrect/incomplete

| # | Package | Issue | Spec |
|---|---------|-------|------|
| M1 | keystone | Token response missing `is_domain` field | SPEC-002 FR-001 |
| M2 | keystone | BuildServiceCatalog uses global DB, not injected | Code quality |
| M3 | keystone | Token revocation doesn't require admin for other users' tokens | keystone-iam-v2 §2.3 |
| M4 | keystone | ListUsers returns all users to any authenticated user | Security (IDOR) |
| M5 | nova | server_usages always empty in tenant-usage | SPEC-002 FR-015 |
| M6 | nova | GetServer conditionally omits image/user_id when NULL | SPEC-002 FR-005 |
| M7 | nova | GetQuotaSet response format non-standard | SPEC-002 FR-014 |
| M8 | nova | Console endpoint mishandles DB errors | Reliability |
| M9 | nova | interface_attach IP allocation broken for non-/24 subnets | Data correctness |
| M10 | neutron | rows.Err() never checked in any iteration loop | Reliability |
| M11 | neutron | Security group responses missing embedded rules | SPEC-002 FR-010 |
| M12 | neutron | SG rule fields use zero values instead of null | API contract |
| M13 | neutron | AddRouterInterface response missing network_id, tenant_id, id | SPEC-002 FR-011 |
| M14 | neutron | floatingIP allocation queries all IPs without subnet filter | Performance |
| M15 | cinder | rows.Err() not checked across all list handlers | Reliability |
| M16 | cinder | ListVolumes (brief) omits status, bootable, AZ | SPEC-002 FR-012 |
| M17 | cinder | Detail responses missing availability_zone, volume_type, encrypted | SPEC-002 FR-012 |
| M18 | glance | rows.Err() not checked across all list handlers | Reliability |
| M19 | glance | DeleteImage scans NULL project_id into bare string (crashes) | Reliability |

### Systemic Issues (affect entire codebase)

| Issue | Locations | Impact |
|-------|-----------|--------|
| `rows.Err()` never checked after `for rows.Next()` loops | ~50+ locations across ALL packages | Truncated results returned as HTTP 200 on DB errors |
| No pagination cap on `?limit=N` parameter | All list endpoints | Single request with `?limit=1000000` exhausts server memory |

---

## Spec Coverage Analysis

### keystone-minimal-iam-design-v2

| Requirement | Status | Notes |
|-------------|--------|-------|
| Application credential CRUD | PARTIAL | Handlers exist but crash (missing table) |
| Application credential authentication | NOT IMPLEMENTED | Dispatch doesn't handle the method |
| Access rules for app credentials | NOT IMPLEMENTED | No code exists |
| Policy engine (basic RBAC) | PARTIAL | is_admin flag set but no policy evaluation |
| Token revocation (durable) | BROKEN | DB table exists but never queried on validation |
| bcrypt cost 12 | PARTIAL | App creds use 12, user passwords use 10 |
| Service catalog in token | PARTIAL | Present in auth response, missing in validate |
| Role UUID in token | BROKEN | Name used as ID |

**Coverage: ~30%** of spec requirements implemented correctly.

### SPEC-002 (Horizon Full Compatibility)

| Requirement | Status | Notes |
|-------------|--------|-------|
| FR-001: Token format with catalog | PARTIAL | Missing in ValidateToken |
| FR-003: Microversion headers | NOT IMPLEMENTED | No headers on any response |
| FR-005: Server extended attributes | NOT IMPLEMENTED | Only power_state present |
| FR-006: Marker-based pagination | PARTIAL | Only Nova ListServers fixed |
| FR-009: Network provider attributes | NOT IMPLEMENTED | Missing from responses |
| FR-010: Security group rules in response | NOT IMPLEMENTED | Empty rules array |
| FR-011: Router interface response | INCOMPLETE | Missing 3 fields |
| FR-012: Volume detail attributes | INCOMPLETE | Missing 3 fields |
| FR-014: Quota set format | INCORRECT | Non-standard flat format |
| FR-015: Tenant usage with servers | BROKEN | Always empty |

**Coverage: ~25%** of Horizon-compatibility requirements met.

### SPEC-000 (Compliance Addendum)

| Requirement | Status |
|-------------|--------|
| Contract test suite | NOT STARTED |
| Terraform test suite | NOT STARTED |
| CLI test suite | Partial (bash scripts, not automated) |
| Schema validation | NOT STARTED |
| Zero failures policy | Cannot evaluate without test suites |

**Coverage: ~10%** — compliance testing infrastructure doesn't exist.

---

## Remediation Priority (Effort Estimates)

### Phase 1: Stop the crashes (2-3 hours)
1. Add `application_credential_roles` migration (C2) — 15 min
2. Fix rows.Err() in critical paths (keystone, nova list) — 90 min
3. Fix DeleteImage NULL scan crash (M19) — 10 min
4. Add pagination cap (max 1000) to all list endpoints — 45 min

### Phase 2: Security (2-3 hours)
1. Remove hardcoded credentials from config (C4) — 30 min
2. Abort on default JWT secret (H4) — 15 min
3. Sanitize Host header in catalog (H5) — 30 min
4. Require admin for ListUsers / other-user token revocation (M3, M4) — 45 min
5. Upgrade user password bcrypt to cost 12 (H3) — 15 min
6. IP allocation transaction + row lock (H10) — 60 min

### Phase 3: Horizon compatibility (4-6 hours)
1. Add OS-EXT-* attributes to GetServer/ListServersDetail (C5, C6) — 60 min
2. Add microversion headers to all Nova endpoints (H6) — 45 min
3. Fix ValidateToken response (catalog, roles UUID, is_domain) (H1, H2, M1) — 60 min
4. Neutron marker-based pagination (H9) — 90 min
5. Network provider + router:external fields (H8) — 30 min
6. Security group embedded rules (M11) — 45 min
7. Cinder volume detail attributes (M16, M17) — 30 min

### Phase 4: App credential auth (3-4 hours)
1. Implement `application_credential` auth method dispatch (C1) — 90 min
2. Wire token revocation to query DB on validation (C3) — 60 min
3. Add access rules storage and enforcement — 90 min

### Phase 5: Full rows.Err() pass (2-3 hours)
1. Systematic fix across all ~50 locations — 2-3 hours

**Total estimated effort to reach genuine 8/10: 15-20 hours**

---

## Score Progression (Honest)

```
Initial review:           3/10 (30 blockers, 10 P0 security)
After fix pass #1:        5/10 (14 blockers, 6 security + 4 data + 6 infra)
After fix pass #2:        5.5/10 (5 immediate fixes)
After fix pass #3:        6/10 (14 more fixes: error format, pagination, security)
This assessment (v4):     5.5/10 (revised down — spec coverage measured honestly)
After Phase 1+2:          7/10 (projected — crashes + security resolved)
After Phase 3+4:          8/10 (projected — Horizon + app creds working)
After Phase 5:            8.5/10 (projected — reliability complete)
```

**Why the downward revision**: Previous scores counted "partially implemented" as done. When measured against spec requirements (what Horizon/Terraform/CLI actually need), partial implementations that return wrong data or crash are worse than not implementing at all — they create false confidence.

---

## What Actually Works Today

- Basic password authentication (user/password → JWT token)
- Project-scoped token issuance
- VM CRUD lifecycle (create, list, show, delete) in stub mode
- Network/subnet/port CRUD (basic operations)
- Volume CRUD (basic operations)
- Image metadata CRUD
- OpenStack-compliant error response format
- Marker-based pagination on Nova ListServers (one endpoint)
- App credential CRUD (create, list, show, delete) — if migration added
- Redis cache with prefix scoping
- Heartbeat shutdown safety

---

## What Breaks in Production

| Scenario | What Happens | Root Cause |
|----------|--------------|------------|
| CI/CD authenticates with app credential | 400 Bad Request | C1: method not dispatched |
| Admin revokes token, restarts server | Revoked token works again | C3: in-memory only |
| Horizon loads instance list | Empty columns for task_state, AZ | C5/C6: attributes missing |
| Horizon negotiates microversions | Falls back to minimum version | H6: no headers |
| 1000+ VMs exist, paginate | Inconsistent results | H7/H9: OFFSET pagination |
| Two VMs request IP simultaneously | Duplicate IP assigned | H10: no lock |
| Attacker sends crafted Host header | SSRF via service catalog | H5: no sanitization |
| Fresh install with app credential code | Crash on first API call | C2: missing table |
| DB hiccup during list query | Partial results returned as 200 OK | Systemic: no rows.Err() |

---

## Build Status

- `go build ./...` — PASS
- `go vet ./...` — PASS
- Unit tests — PASS (but coverage is low and doesn't test the gaps above)
- Contract tests — DO NOT EXIST (SPEC-000 requirement)

---

## Methodology

1. Read all three design specifications in full
2. Dispatched 5 parallel review agents with spec excerpts as context
3. Each agent read ALL files in its assigned package(s)
4. Security agent performed cross-cutting analysis
5. Findings deduplicated across agents (55 unique findings)
6. Every finding traced to specific spec requirement or security baseline
7. No assumptions made — every claim references a file:line
