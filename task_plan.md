# Task Plan: Implement v4 Production Readiness Fixes

## Goal
Fix all findings from the v4 comprehensive review (55 issues), bringing O3K from 5.5/10 to 8/10.

## Phases
- [x] Phase 1: Stop the crashes — C2 (missing migration), M19 (NULL scan crash), pagination cap, critical rows.Err()
- [x] Phase 2: Security — C4 (hardcoded creds), H4 (JWT default abort), H5 (host header), H3 (bcrypt cost), H10 (IP race), M3/M4 (authz)
- [x] Phase 3: Horizon compat — C5/C6 (OS-EXT-* attrs), H1/H2 (ValidateToken), H6 (microversions), H8 (network fields), H9 (neutron pagination), M11 (SG rules), M16/M17 (cinder fields)
- [x] Phase 4: App credential auth + token revocation — C1 (auth method dispatch), C3 (DB-backed revocation check)
- [x] Phase 5: Systemic rows.Err() pass — 67 locations fixed across 39 files
- [x] Phase 6: Verify — build OK, vet OK, all tests pass

## Decisions Made
- Work on existing branch `fix/production-readiness-security-data`
- Phases 1-4 dispatched as parallel subagents (different files)
- Phase 5 runs after 1-4 complete (touches all packages)
- Phase 6 is final verification

## Errors Encountered
- None

## Status
**COMPLETE** — All 6 phases done. Build passes, all tests pass.
