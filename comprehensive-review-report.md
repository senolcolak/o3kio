# O3K Production Readiness Assessment

**Date**: 2026-05-07
**Review Method**: Three-wave comprehensive review (Wave 0: 25 per-package agents, Wave 1: 11 cross-cutting agents)
**Verdict**: **NOT PRODUCTION READY** — 30 blocking issues across security, data correctness, and infrastructure reliability

---

## Executive Summary

O3K is a well-structured Go codebase with strong foundations (single-binary design, clean service separation, good CI/CD pipeline, rich contract tests). However, it has critical gaps in **authentication/authorization**, **data correctness under concurrency**, and **multi-tenant isolation** that make it unsuitable for any deployment handling real tenant data.

**Production Readiness Score: 3/10**

| Category | Score | Blockers |
|----------|-------|----------|
| Security & Auth | 2/10 | 10 critical issues |
| Data Correctness | 3/10 | 8 critical issues |
| Infrastructure Reliability | 4/10 | 7 critical issues |
| API Compatibility | 5/10 | Pagination, state machines |
| Code Quality | 6/10 | Good structure, some patterns |
| CI/CD & Testing | 6/10 | Good contract tests, weak unit tests |
| Documentation | 5/10 | CLAUDE.md stale, config mismatches |

---

## P0 — Security Blockers (Fix Before ANY Deployment)

| # | Issue | Location | Impact |
|---|-------|----------|--------|
| 1 | **No auth on Keystone admin port** | `cmd/o3k/main.go:409-442` | Full admin API unauthenticated |
| 2 | **No RBAC on Keystone write ops** | `internal/keystone/handlers.go` | Any user escalates to admin |
| 3 | **JWT secret in source control** | `config/o3k.yaml:11` | Attacker forges any token |
| 4 | **DB password in source control** | `config/o3k.yaml:2` | Direct database access |
| 5 | **Metadata X-Instance-ID bypass** | `internal/metadata/service.go:78` | Any client reads any instance's secrets |
| 6 | **App credentials stored plaintext** | `internal/keystone/application_credentials.go:134` | DB read = all secrets |
| 7 | **Token revocation is a no-op** | `internal/keystone/handlers.go:237` | Compromised tokens irrevocable for 24h |
| 8 | **Cross-tenant metadata access** | `internal/nova/handlers.go:1671` | Project A reads Project B's data |
| 9 | **Cross-tenant usage disclosure** | `internal/nova/tenant_usage.go:14` | Any user sees all projects' usage |
| 10 | **Public image ownership broken** | `internal/glance/images.go:209` | Any user deletes public images |

---

## P1 — Data Correctness Blockers

| # | Issue | Location | Impact |
|---|-------|----------|--------|
| 11 | **Metadata SQL column mismatches** | `internal/metadata/service.go:131,182,211` | Cloud-init completely broken |
| 12 | **Quota TOCTOU race** | `internal/nova/quotas.go:237` | Unlimited resource creation |
| 13 | **Volume double-attach race** | `internal/nova/volume_attachment.go:60` | Two VMs attach same volume |
| 14 | **Resize loses old flavor** | `internal/nova/advanced_actions.go:329` | Revert resize is broken |
| 15 | **Pagination skips resources** | All list endpoints | Same-timestamp items dropped |
| 16 | **Scheduler errors discarded** | `internal/scheduler/worker.go:149` | Tasks stuck, capacity locked |
| 17 | **err == pgx.ErrNoRows** | 122 call sites | 500 instead of 404 if error wraps |
| 18 | **Stub BUILD never ACTIVE** | `internal/nova/handlers.go:552` | Terraform times out |

---

## P2 — Infrastructure Reliability Blockers

| # | Issue | Location | Impact |
|---|-------|----------|--------|
| 19 | **ceph_rbd.go won't compile** | `pkg/storage/ceph_rbd.go` | No Ceph backend in production |
| 20 | **No context to libvirt** | `pkg/hypervisor/libvirt.go:111` | Infinite hang on libvirt failure |
| 21 | **FlushAll ignores prefix** | `pkg/cache/cache.go:176` | Nukes entire Redis |
| 22 | **IP allocation collision** | `internal/neutron/ports.go:73` | Duplicate IPs |
| 23 | **StopHeartbeat double-close** | `internal/compute/node_registry.go:124` | Process crash |
| 24 | **Silent unnetworked VM** | `internal/nova/handlers.go:439` | VM created without network |
| 25 | **S3 errors = "not found"** | `pkg/storage/image_store.go:557` | Auth failures masked |

---

## Cross-Cutting Patterns Requiring Systematic Fix

1. **`rows.Err()` never checked** — 20+ query loops silently return partial data
2. **`err == pgx.ErrNoRows` not `errors.Is`** — 122 fragile sites
3. **No context.Context to exec.Command** — networking, hypervisor, metadata
4. **Unchecked type assertions** — 4+ panic sites
5. **fmt.Printf for logging** — 30+ sites bypass zerolog
6. **Zero unit tests** — metadata, placement, storage, cache packages
7. **`interface{}` not `any`** — 50+ sites (Go 1.26 target)

---

## What's Working Well

- Single-binary architecture with clean service separation
- Gin HTTP framework properly structured
- 84 gophercloud contract tests for API compatibility
- Multi-arch CI/CD with Docker images and release automation
- 62 sequential database migrations with up/down pairs
- Stub mode for safe macOS development
- Detailed, current CHANGELOG

---

## Recommended Fix Priority

### Sprint 1: Security (1-2 days)
1. Add AuthMiddleware + RequireRole("admin") to Keystone router
2. Remove secrets from config — require env vars
3. Add project_id filter to all cross-tenant queries
4. Remove X-Instance-ID bypass or gate on test mode
5. Hash app credential secrets with bcrypt

### Sprint 2: Data Correctness (2-3 days)
6. Fix metadata SQL columns (fixed_ips, meta_key, userdata)
7. Atomic quota enforcement (UPDATE WHERE used + N <= limit)
8. Conditional volume attach (WHERE status = 'available')
9. Store old_flavor_id for resize revert
10. Composite cursor pagination (created_at, id)
11. Error handling in scheduler state writes

### Sprint 3: Infrastructure (1-2 days)
12. Fix ceph_rbd.go type system
13. Context timeout on libvirt operations
14. FlushAll → DeletePattern("*")
15. Proper IPAM for IP allocation
16. sync.Once on StopHeartbeat
17. Fail CreateServer if port allocation fails

---

## Methodology

- **Wave 0**: 25 per-package agents read ALL code in each package
- **Wave 1**: 11 cross-cutting agents (security, silent-failures, architecture, business-logic, type-design, code-quality, test-analyzer, comment-analyzer, language-specialist, docs-validator, ADR-compliance)
- **Total agents dispatched**: 36
- **Unique findings (deduplicated)**: ~85
- **Production blockers**: 30
