#!/usr/bin/env bash
# o3k-backup.sh — capture a consistent point-in-time backup of an O3K deployment.
#
# Backs up:
#   - SQLite state DB (via `VACUUM INTO` for an online-consistent copy), OR
#     PostgreSQL state (via pg_dump) if --postgres-url is provided
#   - Bootstrap secrets: jwt-secret, initial-password, agent-token
#   - Local image storage (if present)
#   - Local volume storage (if present)
#
# Output: a single tar.gz archive with a manifest.
#
# Usage:
#   o3k-backup.sh [--data-dir DIR] [--out FILE] [--postgres-url URL]
#
# Environment overrides:
#   O3K_DATA_DIR   default data dir (defaults to ~/.o3k for non-root, /var/lib/o3k for root)
#
# Exit codes: 0 success, 1 usage, 2 dependency missing, 3 backup failure.

set -euo pipefail

DATA_DIR="${O3K_DATA_DIR:-}"
OUT_FILE=""
PG_URL=""

usage() {
  sed -n '2,20p' "$0" | sed 's/^# \{0,1\}//'
  exit 1
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --data-dir) DATA_DIR="$2"; shift 2 ;;
    --out)      OUT_FILE="$2"; shift 2 ;;
    --postgres-url) PG_URL="$2"; shift 2 ;;
    -h|--help)  usage ;;
    *) echo "unknown flag: $1" >&2; usage ;;
  esac
done

# Resolve data dir if not explicitly set.
if [[ -z "$DATA_DIR" ]]; then
  if [[ "$(id -u)" = "0" ]]; then
    DATA_DIR="/var/lib/o3k"
  else
    DATA_DIR="$HOME/.o3k"
  fi
fi

if [[ ! -d "$DATA_DIR" ]]; then
  echo "data dir not found: $DATA_DIR" >&2
  exit 3
fi

# Default output filename.
TS="$(date -u +%Y%m%dT%H%M%SZ)"
OUT_FILE="${OUT_FILE:-o3k-backup-${TS}.tar.gz}"

# Stage everything in a temp dir, then tar it.
STAGE="$(mktemp -d -t o3k-backup-XXXXXX)"
trap 'rm -rf "$STAGE"' EXIT

echo "==> staging in $STAGE"
mkdir -p "$STAGE/payload"

# ---- Database ------------------------------------------------------------
if [[ -n "$PG_URL" ]]; then
  command -v pg_dump >/dev/null 2>&1 || { echo "pg_dump not found"; exit 2; }
  echo "==> dumping PostgreSQL"
  pg_dump --format=custom --no-owner --no-privileges \
    --file="$STAGE/payload/state.pgdump" "$PG_URL"
  DB_KIND="postgres"
elif [[ -f "$DATA_DIR/db/state.db" ]]; then
  command -v sqlite3 >/dev/null 2>&1 || { echo "sqlite3 not found"; exit 2; }
  echo "==> snapshotting SQLite via VACUUM INTO"
  mkdir -p "$STAGE/payload/db"
  # VACUUM INTO yields a consistent online copy without blocking writers.
  sqlite3 "$DATA_DIR/db/state.db" "VACUUM INTO '$STAGE/payload/db/state.db'"
  DB_KIND="sqlite"
else
  echo "no SQLite db at $DATA_DIR/db/state.db and no --postgres-url given" >&2
  exit 3
fi

# ---- Secrets -------------------------------------------------------------
echo "==> copying bootstrap secrets"
for f in jwt-secret initial-password agent-token; do
  if [[ -f "$DATA_DIR/$f" ]]; then
    cp -p "$DATA_DIR/$f" "$STAGE/payload/$f"
  fi
done

# ---- Local object storage ------------------------------------------------
if [[ -d "$DATA_DIR/images" ]]; then
  echo "==> copying images/"
  cp -a "$DATA_DIR/images" "$STAGE/payload/images"
fi
if [[ -d "$DATA_DIR/volumes" ]]; then
  echo "==> copying volumes/"
  cp -a "$DATA_DIR/volumes" "$STAGE/payload/volumes"
fi

# ---- Manifest ------------------------------------------------------------
O3K_VERSION="$(o3k --version 2>/dev/null || echo unknown)"
cat > "$STAGE/payload/manifest.json" <<EOF
{
  "schema_version": 1,
  "created_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "data_dir": "$DATA_DIR",
  "db_kind": "$DB_KIND",
  "o3k_version": "$O3K_VERSION",
  "host": "$(hostname)"
}
EOF

# ---- Pack ----------------------------------------------------------------
echo "==> writing $OUT_FILE"
tar -C "$STAGE" -czf "$OUT_FILE" payload
SHA="$(shasum -a 256 "$OUT_FILE" | awk '{print $1}')"
echo "$SHA  $(basename "$OUT_FILE")" > "${OUT_FILE}.sha256"

echo
echo "backup complete:"
echo "  archive:  $OUT_FILE"
echo "  sha256:   $SHA"
echo "  db kind:  $DB_KIND"
echo "  size:     $(du -h "$OUT_FILE" | awk '{print $1}')"
