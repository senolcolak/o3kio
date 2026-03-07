#!/bin/bash
# O3K Integration Test Suite
# Tests all OpenStack API endpoints and Horizon compatibility

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Test counters
TESTS_PASSED=0
TESTS_FAILED=0
TESTS_TOTAL=0

# Configuration
O3K_URL="${O3K_URL:-http://localhost:35357}"
ADMIN_USER="${ADMIN_USER:-admin}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-secret}"
PROJECT_NAME="${PROJECT_NAME:-default}"

# Function to print test status
print_test() {
    local test_name=$1
    echo -e "\n${YELLOW}[TEST]${NC} $test_name"
}

print_pass() {
    echo -e "${GREEN}[PASS]${NC} $1"
    ((TESTS_PASSED++))
    ((TESTS_TOTAL++))
}

print_fail() {
    echo -e "${RED}[FAIL]${NC} $1"
    ((TESTS_FAILED++))
    ((TESTS_TOTAL++))
}

print_info() {
    echo -e "  ℹ  $1"
}

# Function to check if O3K is running
check_o3k_running() {
    print_test "Checking if O3K is running"
    if curl -s -f "${O3K_URL}/v3" > /dev/null 2>&1; then
        print_pass "O3K is running on ${O3K_URL}"
        return 0
    else
        print_fail "O3K is not running on ${O3K_URL}"
        echo "Start O3K with: ./o3k --config config/o3k.yaml"
        exit 1
    fi
}

# Test 1: Keystone Authentication
test_keystone_auth() {
    print_test "Keystone: Unscoped token authentication"

    response=$(curl -s -i -X POST "${O3K_URL}/v3/auth/tokens" \
        -H "Content-Type: application/json" \
        -d '{
            "auth": {
                "identity": {
                    "methods": ["password"],
                    "password": {
                        "user": {
                            "name": "'$ADMIN_USER'",
                            "domain": {"name": "Default"},
                            "password": "'$ADMIN_PASSWORD'"
                        }
                    }
                }
            }
        }')

    # Extract token from X-Subject-Token header
    TOKEN=$(echo "$response" | grep -i "X-Subject-Token:" | awk '{print $2}' | tr -d '\r')

    if [ -n "$TOKEN" ]; then
        print_pass "Unscoped token obtained: ${TOKEN:0:20}..."
        export OS_TOKEN=$TOKEN
    else
        print_fail "Failed to obtain unscoped token"
        echo "$response"
        return 1
    fi

    # Test scoped token
    print_test "Keystone: Scoped token authentication"

    response=$(curl -s -i -X POST "${O3K_URL}/v3/auth/tokens" \
        -H "Content-Type: application/json" \
        -d '{
            "auth": {
                "identity": {
                    "methods": ["password"],
                    "password": {
                        "user": {
                            "name": "'$ADMIN_USER'",
                            "domain": {"name": "Default"},
                            "password": "'$ADMIN_PASSWORD'"
                        }
                    }
                },
                "scope": {
                    "project": {
                        "name": "'$PROJECT_NAME'",
                        "domain": {"name": "Default"}
                    }
                }
            }
        }')

    SCOPED_TOKEN=$(echo "$response" | grep -i "X-Subject-Token:" | awk '{print $2}' | tr -d '\r')

    if [ -n "$SCOPED_TOKEN" ]; then
        print_pass "Scoped token obtained: ${SCOPED_TOKEN:0:20}..."
        export OS_TOKEN=$SCOPED_TOKEN

        # Check for service catalog
        if echo "$response" | grep -q "catalog"; then
            print_pass "Service catalog present in response"
        else
            print_fail "Service catalog missing from scoped token response"
        fi
    else
        print_fail "Failed to obtain scoped token"
        return 1
    fi
}

