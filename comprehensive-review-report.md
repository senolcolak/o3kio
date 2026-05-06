# O3K Production Readiness Assessment — v2 (Post-Fix)

**Date**: 2026-05-07
**Review Method**: Focused reassessment (4 specialized agents: security, business-logic, infrastructure, API/concurrency)
**Branch**: `fix/production-readiness-security-data` (commit 8a1f3ee)
**Previous Score**: 3/10 (30 blockers)
**Current Verdict**: **NOT PRODUCTION READY** — 14 remaining blockers (down from 30)

---

## Executive Summary

The security and data correctness fix pass addressed 18 of the original 30 blockers. The codebase has improved from 3/10 to **5/10** overall. The most dangerous unauthenticated-admin and cross-tenant-read paths are closed. However, 5 HIGH-severity security issues remain (including 2 NEW IDOR vulnerabilities introduced by the fix), the resize operation has a new CRITICAL atomicity bug, and all 7 infrastructure reliability issues persist.

**Production Readiness Score: 5/10** (was 3/10)

| Category | Previous | Current | Change | Remaining Blockers |
|----------|----------|---------|--------|-------------------|
| Security & Auth | 2/10 | 4/10 | +2 | 5 HIGH + 1 MEDIUM |
| Data Correctness | 3/10 | 6/10 | +3 | 1 CRITICAL + 3 HIGH |
| Infrastructure Reliability | 4/10 | 4.5/10 | +0.5 | 6 confirmed issues |
| API Compatibility | 5/10 | 7/10 | +2 | 1 contract break introduced |
| Concurrency | — | 8/10 | new | Fix code is mostly sound |
| Code Quality | 6/10 | 6/10 | — | 94 rows.Err() missing, 119 err== sites |

---

## What Was Fixed (18 issues resolved)

| # | Issue | Fix Quality |
|---|-------|-------------|
| 1 | No auth on Keystone admin port | SOLID — AuthMiddleware wired globally |
| 2 | No RBAC on Keystone write ops | WEAK — admin gate works but ChangePassword/AppCreds exposed |
| 5 | Metadata X-Instance-ID bypass | SOLID — gated on testMode=stub |
| 8 | Cross-tenant metadata access | SOLID — IP-based lookup in production |
| 9 | Cross-tenant usage disclosure | SOLID — admin check with project_id filter |
| 11 | Metadata SQL column mismatches | CORRECT — meta_key, meta_value, fixed_ips JSONB |
| 12 | Quota TOCTOU race | PARTIAL — process-local mutex (single-node only) |
| 13 | Volume double-attach race | CORRECT — conditional UPDATE WHERE status='available' |
| 14 | Resize loses old flavor | PARTIAL — stores old_flavor_id but non-atomic |
| 16 | Scheduler errors discarded | PARTIAL — zerolog added, one fmt.Printf remains |
| 17 | err == pgx.ErrNoRows | PARTIAL — error_helpers.go fixed, 119 sites remain |
| 18 | Stub BUILD never ACTIVE | CORRECT — goroutine transition with guard |

---

## P0 — Remaining Security Blockers

| # | Issue | Severity | Status |
|---|-------|----------|--------|
| NEW | **IDOR: ChangePassword allows cross-user password change** | HIGH | Any user can change any other user's password (if they know current pw) |
| NEW | **IDOR: Application credential management for any user** | HIGH | Any user can create/delete credentials for any other user |
| 4 | **DB password in source control** | HIGH | `config/o3k.yaml` still has `secret` password |
| 6 | **App credentials stored plaintext** | HIGH | `secret_hash` column stores raw base64, not bcrypt |
| 7 | **Token revocation is a no-op** | HIGH | `DELETE /v3/auth/tokens` returns 204 but does nothing |
| 10 | **Public image ownership broken** | MEDIUM | Any user can delete/modify public images |

### New IDOR Vulnerabilities (introduced by fix)

The RBAC fix correctly gated admin-only routes but left `ChangePassword` and application credential endpoints on the non-admin group. These are exploitable by any authenticated user:

```
POST /v3/users/<victim-id>/password          — change any user's password
POST /v3/users/<victim-id>/application_credentials — create creds for any user
DELETE /v3/users/<victim-id>/application_credentials/<id> — delete any user's creds
```

**Fix**: Add ownership check (`token.user_id == URL :id || isAdmin`).

---

## P1 — Data Correctness Blockers

| # | Issue | Severity | Impact |
|---|-------|----------|--------|
| NEW | **Resize flavor/metadata non-atomic** | CRITICAL | Process crash between UPDATE and metadata INSERT = irreversible state |
| 12 | **Quota enforcement process-local only** | HIGH | Multi-instance deployments can over-provision |
| 15 | **Pagination skips resources** | HIGH | OFFSET-based pagination unstable under concurrent inserts |
| NEW | **ResetServerMetadata missing project_id** | MEDIUM | Cross-tenant metadata mutation |
| NEW | **ConfirmResize leaks _old_flavor_id** | LOW | Metadata pollution visible to cloud-init |

