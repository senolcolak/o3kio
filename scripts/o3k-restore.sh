#!/usr/bin/env bash
# o3k-restore.sh — restore an O3K deployment from a backup archive produced by
# o3k-backup.sh.
#
# This is destructive: existing files in the data dir are overwritten.
# O3K MUST be stopped before restoring. The script refuses to run if it can
# detect a live O3K process.
#
# Usage:
#   o3k-restore.sh --archive FILE [--data-dir DIR] [--postgres-url URL] [--force]
#
# Exit codes: 0 success, 1 usage, 2 dependency missing, 3 restore failure,
#             4 refused (live process detected, no --force).

set -euo pipefail

ARCHIVE=""
DATA_DIR="${O3K_DATA_DIR:-}"
PG_URL=""
FORCE=0

usage() {
  sed -n '2,15p' "$0" | sed 's/^# \{0,1\}//'
  exit 1
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --archive)      ARCHIVE="$2"; shift 2 ;;
    --data-dir)     DATA_DIR="$2"; shift 2 ;;
    --postgres-url) PG_URL="$2"; shift 2 ;;
    --force)        FORCE=1; shift ;;
    -h|--help)      usage ;;
    *) echo "unknown flag: $1" >&2; usage ;;
  esac
done

[[ -z "$ARCHIVE" ]] && { echo "--archive is required" >&2; usage; }
[[ ! -f "$ARCHIVE" ]] && { echo "archive not found: $ARCHIVE" >&2; exit 1; }

if [[ -z "$DATA_DIR" ]]; then
  if [[ "$(id -u)" = "0" ]]; then
    DATA_DIR="/var/lib/o3k"
  else
    DATA_DIR="$HOME/.o3k"
  fi
fi

# Refuse to clobber a live deployment.
if pgrep -x o3k >/dev/null 2>&1 && [[ "$FORCE" -eq 0 ]]; then
  echo "live o3k process detected — stop it first or pass --force" >&2
  exit 4
fi

# Verify checksum if present.
if [[ -f "${ARCHIVE}.sha256" ]]; then
  echo "==> verifying sha256"
  (cd "$(dirname "$ARCHIVE")" && shasum -a 256 -c "$(basename "$ARCHIVE").sha256")
fi

# Unpack to temp.
STAGE="$(mktemp -d -t o3k-restore-XXXXXX)"
trap 'rm -rf "$STAGE"' EXIT
echo "==> unpacking $ARCHIVE"
tar -C "$STAGE" -xzf "$ARCHIVE"

PAYLOAD="$STAGE/payload"
[[ ! -d "$PAYLOAD" ]] && { echo "archive missing payload/" >&2; exit 3; }
[[ ! -f "$PAYLOAD/manifest.json" ]] && { echo "archive missing manifest.json" >&2; exit 3; }

DB_KIND="$(grep -o '"db_kind":[[:space:]]*"[^"]*"' "$PAYLOAD/manifest.json" | sed 's/.*"\([^"]*\)"$/\1/')"
echo "==> manifest: db_kind=$DB_KIND"

# ---- Database ------------------------------------------------------------
case "$DB_KIND" in
  sqlite)
    [[ ! -f "$PAYLOAD/db/state.db" ]] && { echo "missing state.db in archive" >&2; exit 3; }
    mkdir -p "$DATA_DIR/db"
    cp -p "$PAYLOAD/db/state.db" "$DATA_DIR/db/state.db"
    echo "    restored sqlite -> $DATA_DIR/db/state.db"
    ;;
  postgres)
    [[ -z "$PG_URL" ]] && { echo "archive is postgres, --postgres-url required" >&2; exit 1; }
    command -v pg_restore >/dev/null 2>&1 || { echo "pg_restore not found"; exit 2; }
    [[ ! -f "$PAYLOAD/state.pgdump" ]] && { echo "missing state.pgdump in archive" >&2; exit 3; }
    pg_restore --clean --if-exists --no-owner --no-privileges \
      --dbname="$PG_URL" "$PAYLOAD/state.pgdump"
    echo "    restored postgres -> $PG_URL"
    ;;
  *) echo "unknown db_kind: $DB_KIND" >&2; exit 3 ;;
esac

# ---- Secrets -------------------------------------------------------------
mkdir -p "$DATA_DIR"
for f in jwt-secret initial-password agent-token; do
  if [[ -f "$PAYLOAD/$f" ]]; then
    cp -p "$PAYLOAD/$f" "$DATA_DIR/$f"
    chmod 0600 "$DATA_DIR/$f"
  fi
done
echo "    restored secrets"

# ---- Local storage -------------------------------------------------------
if [[ -d "$PAYLOAD/images" ]]; then
  rm -rf "$DATA_DIR/images"
  cp -a "$PAYLOAD/images" "$DATA_DIR/images"
  echo "    restored images/"
fi
if [[ -d "$PAYLOAD/volumes" ]]; then
  rm -rf "$DATA_DIR/volumes"
  cp -a "$PAYLOAD/volumes" "$DATA_DIR/volumes"
  echo "    restored volumes/"
fi

echo
echo "restore complete. Start O3K and verify with: openstack token issue"
