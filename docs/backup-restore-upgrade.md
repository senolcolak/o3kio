# Backup, Restore, and Upgrade

This runbook covers backing up an O3K deployment, restoring from a backup, and
performing version upgrades safely. It assumes you are running O3K either in
zero-config mode (SQLite, default) or with PostgreSQL via `docker-compose`.

> **Status:** alpha. The procedures below have been smoke-tested against
> SQLite. The PostgreSQL path uses standard `pg_dump` / `pg_restore` and is
> covered by their guarantees, but has not been exercised against a
> production-sized O3K dataset. Test backups by restoring them.

---

## What gets backed up

| Item | Source | Why it matters |
|------|--------|----------------|
| State database | `<data_dir>/db/state.db` (SQLite) **or** PostgreSQL | All projects, users, instances, networks, volumes, images metadata |
| `jwt-secret` | `<data_dir>/jwt-secret` | Without it, every issued token is invalidated on restore |
| `initial-password` | `<data_dir>/initial-password` | First-boot admin credential |
| `agent-token` | `<data_dir>/agent-token` | Compute-agent registration token |
| Image storage | `<data_dir>/images/` | Glance local backend payloads |
| Volume storage | `<data_dir>/volumes/` | Cinder local backend payloads |

**Not backed up** (out of scope for the local tooling — manage these in their
own systems):

- Ceph RBD pools (`storage_mode: rbd`)
- S3 buckets (`storage_mode: s3`)
- libvirt domain XML and qcow2 images on hypervisor hosts
- Configuration files (`config/o3k.yaml`) — keep these in version control

The default `<data_dir>` is `/var/lib/o3k` for root and `~/.o3k` for non-root,
overridable via `O3K_DATA_DIR`.

---

## Backup

### SQLite (zero-config)

```bash
./scripts/o3k-backup.sh
# → o3k-backup-20260529T140000Z.tar.gz
# → o3k-backup-20260529T140000Z.tar.gz.sha256
```

The script uses SQLite's `VACUUM INTO` to produce a consistent online snapshot
without blocking writers. It is safe to run while O3K is serving traffic.

Common flags:

```bash
./scripts/o3k-backup.sh --data-dir /var/lib/o3k --out /backups/o3k-$(date -u +%F).tar.gz
```

### PostgreSQL

```bash
./scripts/o3k-backup.sh \
  --postgres-url "postgresql://lightstack:$POSTGRES_PASSWORD@localhost:5432/lightstack"
```

Internally this calls `pg_dump --format=custom`, which is the recommended
format for `pg_restore` (parallel restore, selective table restore, etc.). The
local secrets directory is still archived alongside the dump.

### Output format

The archive is a `tar.gz` containing a `payload/` directory:

```
payload/
├── manifest.json          # schema_version, created_at, db_kind, host, o3k_version
├── db/state.db            # (sqlite mode)
├── state.pgdump           # (postgres mode)
├── jwt-secret
├── initial-password
├── agent-token
├── images/
└── volumes/
```

A sidecar `.sha256` file is written next to the archive. Verify before
restoring:

```bash
shasum -a 256 -c o3k-backup-20260529T140000Z.tar.gz.sha256
```

### Scheduling

For a daily backup with 7-day retention, drop this in `cron`:

```cron
0 2 * * *  /opt/o3k/scripts/o3k-backup.sh --out /backups/o3k-$(date -u +\%F).tar.gz && \
           find /backups -name 'o3k-*.tar.gz' -mtime +7 -delete
```

Encrypt at rest if your backup target is shared:

```bash
./scripts/o3k-backup.sh --out /tmp/o3k.tgz && \
  gpg --symmetric --cipher-algo AES256 /tmp/o3k.tgz && \
  rm /tmp/o3k.tgz
```

---

## Restore

> **Stop O3K first.** Restoring over a live deployment corrupts the running
> SQLite database and causes inconsistent state. The script refuses to run if
> it detects a live `o3k` process; pass `--force` only when you are certain
> the process is dead.

### SQLite

```bash
# 1. Stop o3k.
systemctl stop o3k          # or: docker compose -f deployments/docker-compose.yml down

# 2. Restore.
./scripts/o3k-restore.sh --archive o3k-backup-20260529T140000Z.tar.gz

# 3. Start.
systemctl start o3k

# 4. Smoke-test.
openstack token issue
openstack project list
```

