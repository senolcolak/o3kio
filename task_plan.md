# Task Plan: Fix v6 CRITICALs and HIGHs

## Goal
Fix all 6 CRITICAL and 16 HIGH findings from the v6 review, bringing O3K from 7.5/10 to ~8.5/10.

## Phases
- [ ] Phase 1: Triage — separate quick fixes from large features
- [ ] Phase 2: Implement quick CRITICALs + all HIGHs via parallel subagents
- [ ] Phase 3: Verify — build OK, vet OK, all tests pass
- [ ] Phase 4: Commit

## Decisions Made
- C1 (access rules), C2 (legacy_auth), C3 (policy engine): These are multi-day features, NOT quick fixes. Will defer to separate PRs.
- C4+C5 (Cinder response shapes), C6 (Neutron bridge ordering): Quick fixes, include.
- H1-H16: All implementable in this pass.
- Dispatch by file grouping to avoid merge conflicts between agents.

## Agent Assignments
- Agent 1: Keystone (H1-H4) — unrestricted enforcement, roleRows leak, rows.Err, methods field
- Agent 2: Nova (H5-H9) — launched_at, flavor name, console URLs, quota defaults, snapshotCount
- Agent 3: Neutron (C6, H10-H14) — bridge order, pagination, network_type, port lock, SG rules, null fields
- Agent 4: Cinder+Glance (C4, C5, H15, H16) — volume response shapes, volume_type persistence, glance owner

## Errors Encountered
- (none yet)

## Status
**Currently in Phase 2** — Dispatching subagents
