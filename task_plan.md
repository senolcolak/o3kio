# Task Plan: Implement Kimi Readiness Audit Gaps

## Goal
Close the gaps identified in the kimi readiness audit to bring O3K from 45% to 65%+ readiness, focusing on trust cleanup, security hardening, and real infrastructure proof.

## Source Document
`docs/kimi-analyse-for-completion.md` — external audit at commit `daf8a11`

## Phases
- [ ] Phase 1: Trust Cleanup (Blockers — target 65% readiness)
- [ ] Phase 2: Real Infrastructure Proof
- [ ] Phase 3: SCOS/SCS Alignment
- [ ] Phase 4: Pilot Readiness

---

## Phase 1: Trust Cleanup (P0 blockers + CI hardening)

### 1.1 Security: Remove default credentials
- **File:** `migrations/002_seed_data.up.sql`
- **Action:** Remove hardcoded bcrypt hash of "secret". Generate random admin password at first boot in `internal/server/seed.go` using `crypto/rand`, print once to stderr.
- **Also:** `deployments/docker-compose.yml` — remove `POSTGRES_PASSWORD: secret`, use env var with generated default.
- **Acceptance:** `docker compose up` does not expose a known password.

### 1.2 Security: TLS for HTTP services
- **File:** `cmd/o3k/main.go` (lines ~809–1000, all `http.Server` instances)
- **Action:** Add `--tls-cert-file` / `--tls-key-file` CLI flags for all 5 HTTP servers (Keystone, Nova, Neutron, Cinder, Glance). If flags absent, document mandatory reverse proxy requirement with HSTS.
- **Acceptance:** Services start with TLS certs; docs cover reverse proxy pattern.

### 1.3 Security: Wire policy engine into middleware
- **File:** `internal/middleware/policy.go` (line 22)
- **Action:** Replace hardcoded role checks with a call to `PolicyEngine.Enforce()`. Add fallback to coarse role checks if no policy rules match. Write at least one end-to-end policy test.
- **Also:** Update all docs to accurately describe RBAC enforcement level.
- **Acceptance:** `PolicyMiddleware` calls `engine.Enforce()`; one integration test passes.

### 1.4 CI: Make contract tests blocking
- **File:** `.github/workflows/ci.yml` (lines 144, 154–157, 164–180)
- **Action:** Remove `continue-on-error: true`. Set floor to 95%. Require contract tests in the `status` job. Fix or remove consistently failing tests.
- **Acceptance:** CI fails if contract pass rate < 95%.

### 1.5 Storage: Implement Glance RBD backend
- **File:** `pkg/storage/image_store.go` (lines 203–216, 346–358, 446–457, 529–541)
- **Action:** Implement RBD image upload/download/delete/exists using `go-ceph` (already a dependency for Cinder). Use `//go:build ceph` build tag.
- **Acceptance:** Glance image CRUD works against a Ceph cluster; contract tests pass for RBD mode.

### 1.6 Config: Make Ceph monitor list configurable
- **File:** `pkg/hypervisor/xml_template.go` (line 110)
- **Action:** Replace hardcoded `127.0.0.1:6789` with a config field read from `config/o3k.yaml`. Inject at XML generation time.
- **Acceptance:** Ceph monitor address is configurable; no hardcoded IPs in output XML.

### 1.7 CI: Make golangci-lint blocking + expand ruleset
- **File:** `.github/workflows/ci.yml` (line 77), `.golangci.yml`
- **Action:** Remove `continue-on-error: true` from lint step. Expand `.golangci.yml` to include `errcheck`, `gosec`, `staticcheck`, `bodyclose`. Fix all newly surfaced issues.
- **Acceptance:** CI fails on lint errors; `gosec` passes with no HIGH/CRITICAL findings.

### 1.8 Security: Add govulncheck to CI
- **File:** `.github/workflows/ci.yml`
- **Action:** Add a `govulncheck ./...` step before the build step. Block on HIGH/CRITICAL findings.
- **Acceptance:** CI runs `govulncheck`; passes with no HIGH/CRITICAL findings.

### 1.9 Open Source: Add SECURITY.md
- **File:** `SECURITY.md` (new)
- **Action:** Write vulnerability disclosure policy, security contact, supported versions table, and links to dependency scanning reports.
- **Acceptance:** `SECURITY.md` exists at repo root; policy is actionable.

### 1.10 Docs: Fix outdated documentation
- **Files:** `docs/REAL_MODE_TESTING.md`, any docs claiming inconsistent contract test %
- **Action:** Rewrite `REAL_MODE_TESTING.md` to reflect actual capabilities (cloud-init, VNC console, network attachment). Remove false limitations. Fix inconsistent contract test % across docs.
- **Acceptance:** Docs match current code capabilities.