### PostgreSQL

```bash
./scripts/o3k-restore.sh \
  --archive o3k-backup-20260529T140000Z.tar.gz \
  --postgres-url "postgresql://lightstack:$POSTGRES_PASSWORD@localhost:5432/lightstack"
```

`pg_restore` is invoked with `--clean --if-exists`, which drops and recreates
each object. The target database must exist.

### Verifying a restore

After restoring on a fresh host, confirm:

| Check | Command | Expected |
|-------|---------|----------|
| Identity works | `openstack token issue` | Token payload returned |
| Projects intact | `openstack project list` | All pre-backup projects |
| Instances intact | `openstack server list --all-projects` | All pre-backup VMs in expected state |
| Networks intact | `openstack network list --all-projects` | All pre-backup networks |
| Images intact | `openstack image list` | All pre-backup images |
| Volumes intact | `openstack volume list --all-projects` | All pre-backup volumes |

If JWT secrets were rotated as part of disaster recovery, every active token
becomes invalid; clients must re-authenticate. This is expected.

---

## Upgrade

O3K follows semantic versioning. The upgrade contract is:

| Bump | API contract | Migrations | Action |
|------|--------------|------------|--------|
| Patch (`0.5.0 → 0.5.1`) | unchanged | none | drop in new binary |
| Minor (`0.5.x → 0.6.0`) | additive | additive only | back up, drop in, restart |
| Major (`0.x → 1.0`) | may break | may rewrite | back up, read CHANGELOG, plan rollback |

**Always back up before upgrading**, even patch releases. Rollback for failed
minor upgrades is a restore from the pre-upgrade backup.

### Standard upgrade procedure

```bash
# 1. Capture state.
./scripts/o3k-backup.sh --out /backups/pre-upgrade-$(date -u +%F).tar.gz

# 2. Note current version.
o3k --version > /backups/pre-upgrade-version.txt

# 3. Stop the service.
systemctl stop o3k

# 4. Replace the binary (or pull new image).
curl -LO https://github.com/senolcolak/o3kio/releases/download/vX.Y.Z/o3k
chmod +x o3k && sudo mv o3k /usr/local/bin/

# 5. Apply migrations (binary auto-applies on start; this is a dry run).
o3k --config /etc/o3k/o3k.yaml --migrate-only

# 6. Start.
systemctl start o3k

# 7. Smoke-test (see "Verifying a restore" table above).

# 8. If anything is wrong, roll back.
systemctl stop o3k
./scripts/o3k-restore.sh --archive /backups/pre-upgrade-$(date -u +%F).tar.gz --force
# Reinstall the previous binary, then start.
```

### Container upgrades

```bash
docker compose -f deployments/docker-compose.yml down
docker pull ghcr.io/senolcolak/o3kio:vX.Y.Z
# Update the tag in docker-compose.yml, then:
docker compose -f deployments/docker-compose.yml up -d
```

The Postgres container is unaffected; only `o3k` is replaced. Migrations run
on first start of the new image.

---

## Disaster recovery checklist

A minimum viable DR plan:

- [ ] Daily backups via cron, retained ≥7 days
- [ ] Backups stored off-host (S3, NFS mount, encrypted external disk)
- [ ] Backup integrity verified by checksum on every restore
- [ ] At least one restore drill per quarter on a non-production host
- [ ] Runbook (this document) accessible without access to O3K
- [ ] Rollback target: the most recent successful backup, RPO ≤24h

---

## Known limitations

- The script captures local file storage only. If you use Ceph RBD or S3, you
  must back those up through their native tooling (Ceph snapshots, S3
  versioning) and restore them in the same window as the O3K state.
- `VACUUM INTO` on SQLite scales linearly with database size. For a 10 GB
  database, expect tens of seconds of extra disk I/O while the snapshot is
  being written.
- The restore script will refuse to run if a live `o3k` process is detected.
  This guard is best-effort (it greps for the process name); ensure the
  process is genuinely stopped before relying on it.
- There is no incremental backup mode. Each archive is a full snapshot.
