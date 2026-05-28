# SCS Alignment

This document maps O3K to the [Sovereign Cloud Stack (SCS)](https://scs.community/)
standards. SCS is a federated cloud reference defined by a set of versioned
specifications maintained at <https://docs.scs.community/standards/>. SCS
compliance is a precondition for adoption inside the SCS federation and for
sovereign-cloud pilots that take their cues from SCS.

The goal here is *interface compatibility* — an SCS-aware client (Terraform,
the OpenStack CLI, Horizon, the SCS conformance tests) should see the same
shapes against O3K that it sees against any other SCS-conformant cloud.
Behaviour parity is incremental and tracked per spec below.

## Status legend

| Symbol | Meaning |
|--------|---------|
| ✅ | Implemented and seeded; conformance test passes |
| 🟡 | Partially implemented — names/shapes are present, semantics incomplete |
| ⬜ | Not yet started — tracked in [`docs/kimi-analyse-for-completion.md`](kimi-analyse-for-completion.md) |

## Spec coverage

| SCS standard | Domain | O3K status |
|--------------|--------|------------|
| [SCS-0100-v3](https://docs.scs.community/standards/scs-0100-v3-flavor-naming) — Flavor Naming | Compute | 🟡 names land via SCS-0103 seed; parser/validator not yet shipped |
| [SCS-0103-v1](https://docs.scs.community/standards/scs-0103-v1-standard-flavors) — Mandatory & Recommended Flavors | Compute | ✅ 15 mandatory flavors seeded (migration 075) |
| [SCS-0102](https://docs.scs.community/standards/scs-0102-v1-image-metadata) — Image Metadata | Glance | ⬜ Phase 3 follow-up |
| [SCS-0104](https://docs.scs.community/standards/scs-0104-v1-standard-images) — Standard Images | Glance | ⬜ Phase 3 follow-up |
| SCS-0110 — Volume Types | Cinder | ✅ 3 reference volume types seeded (migration 076) — covered by [SCS-0114-v1](https://docs.scs.community/standards/scs-0114-v1-volume-type-standard/) |
| [SCS-0114-v1](https://docs.scs.community/standards/scs-0114-v1-volume-type-standard/) — Volume Type Standard | Cinder | ✅ description tags + queryable `scs:*` extra-specs (migration 076) |
| [SCS-0300-v1](https://docs.scs.community/standards/scs-0300-v1-requirements-for-sso-identity-federation) — SSO Identity Federation | Keystone | 🟡 OIDC verification + JIT provisioning shipped (`internal/keystone/federation*.go`); LDAP/OAuth2-direct deferred |
| SCS audit logging | Keystone/all | ✅ CADF events emitted on every authenticated mutation (`internal/middleware/audit.go`) |

## SCS-0100-v3 — Flavor Naming

The naming scheme is `SCS-<vCPUs><cpu-type>-<RAM_GiB>[-<disk_GB><disk-type>]`,
with optional extension fields after that for accelerators, hypervisor type,
and so on. The mandatory minimum is the prefix above.

| Token | Meaning | Values |
|-------|---------|--------|
| `vCPUs` | integer count of vCPUs | `1`, `2`, `4`, `8`, `16`, … |
| `cpu-type` | scheduling guarantee on the vCPU | `V` shared-core (default), `L` low-resource (over-commit allowed), `T` dedicated thread, `C` dedicated core |
| `RAM_GiB` | RAM size in GiB | `1`, `2`, `4`, `8`, `16`, `32`, … |
| `disk_GB` | optional pre-attached root disk size in GB | absent = no disk; `20`, `100`, … |
| `disk-type` | optional disk media | absent = unspecified, `s` SSD, `n` NVMe, `h` HDD |

O3K stores RAM in MB internally (`flavors.ram_mb`), so the seed converts
GiB → MB by multiplying by 1024. Flavors with no pre-attached root disk are
seeded with `disk_gb = 0`; clients are expected to attach a Cinder volume at
server-create time.

A separate validator/parser library is **not** yet shipped — names are
treated as opaque strings. This is fine for SCS-0103 conformance but blocks
generic SCS-0100 conformance for non-mandatory flavors. Tracked in the
audit doc.

## SCS-0103-v1 — Mandatory & Recommended Flavors

Migration `075_scs_standard_flavors.up.sql` (and its sqlite mirror) seeds
the 15 mandatory flavors from SCS-0103-v1, plus the corresponding
`flavor_extra_specs` rows that mirror the suffix encoding so an SCS-aware
client can filter on `scs:cpu-type` and `scs:disk0-type` directly.

| Flavor | vCPUs | RAM (GiB) | Root disk | `scs:cpu-type` | `scs:disk0-type` |
|--------|------:|----------:|----------:|----------------|------------------|
| `SCS-1L-1`        | 1  |  1 |   — | `crowded-core` | — |
| `SCS-1V-2`        | 1  |  2 |   — | `shared-core`  | — |
| `SCS-1V-4`        | 1  |  4 |   — | `shared-core`  | — |
| `SCS-1V-8`        | 1  |  8 |   — | `shared-core`  | — |
| `SCS-2V-4`        | 2  |  4 |   — | `shared-core`  | — |
| `SCS-2V-4-20s`    | 2  |  4 |  20 GB SSD | `shared-core` | `ssd` |
| `SCS-2V-8`        | 2  |  8 |   — | `shared-core`  | — |
| `SCS-2V-16`       | 2  | 16 |   — | `shared-core`  | — |
| `SCS-4V-8`        | 4  |  8 |   — | `shared-core`  | — |
| `SCS-4V-16`       | 4  | 16 |   — | `shared-core`  | — |
| `SCS-4V-16-100s`  | 4  | 16 | 100 GB SSD | `shared-core` | `ssd` |
| `SCS-4V-32`       | 4  | 32 |   — | `shared-core`  | — |
| `SCS-8V-16`       | 8  | 16 |   — | `shared-core`  | — |
| `SCS-8V-32`       | 8  | 32 |   — | `shared-core`  | — |
| `SCS-16V-32`      | 16 | 32 |   — | `shared-core`  | — |

These are seeded both by the SQL migration (PostgreSQL & SQLite) and by the
in-code seed in `internal/server/seed.go` so that zero-config installs
(`./o3k`) and docker-compose installs see the same flavor set.

### Conformance check

```bash
openstack flavor list -f value -c Name | grep '^SCS-' | sort
# Expected: 15 lines starting with SCS-1L-1 .. SCS-16V-32

openstack flavor show SCS-2V-4-20s -f json | jq '.properties'
# Expected: {"scs:cpu-type": "shared-core", "scs:disk0-type": "ssd"}
```

The legacy `m1.*` flavors (`m1.tiny` through `m1.xlarge`) remain seeded for
backwards compatibility with existing tests and tutorials. They are not part
of the SCS standard and operators may safely disable them by deleting the
rows after first boot.

## SCS-0114-v1 — Volume Type Standard

[SCS-0114-v1](https://docs.scs.community/standards/scs-0114-v1-volume-type-standard/)
advertises encryption and replication capabilities through *description tags*
of the form `[scs:encrypted]` and `[scs:replicated]` rather than through a
prescribed extra-spec key. Migration `076_scs_volume_types.up.sql` (and its
sqlite mirror) seeds three reference volume types covering the documented
combinations, with the description tags AND a parallel set of queryable
`scs:*` extra-specs so SCS-aware clients can filter on them through the
standard volume-type extra-specs API.

| Volume type | Description (excerpt) | `scs:encrypted` | `scs:replicated` | `scs:availability-zone` |
|-------------|----------------------|-----------------|------------------|--------------------------|
| `scs-default`    | "SCS default volume type — unencrypted, single-AZ" | `false` | `false` | `nova` |
| `scs-encrypted`  | "SCS encrypted volume type [scs:encrypted]"        | `true`  | `false` | `nova` |
| `scs-replicated` | "SCS replicated volume type [scs:replicated]"      | `false` | `true`  | `nova` |

These are seeded both by the SQL migration (PostgreSQL & SQLite) and by the
in-code seed in `internal/server/seed.go` so that zero-config installs
(`./o3k`) and docker-compose installs see the same volume-type set. PostgreSQL
stores `extra_specs` as `JSONB`; SQLite stores the same JSON document as
`TEXT`.

### Conformance check

```bash
openstack volume type list -f value -c Name | grep '^scs-' | sort
# Expected: scs-default, scs-encrypted, scs-replicated

openstack volume type show scs-encrypted -f json | jq '.properties'
# Expected: {"scs:encrypted": "true", "scs:replicated": "false", "scs:availability-zone": "nova"}
```

A persistent replication-aware backend (Ceph RBD mirroring, DRBD, …) is *not*
yet wired up — `scs-replicated` advertises replication intent but the local
storage backend does not enforce it. Operators who need actual cross-AZ
replication should pair this with a replicated Cinder backend.

## SCS audit logging — CADF

Every authenticated mutating request (POST/PUT/PATCH/DELETE) emits a
structured [CADF](https://www.dmtf.org/standards/cadf) (Cloud Auditing Data
Federation, DMTF DSP0262) event so operators can reconstruct
who-did-what-to-which-resource without trawling raw access logs.

Implementation: `internal/middleware/audit.go`, mounted on Keystone, Nova,
Neutron, Cinder, Glance, and Placement immediately after `AuthMiddleware`.
Read traffic (GET/HEAD/OPTIONS) is intentionally excluded — SCS audit
guidance only requires mutations, and read events would dominate volume.

Events are emitted as tagged zerolog lines with `audit_event=true`. A log
shipper (fluent-bit, vector, …) can filter on that tag and route to a SIEM:

```bash
# Local dev — tail mutating events only
./o3k 2>&1 | jq -c 'select(.audit_event == true)'
```

| CADF field | O3K source | Example |
|------------|------------|---------|
| `eventType` | constant | `activity` |
| `id` | per-event UUID | `9d3f…` |
| `eventTime` | wall-clock | `2026-05-28T11:22:33.123456Z` |
| `action` | HTTP method → CADF verb | `create`, `update`, `delete` |
| `outcome` | response status | `success` (2xx) or `failure` |
| `initiator.id` | JWT `user_id` | UUID |
| `initiator.name` | JWT `user_name` | `admin` |
| `initiator.project_id` | JWT `project_id` | UUID |
| `initiator.host.address` | client IP | `10.0.0.42` |
| `target.typeURI` | path → `<service>/<resource>` | `compute/server` |
| `target.id` | resource UUID in path (when targeted) | UUID |
| `observer.id` / `observer.typeURI` | constants | `o3k` / `service/security` |
| `requestPath` | raw request path | `/v2.1/<project>/servers/<id>` |
| `reason.reasonCode` | HTTP status | `200`, `409` |
| `request_id` | request_id header (when present) | UUID |

Sample event for `DELETE /v2.1/<project>/servers/<id>`:

```json
{
  "audit_event": true,
  "eventType": "activity",
  "id": "9d3f…",
  "eventTime": "2026-05-28T11:22:33.123456Z",
  "action": "delete",
  "outcome": "success",
  "initiator.id": "user-uuid",
  "initiator.name": "admin",
  "initiator.project_id": "project-uuid",
  "initiator.host.address": "10.0.0.42",
  "target.typeURI": "compute/server",
  "target.id": "server-uuid",
  "observer.id": "o3k",
  "observer.typeURI": "service/security",
  "requestPath": "/v2.1/project-uuid/servers/server-uuid",
  "reason.reasonCode": 204,
  "request_id": "req-uuid"
}
```

The audit trail is the log stream itself — there is no `audit_events` DB
table, matching O3K's broader "no message queue, synchronous ops" stance.
A persisted table can follow if pilots demand searchable history.

## SCS-0300-v1 — SSO Identity Federation

O3K's Keystone trusts an external IdP (Keycloak, Zitadel, Auth0, …) and
exchanges verified IdP credentials for a normal O3K JWT. The token is
**structurally identical** to a password-issued token — existing
`AuthMiddleware` works unchanged — only the `methods` claim differs
(`["openid"]` for federated logins, `["password"]` for local).

**v1 protocol coverage**: OIDC only. SCS-0300-v1 itself does not mandate
a protocol set ("Conformance Tests" is empty in the published v1), so
LDAP and OAuth2-direct are tracked as follow-up slices.

### Configuration

Federation is opt-in via `keystone.federation.enabled`. Per-provider
configs live under `keystone.federation.providers`:

```yaml
keystone:
  federation:
    enabled: true
    providers:
      - name: keycloak-prod
        protocol: openid
        issuer: https://keycloak.example.com/realms/scs
        client_id: o3k
        client_secret: ${KEYCLOAK_CLIENT_SECRET}
        auto_provision: true
        username_claim: preferred_username
        groups_claim: groups
        default_project: default
        default_role: member
```

`client_secret` is deliberately **not** stored in the database — it stays
in YAML or env so a DB dump never leaks IdP credentials.

### Flow

1. Caller obtains an OIDC ID token from the IdP (browser SSO or
   device-code flow — out of O3K's scope).
2. Caller POSTs to `/v3/auth/tokens` with `identity.methods=["openid"]`
   and the ID token in `identity.federated.credential`.
3. Keystone verifies the token signature against the IdP's JWKS
   (auto-rotating, fetched at discovery time) and validates `iss`, `aud`,
   `exp`.
4. JIT provisioning: a deterministic UUID v5 is derived from
   `(issuer, subject)` so re-logins always resolve to the same O3K user.
   When `auto_provision: true`, an unknown subject creates a new user
   row with `password_hash="!federated"` (an invalid bcrypt sentinel —
   federated users can never fall back to password auth).
5. Default-role assignment lands the new user on the configured
   `default_project` with `default_role`.
6. Keystone returns a normal O3K JWT; downstream services (Nova,
   Neutron, …) need no changes.

### Files

- `internal/keystone/federation.go` — provider interface, config types,
  registry
- `internal/keystone/federation_oidc.go` — `OIDCProvider` (discovery +
  JWKS-rotating verifier via `github.com/coreos/go-oidc/v3`)
- `internal/keystone/federation_auth.go` — `AuthenticateFederated`,
  JIT provisioning, scope resolution
- `migrations/00XX_federation.sql` — `federation_providers` and
  `federation_role_mappings` tables (claim-to-role mapping is staged in
  the schema; only default-role logic ships in this slice)

### Out of scope for this slice

- LDAP and OAuth2-direct adapters
- Browser-side `/v3/auth/OS-FEDERATION/.../websso` redirect flow (CLI
  flow via direct ID-token POST is the v1 surface)
- Claim-to-role mapping using `federation_role_mappings` (table exists,
  lookup logic is a follow-up)

## Forward roadmap

The following are queued in [`docs/kimi-analyse-for-completion.md`](kimi-analyse-for-completion.md)
under Phase 3:

- **SCS-0102 image metadata** — extend Glance to enforce the SCS image
  metadata properties (`os_distro`, `os_version`, `architecture`,
  `hw_disk_bus`, `hw_rng_model`, `hw_scsi_model`, `hypervisor_type`,
  `image_build_date`, `image_original_user`, `image_source`,
  `patchlevel`, `provided_until`, `replace_frequency`).
- **SCS-0300-v1 LDAP / OAuth2-direct adapters** — extend the federation
  surface beyond OIDC. The provider interface and JIT path are already
  in place (see "SCS-0300-v1 — SSO Identity Federation" above); each
  protocol lands as its own adapter implementing `FederationProvider`.

Each of these is its own slice; this document is the index they will hang
off as they land.

## References

- SCS standards root: <https://docs.scs.community/standards/>
- Standards repo: <https://github.com/SovereignCloudStack/standards>
- Conformance test suite: <https://github.com/SovereignCloudStack/standards/tree/main/Tests>
