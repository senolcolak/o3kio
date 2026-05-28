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

### 4.2 Supply chain: SBOMs + signed releases
- **Action:** Generate SBOMs with `syft` in release workflow. Sign artifacts with cosign/sigstore. Publish checksums.
- **Acceptance:** Releases include SBOM + signed checksums.

### 4.3 Observability: Grafana dashboards + alerting
- **Action:** Add Grafana dashboard JSON configs for Prometheus metrics. Add example alerting rules. Document SLO/SLI.
- **File:** `docs/grafana/` (new directory)

### 4.4 Hardening: Defaults
- **Action:** Bind to 127.0.0.1 by default (not 0.0.0.0). Require strong JWT secret (reject weak/empty). Disable stub mode in production builds.

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
**Currently in Phase 3 — SCS Alignment**

Slice progress:
- ✅ Slice 1: SCS-0103 mandatory flavors (PR #24, merged at 109160c)
- ✅ Slice 2: SCS-0102 image metadata (PR #25, open)
- ✅ Slice 3: SCS-0100-v3 flavor name validator/parser (this PR)
- ⬜ Slice 4: SCS-0104 standard images
- ⬜ Slice 5: SCS-0110 volume types
- ⬜ Slice 6: Audit logging middleware
- ⬜ Slice 7: SPEC-002 federated identity

---

## Slice 3 — SCS-0100-v3 flavor name validator/parser

### Goal
Ship a parser/validator for SCS-0100-v3 flavor names so any flavor whose name
starts with `SCS-` is checked against the standard at create time, and the
parsed components are available as extra-specs for SCS-aware clients.

### Scope
- New package `pkg/scs` with:
  - `ParseFlavorName(name) (FlavorName, error)` — parse the mandatory prefix
    `SCS-<vCPUs><cpu-type>-<RAM_GiB>[-<disk_GB><disk-type>]` plus optional
    extension fields (hypervisor, accelerator, CPU brand, etc.).
  - `Validate(name) error` — wrapper that returns nil for non-SCS names and
    a descriptive error for malformed `SCS-*` names.
- Wire into Nova's `CreateFlavor` handler: if `name` starts with `SCS-`,
  validate; on success, mirror parsed components into `flavor_extra_specs`
  rows (`scs:cpu-type`, `scs:disk0-type`, etc.) so behaviour matches the
  Slice 1 seed.
- Update `docs/scs-alignment.md`: flip SCS-0100 row from 🟡 to ✅, document
  the validator behaviour and conformance check.

### Out of scope
- Cross-checking that vCPUs/RAM in the name match the numeric `vcpus`/`ram_mb`
  fields (separate slice; the spec allows operators to define their own
  flavor sizing).
- Extension field semantics beyond parsing (e.g. validating that an
  accelerator suffix corresponds to a real PCI device).

### Acceptance
- `openstack flavor create SCS-2V-4` succeeds; `flavor show` reflects
  `scs:cpu-type=shared-core` in extra specs.
- `openstack flavor create SCS-bogus` returns 400 with a parser error.
- Non-SCS names (e.g. `m1.tiny`) are unaffected.
- Unit tests cover happy path, every cpu-type letter, every disk-type
  letter, datasets from SCS-0103, and a representative set of malformed
  inputs.

## Errors Encountered (Slice 3)
- (None yet.)
