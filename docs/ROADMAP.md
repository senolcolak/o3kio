# O3K Gap Closure Roadmap

**Date**: 2026-05-08
**Based on**: Comprehensive Review v3 (Wave 0 + Wave 1) + code-level verification by 3 research agents
**Goal**: Close the gap between current implementation (~40% fidelity) and SPEC-000 compliance (~90% fidelity)

---

## Corrections to Original Review

The research agents found several claims from the review were **inaccurate or already mitigated**:

| Original Claim | Reality |
|---------------|---------|
| "No RBAC on Keystone writes" | `RequireRole("admin")` already enforced on all write endpoints |
| "App credentials stored plaintext" | Already bcrypt-hashed at cost 12 |
| "Token revocation process-local only" | Persisted to `revoked_tokens` table; startup preload missing |
| "JWT secret committed to source" | Guard in config.go exits process if default used in production |
| "Cinder routes not under /v3/:project_id" | Dual-registration exists; both paths work |
| "Nova resize/migrate/shelve missing" | All implemented with state validation |
| "Nova server groups missing" | Fully implemented |
| "Nova availability zones missing" | Real implementation querying host_aggregates |
| "Nova quotas missing" | Fully implemented |
| "Cinder volume types missing" | Fully implemented |
| "Cinder volume transfer missing" | Fully implemented |

**Revised finding count**: ~85 real issues (down from 155 originally reported).

---

## Phase 1: Security & Safety (10 hours)

Priority: Must fix before any non-localhost deployment.

| # | Issue | File:Line | Fix | Hours |
|---|-------|-----------|-----|-------|
| 1 | Metadata X-Instance-ID bypass in stub mode | `internal/metadata/service.go:82-99` | Remove `testMode` + header bypass entirely; IP-lookup works in stub mode | 1h |
| 2 | No HTTP server timeouts (7 servers) | `cmd/o3k/main.go:455-584` | Add ReadTimeout=30s, WriteTimeout=60s, IdleTimeout=120s; Glance gets 10min WriteTimeout for uploads | 1h |
| 3 | libvirt real methods skip mutex | `pkg/hypervisor/libvirt.go` (10 methods) | Add `m.mu.Lock()/defer m.mu.Unlock()` to all `*Real` methods | 1h |
| 4 | Unchecked type assertions (~15 sites) | `internal/cinder/volumes.go:973`, `internal/middleware/logging.go:168`, others | Add comma-ok pattern to all bare assertions | 2h |
| 5 | `err == pgx.ErrNoRows` (123 sites) | All services | `sed` to `errors.Is(err, pgx.ErrNoRows)` + add imports | 2h |
| 6 | Token revocation startup preload + timeout | `internal/keystone/auth.go:813-872` | Load non-expired tokens from DB into sync.Map on init; add 5s timeout to slow-path query | 1.5h |
| 7 | 3 Keystone GET endpoints leak cross-project data | `internal/keystone/handlers.go:583-605` | Add project_id ownership check to GetProject, similar for GetUser/GetDomain | 1h |
| 8 | Tighten JWT secret env check | `internal/common/config.go:193-200` | Make unset `O3K_ENV` behave as production (already mostly correct, verify edge case) | 0.5h |

**Total: 10 hours. No new migrations needed. No dependencies between items.**

---

## Phase 2: Response Schema Completeness (15.5 hours, 3 migrations)

Priority: Make OpenStack clients stop crashing on nil fields.

### Nova (4.5h + 1 migration)

| Field | Location | Fix |
|-------|----------|-----|
| `key_name` | `handlers.go:959` | Add to SELECT + response; needs column check |
| `metadata` inline | `handlers.go:959` | Second query to `instance_metadata` table |
| `fault` object (when ERROR) | `handlers.go:959` | Read `fault_message`; need migration for column |
| `config_drive` | `handlers.go:959` | Return `""` (constant) |
| `progress` | `handlers.go:959` | Return `0` for ACTIVE, `25`/`50` for BUILD |
| `locked` | `handlers.go:959` | Column exists, add to response |
| `description` | `handlers.go:959` | Return `null` (no column yet, constant) |
| `flavor` embedded (v2.47+) | `handlers.go:983` | Query flavor table, return full `{vcpus, ram, disk, ...}` |