# Test 2: Keystone User/Project Management
test_keystone_resources() {
    print_test "Keystone: List users"

    response=$(curl -s -H "X-Auth-Token: $OS_TOKEN" "${O3K_URL}/v3/users")

    if echo "$response" | grep -q "users"; then
        print_pass "User list retrieved"
        print_info "Found $(echo "$response" | grep -o '"name"' | wc -l) users"
    else
        print_fail "Failed to retrieve user list"
    fi

    print_test "Keystone: List projects"

    response=$(curl -s -H "X-Auth-Token: $OS_TOKEN" "${O3K_URL}/v3/projects")

    if echo "$response" | grep -q "projects"; then
        print_pass "Project list retrieved"
        print_info "Found $(echo "$response" | grep -o '"name"' | wc -l) projects"
    else
        print_fail "Failed to retrieve project list"
    fi
}

# Test 3: Nova Server Operations
test_nova_servers() {
    print_test "Nova: List servers"

    response=$(curl -s -H "X-Auth-Token: $OS_TOKEN" "http://localhost:8774/v2.1/servers")

    if echo "$response" | grep -q "servers"; then
        print_pass "Server list retrieved"
        print_info "Found $(echo "$response" | grep -o '"id"' | wc -l) servers"
    else
        print_fail "Failed to retrieve server list"
    fi

    print_test "Nova: List flavors"

    response=$(curl -s -H "X-Auth-Token: $OS_TOKEN" "http://localhost:8774/v2.1/flavors")

    if echo "$response" | grep -q "flavors"; then
        print_pass "Flavor list retrieved"

        # Extract first flavor ID for later tests
        FLAVOR_ID=$(echo "$response" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
        export FLAVOR_ID
        print_info "Using flavor: $FLAVOR_ID"
    else
        print_fail "Failed to retrieve flavor list"
    fi

    print_test "Nova: List hypervisors"

    response=$(curl -s -H "X-Auth-Token: $OS_TOKEN" "http://localhost:8774/v2.1/os-hypervisors")

    if echo "$response" | grep -q "hypervisors"; then
        print_pass "Hypervisor list retrieved"
    else
        print_fail "Failed to retrieve hypervisor list"
    fi
}

# Test 4: Neutron Network Operations
test_neutron_networks() {
    print_test "Neutron: List networks"

    response=$(curl -s -H "X-Auth-Token: $OS_TOKEN" "http://localhost:9696/v2.0/networks")

    if echo "$response" | grep -q "networks"; then
        print_pass "Network list retrieved"
        print_info "Found $(echo "$response" | grep -o '"id"' | wc -l) networks"
    else
        print_fail "Failed to retrieve network list"
    fi

    print_test "Neutron: Create network"

    NETWORK_NAME="test-network-$$"
    response=$(curl -s -X POST -H "X-Auth-Token: $OS_TOKEN" \
        -H "Content-Type: application/json" \
        "http://localhost:9696/v2.0/networks" \
        -d '{
            "network": {
                "name": "'$NETWORK_NAME'",
                "admin_state_up": true
            }
        }')

    if echo "$response" | grep -q "$NETWORK_NAME"; then
        NETWORK_ID=$(echo "$response" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
        export NETWORK_ID
        print_pass "Network created: $NETWORK_ID"
    else
        print_fail "Failed to create network"
        echo "$response"
    fi

    print_test "Neutron: Create subnet"

    response=$(curl -s -X POST -H "X-Auth-Token: $OS_TOKEN" \
        -H "Content-Type: application/json" \
        "http://localhost:9696/v2.0/subnets" \
        -d '{
            "subnet": {
                "network_id": "'$NETWORK_ID'",
                "ip_version": 4,
                "cidr": "192.168.100.0/24",
                "gateway_ip": "192.168.100.1"
            }
        }')

    if echo "$response" | grep -q "192.168.100"; then
        SUBNET_ID=$(echo "$response" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
        export SUBNET_ID
        print_pass "Subnet created: $SUBNET_ID"
    else
        print_fail "Failed to create subnet"
    fi

    print_test "Neutron: List security groups"

    response=$(curl -s -H "X-Auth-Token: $OS_TOKEN" "http://localhost:9696/v2.0/security-groups")

    if echo "$response" | grep -q "security_groups"; then
        print_pass "Security group list retrieved"
    else
        print_fail "Failed to retrieve security group list"
    fi
}

# Test 5: Cinder Volume Operations
test_cinder_volumes() {
    print_test "Cinder: List volumes"

    # Extract project ID from token
    PROJECT_ID=$(curl -s -H "X-Auth-Token: $OS_TOKEN" "${O3K_URL}/v3/auth/projects" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)

    response=$(curl -s -H "X-Auth-Token: $OS_TOKEN" "http://localhost:8776/v3/${PROJECT_ID}/volumes")

    if echo "$response" | grep -q "volumes"; then
        print_pass "Volume list retrieved"
        print_info "Found $(echo "$response" | grep -o '"id"' | wc -l) volumes"
    else
        print_fail "Failed to retrieve volume list"
    fi

    print_test "Cinder: Create volume"

    VOLUME_NAME="test-volume-$$"
    response=$(curl -s -X POST -H "X-Auth-Token: $OS_TOKEN" \
        -H "Content-Type: application/json" \
        "http://localhost:8776/v3/${PROJECT_ID}/volumes" \
        -d '{
            "volume": {
                "name": "'$VOLUME_NAME'",
                "size": 1
            }
        }')

    if echo "$response" | grep -q "$VOLUME_NAME"; then
        VOLUME_ID=$(echo "$response" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
        export VOLUME_ID
        print_pass "Volume created: $VOLUME_ID"
    else
        print_fail "Failed to create volume"
        echo "$response"
    fi

    print_test "Cinder: List volume types"

    response=$(curl -s -H "X-Auth-Token: $OS_TOKEN" "http://localhost:8776/v3/${PROJECT_ID}/types")

    if echo "$response" | grep -q "volume_types"; then
        print_pass "Volume type list retrieved"
    else
        print_fail "Failed to retrieve volume type list"
    fi
}

# Test 6: Glance Image Operations
test_glance_images() {
    print_test "Glance: List images"

    response=$(curl -s -H "X-Auth-Token: $OS_TOKEN" "http://localhost:9292/v2/images")

    if echo "$response" | grep -q "images"; then
        print_pass "Image list retrieved"
        print_info "Found $(echo "$response" | grep -o '"id"' | wc -l) images"
    else
        print_fail "Failed to retrieve image list"
    fi

    print_test "Glance: Create image metadata"

    IMAGE_NAME="test-image-$$"
    response=$(curl -s -X POST -H "X-Auth-Token: $OS_TOKEN" \
        -H "Content-Type: application/json" \
        "http://localhost:9292/v2/images" \
        -d '{
            "name": "'$IMAGE_NAME'",
            "disk_format": "raw",
            "container_format": "bare",
            "visibility": "private"
        }')

    if echo "$response" | grep -q "$IMAGE_NAME"; then
        IMAGE_ID=$(echo "$response" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
        export IMAGE_ID
        print_pass "Image metadata created: $IMAGE_ID"
    else
        print_fail "Failed to create image metadata"
        echo "$response"
    fi

    print_test "Glance: Upload image data"

    # Create small test file (1MB)
    dd if=/dev/urandom of=/tmp/test-image-$$.raw bs=1M count=1 2>/dev/null

    response=$(curl -s -w "\n%{http_code}" -X PUT -H "X-Auth-Token: $OS_TOKEN" \
        -H "Content-Type: application/octet-stream" \
        "http://localhost:9292/v2/images/${IMAGE_ID}/file" \
        --data-binary @/tmp/test-image-$$.raw)

    http_code=$(echo "$response" | tail -1)

    if [ "$http_code" = "204" ]; then
        print_pass "Image data uploaded successfully"
    else
        print_fail "Failed to upload image data (HTTP $http_code)"
    fi

    # Cleanup temp file
    rm -f /tmp/test-image-$$.raw

    print_test "Glance: Download image data"

    response=$(curl -s -w "\n%{http_code}" -H "X-Auth-Token: $OS_TOKEN" \
        "http://localhost:9292/v2/images/${IMAGE_ID}/file" \
        -o /tmp/downloaded-image-$$.raw)

    http_code=$(echo "$response" | tail -1)

    if [ "$http_code" = "200" ]; then
        size=$(stat -f%z /tmp/downloaded-image-$$.raw 2>/dev/null || stat -c%s /tmp/downloaded-image-$$.raw 2>/dev/null)
        print_pass "Image data downloaded successfully ($size bytes)"
    else
        print_fail "Failed to download image data (HTTP $http_code)"
    fi

    # Cleanup
    rm -f /tmp/downloaded-image-$$.raw
}

# Test 7: Cross-Service Integration
test_cross_service() {
    print_test "Cross-Service: Create VM with network and volume"

    if [ -z "$NETWORK_ID" ] || [ -z "$FLAVOR_ID" ] || [ -z "$IMAGE_ID" ]; then
        print_fail "Prerequisites missing (network, flavor, or image)"
        return 1
    fi

    SERVER_NAME="test-server-$$"
    response=$(curl -s -X POST -H "X-Auth-Token: $OS_TOKEN" \
        -H "Content-Type: application/json" \
        "http://localhost:8774/v2.1/servers" \
        -d '{
            "server": {
                "name": "'$SERVER_NAME'",
                "flavorRef": "'$FLAVOR_ID'",
                "imageRef": "'$IMAGE_ID'",
                "networks": [{"uuid": "'$NETWORK_ID'"}],
                "block_device_mapping_v2": [
                    {
                        "boot_index": 0,
                        "uuid": "'$IMAGE_ID'",
                        "source_type": "image",
                        "destination_type": "local"
                    }
                ]
            }
        }')

    if echo "$response" | grep -q "$SERVER_NAME"; then
        SERVER_ID=$(echo "$response" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
        export SERVER_ID
        print_pass "Server created: $SERVER_ID"

        # Wait for server to become active
        print_info "Waiting for server to become active..."
        for i in {1..10}; do
            sleep 2
            status=$(curl -s -H "X-Auth-Token: $OS_TOKEN" \
                "http://localhost:8774/v2.1/servers/${SERVER_ID}" | \
                grep -o '"status":"[^"]*"' | head -1 | cut -d'"' -f4)

            if [ "$status" = "ACTIVE" ] || [ "$status" = "ERROR" ]; then
                break
            fi
        done

        if [ "$status" = "ACTIVE" ]; then
            print_pass "Server is active"
        else
            print_fail "Server did not become active (status: $status)"
        fi
    else
        print_fail "Failed to create server"
        echo "$response"
    fi
}

# Test 8: Cleanup
test_cleanup() {
    print_test "Cleanup: Deleting test resources"

    # Delete server
    if [ -n "$SERVER_ID" ]; then
        curl -s -X DELETE -H "X-Auth-Token: $OS_TOKEN" \
            "http://localhost:8774/v2.1/servers/${SERVER_ID}" > /dev/null
        print_info "Deleted server: $SERVER_ID"
    fi

    # Delete image
    if [ -n "$IMAGE_ID" ]; then
        curl -s -X DELETE -H "X-Auth-Token: $OS_TOKEN" \
            "http://localhost:9292/v2/images/${IMAGE_ID}" > /dev/null
        print_info "Deleted image: $IMAGE_ID"
    fi

    # Delete volume
    if [ -n "$VOLUME_ID" ]; then
        PROJECT_ID=$(curl -s -H "X-Auth-Token: $OS_TOKEN" "${O3K_URL}/v3/auth/projects" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
        curl -s -X DELETE -H "X-Auth-Token: $OS_TOKEN" \
            "http://localhost:8776/v3/${PROJECT_ID}/volumes/${VOLUME_ID}" > /dev/null
        print_info "Deleted volume: $VOLUME_ID"
    fi

    # Delete network (will cascade delete subnet)
    if [ -n "$NETWORK_ID" ]; then
        curl -s -X DELETE -H "X-Auth-Token: $OS_TOKEN" \
            "http://localhost:9696/v2.0/networks/${NETWORK_ID}" > /dev/null
        print_info "Deleted network: $NETWORK_ID"
    fi

    print_pass "Cleanup completed"
}

# Test 9: Stress Test (Concurrent Operations)
test_stress() {
    print_test "Stress Test: Creating 5 concurrent networks"

    pids=()
    for i in {1..5}; do
        (
            curl -s -X POST -H "X-Auth-Token: $OS_TOKEN" \
                -H "Content-Type: application/json" \
                "http://localhost:9696/v2.0/networks" \
                -d '{"network": {"name": "stress-test-'$i'-$$", "admin_state_up": true}}' \
                > /dev/null
        ) &
        pids+=($!)
    done

    # Wait for all requests
    failed=0
    for pid in "${pids[@]}"; do
        if ! wait $pid; then
            ((failed++))
        fi
    done

    if [ $failed -eq 0 ]; then
        print_pass "All 5 concurrent network creations succeeded"
    else
        print_fail "$failed out of 5 concurrent network creations failed"
    fi

    # Cleanup stress test networks
    response=$(curl -s -H "X-Auth-Token: $OS_TOKEN" "http://localhost:9696/v2.0/networks")
    stress_networks=$(echo "$response" | grep -o '"id":"[^"]*"' | grep -A1 "stress-test" | grep '"id"' | cut -d'"' -f4)

    for net_id in $stress_networks; do
        curl -s -X DELETE -H "X-Auth-Token: $OS_TOKEN" \
            "http://localhost:9696/v2.0/networks/${net_id}" > /dev/null 2>&1
    done
}

# Test 10: Version Discovery
test_version_discovery() {
    print_test "Version Discovery: Keystone"
    response=$(curl -s "${O3K_URL}/")
    if echo "$response" | grep -q "versions"; then
        print_pass "Keystone version discovery works"
    else
        print_fail "Keystone version discovery failed"
    fi

    print_test "Version Discovery: Nova"
    response=$(curl -s "http://localhost:8774/")
    if echo "$response" | grep -q "versions"; then
        print_pass "Nova version discovery works"
    else
        print_fail "Nova version discovery failed"
    fi

    print_test "Version Discovery: Neutron"
    response=$(curl -s "http://localhost:9696/")
    if echo "$response" | grep -q "versions"; then
        print_pass "Neutron version discovery works"
    else
        print_fail "Neutron version discovery failed"
    fi

    print_test "Version Discovery: Glance"
    response=$(curl -s "http://localhost:9292/")
    if echo "$response" | grep -q "versions"; then
        print_pass "Glance version discovery works"
    else
        print_fail "Glance version discovery failed"
    fi
}

# Main test execution
main() {
    echo "======================================"
    echo "O3K Integration Test Suite"
    echo "======================================"
    echo ""

    check_o3k_running

    echo ""
    echo "======================================"
    echo "Phase 1: Authentication & Authorization"
    echo "======================================"
    test_keystone_auth
    test_keystone_resources

    echo ""
    echo "======================================"
    echo "Phase 2: Service Endpoints"
    echo "======================================"
    test_version_discovery
    test_nova_servers
    test_neutron_networks
    test_cinder_volumes
    test_glance_images

    echo ""
    echo "======================================"
    echo "Phase 3: Cross-Service Integration"
    echo "======================================"
    test_cross_service

    echo ""
    echo "======================================"
    echo "Phase 4: Stress Testing"
    echo "======================================"
    test_stress

    echo ""
    echo "======================================"
    echo "Phase 5: Cleanup"
    echo "======================================"
    test_cleanup

    echo ""
    echo "======================================"
    echo "Test Summary"
    echo "======================================"
    echo -e "Total Tests: $TESTS_TOTAL"
    echo -e "${GREEN}Passed: $TESTS_PASSED${NC}"
    echo -e "${RED}Failed: $TESTS_FAILED${NC}"

    if [ $TESTS_FAILED -eq 0 ]; then
        echo ""
        echo -e "${GREEN}✓ All tests passed!${NC}"
        exit 0
    else
        echo ""
        echo -e "${RED}✗ Some tests failed${NC}"
        exit 1
    fi
}

# Run main
main
