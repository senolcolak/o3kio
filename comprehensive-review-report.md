# Comprehensive Review Report - O3K

**Date**: 2026-05-06
**Branch**: `fix/comprehensive-review`
**Scope**: Full codebase (10 packages, ~15k LOC Go)
**Method**: Wave 0 per-package deep review (10 parallel agents)

---

## Summary

| Severity | Found | Fixed | Remaining |
|----------|-------|-------|-----------|
| CRITICAL | 18 | 14 | 4 |
| HIGH | 25 | 18 | 7 |
| MEDIUM | 20 | 0 | 20 |
| LOW | 12 | 0 | 12 |

**Total fixes applied**: 32 across 27 files (two commit batches + pending)

---

## CRITICAL Fixes Applied

| # | File | Issue | Fix |
|---|------|-------|-----|
| 1 | `internal/middleware/auth.go` | Auth bypass via `HasSuffix` | Exact path matching |
| 2 | `internal/neutron/ports.go` | Cross-tenant security group rule leakage | JOIN + project_id filter |
| 3 | `internal/neutron/trunks.go` | SQL placeholder rune arithmetic breaks at argPos > 9 | `fmt.Sprintf("$%d", argPos)` |
| 4 | `internal/cinder/volumes.go` | Name-vs-UUID Ceph deletion (storage leak) | Fetch actual UUID before Ceph call |
| 5 | `internal/keystone/application_credentials.go` | `rand.Read` error silently ignored (zero-secret) | Propagate error |
| 6 | `internal/tunnel/dispatch.go` | Concurrent gRPC stream.Send race | `SafeSend` with mutex |
| 7 | `internal/scheduler/reconciler.go` | Missing transaction on FOR UPDATE SKIP LOCKED | Wrapped in `WithTx` |
| 8 | `internal/scheduler/worker.go` | Retry threshold inconsistency (2 vs 3) | Unified constant |
| 9 | `internal/nova/flavors.go` | GetFlavorExtraSpecs returns empty map (scan without assign) | Added map assignment |
| 10 | `internal/nova/handlers.go` | ListServersDetail NULL panic | `sql.NullString`/`sql.NullTime` |
| 11 | `internal/neutron/network.go` | MTU hardcoded 1500 instead of DB value | Return DB value |
| 12 | `internal/cinder/transfers.go` | Non-atomic volume transfer accept | Wrapped in `WithTx` |
| 13 | `pkg/hypervisor/xml_template.go` | XML injection via unescaped user strings | Added `xmlEscape()` |
| 14 | `pkg/networking/netns.go` | Namespace prefix match (wrong namespace deleted) | Exact field match |

## CRITICAL Remaining (Architectural)

| # | Issue | Reason Deferred |
|---|-------|-----------------|
| 1 | Keystone management endpoints unauthenticated | Requires router-group restructure |
| 2 | Keystone plaintext `secret_hash` storage | Needs migration + bcrypt integration |
| 3 | Hypervisor resume cold-boots instead of restore | Requires libvirt ManagedSave API |
| 4 | Neutron IP allocation collision (time-based) | Needs IPAM redesign with DB-level uniqueness |

---

## HIGH Fixes Applied