**Migration**: Add `fault_message TEXT` to instances table (may already exist from prior code — verify).

### Neutron (2.5h + 1 migration)

| Field | Location | Fix |
|-------|----------|-----|
| `binding:vif_type` | `ports.go:418` | Return `"ovs"` (constant) |
| `binding:vnic_type` | `ports.go:418` | Return `"normal"` (constant) |
| `binding:host_id` | `ports.go:418` | New column `binding_host_id`, populated by Nova on port bind |
| `binding:vif_details` | `ports.go:418` | Return `{"port_filter": true}` (constant) |
| `binding:profile` | `ports.go:418` | Return `{}` (constant) |
| `port_security_enabled` | `ports.go:418` | Return `true` (constant, default behavior) |
| `allowed_address_pairs` | `ports.go:418` | Return `[]` (constant for now) |
| `project_id` | `ports.go:418` | Alias of existing `tenant_id` |

**Migration**: Add `binding_host_id TEXT` to ports table.

### Cinder (3.5h, no new migrations)

| Field | Location | Fix |
|-------|----------|-----|
| `description` | `volumes.go:546` | Column exists; add to SELECT + response |
| `user_id` | `volumes.go:546` | Column exists; add to response |
| `multiattach` | `volumes.go:546` | Return `false` (constant) |
| `replication_status` | `volumes.go:546` | Return `"disabled"` (constant) |
| `migration_status` | `volumes.go:546` | Return `null` (constant) |
| `links` | `volumes.go:546` | Build `[{rel:self, href:...}]` |
| `metadata` inline | `volumes.go:546` | Second query to `volume_metadata` table |
| `attachments` complete | `volumes.go:546` | Add `attachment_id`, `host_name`, `volume_id`, `id` to attachment objects |

### Glance (3h + 1 migration)

| Field | Location | Fix |
|-------|----------|-----|
| `protected` persisted | `images.go:390` | Add column to images table; read/write in handlers |
| Tags on CreateImage | `images.go:142` | Insert into `image_tags` table (currently silently dropped) |
| Checksum on upload | `images.go:586` | Compute MD5 during stream copy, store in `checksum` column |
| `locations` | `images.go:390` | Return `[]` (constant) |
| `virtual_size` | `images.go:390` | Return `null` (constant) |

**Migration**: Add `protected BOOLEAN DEFAULT false` to images table.

### Keystone (2h, no migrations)

| Field | Location | Fix |
|-------|----------|-----|
| `audit_ids` | `auth.go:313` | Return `[base64(random 16 bytes)]` |
| `is_domain` | `auth.go:313` | Return `false` |
| Catalog internal/admin interfaces | `auth.go` BuildServiceCatalog fallback | Add `internal` and `admin` interface entries |

---

## Phase 3: Query Filter Implementation (15.5 hours)

Priority: Make list endpoints usable for Horizon, CLI, and Terraform.

| Endpoint | File:Line | Filters Needed | Hours |
|----------|-----------|----------------|-------|
| Nova ListServers / ListServersDetail | `handlers.go:611,682` | status, name, image, flavor, ip, changes-since, all_tenants, host | 3h |
| Neutron ListNetworks | `network.go:522` | name, status, admin_state_up, shared, router:external | 2h |
| Neutron ListPorts | `ports.go:221` | network_id, device_id, device_owner, status, mac_address | 2h |
| Neutron ListSubnets | `network.go:905` | network_id, name, cidr, ip_version | 1.5h |
| Cinder ListVolumes | `volumes.go:307` | status, name, bootable, availability_zone, volume_type, all_tenants | 2h |
| Glance ListImages | `images.go:240` | name, status, visibility, disk_format, container_format, tag, owner | 3h |
| Keystone ListUsers | `handlers.go:443` | name, enabled, domain_id | 1h |
| Keystone ListProjects | `handlers.go:528` | name, enabled, domain_id | 1h |

**Pattern**: Each endpoint already has a `WHERE project_id = $1` clause. Filters are appended as additional `AND field = $N` conditions with parameter binding. This is mechanical but requires careful SQL escaping.

---

## Phase 4: State Machine Validation (5 hours)

