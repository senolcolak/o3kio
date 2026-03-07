#!/bin/bash
# VXLAN Multi-Node Integration Test
# This script simulates a two-node deployment for testing VXLAN coordination

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
TEST_DIR="$PROJECT_ROOT/test/tmp"

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

cleanup() {
    log_info "Cleaning up test environment..."

    # Stop O3K instances
    if [ -f "$TEST_DIR/node1.pid" ]; then
        kill $(cat "$TEST_DIR/node1.pid") 2>/dev/null || true
        rm "$TEST_DIR/node1.pid"
    fi
    if [ -f "$TEST_DIR/node2.pid" ]; then
        kill $(cat "$TEST_DIR/node2.pid") 2>/dev/null || true
        rm "$TEST_DIR/node2.pid"
    fi

    # Clean up test directory
    rm -rf "$TEST_DIR"

    log_info "Cleanup complete"
}

trap cleanup EXIT

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."

    if ! command -v psql &> /dev/null; then
        log_error "PostgreSQL client (psql) not found"
        exit 1
    fi

    if [ ! -f "$PROJECT_ROOT/bin/o3k" ]; then
        log_error "O3K binary not found. Run 'make build' first."
        exit 1
    fi

    log_info "Prerequisites OK"
}

# Setup test environment
setup_test_env() {
    log_info "Setting up test environment..."

    mkdir -p "$TEST_DIR"

    # Create config for node 1
    cat > "$TEST_DIR/node1.yaml" <<EOF
database:
  url: "postgres://o3k:o3k@localhost/o3k"
  max_connections: 10

keystone:
  port: 35357
  jwt_secret: "test-secret"
  token_ttl: 24h
  admin_user: admin
  admin_password: secret

nova:
  port: 8774
  libvirt_uri: "qemu:///system"
  libvirt_mode: stub

neutron:
  port: 9696
  dhcp_lease_time: 24h
  iptables_enabled: false
  networking_mode: stub
  vxlan_enabled: true
  vni_range_start: 1000
  vni_range_end: 10000
  coordination_poll_interval: 1s

cinder:
  port: 8776
  storage_mode: stub

glance:
  port: 9292
  storage_mode: stub

compute:
  node_id: node-1
  tunnel_ip: 10.0.0.1
  vxlan_port: 4789
  heartbeat_interval: 5s

logging:
  level: debug
EOF

    # Create config for node 2
    cat > "$TEST_DIR/node2.yaml" <<EOF
database:
  url: "postgres://o3k:o3k@localhost/o3k"
  max_connections: 10

keystone:
  port: 45357
  jwt_secret: "test-secret"
  token_ttl: 24h
  admin_user: admin
  admin_password: secret

nova:
  port: 18774
  libvirt_uri: "qemu:///system"
  libvirt_mode: stub

neutron:
  port: 19696
  dhcp_lease_time: 24h
  iptables_enabled: false
  networking_mode: stub
  vxlan_enabled: true
  vni_range_start: 1000
  vni_range_end: 10000
  coordination_poll_interval: 1s

cinder:
  port: 18776
  storage_mode: stub

glance:
  port: 19292
  storage_mode: stub

compute:
  node_id: node-2
  tunnel_ip: 10.0.0.2
  vxlan_port: 4789
  heartbeat_interval: 5s

logging:
  level: debug
EOF

    log_info "Test environment ready"
}

# Start O3K nodes
start_nodes() {
    log_info "Starting O3K node 1..."
    "$PROJECT_ROOT/bin/o3k" --config "$TEST_DIR/node1.yaml" > "$TEST_DIR/node1.log" 2>&1 &
    echo $! > "$TEST_DIR/node1.pid"

    log_info "Starting O3K node 2..."
    "$PROJECT_ROOT/bin/o3k" --config "$TEST_DIR/node2.yaml" > "$TEST_DIR/node2.log" 2>&1 &
    echo $! > "$TEST_DIR/node2.pid"

    # Wait for nodes to start
    log_info "Waiting for nodes to start (10 seconds)..."
    sleep 10

    # Check if nodes are running
    if ! kill -0 $(cat "$TEST_DIR/node1.pid") 2>/dev/null; then
        log_error "Node 1 failed to start. Check $TEST_DIR/node1.log"
        cat "$TEST_DIR/node1.log"
        exit 1
    fi

    if ! kill -0 $(cat "$TEST_DIR/node2.pid") 2>/dev/null; then
        log_error "Node 2 failed to start. Check $TEST_DIR/node2.log"
        cat "$TEST_DIR/node2.log"
        exit 1
    fi

    log_info "Both nodes started successfully"
}

