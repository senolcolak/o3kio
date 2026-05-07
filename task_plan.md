# Task Plan: Fix Critical and High Findings from v7 Review

## Goal
Fix all 2 CRITICALs and 14 HIGHs identified in the v7 comprehensive review, bringing the score from 8.0 toward 8.8.

## Phases
- [ ] Phase 1: Create branch, group fixes by file to avoid conflicts
- [ ] Phase 2: Implement security fixes (C1, C2, H1, H2, H3, H14)
- [ ] Phase 3: Implement Nova response fixes (H4, H5, H6, H7, H8)
- [ ] Phase 4: Implement Neutron fixes (H9, H10, H11)
- [ ] Phase 5: Implement Cinder fixes (H12, H13)
- [ ] Phase 6: Verify — build, vet, tests pass
- [ ] Phase 7: Commit

## Fix Assignments (grouped by file to avoid conflicts)

### Agent 1: Keystone Security (C1 + H2 + H3)
- C1: Add is_app_credential + unrestricted to JWT claims; enforce in CreateAppCred
- H2: Add visited-set cycle detection to policy engine evaluate
- H3: Validate requested roles against caller's actual roles in CreateAppCred

### Agent 2: Middleware + Config Security (H1 + H14)
- H1: Add service field matching to EnforceAccessRules (+ pass service name from main.go)
- H14: Reject empty JWT secret in LoadConfig

### Agent 3: Glance Cache Fix (C2)
- C2: Fix DeleteImage cache key to include projectID prefix

### Agent 4: Nova Response Fixes (H4 + H5 + H6 + H7 + H8)
- H4: Console URL from config (not request Host)
- H5: Tenant usage honor start/end params
- H6: Add security_groups to server responses
- H7: Add OS-EXT-SRV-ATTR:root_device_name and launch_index
- H8: Add links array to GetServer and ListServersDetail

### Agent 5: Neutron Fixes (H9 + H10 + H11)
- H9: Batch security-group query in ListPorts (fix N+1)
- H10: Add missing fields to RemoveRouterInterface response
- H11: Reorder CreateRouter to DB-first, namespace-after

### Agent 6: Cinder Fixes (H12 + H13)
- H12: Add availability_zone column, read from DB
- H13: Add encrypted column, read from DB

## Decisions Made
- Group by file to enable parallel subagents without merge conflicts
- Each agent works on distinct files (no overlap)
- Migration for Cinder AZ+encrypted: 068_volume_fields.up.sql

## Errors Encountered
- (none yet)

## Status
**Currently in Phase 1** — Creating branch and dispatching agents