Priority: Prevent data corruption from operations in invalid states.

| Resource | Operation | File:Line | Required State | Hours |
|----------|-----------|-----------|----------------|-------|
| Nova Server | Delete | `handlers.go:1000` | Not BUILD (force-delete required) | 1h |
| Nova Server | os-stop | `handlers.go:1283` | Must be ACTIVE | 0.5h |
| Nova Server | os-start | `handlers.go:1291` | Must be SHUTOFF | 0.5h |
| Cinder Volume | os-attach | `volumes.go:659` | Must be `available` | 1h |
| Cinder Volume | os-detach | `volumes.go:679` | Must be `in-use` | 0.5h |
| Cinder Volume | os-extend | `volumes.go:732` | Must be `available` | 0.5h |
| Glance Image | Delete | `images.go:471` | Check `protected != true` | 1h |

---

## Phase 5: Missing Critical Functionality (6 hours, 1 migration)

Priority: Close the most impactful remaining gaps.

| Item | File | What's Needed | Hours |
|------|------|---------------|-------|
| Neutron `router:external` field | `network.go:503,585,667` | Add `is_external` boolean column; expose in create/update/get | 3h |
| Neutron `allowed_address_pairs` | `ports.go:107,201` | Add JSONB column; parse in create/update; return in response | 3h |

**Migration**: Add `is_external BOOLEAN DEFAULT false` to networks table, `allowed_address_pairs JSONB DEFAULT '[]'` to ports table.

**Not needed** (already implemented — confirmed by research):
- Nova resize/confirm/revert ✓
- Nova migrate/live_migrate ✓
- Nova shelve/unshelve ✓
- Nova server groups ✓
- Nova availability zones ✓
- Nova quotas ✓
- Cinder volume types ✓
- Cinder volume transfer ✓

---

## Summary

| Phase | Scope | Hours | Migrations | Result |
|-------|-------|-------|------------|--------|
| 1 | Security & Safety | 10h | 0 | Safe to deploy outside localhost |
| 2 | Response Schemas | 15.5h | 3 | Clients stop crashing on nil fields |
| 3 | Query Filters | 15.5h | 0 | List endpoints usable by Horizon/CLI/Terraform |
| 4 | State Machines | 5h | 0 | Operations reject invalid states (409 Conflict) |
| 5 | Missing Functionality | 6h | 1 | Floating IPs work; port security works |
| **Total** | | **52h** | **4** | **~75% fidelity (up from ~40%)** |

---

## What This Plan Does NOT Cover

These are real gaps but lower priority:

1. **SPEC-002 Authentication** (OAuth2, SAML, LDAP, MFA) — ~200h of work, enterprise-only
2. **SPEC-001 Modular Architecture** — refactoring, not feature work; defer until API is solid
3. **Observability** (metrics, tracing, health endpoints) — important for production but not for API compatibility
4. **Performance** (N+1 queries, connection pooling) — matters at scale, not for correctness
5. **Full microversion support** (Nova advertises 2.93) — reduce to what's actually implemented
6. **Test infrastructure** (CI integration, contract test automation) — operational, not functional

---

## Estimated Timeline

| Phase | Sequential | With 2 parallel engineers |
|-------|-----------|--------------------------|
| Phase 1 | 2 days | 1 day |
| Phase 2 | 3 days | 1.5 days |
| Phase 3 | 3 days | 1.5 days |
| Phase 4 | 1 day | 0.5 days |
| Phase 5 | 1 day | 0.5 days |
| **Total** | **10 days** | **5 days** |

After completion: ~75% API fidelity, safe to deploy, Horizon basic workflows functional, CLI filtering works, Terraform simple plans succeed.

---

## Fidelity Progression

```
Current:       ████░░░░░░ 40%
After Phase 1: ████░░░░░░ 40% (same fidelity, now safe)
After Phase 2: ██████░░░░ 55% (clients stop crashing)
After Phase 3: ████████░░ 70% (list endpoints work)
After Phase 4: ████████░░ 73% (correct error codes)
After Phase 5: ████████░░ 75% (floating IPs + port security)
```

To reach 90%+ would require: full microversion audit, missing admin endpoints, event notifications, and ~100h more work.