| # | File | Issue | Fix |
|---|------|-------|-----|
| 1 | `pkg/networking/security_groups.go` | 3 bare `sgID[:28]` slices (panic) | `sgChainName()` helper |
| 2 | `pkg/networking/security_groups.go` | Wrong iptables chain (INPUT/OUTPUT) | FORWARD |
| 3 | `pkg/networking/router.go` | `routerID[:11]` panic | Length check |
| 4 | `pkg/networking/router.go` | `interfaceName[:9]` panic | Length check |
| 5 | `pkg/networking/router.go` | `namespaceExists` substring match | Exact field match |
| 6 | `internal/glance/images.go` | Bare type assertions in UpdateImage | Comma-ok pattern |
| 7 | `internal/glance/images.go` | Exec error discarded | Error check + return |
| 8 | `internal/nova/advanced_actions.go` | Lowercase status written to DB | `strings.ToUpper()` |
| 9 | `internal/neutron/ports.go` | DeleteSecurityGroupRule cross-tenant | Project_id subquery |
| 10 | `pkg/networking/dhcp.go` | `runningPIDs` map race condition | Mutex on all accesses |
| 11 | `pkg/networking/dhcp.go` | `os.Signal(nil)` panic in IsRunning | `syscall.Signal(0)` |
| 12 | `internal/cinder/volumes.go` | 7 bare type assertions in VolumeAction | Comma-ok + 400 |
| 13 | `internal/neutron/floatingip.go` | 4 bare `routerID[:7]` slices | Length checks |
| 14 | `internal/neutron/floatingip.go` | Hardcoded fallback IP `10.0.1.10` | Query port's actual IP |
| 15 | `internal/tunnel/client.go` | Concurrent Send in heartbeat goroutine | Mutex-wrapped |
| 16 | `internal/keystone/domains.go` | Rune arithmetic placeholder bug | `fmt.Sprintf` |
| 17 | `internal/keystone/credentials.go` | Same rune arithmetic bug | `fmt.Sprintf` |
| 18 | `internal/database/tx.go` | WithTx panics on (nil, nil) | Nil guard |

## HIGH Remaining

| # | Issue | Reason Deferred |
|---|-------|-----------------|
| 1 | Tunnel TryAcquireInflight never called | Capacity limiting architectural decision |
| 2 | Nova DeleteServer doesn't delete port records | Cascade design needed |
| 3 | Nova CreateServer ignores DB insert error | Needs broader error audit |
| 4 | Cinder os-extend allows shrinking | Business rule needs spec clarification |
| 5 | Neutron VNI allocation race | Needs distributed lock |
| 6 | Hypervisor libvirt operations no mutex | Requires libvirt pool-level locking |
| 7 | Tunnel agent removal doesn't cancel pending | Needs task lifecycle management |

---

## Verification

```
go build ./...                              # Clean (0 errors)
go test ./internal/... ./pkg/... -short     # All pass
```

---

## Files Modified

### Batch 1 (committed as 83f05d9)
```
internal/cinder/transfers.go
internal/compat/embedded.go
internal/database/tx.go
internal/keystone/credentials.go
internal/keystone/domains.go
internal/neutron/network.go
internal/nova/flavors.go
internal/nova/handlers.go
internal/scheduler/reconciler.go
internal/scheduler/worker.go
internal/tunnel/client.go
pkg/hypervisor/xml_template.go
pkg/networking/netns.go
pkg/networking/security_groups.go
```

### Batch 2 (pending commit)
```
internal/cinder/volumes.go                   (+101/-24)
internal/glance/images.go                    (+13/-4)
internal/keystone/application_credentials.go (+11/-3)
internal/middleware/auth.go                  (+12/-8)
internal/neutron/floatingip.go               (+37/-8)
internal/neutron/ports.go                    (+19/-7)
internal/neutron/trunks.go                   (+11/-8)
internal/nova/advanced_actions.go            (+8/-3)
internal/tunnel/dispatch.go                  (+2/-1)
internal/tunnel/server.go                    (+8/-0)
pkg/networking/dhcp.go                       (+9/-3)
pkg/networking/router.go                     (+20/-4)
pkg/networking/security_groups.go            (+12/-6)
```

---

## Cross-Package Patterns Found

1. **Unvalidated string slicing**: `id[:N]` without length checks (10+ locations, all fixed)
2. **Bare type assertions**: `x.(T)` without comma-ok (15+ locations, all fixed)
3. **Rune arithmetic for SQL placeholders**: `string(rune('0'+N))` (3 files, all fixed)
4. **Missing mutex guards on shared maps**: dhcp.go, tunnel hub (fixed)
5. **iptables wrong chain direction**: INPUT/OUTPUT instead of FORWARD (fixed)
6. **fmt.Printf instead of structured logging**: 20+ locations (LOW, not fixed)
