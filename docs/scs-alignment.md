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
| [SCS-0100-v3](https://docs.scs.community/standards/scs-0100-v3-flavor-naming) — Flavor Naming | Compute | ✅ parser/validator shipped (`pkg/scs`); SCS-* names validated at create time, mirrored into `scs:*` extra-specs |
| [SCS-0103-v1](https://docs.scs.community/standards/scs-0103-v1-standard-flavors) — Mandatory & Recommended Flavors | Compute | ✅ 15 mandatory flavors seeded (migration 075) |
| [SCS-0102](https://docs.scs.community/standards/scs-0102-v1-image-metadata) — Image Metadata | Glance | ⬜ Phase 3 follow-up |
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

A separate validator/parser library lives in `pkg/scs`. Nova's
`CreateFlavor` handler runs every flavor name through it: names that don't
start with `SCS-` pass through unchanged (operators are free to define
their own naming scheme), and names that do are parsed against the
SCS-0100-v3 grammar — anything malformed is rejected with HTTP 400 so a
typo doesn't silently land as a non-conformant flavor. On a successful
parse, the parsed components (`scs:cpu-type`, and `scs:disk0-type` when a
root disk is present) are mirrored into `flavor_extra_specs`, matching
the shape produced by the SCS-0103 seed migration so an SCS-aware client
sees the same keys regardless of how the flavor was created.

The parser covers the full SCS-0100-v3 mandatory prefix (vCPU count and
type letter `C`/`T`/`V`/`L` with optional `i` insecure suffix, RAM with
optional `u` no-ECC and `o` oversubscribed suffixes in that order, and
the optional disk segment with `M×N` multiplier and `n`/`h`/`s`/`p`
type letter) plus the hypervisor extension (`_kvm`, `_xen`, `_chy`,
`_vmw`, `_hyv`, `_bms`). CPU-arch, GPU, and infiniband extensions parse
without error but their components aren't surfaced yet.

### Conformance check

```bash
openstack flavor create --ram 4096 --vcpus 2 --disk 20 SCS-2V-4-20s
openstack flavor show SCS-2V-4-20s -f json | jq '.properties'
# Expected: {"scs:cpu-type": "shared-core", "scs:disk0-type": "ssd"}

openstack flavor create --ram 4096 --vcpus 2 --disk 0 SCS-bogus
# Expected: HTTP 400 — invalid SCS-0100 flavor name: ...
```

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

## Forward roadmap

The following are queued in [`docs/kimi-analyse-for-completion.md`](kimi-analyse-for-completion.md)
under Phase 3:

- **SCS-0102 image metadata** — extend Glance to enforce the SCS image
  metadata properties (`os_distro`, `os_version`, `architecture`,
  `hw_disk_bus`, `hw_rng_model`, `hw_scsi_model`, `hypervisor_type`,
  `image_build_date`, `image_original_user`, `image_source`,
  `patchlevel`, `provided_until`, `replace_frequency`).
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