### 1.11 Cleanup: Remove broken artifacts
- **Action:** Delete `rbac_test.go.broken` and any other broken/leftover files. Fix hardcoded paths in scripts.
- **Acceptance:** No `.broken` files in tree; scripts use relative/config paths.

### 1.12 README: Honest messaging
- **File:** `README.md`
- **Action:** Add prominent alpha/developer-preview banner. Replace overclaiming with honest "What Works / Experimental / Missing" table. Add Production Safety Warning box. Remove "100% compatibility", "production ready", "104% endpoint coverage" claims.
- **Acceptance:** README sets accurate expectations; no false claims remain.

---

## Phase 2: Real Infrastructure Proof

### 2.1 CI: Real-mode VM lifecycle job
- **Action:** Add GitHub Actions job that runs `qemu:///system` VM create/list/delete (KVM-enabled runner). Assert success. Tag as `real-mode-smoke`.
- **Acceptance:** Passing "Real Mode Smoke Test" job in CI.

### 2.2 Go integration tests: libvirt + Ceph
- **Action:** Write Go integration tests for hypervisor and storage packages behind build tags (`//go:build integration`). Test VM lifecycle and Ceph volume attach.
- **Acceptance:** `go test -tags integration ./pkg/hypervisor/... ./pkg/storage/...` passes.

### 2.3 eBPF: Add CI build and test
- **Action:** Add CI step to compile `secgroup.c` (eBPF) and test XDP attach on a veth pair. Not optional.
- **Acceptance:** CI compiles eBPF and verifies XDP attach.

### 2.4 VXLAN: Multi-node test
- **Action:** Add test script/CI job that starts two container agents, verifies VXLAN FDB sync between them.
- **Acceptance:** VXLAN multi-node test passes in CI.

### 2.5 Docs: Add `docs/real-kvm-ceph-lab.md`
- **Action:** Step-by-step guide to deploying on a 3-node lab (real libvirt + Ceph + S3). Include verification commands and failure modes.
- **Acceptance:** A new engineer can follow the guide and deploy successfully.

---

## Phase 3: SCOS/SCS Alignment

### 3.1 Docs: Create `docs/scs-alignment.md`
- **Action:** Standards mapping document: SCS flavor naming, image metadata, volume types, IAM, audit. Gap matrix with implemented vs planned.

### 3.2 Flavors: Implement SCS standard flavor names
- **Action:** Add seed data for SCS-standard flavors: `SCS-1V-4-20`, `SCS-2V-8-50`, etc. with correct `scs:*` extra specs.
- **Acceptance:** `openstack flavor list` shows SCS-standard names.

### 3.3 Glance: SCS image metadata
- **Action:** Populate and validate SCS-standard image properties: `os_distro`, `os_version`, `architecture`, `image_original_user`, `hw_*` fields in Glance API.
- **Acceptance:** Glance API accepts/returns SCS-standard properties.

### 3.4 Cinder: SCS volume type standards
- **Action:** Define SCS volume type metadata: `scs:volume-type`, encryption labels, AZ labels.
- **Acceptance:** Volume types follow SCS naming conventions.

### 3.5 IAM: OIDC/OAuth2/LDAP federation (SPEC-002)
- **Action:** Implement federated login flow (Keycloak/OIDC). Document Horizon login via OIDC.
- **Acceptance:** Federated login flow tested; documented in `docs/`.

### 3.6 Security: Begin Barbican implementation
- **Action:** Add `o3k-barbican` service with secret CRUD. Wire into Cinder for volume encryption POC.
- **Acceptance:** Secrets API functional; Cinder POC encrypts a volume.

### 3.7 Audit: API audit logging middleware
- **Action:** Add structured audit logging middleware to all services. Log: user, project, method, path, status, IP, timestamp. Export as JSON/CEF.
- **Acceptance:** Every mutating request logged with required fields.

---

## Phase 4: Pilot Readiness