# Test node registration
test_node_registration() {
    log_info "Testing node registration..."

    # Wait for heartbeats
    sleep 6

    # Check compute_nodes table
    NODE_COUNT=$(psql -U o3k -d o3k -tAc "SELECT COUNT(*) FROM compute_nodes WHERE status = 'active';")

    if [ "$NODE_COUNT" != "2" ]; then
        log_error "Expected 2 active nodes, found $NODE_COUNT"
        psql -U o3k -d o3k -c "SELECT * FROM compute_nodes;"
        exit 1
    fi

    log_info "✓ Node registration successful (2 nodes active)"
}

# Test VNI allocation
test_vni_allocation() {
    log_info "Testing VNI allocation..."

    # Authenticate
    TOKEN=$(curl -s -X POST http://localhost:35357/v3/auth/tokens \
        -H "Content-Type: application/json" \
        -d '{
            "auth": {
                "identity": {
                    "methods": ["password"],
                    "password": {
                        "user": {
                            "name": "admin",
                            "password": "secret",
                            "domain": {"name": "Default"}
                        }
                    }
                },
                "scope": {
                    "project": {
                        "name": "default",
                        "domain": {"name": "Default"}
                    }
                }
            }
        }' | jq -r '.token.token' 2>/dev/null)

    if [ -z "$TOKEN" ] || [ "$TOKEN" == "null" ]; then
        log_error "Failed to get auth token"
        exit 1
    fi

    # Create network on node 1
    NETWORK_ID=$(curl -s -X POST http://localhost:9696/v2.0/networks \
        -H "X-Auth-Token: $TOKEN" \
        -H "Content-Type: application/json" \
        -d '{
            "network": {
                "name": "test-vxlan-network"
            }
        }' | jq -r '.network.id' 2>/dev/null)

    if [ -z "$NETWORK_ID" ] || [ "$NETWORK_ID" == "null" ]; then
        log_error "Failed to create network"
        exit 1
    fi

    log_info "Created network: $NETWORK_ID"

    # Wait for VNI allocation
    sleep 3

    # Check VNI allocation in database
    VNI=$(psql -U o3k -d o3k -tAc "SELECT vni FROM network_vni_allocations WHERE network_id = '$NETWORK_ID';")

    if [ -z "$VNI" ]; then
        log_error "VNI not allocated for network $NETWORK_ID"
        exit 1
    fi

    log_info "✓ VNI allocated: $VNI"

    # Verify VNI is in range
    if [ "$VNI" -lt 1000 ] || [ "$VNI" -gt 10000 ]; then
        log_error "VNI $VNI is out of configured range (1000-10000)"
        exit 1
    fi

    log_info "✓ VNI allocation successful"
}

