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
| [SCS-0102](https://docs.scs.community/standards/scs-0102-v1-image-metadata) — Image Metadata | Glance | ✅ properties accepted, validated, and round-tripped |
| [SCS-0104](https://docs.scs.community/standards/scs-0104-v1-standard-images) — Standard Images | Glance | ⬜ Phase 3 follow-up |
| SCS-0110 — Volume Types | Cinder | ⬜ Phase 3 follow-up |
| SPEC-002 — Federated Identity (OIDC/OAuth2/LDAP) | Keystone | ⬜ Phase 3 follow-up |
| SCS audit logging | Keystone/all | ⬜ Phase 3 follow-up |

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

## SCS-0102-v1 — Image Metadata

SCS-0102 defines a vocabulary of properties that an image carries so a
client can decide *what* the image is, *how stale* it is, and *what
hardware* it expects without booting it. O3K accepts these on `image
create` / `image set` and round-trips them through `image show`.

Properties land as **top-level fields** in the v2 wire format (matching
how OpenStack Glance surfaces image properties — they are merged into the
top-level resource, not nested under a `properties` key). Internally
they live in the `images.properties` JSON column (JSONB on PostgreSQL,
TEXT on SQLite).

### Property coverage

| Property | Type | Validation |
|----------|------|-----------|
| `architecture` | string | free-text (e.g. `x86_64`, `aarch64`) |
| `hw_disk_bus` | string | free-text (`scsi`, `virtio`, `ide`, …) |
| `hw_rng_model` | string | free-text |
| `hw_scsi_model` | string | free-text |
| `hw_video_ram` | number | numeric |
| `hw_mem_encryption` | bool | `true`/`false` |
| `hw_pmu` | bool | `true`/`false` |
| `hw_vif_multiqueue_enabled` | bool | `true`/`false` |
| `hypervisor_type` | string | free-text |
| `os_distro` | string | free-text |
| `os_version` | string | free-text |
| `os_purpose` | enum | `generic` \| `minimal` \| `k8snode` \| `gpu` \| `network` \| `custom` |
| `os_secure_boot` | bool | `true`/`false` |
| `os_hash_algo` | enum | `sha256` \| `sha512` |
| `os_hash_value` | string | free-text |
| `image_build_date` | date | `YYYY-MM-DD` or `YYYY-MM-DD hh:mm[:ss]` |
| `image_source` | string | free-text URL |
| `image_description` | string | free-text URL |
| `image_original_user` | string | free-text |
| `patchlevel` | string | free-text |
| `replace_frequency` | enum | `yearly` \| `quarterly` \| `monthly` \| `weekly` \| `daily` \| `critical_bug` \| `never` |
| `provided_until` | date | `YYYY-MM-DD` \| `none` \| `notice` |
| `maintained_until` | date | `YYYY-MM-DD` \| `none` \| `notice` |
| `uuid_validity` | string | `YYYY-MM-DD` \| `last-N` \| `forever` \| `none` \| `notice` |
| `hotfix_hours` | number | numeric |
| `license_included` | bool | mutually exclusive with `license_required = true` |
| `license_required` | bool | mutually exclusive with `license_included = true` |
| `subscription_included` | bool | `true`/`false` |
| `subscription_required` | bool | `true`/`false` |

Unknown property names are accepted and stored verbatim — Glance is a
metadata bag and SCS-aware clients may ship custom keys. Validation runs
on create and on every `image set` patch; an invalid value rejects the
whole request with `400 Bad Request`.

### Conformance check

```bash
# Set SCS properties on create
openstack image create my-ubuntu \
    --disk-format qcow2 --container-format bare \
    --property os_distro=ubuntu \
    --property os_version=24.04 \
    --property architecture=x86_64 \
    --property hw_disk_bus=scsi \
    --property replace_frequency=monthly \
    --property provided_until=2029-04-30 \
    --property uuid_validity=last-2

# They surface as top-level fields, not nested under "properties"
openstack image show my-ubuntu -f json | jq '{os_distro, replace_frequency, provided_until}'
# Expected: {"os_distro":"ubuntu","replace_frequency":"monthly","provided_until":"2029-04-30"}

# Patch is JSON Patch over /<key> (not /properties/<key>)
openstack image set --property os_version=24.04.1 my-ubuntu
openstack image show my-ubuntu -f value -c os_version
# Expected: 24.04.1

# Invalid enum value is rejected
openstack image create bad --disk-format qcow2 --container-format bare \
    --property replace_frequency=annually
# Expected: 400 — replace_frequency must be one of yearly|quarterly|monthly|...
```

The mandatory-property *presence* check is deliberately not enforced at
the API layer — federation policy decides which images may omit which
properties. The validator only rejects values that violate the standard
when supplied.

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

## Forward roadmap

The following are queued in [`docs/kimi-analyse-for-completion.md`](kimi-analyse-for-completion.md)
under Phase 3:

- **SCS-0110 volume types** — define and seed default Cinder volume types
  with SCS extra-specs (`scs:availability-zone`, `scs:replication`).
- **SPEC-002 federated identity** — wire Keystone to OIDC/OAuth2/LDAP
  identity providers so federated SCS users can authenticate against an
  external IdP.
- **Audit logging** — emit structured CADF-shaped audit events from the
  Keystone middleware on every authenticated request.

Each of these is its own slice; this document is the index they will hang
off as they land.

## References

- SCS standards root: <https://docs.scs.community/standards/>
- Standards repo: <https://github.com/SovereignCloudStack/standards>
- Conformance test suite: <https://github.com/SovereignCloudStack/standards/tree/main/Tests>
