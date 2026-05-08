# O3K API Coverage Report

**Date**: 2026-05-07 (revised)
**Status**: 342 routes registered — ~40% fidelity

## What "342 Endpoints" Actually Means

O3K has **342 HTTP route handlers registered** across 5 services. This means the routes exist and return JSON responses. It does NOT mean they behave identically to OpenStack.

**Route registered ≠ fully implemented.** A fully-implemented endpoint means:
- Correct response schema (all fields OpenStack clients expect)
- Query filter support (status, name, project_id, etc.)
- State machine validation (returns 409 Conflict for invalid operations)
- Proper error codes (404, 409, 413, not generic 500)
- Pagination with `links` object
- Microversion-aware behavior

## Honest Coverage by Service

| Service | Routes | CRUD Works | Filters Work | Full Schema | Fidelity |
|---------|--------|-----------|--------------|-------------|----------|
| Keystone | 61 | Yes | No | ~50% | ~50% |
| Nova | 72 | Yes | No | ~40% | ~40% |
| Neutron | 98 | Yes | No | ~45% | ~45% |
| Cinder | 73 | Yes | No | ~35% | ~35% |
| Glance | 38 | Yes | No | ~40% | ~40% |
| **Total** | **342** | **Yes** | **No** | **~42%** | **~40%** |

## What Works

- Create/list/show/delete for all primary resources
- Keystone password authentication → JWT token
- Service catalog in token response (basic)
- Nova: server create/delete, flavor CRUD, keypair CRUD
- Neutron: network/subnet/port/router/security-group CRUD
- Cinder: volume/snapshot CRUD, basic attach/detach
- Glance: image metadata + binary upload/download

## What Does NOT Work

### Missing Across All Services
- **Query filters**: No list endpoint respects query parameters (status, name, etc.)
- **Response fields**: 30-50% of expected fields missing per service
- **Pagination links**: No `links` object in list responses
- **State validation**: Operations succeed in invalid states (e.g., delete attached volume)
- **Error codes**: Many errors return wrong HTTP status codes

### Per-Service Gaps

**Keystone**:
- No domain-scoped tokens
- Token response missing full role/catalog detail objects
- No regions endpoint
- No credential management (functional)
- RBAC not enforced on write operations

**Nova**:
- Advertises microversion 2.93, implements ~2.60
- Server actions missing (resize, migrate, shelve, evacuate, rebuild)
- No server groups, availability zones, quotas
- No flavor embedding in server response (v2.47+)
- Missing OS-EXT-* namespace fields

**Neutron**:
- `router:external` always false (floating IPs can't work)
- No port binding fields (breaks Horizon network tab)
- No allowed_address_pairs, QoS, subnet pools
- IPv6 handling has panic on edge cases

**Cinder**:
- Routes not under `/v3/:project_id` (breaks standard clients)
- No volume type management
- No state machine (delete attached volume succeeds)
- Attachment missing `connection_info`

**Glance**:
- Tags silently dropped (accepted but never stored)
- No `protected` column
- No checksum computation on upload
- Image import workflows missing
- Image members (sharing) missing

## Comparison to Real OpenStack

| Aspect | Real OpenStack | O3K |
|--------|---------------|-----|
| Routes registered | ~330 | 342 |
| Routes fully functional | ~330 | ~140 |
| Query filters work | Yes | No |
| Complete response schemas | Yes | ~40% |
| State machine validation | Yes | No |
| RBAC enforcement | Yes | Partial |
| Horizon full compatibility | Yes | Partial |
| Terraform full compatibility | Yes | Partial |

## Conclusion

O3K has registered routes for all core OpenStack endpoints. The basic CRUD happy path works for demonstration purposes. However, the implementation lacks the depth needed for production use — missing filters, incomplete schemas, no state validation, and security gaps.

**For production readiness**, the following must be completed:
1. Response schema completeness (all fields clients expect)
2. Query filter implementation (all list endpoints)
3. State machine validation (all stateful resources)
4. Security hardening (RBAC, auth bypass fixes)
5. Error code correctness (HTTP status codes)

---

**Revised**: 2026-05-07
