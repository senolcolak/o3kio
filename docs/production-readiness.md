# Production Readiness

This document is an honest assessment of what is safe to run today, what
is **not**, and a pre-flight checklist for operators considering O3K
beyond a developer laptop.

> **Bottom line.** O3K is **alpha software**. It is suitable for
> evaluation, development, lab clusters, and CI scaffolding. It is
> **not** suitable for tenants you cannot stop, data you cannot lose,
> or workloads with regulatory exposure. If you need a hard guarantee
> in the next 12 months, run mainline OpenStack.

---

## What "production-ready" means here

We use this rubric. A capability is production-ready when **all four**
hold:

1. The code path is exercised by automated tests in CI on every PR.
2. There is a documented operator procedure for failure recovery.
3. Defaults are safe for the threat model in [SECURITY.md](../SECURITY.md).
4. We have run it under realistic load for a non-trivial duration.

By that bar, no service in O3K is fully production-ready today. The
table below grades each area against the rubric so you can decide what
to use it for.

| Area | Production-ready? | Notes |
|---|---|---|
| Core API surface (CRUD across 5 services) | **Evaluation only** | ~70–80% fidelity vs. real OpenStack clients; missing fields and filters surface as Terraform / gophercloud errors. |
| Identity / Keystone (password + JWT) | **Evaluation only** | Works; lacks federation maturity, no FIPS crypto profile. |
| OIDC federation (SCS-0300-v1) | **Lab only** | Implemented in `internal/keystone/federation.go`; tested against stub IdP, not against a hardened Keycloak/Zitadel deployment. |
| RBAC policy enforcement | **Evaluation only** | Policy engine wired in, but rule coverage is incomplete and not parity with `policy.json` defaults. |
| TLS termination | **Production-shaped** | Native TLS via `--tls-cert-file` / `--tls-key-file`; reverse-proxy pattern documented. |
| Audit logging (CADF) | **Production-shaped** | Mounted across all 6 auth-bearing services; emits JSON to stdout. Sink integration is the operator's responsibility. |
| Backup / restore | **Production-shaped** | `scripts/o3k-backup.sh` + `scripts/o3k-restore.sh`; smoke-tested; see [backup-restore-upgrade.md](backup-restore-upgrade.md). |
| Real-mode hypervisor (libvirt/KVM) | **Lab only** | Code paths exist and have been exercised manually; not in CI; no soak tests. |
| Real-mode networking (iptables/eBPF, VXLAN) | **Lab only** | Single-node iptables works; multi-node VXLAN is experimental. |
| Real-mode storage (Ceph RBD, S3) | **Lab only** | Build-tag-gated, live cluster integration not in CI. |
| Multi-node coordination | **Not ready** | Compute-node registry exists; no leader election, no fencing, no live migration. |
| Quotas / billing / chargeback | **Not implemented** | No usage metering, no chargeback signals. |
| Disaster recovery automation | **Operator-driven** | Restore is manual; no replicated-write database, no cross-site failover. |

If a capability is **Lab only** or **Not ready**, treat its presence as
"available for testing" rather than a service you can lean on.

---

## Pre-flight checklist

Walk this list **in order** before pointing real users at an O3K
deployment. Each item links to the canonical reference. Skipping any
of these turns an honest alpha into an unsafe one.

### 1. Identity and secrets

- [ ] **Set a strong `O3K_JWT_SECRET`** (≥32 bytes, random). The
      binary refuses to start with the default secret unless
      `O3K_ENV=development|test`. See `internal/common/config.go`.
- [ ] **Rotate the bootstrap admin password.** First boot generates one
      and writes it to `$O3K_DATA_DIR/initial-password` (zero-config) or
      prints it once to stderr (Docker). Change it via
      `openstack user set --password ... admin` and delete the
      bootstrap file.
- [ ] **Set `O3K_ENV=production`.** This activates stub-mode refusal
      in `ValidateConfig` — `nova.libvirt_mode=stub`,
      `neutron.networking_mode=stub`, `cinder.storage_mode=stub`, and
      `glance.storage_mode=stub` all become startup failures.
- [ ] **Bind to a non-public interface.** `server.bind_host` defaults
      to `127.0.0.1`; only loosen it if you control the network in
      front of O3K (reverse proxy, security group, mTLS).
- [ ] **Store the database password in an environment variable**, not
      in a checked-in `config/o3k.yaml`. Use whatever secret manager
      your platform offers (Vault, sealed-secrets, AWS Secrets Manager,
      etc.).

### 2. Transport security

- [ ] **Terminate TLS.** Either:
      - Provide `--tls-cert-file` and `--tls-key-file` to O3K directly
        (recommended for single-node), or
      - Run O3K bound to `127.0.0.1` behind nginx / Caddy / HAProxy
        with HSTS (`max-age=31536000; includeSubDomains`).
      See [tls-configuration.md](tls-configuration.md).
- [ ] **Set `O3K_ENDPOINT_HOST`** to the externally-visible hostname so
      the Keystone service catalog returns correct URLs.
- [ ] **Strip and re-set `X-Forwarded-*` headers** at the proxy. Do
      not trust client-supplied values.

### 3. Database

- [ ] **Choose PostgreSQL** for anything beyond a single-host
      evaluation. SQLite is fine for embedded edge cases and CI; it is
      not appropriate for a deployment that needs concurrent writers,
      replication, or point-in-time recovery.