### 4.1 Operations: Backup/restore tooling and docs
- **Action:** Add `o3k backup` / `o3k restore` commands or scripts. Document DB backup (SQLite + PostgreSQL), image storage backup, version upgrade runbook.
- **File:** `docs/backup-restore-upgrade.md` (new)
- **Status:** ✅ Done.
  - `scripts/o3k-backup.sh` — SQLite via `VACUUM INTO` (online-consistent, non-blocking) or PostgreSQL via `pg_dump --format=custom`. Captures secrets (`jwt-secret`, `initial-password`, `agent-token`), local images, local volumes. Emits `tar.gz` + `.sha256` sidecar with manifest.
  - `scripts/o3k-restore.sh` — verifies sha256, refuses live `o3k` process without `--force`, restores DB + secrets + storage with `0600` perms on secrets. Supports both SQLite and PostgreSQL.
  - `docs/backup-restore-upgrade.md` — what gets backed up (and what doesn't: Ceph RBD, S3, libvirt domains), backup/restore procedures for SQLite + PostgreSQL, scheduling via cron, upgrade contract (patch/minor/major), disaster-recovery checklist, known limitations.
  - End-to-end smoke test passes locally: backup → restore → SQLite row preserved, secrets identical, images and volumes match.

### 4.2 Supply chain: SBOMs + signed releases
- **Action:** Generate SBOMs with `syft` in release workflow. Sign artifacts with cosign/sigstore. Publish checksums.
- **Acceptance:** Releases include SBOM + signed checksums.

### 4.3 Observability: Grafana dashboards + alerting
- **Action:** Add Grafana dashboard JSON configs for Prometheus metrics. Add example alerting rules. Document SLO/SLI.
- **File:** `docs/grafana/` (new directory)

### 4.4 Hardening: Defaults
- **Action:** Bind to 127.0.0.1 by default (not 0.0.0.0). Require strong JWT secret (reject weak/empty). Disable stub mode in production builds.
- **Status:** ✅ Done.
  - `BindAddress()` defaults to `127.0.0.1` when `server.bind_host` is unset (`internal/common/config.go:333-342`).
  - JWT-secret strength gate refuses default `change-me-in-production` unless `O3K_ENV=development|test` (`internal/common/config.go:260-267`); strength check (≥32 chars) lives in `cmd/o3k/main.go:300-303`.
  - Production stub-mode guard added in `ValidateConfig()` — `O3K_ENV=production` refuses `nova.libvirt_mode=stub`, `neutron.networking_mode=stub`, `cinder.storage_mode=stub`, `glance.storage_mode=stub`. Tests in `internal/common/config_test.go::TestValidateConfigProductionStubGuard`.

### 4.5 Community: Issue/PR templates + CoC
- **Files:** `.github/ISSUE_TEMPLATE/bug_report.md`, `.github/ISSUE_TEMPLATE/feature_request.md`, `CODE_OF_CONDUCT.md`
- **Action:** Add standard OS templates.

### 4.6 Docs: Production readiness guide
- **File:** `docs/production-readiness.md` (new)
- **Action:** Honest assessment of what is safe today. Pre-flight checklist (change password, enable TLS, configure Ceph, etc.).

---

## Key Questions
1. Is Phase 1 a single sprint or broken into multiple PRs?
2. Should TLS (1.2) be opt-in via flags, or should we gate HTTP-only to localhost?
3. Does the real-mode CI job need a self-hosted runner, or can GitHub Actions KVM work?

## Decisions Made
- Priority order: Phase 1 (trust) → Phase 2 (proof) → Phase 3 (SCS) → Phase 4 (ops)
- Phase 1 items 1.1–1.4 are absolute blockers; implement first
- Each phase produces a PR; phases are sequential

## Errors Encountered
- (none yet)

## Status
**Phase 3 Slice 7 — SPEC-002 federated identity (OIDC/OAuth2/LDAP).**

Slice 6 (CADF audit logging) merged as PR #29 — middleware mounted across all 6 auth-bearing services, 7 tests passing, `docs/scs-alignment.md` audit row flipped ⬜→✅.

### Slice 7 phases
- [x] Phase 0: Merge Slice 6 PR #29
- [x] Phase 1: Understand SCS-0300 — read SCS federated identity spec, scan existing keystone auth code (`internal/keystone/`, `/v3/auth/tokens` handler), identify integration surface (login flow, token issuance, user/project provisioning)
- [x] Phase 2: Plan approach — pick provider abstraction (OIDC first via `coreos/go-oidc/v3`), config schema, JIT user provisioning policy, mapping from IdP claims to O3K roles
- [x] Phase 3: Implement — provider interface + OIDC adapter, federated `/v3/auth/tokens` flow with `identity.methods=["openid"]`, JIT user/project create, config wiring, tests
- [ ] Phase 4: Verify and deliver — unit tests, integration test against a stub IdP (or `go-oidc` mock), docs (`docs/scs-alignment.md` row + new federation section), PR

### Phase 1 findings (2026-05-28)
**Spec ID correction:** the actual spec is **SCS-0300-v1** "Requirements for SSO identity federation" (Effective track, IAM). The "SPEC-002" label used internally and in `docs/scs-alignment.md` row 32 is wrong — needs renaming. SCS-0301 (naming, Draft) and SCS-0302 (Domain Manager, Effective) are sibling IAM-track standards but out of scope for this slice.

**SCS-0300-v1 conformance surface:** the published v1 contains design considerations (Keycloak vs Zitadel) but **no formal conformance tests** — the "Conformance Tests" section is OPTIONAL and empty. Minimum viable surface is therefore an implementer's judgment call, not a spec mandate. The IAM-federation drafts (`github.com/SovereignCloudStack/standards/tree/main/Drafts/IAM-federation`, especially `keystone-keycloak-federation.md`) document the API surface to mirror.

**Federation API surface to expose** (mirrored from Keystone+Keycloak reference):
- `/v3/auth/OS-FEDERATION/identity_providers/<idp>/protocols/openid/websso` — browser flow, Authorization Code grant
- `/v3/OS-FEDERATION/identity_providers/<idp>/protocols/mapped/auth` — CLI flow, OAuth2 bearer + JWKS verification
- `kc_idp_hint` URL parameter for identity brokering
- `v3oidcpassword` flow env-var contract (`OS_AUTH_TYPE`, `OS_IDENTITY_PROVIDER`, `OS_DISCOVERY_ENDPOINT`, …) for OpenStack CLI compatibility

**Existing Keystone auth integration points:**
- `internal/keystone/handlers.go:224` — `AuthenticateToken` handler dispatches on `req.Auth.Identity.{Token,ApplicationCredential,Password}`. Federated branch slots in as a fourth `else if`.
- `internal/keystone/handlers.go:74` — `RegisterRoutes` is where new `/v3/OS-FEDERATION/...` route group hangs off.
- `internal/keystone/auth.go:170` — `AuthRequest` struct needs a new `Federated *struct{...}` field on `Auth.Identity`.
- `internal/keystone/auth.go:243` — `AuthenticatePassword` is the implementation template for `AuthenticateFederated` (domain lookup → user lookup → scope check → role fetch → HS256 JWT signing). Roughly 200 lines, mostly reusable; the verify step swaps from bcrypt to OIDC ID-token verification.
- `internal/keystone/auth.go:229` — `deterministicUUID(parts...)` is the right primitive for JIT provisioning to map `(issuer, subject)` → stable O3K user UUID.

### Phase 1 decisions

1. **Minimum surface for v1 = OIDC only.** SCS-0300 doesn't mandate protocol coverage, OIDC is the modern default, Keycloak/Zitadel/Auth0/Okta all support it natively, and a single library (`coreos/go-oidc/v3`) covers the verification path. LDAP and OAuth2-direct become follow-up slices.
2. **JIT auto-provisioning on first federated login.** Matches Zitadel's reference pattern, lowest operator friction. Admin pre-provisioning becomes a config knob (`auto_provision: false` per provider) for ops that want stricter control.
3. **Claim-to-role mapping in a new DB table.** Queryable, auditable, manageable via existing role-assignment APIs. Static-config alternative would be invisible at runtime and force restarts to change. Schema sketch: `(provider_id, claim_name, claim_value, role_id, project_id?)`.
4. **Federated session → normal O3K JWT (HS256).** `Methods` field on the issued token reads `["openid"]` (browser) or `["mapped"]` (CLI). Existing `AuthMiddleware` works unchanged because the JWT is structurally identical to a password-issued one.
5. **Hook-in plan:**
   - Add `Federated *struct{...}` field to `AuthRequest.Auth.Identity` in `auth.go:170`
   - Add fourth dispatch branch in `AuthenticateToken` at `handlers.go:242`
   - New `internal/keystone/federation.go` for `FederationProvider` interface + OIDC adapter + JIT provisioning
   - New route group registered in `RegisterRoutes` at `handlers.go:74`
   - New migration for `federation_providers` and `federation_role_mappings` tables
   - Library: `github.com/coreos/go-oidc/v3` (mature, JWKS rotation, ID-token verification)

### Phase 3 status
- Slice 1 (SCS-0103 mandatory flavors) — merged PR #24
- Slice 2 (SCS-0102 image metadata) — open PR #25
- Slice 3 (SCS-0100-v3 flavor name validator) — open PR #26
- Slice 4 (SCS-0104 standard images) — open PR #27
- Slice 5 (SCS-0114 volume types) — merged PR #28
- Slice 6 (CADF audit logging) — merged PR #29
- Slice 7 (SPEC-002 federated identity) — **in progress, this branch**

**Phases 1–3 complete.** Phase 4 in progress — federation unit tests added (`internal/keystone/federation_test.go`, 11 cases covering YAML defaults, registry lookup, validation paths, verify-failure mapping, happy-path token issuance, domain-scope rejection), docs updated (`docs/scs-alignment.md` row + new SCS-0300-v1 section), branch `slice-7-federation`. Next: open PR off main.