---

## P2 — Infrastructure Reliability (unchanged)

| # | Issue | Line | Impact | Effort |
|---|-------|------|--------|--------|
| 19 | ceph_rbd.go won't compile | — | **RESOLVED** (build tag) | — |
| 20 | No context to libvirt | 117 | MEDIUM | TRIVIAL |
| 21 | FlushAll ignores prefix | 181 | HIGH | TRIVIAL |
| 22 | IP allocation collision | 527 | HIGH | SMALL |
| 23 | StopHeartbeat double-close | 125 | HIGH | SMALL |
| 24 | Silent unnetworked VM | 465 | MEDIUM | TRIVIAL |
| 25 | S3 errors = "not found" | 557 | HIGH | TRIVIAL |

Note: Issue #19 (ceph_rbd) was a false positive — it compiles correctly with `//go:build ceph` tag.

---

## Cross-Cutting Technical Debt

| Pattern | Sites | Impact |
|---------|-------|--------|
| `rows.Err()` never checked after query loops | 94 | HIGH — silent partial results |
| `err == pgx.ErrNoRows` direct comparison | 119 | HIGH — breaks under error wrapping |
| `exec.Command` without context | 46 | MEDIUM — un-cancellable subprocesses |
| `fmt.Printf` instead of zerolog | 35 | LOW — breaks log aggregation |
| Unchecked type assertions | 5 | HIGH — panic sites in prod paths |

---

## API Contract Issue (introduced by fix)

`AttachVolume` now returns 409 for all failure cases (volume not found, wrong project, already in-use). OpenStack API requires:
- Missing volume → 404
- Already in-use → 400 (`InvalidVolumeAttachment`)

Terraform's OpenStack provider checks for 404 and will retry incorrectly on 409.

---

## Recommended Next Fix Priority

### Immediate (before merge) — 2-3 hours

1. **IDOR fixes** — Add ownership check to `ChangePassword`, `CreateApplicationCredential`, `DeleteApplicationCredential` (15 min)
2. **Resize atomicity** — Wrap flavor UPDATE + metadata INSERT in a transaction (15 min)
3. **AttachVolume status codes** — Return 404 for missing volume, 400 for in-use (10 min)
4. **ResetServerMetadata project_id** — Add `AND project_id = $2` (5 min)
5. **ConfirmResize cleanup** — Delete `_old_flavor_id` metadata on confirm (5 min)

### Sprint 2 — Security hardening (1 day)

6. Hash app credential secrets with bcrypt
7. Remove DB password from config, enforce env var
8. Implement token denylist (Redis or DB table with TTL)
9. Fix public image delete/update ownership check

### Sprint 3 — Infrastructure (1 day)

10. FlushAll → DeletePattern with prefix
11. StopHeartbeat → sync.Once
12. IP allocation → SELECT FOR UPDATE with uniqueness check
13. Silent unnetworked VM → fail CreateServer on port error
14. S3 error propagation in imageExistsS3

### Sprint 4 — Technical debt (2-3 days)

15. Add `rows.Err()` checks to all 94 query loops
16. Convert 119 `err == pgx.ErrNoRows` to `errors.Is`
17. Add context to 46 exec.Command calls
18. Convert 35 fmt.Printf to zerolog
19. Fix 5 unchecked type assertions

---

## Score Progression

```
Initial review:  3/10 (30 blockers, 10 P0 security)
After fix pass:  5/10 (14 blockers, 6 security + 4 data + 6 infra)
After Sprint 1:  6/10 (projected — closes IDORs, atomicity, API contract)
After Sprint 2:  7/10 (projected — closes remaining security)
After Sprint 3:  8/10 (projected — closes infrastructure)
After Sprint 4:  9/10 (projected — closes technical debt)
```

---

## What's Working Well

- Single-binary architecture with clean service separation
- Auth middleware framework is sound (JWT, bcrypt, parameterized queries)
- Volume attachment atomic claim pattern is correct and production-grade
- Cross-tenant isolation on nova/metadata/usage endpoints is solid
- 84 gophercloud contract tests for API compatibility
- Build passes cleanly; go vet clean on production code
- Stub mode correctly isolated from production code paths

---

## Methodology

- **Security agent**: Reviewed all 7 auth-related files, rated each fix, identified 2 new IDOR vulnerabilities
- **Business logic agent**: Verified all 8 data fixes, found resize atomicity CRITICAL bug
- **Infrastructure agent**: Confirmed 6 of 7 P2 issues persist (1 was false positive), quantified cross-cutting debt
- **API/Concurrency agent**: Verified fix concurrency is sound, found 1 API contract break
