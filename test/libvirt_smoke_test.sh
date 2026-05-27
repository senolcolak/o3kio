#!/bin/bash
# Libvirt real-mode smoke test
#
# Boots a real VM through Nova (libvirt_mode: real) and asserts:
#   - server transitions BUILD -> ACTIVE
#   - the libvirt domain exists (virsh list --name shows it)
#   - server delete removes both the API record and the libvirt domain
#
# Requires:
#   - Linux with /dev/kvm present and writable by the runner user
#   - libvirt-daemon-system, qemu-system-x86 installed
#   - An O3K binary at bin/o3k
#
# Run with:
#   sudo -E ./test/libvirt_smoke_test.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
TEST_DIR="$PROJECT_ROOT/test/tmp/libvirt-smoke"

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

log()  { echo -e "${GREEN}[INFO]${NC}  $*"; }
warn() { echo -e "${YELLOW}[WARN]${NC}  $*"; }
fail() { echo -e "${RED}[FAIL]${NC}  $*" >&2; exit 1; }

require() {
    command -v "$1" >/dev/null 2>&1 || fail "missing required command: $1"
}

cleanup() {
    log "cleanup"
    if [ -f "$TEST_DIR/o3k.pid" ]; then
        kill "$(cat "$TEST_DIR/o3k.pid")" 2>/dev/null || true
        rm -f "$TEST_DIR/o3k.pid"
    fi
    # Remove any leftover libvirt domains created by this test
    if command -v virsh >/dev/null 2>&1; then
        for d in $(virsh -c qemu:///system list --all --name 2>/dev/null | grep -E '^o3k-smoke-' || true); do
            virsh -c qemu:///system destroy "$d" 2>/dev/null || true
            virsh -c qemu:///system undefine "$d" 2>/dev/null || true
        done
    fi
}
trap cleanup EXIT

# --- preflight ---
require curl
require jq
require virsh
[ -w /dev/kvm ] || fail "/dev/kvm not writable by $(whoami) — KVM acceleration unavailable"

mkdir -p "$TEST_DIR"

# --- start o3k in real mode ---
log "starting o3k in libvirt real mode"
export O3K_DATA_DIR="$TEST_DIR/data"
export O3K_JWT_SECRET="${O3K_JWT_SECRET:-libvirt-smoke-jwt-secret-32chars-or-more-padding}"
export O3K_ADMIN_PASSWORD="${O3K_ADMIN_PASSWORD:-secret}"

mkdir -p "$O3K_DATA_DIR"

# Override libvirt_mode via a config file we generate. Keep everything else
# at defaults so this test only exercises the new real-mode path.
CONFIG="$TEST_DIR/o3k.yaml"
cat > "$CONFIG" <<EOF
database:
  datastore: "sqlite://$O3K_DATA_DIR/state.db"
keystone:
  port: 35357
  jwt_secret: "${O3K_JWT_SECRET}"
  token_ttl: 24h
  admin_user: admin
nova:
  port: 8774
  libvirt_uri: "qemu:///system"
  libvirt_mode: real
neutron:
  port: 9696
  networking_mode: stub
cinder:
  port: 8776
  storage_mode: stub
glance:
  port: 9292
  storage_mode: stub
EOF

"$PROJECT_ROOT/bin/o3k" --config "$CONFIG" > "$TEST_DIR/o3k.log" 2>&1 &
echo $! > "$TEST_DIR/o3k.pid"

# Wait for keystone to come up
for i in $(seq 1 60); do
    if curl -sf http://localhost:35357/v3 >/dev/null 2>&1; then
        log "o3k responding after ${i}s"
        break
    fi
    sleep 1
    if [ "$i" -eq 60 ]; then
        cat "$TEST_DIR/o3k.log" >&2
        fail "o3k did not start within 60s"
    fi
done

# --- authenticate ---
log "issuing scoped admin token"
TOKEN_RESPONSE=$(mktemp)
curl -s -i -X POST http://localhost:35357/v3/auth/tokens \
    -H "Content-Type: application/json" \
    -d "{\"auth\":{\"identity\":{\"methods\":[\"password\"],\"password\":{\"user\":{\"name\":\"admin\",\"domain\":{\"name\":\"Default\"},\"password\":\"$O3K_ADMIN_PASSWORD\"}}},\"scope\":{\"project\":{\"name\":\"default\",\"domain\":{\"name\":\"Default\"}}}}}" \
    > "$TOKEN_RESPONSE"

TOKEN=$(grep -i '^X-Subject-Token:' "$TOKEN_RESPONSE" | awk '{print $2}' | tr -d '\r\n')
[ -n "$TOKEN" ] || { cat "$TOKEN_RESPONSE" >&2; fail "no token returned"; }
PROJECT_ID=$(grep -A 10000 '^{' "$TOKEN_RESPONSE" | jq -r '.token.project.id')
[ -n "$PROJECT_ID" ] && [ "$PROJECT_ID" != "null" ] || fail "no project id in token response"

# --- look up a flavor + image ---
FLAVOR_ID=$(curl -sf -H "X-Auth-Token: $TOKEN" http://localhost:8774/v2.1/flavors | \
    jq -r '.flavors[] | select(.name=="m1.tiny") | .id')
[ -n "$FLAVOR_ID" ] || fail "could not find m1.tiny flavor"

# Glance is in stub mode for this test; the smoke test for real Glance is
# the separate glance-rbd CI job. We just need *some* image ref so Nova's
# create handler accepts the request. Stub Glance returns a fixed list.
IMAGE_ID=$(curl -sf -H "X-Auth-Token: $TOKEN" http://localhost:9292/v2/images | \
    jq -r '.images[0].id // empty')
if [ -z "$IMAGE_ID" ]; then
    warn "no image available; uploading a tiny placeholder"
    IMAGE_ID=$(curl -sf -X POST http://localhost:9292/v2/images \
        -H "X-Auth-Token: $TOKEN" -H "Content-Type: application/json" \
        -d '{"name":"smoke-placeholder","disk_format":"raw","container_format":"bare"}' | \
        jq -r '.id')
fi
[ -n "$IMAGE_ID" ] || fail "no image id available"

# --- create server ---
SERVER_NAME="o3k-smoke-$(date +%s)"
log "creating server $SERVER_NAME"
CREATE_RESP=$(curl -sf -X POST http://localhost:8774/v2.1/servers \
    -H "X-Auth-Token: $TOKEN" -H "Content-Type: application/json" \
    -d "{\"server\":{\"name\":\"$SERVER_NAME\",\"flavorRef\":\"$FLAVOR_ID\",\"imageRef\":\"$IMAGE_ID\"}}")
SERVER_ID=$(echo "$CREATE_RESP" | jq -r '.server.id')
[ -n "$SERVER_ID" ] && [ "$SERVER_ID" != "null" ] || { echo "$CREATE_RESP" >&2; fail "create returned no id"; }
log "server id: $SERVER_ID"

# --- wait for ACTIVE ---
for i in $(seq 1 120); do
    STATUS=$(curl -sf -H "X-Auth-Token: $TOKEN" "http://localhost:8774/v2.1/servers/$SERVER_ID" | jq -r '.server.status')
    log "  status[$i]: $STATUS"
    case "$STATUS" in
        ACTIVE) break ;;
        ERROR)  fail "server entered ERROR state" ;;
    esac
    sleep 1
    [ "$i" -eq 120 ] && fail "server did not reach ACTIVE within 120s (last status: $STATUS)"
done

# --- assert libvirt domain exists ---
log "verifying libvirt domain exists"
if ! virsh -c qemu:///system list --all --name | grep -qx "instance-$SERVER_ID"; then
    virsh -c qemu:///system list --all >&2 || true
    fail "libvirt domain instance-$SERVER_ID not found"
fi

# --- delete + assert removal ---
log "deleting server"
curl -sf -X DELETE -H "X-Auth-Token: $TOKEN" "http://localhost:8774/v2.1/servers/$SERVER_ID"

for i in $(seq 1 30); do
    if ! curl -sf -H "X-Auth-Token: $TOKEN" "http://localhost:8774/v2.1/servers/$SERVER_ID" >/dev/null 2>&1; then
        log "  server gone from API after ${i}s"
        break
    fi
    sleep 1
    [ "$i" -eq 30 ] && fail "server still present in API after delete"
done

if virsh -c qemu:///system list --all --name | grep -qx "instance-$SERVER_ID"; then
    fail "libvirt domain instance-$SERVER_ID still present after server delete"
fi

log "libvirt smoke test PASSED"