- [ ] **Enable `sslmode=verify-full`** on the PostgreSQL DSN if the
      database is on a different host.
- [ ] **Limit connection pool size** to what your PostgreSQL instance
      can serve under peak. Default is 20.
- [ ] **Schedule daily backups** with retention. See
      [backup-restore-upgrade.md](backup-restore-upgrade.md). Off-host
      the archives.

### 4. Real-mode dependencies (only if leaving stub)

- [ ] **libvirt/KVM**: Linux host with `qemu-kvm`, `libvirtd`, and
      hardware virtualization. The `o3k` binary needs membership in
      the `libvirt` group (or equivalent) to talk to
      `qemu:///system`. Verify with `virsh list`.
- [ ] **Ceph (RBD)**: build with `-tags ceph`. Provide a working
      `ceph.conf` and `client.<id>.keyring` readable by the o3k user.
      Set `cinder.ceph_monitors` (no hardcoded `127.0.0.1`).
- [ ] **S3**: provide credentials via the standard AWS env vars, or an
      IAM role on the host. Set `O3K_S3_ENDPOINT` for non-AWS
      providers.
- [ ] **Networking (real mode)**: requires `CAP_NET_ADMIN`. iptables
      rules will be created; eBPF mode requires a recent kernel and
      compiled `secgroup.o`.

### 5. Observability

- [ ] **Scrape `/metrics`** on every service from Prometheus. Grafana
      dashboards are not yet shipped (Phase 4 Slice 4.3).
- [ ] **Capture audit logs** from stdout into your SIEM / ELK / Loki
      stack. The CADF middleware emits structured JSON; mounted in all
      6 auth-bearing services.
- [ ] **Tracing**: configure `otel.exporter` to your collector if you
      want spans. The default stdout exporter is fine for development
      and useless in production.
- [ ] **Health checks**: probe `/healthz` (liveness) and `/readyz`
      (readiness) on each service port.

### 6. Operations

- [ ] **Document the upgrade path.** Patch / minor / major contract is
      in [backup-restore-upgrade.md](backup-restore-upgrade.md).
      Always back up before upgrading, even patch releases.
- [ ] **Run a restore drill** before going live. A backup that has
      never been restored is a hope, not a backup.
- [ ] **Capacity-plan the data dir.** Local image and volume backends
      sit in `$O3K_DATA_DIR/{images,volumes}`; size for peak. Switch
      to RBD or S3 if growth is unbounded.
- [ ] **Review `SECURITY.md`** with whoever signs off on production
      deployments. Make sure the threat model matches your tenant
      assumptions.

### 7. What you must accept

If you proceed past this checklist, you are accepting:

- API fidelity is not 100%. Some Terraform data sources will fail.
  Some gophercloud calls will return nil dereferences on missing
  response fields. Test your specific client workload against a
  staging deployment first.
- There is no live migration, no automated failover, no RPO < 24h
  story without third-party tooling.
- Tenant isolation is **process-level**, not hypervisor-hardened. If
  your threat model includes hostile tenants, do not deploy O3K.
- The project is alpha. Behaviour can change between minor releases.
  Pin versions. Read the CHANGELOG.

---

## What we have not built yet

To set realistic expectations, here is what is on the roadmap but not
in main:

- Grafana dashboards and alerting rules (Phase 4 Slice 4.3)
- Signed releases + SBOMs (Phase 4 Slice 4.2)
- Quota enforcement and chargeback hooks
- Live migration, evacuation, host maintenance mode
- Cell-based scaling beyond a single control plane
- LDAP / SAML federation (only OIDC is implemented)
- Barbican-backed volume encryption (POC only)
- Full `policy.json` parity with mainline OpenStack
- Conformance-test parity with Devstack

If your production plan depends on any of the above, wait. Or
contribute. See [docs/CONTRIBUTING.md](CONTRIBUTING.md).

---

## When to deploy O3K today

Reasonable use cases:

- **Development clusters** where teams need a Keystone + Nova + Neutron
  surface without standing up Devstack.
- **CI scaffolding** for code that targets the OpenStack API.
- **Edge / single-node lab** where the operator and the tenant are the
  same person.
- **Teaching / demo environments** that need a quick OpenStack-shaped
  control plane.

Unreasonable use cases:

- Multi-tenant production with paying customers.
- Anything regulated (PCI, HIPAA, FedRAMP, EU-cloud).
- Workloads that require active-active replication or RPO < 24h.
- Deployments where a tenant escape would be a serious incident.

---

## Pre-flight summary card

A one-screen version of the checklist for operators who already know
the territory:

```
□ O3K_JWT_SECRET ≥32 bytes, unique
□ Bootstrap admin password rotated
□ O3K_ENV=production set
□ server.bind_host = 127.0.0.1 (or trusted interface only)
□ DB password in env var, not in config file
□ TLS terminated (native or reverse proxy)
□ HSTS header at the proxy
□ PostgreSQL with sslmode=verify-full (not SQLite)
□ Daily backup cron + off-host retention
□ Restore drill completed at least once
□ /metrics scraped, audit logs shipped
□ /healthz and /readyz probed
□ Real-mode dependencies validated (libvirt / Ceph / S3 / netlink)
□ Upgrade contract reviewed
□ SECURITY.md threat model accepted
```

If any box is unchecked, you are not ready.