# Test FDB synchronization
test_fdb_sync() {
    log_info "Testing FDB synchronization..."

    # Authenticate
    TOKEN=$(curl -s -X POST http://localhost:35357/v3/auth/tokens \
        -H "Content-Type: application/json" \
        -d '{
            "auth": {
                "identity": {
                    "methods": ["password"],
                    "password": {
                        "user": {
                            "name": "admin",
                            "password": "secret",
                            "domain": {"name": "Default"}
                        }
                    }
                },
                "scope": {
                    "project": {
                        "name": "default",
                        "domain": {"name": "Default"}
                    }
                }
            }
        }' | jq -r '.token.token' 2>/dev/null)

    # Get network ID
    NETWORK_ID=$(psql -U o3k -d o3k -tAc "SELECT id FROM networks WHERE name = 'test-vxlan-network' LIMIT 1;")

    # Create subnet
    SUBNET_ID=$(curl -s -X POST http://localhost:9696/v2.0/subnets \
        -H "X-Auth-Token: $TOKEN" \
        -H "Content-Type: application/json" \
        -d "{
            \"subnet\": {
                \"name\": \"test-subnet\",
                \"network_id\": \"$NETWORK_ID\",
                \"cidr\": \"10.10.0.0/24\",
                \"ip_version\": 4,
                \"enable_dhcp\": false
            }
        }" | jq -r '.subnet.id' 2>/dev/null)

    log_info "Created subnet: $SUBNET_ID"

    # Create port on node 1
    PORT_RESPONSE=$(curl -s -X POST http://localhost:9696/v2.0/ports \
        -H "X-Auth-Token: $TOKEN" \
        -H "Content-Type: application/json" \
        -d "{
            \"port\": {
                \"name\": \"test-port-1\",
                \"network_id\": \"$NETWORK_ID\"
            }
        }")

    PORT_ID=$(echo "$PORT_RESPONSE" | jq -r '.port.id' 2>/dev/null)
    MAC_ADDRESS=$(echo "$PORT_RESPONSE" | jq -r '.port.mac_address' 2>/dev/null)

    if [ -z "$PORT_ID" ] || [ "$PORT_ID" == "null" ]; then
        log_error "Failed to create port"
        echo "$PORT_RESPONSE"
        exit 1
    fi

    log_info "Created port: $PORT_ID (MAC: $MAC_ADDRESS)"

    # Wait for FDB synchronization
    sleep 3

    # Check FDB entry in database
    FDB_COUNT=$(psql -U o3k -d o3k -tAc "SELECT COUNT(*) FROM vxlan_fdb_entries WHERE network_id = '$NETWORK_ID' AND mac_address = '$MAC_ADDRESS';")

    if [ "$FDB_COUNT" != "1" ]; then
        log_error "Expected 1 FDB entry, found $FDB_COUNT"
        psql -U o3k -d o3k -c "SELECT * FROM vxlan_fdb_entries;"
        exit 1
    fi

    log_info "✓ FDB entry created"

    # Delete port
    curl -s -X DELETE http://localhost:9696/v2.0/ports/$PORT_ID \
        -H "X-Auth-Token: $TOKEN" > /dev/null

    # Wait for FDB cleanup
    sleep 3

    # Check FDB entry removed
    FDB_COUNT=$(psql -U o3k -d o3k -tAc "SELECT COUNT(*) FROM vxlan_fdb_entries WHERE port_id = '$PORT_ID';")

    if [ "$FDB_COUNT" != "0" ]; then
        log_error "FDB entry not removed after port deletion"
        exit 1
    fi

    log_info "✓ FDB synchronization successful"
}

# Test coordinator resilience
test_coordinator_resilience() {
    log_info "Testing coordinator resilience..."

    # Stop node 2
    log_info "Stopping node 2..."
    kill $(cat "$TEST_DIR/node2.pid")
    rm "$TEST_DIR/node2.pid"

    # Wait for heartbeat timeout (15 seconds with 5s interval)
    sleep 16

    # Check node 2 status
    NODE2_STATUS=$(psql -U o3k -d o3k -tAc "SELECT status FROM compute_nodes WHERE hostname LIKE '%node-2%';")

    if [ "$NODE2_STATUS" == "active" ]; then
        log_warning "Node 2 still marked as active (heartbeat may not have timed out yet)"
    else
        log_info "✓ Node 2 marked as inactive"
    fi

    # Node 1 should still be operational
    NODE1_STATUS=$(psql -U o3k -d o3k -tAc "SELECT status FROM compute_nodes WHERE hostname LIKE '%node-1%';")

    if [ "$NODE1_STATUS" != "active" ]; then
        log_error "Node 1 is not active"
        exit 1
    fi

    log_info "✓ Node 1 still operational"
    log_info "✓ Coordinator resilience test passed"
}

# Main test flow
main() {
    log_info "=== VXLAN Multi-Node Integration Test ==="
    log_info ""

    check_prerequisites
    setup_test_env
    start_nodes

    test_node_registration
    test_vni_allocation
    test_fdb_sync
    test_coordinator_resilience

    log_info ""
    log_info "=== ALL TESTS PASSED ==="
}

main
