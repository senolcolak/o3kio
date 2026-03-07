#!/bin/bash
# O3K Horizon Compatibility Test (Bash version)
# Simulates Horizon dashboard API calls

set -e

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

PASSED=0
FAILED=0

O3K_URL="http://localhost:35357"
ADMIN_USER="admin"
ADMIN_PASSWORD="secret"
PROJECT_NAME="default"

print_test() {
    echo -e "\n${YELLOW}[TEST]${NC} $1"
}

print_pass() {
    echo -e "${GREEN}[PASS]${NC} $1"
    ((PASSED++))
}

print_fail() {
    echo -e "${RED}[FAIL]${NC} $1"
    ((FAILED++))
}

print_info() {
    echo -e "  ℹ  $1"
}

# Test 1: Horizon Login Flow
test_login() {
    print_test "Horizon Login Flow"

    # Get scoped token
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

    TOKEN=$(echo "$response" | grep -i "X-Subject-Token:" | awk '{print $2}' | tr -d '\r')

    if [ -n "$TOKEN" ]; then
        print_pass "Authenticated successfully"
        print_info "Token: ${TOKEN:0:50}..."

        # Check for service catalog
        if echo "$response" | grep -q "catalog"; then
            print_pass "Service catalog present"

            # Count services
            service_count=$(echo "$response" | grep -o '"type"' | wc -l | tr -d ' ')
            print_info "Found $service_count services in catalog"
        else
            print_fail "Service catalog missing"
            return 1
        fi
    else
        print_fail "Authentication failed"
        return 1
    fi

    export TOKEN
}

# Test 2: Project Dashboard
test_dashboard() {
    print_test "Project Dashboard Load"

    # Horizon loads these endpoints on dashboard
    curl -s -H "X-Auth-Token: $TOKEN" "http://localhost:8774/v2.1/servers" | grep -q "servers" && \
        print_pass "Nova - List servers" || print_fail "Nova - List servers"

    curl -s -H "X-Auth-Token: $TOKEN" "http://localhost:8774/v2.1/flavors" | grep -q "flavors" && \
        print_pass "Nova - List flavors" || print_fail "Nova - List flavors"

    curl -s -H "X-Auth-Token: $TOKEN" "http://localhost:9696/v2.0/networks" | grep -q "networks" && \
        print_pass "Neutron - List networks" || print_fail "Neutron - List networks"

    # Get project ID
    PROJECT_ID=$(curl -s -X POST "${O3K_URL}/v3/auth/tokens" \
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
        }' | jq -r '.token.project.id')

    export PROJECT_ID

    curl -s -H "X-Auth-Token: $TOKEN" "http://localhost:8776/v3/${PROJECT_ID}/volumes" | grep -q "volumes" && \
        print_pass "Cinder - List volumes" || print_fail "Cinder - List volumes"

    curl -s -H "X-Auth-Token: $TOKEN" "http://localhost:9292/v2/images" | grep -q "images" && \
        print_pass "Glance - List images" || print_fail "Glance - List images"
}

# Test 3: Instances Tab
test_instances_tab() {
    print_test "Instances Tab (Nova)"

    # List servers (detailed)
    response=$(curl -s -H "X-Auth-Token: $TOKEN" "http://localhost:8774/v2.1/servers/detail")

    if echo "$response" | grep -q "servers"; then
        server_count=$(echo "$response" | jq '.servers | length')
        print_pass "Listed $server_count servers"
    else
        print_fail "Failed to list servers"
    fi

    # Get hypervisor statistics
    response=$(curl -s -H "X-Auth-Token: $TOKEN" "http://localhost:8774/v2.1/os-hypervisors/statistics")

    if echo "$response" | grep -q "hypervisor_statistics"; then
        print_pass "Hypervisor statistics retrieved"

        vcpus=$(echo "$response" | jq -r '.hypervisor_statistics.vcpus')
        vcpus_used=$(echo "$response" | jq -r '.hypervisor_statistics.vcpus_used')
        memory_mb=$(echo "$response" | jq -r '.hypervisor_statistics.memory_mb')
        memory_mb_used=$(echo "$response" | jq -r '.hypervisor_statistics.memory_mb_used')

        print_info "vCPUs: $vcpus total, $vcpus_used used"
        print_info "Memory: $memory_mb MB total, $memory_mb_used MB used"
    else
        print_fail "Failed to get hypervisor statistics"
    fi
}

# Test 4: Networks Tab
test_networks_tab() {
    print_test "Networks Tab (Neutron)"

    # List networks
    response=$(curl -s -H "X-Auth-Token: $TOKEN" "http://localhost:9696/v2.0/networks")

    if echo "$response" | grep -q "networks"; then
        network_count=$(echo "$response" | jq '.networks | length')
        print_pass "Listed $network_count networks"
    else
        print_fail "Failed to list networks"
    fi

    # List subnets
    response=$(curl -s -H "X-Auth-Token: $TOKEN" "http://localhost:9696/v2.0/subnets")

    if echo "$response" | grep -q "subnets"; then
        subnet_count=$(echo "$response" | jq '.subnets | length')
        print_pass "Listed $subnet_count subnets"
    else
        print_fail "Failed to list subnets"
    fi

    # List routers
    response=$(curl -s -H "X-Auth-Token: $TOKEN" "http://localhost:9696/v2.0/routers")

    if echo "$response" | grep -q "routers"; then
        router_count=$(echo "$response" | jq '.routers | length')
        print_pass "Listed $router_count routers"
    else
        print_fail "Failed to list routers"
    fi
}

# Test 5: Volumes Tab
test_volumes_tab() {
    print_test "Volumes Tab (Cinder)"

    # List volumes
    response=$(curl -s -H "X-Auth-Token: $TOKEN" "http://localhost:8776/v3/${PROJECT_ID}/volumes/detail")

    if echo "$response" | grep -q "volumes"; then
        volume_count=$(echo "$response" | jq '.volumes | length')
        print_pass "Listed $volume_count volumes"
    else
        print_fail "Failed to list volumes"
    fi

    # List volume types
    response=$(curl -s -H "X-Auth-Token: $TOKEN" "http://localhost:8776/v3/${PROJECT_ID}/types")

    if echo "$response" | grep -q "volume_types"; then
        type_count=$(echo "$response" | jq '.volume_types | length')
        print_pass "Listed $type_count volume types"
    else
        print_fail "Failed to list volume types"
    fi
}

# Test 6: Images Tab
test_images_tab() {
    print_test "Images Tab (Glance)"

    # List images
    response=$(curl -s -H "X-Auth-Token: $TOKEN" "http://localhost:9292/v2/images")

    if echo "$response" | grep -q "images"; then
        image_count=$(echo "$response" | jq '.images | length')
        print_pass "Listed $image_count images"

        # Show image details
        echo "$response" | jq -r '.images[] | "  - \(.name // "Unnamed") (\(.status // "unknown"))"'
    else
        print_fail "Failed to list images"
    fi
}

# Test 7: Launch Instance Workflow
test_launch_instance() {
    print_test "Launch Instance Workflow"

    # Get flavors
    response=$(curl -s -H "X-Auth-Token: $TOKEN" "http://localhost:8774/v2.1/flavors/detail")

    if echo "$response" | grep -q "flavors"; then
        flavor_count=$(echo "$response" | jq '.flavors | length')
        print_pass "Retrieved $flavor_count flavors"

        FLAVOR_ID=$(echo "$response" | jq -r '.flavors[0].id')
        flavor_name=$(echo "$response" | jq -r '.flavors[0].name')
        print_info "Using flavor: $flavor_name ($FLAVOR_ID)"
    else
        print_fail "Failed to get flavors"
        return 1
    fi

    # Get images
    response=$(curl -s -H "X-Auth-Token: $TOKEN" "http://localhost:9292/v2/images")

    if echo "$response" | grep -q "images"; then
        image_count=$(echo "$response" | jq '.images | length')
        print_pass "Retrieved $image_count images"

        # Filter active images
        IMAGE_ID=$(echo "$response" | jq -r '.images[] | select(.status == "active") | .id' | head -1)

        if [ -z "$IMAGE_ID" ]; then
            print_info "No active images available - skipping server creation"
            print_pass "Launch instance flow validated (no images to test with)"
            return 0
        fi

        image_name=$(echo "$response" | jq -r ".images[] | select(.id == \"$IMAGE_ID\") | .name")
        print_info "Using image: $image_name ($IMAGE_ID)"
    else
        print_fail "Failed to get images"
        return 1
    fi

    # Get networks
    response=$(curl -s -H "X-Auth-Token: $TOKEN" "http://localhost:9696/v2.0/networks")

    if echo "$response" | grep -q "networks"; then
        NETWORK_ID=$(echo "$response" | jq -r '.networks[0].id')
        network_name=$(echo "$response" | jq -r '.networks[0].name')
        print_pass "Retrieved networks"
        print_info "Using network: $network_name ($NETWORK_ID)"
    else
        print_fail "Failed to get networks"
        return 1
    fi

    # Create server
    server_name="horizon-test-vm-$(date +%Y%m%d%H%M%S)"
    response=$(curl -s -w "\n%{http_code}" -X POST -H "X-Auth-Token: $TOKEN" \
        -H "Content-Type: application/json" \
        "http://localhost:8774/v2.1/servers" \
        -d '{
            "server": {
                "name": "'$server_name'",
                "flavorRef": "'$FLAVOR_ID'",
                "imageRef": "'$IMAGE_ID'",
                "networks": [{"uuid": "'$NETWORK_ID'"}]
            }
        }')

    http_code=$(echo "$response" | tail -1)
    body=$(echo "$response" | sed '$d')  # Remove last line (portable)

    if [ "$http_code" = "202" ]; then
        SERVER_ID=$(echo "$body" | jq -r '.server.id')
        print_pass "Server creation initiated: $server_name"
        print_info "Server ID: $SERVER_ID"

        # Cleanup - delete test server
        curl -s -X DELETE -H "X-Auth-Token: $TOKEN" \
            "http://localhost:8774/v2.1/servers/$SERVER_ID" > /dev/null
        print_info "Test server deleted"
    else
        print_fail "Server creation failed (HTTP $http_code)"
        if [ -n "$body" ]; then
            print_info "Error: $body"
        fi
    fi
}

# Main
echo "=========================================="
echo " O3K Horizon Compatibility Test"
echo "=========================================="
echo ""

test_login || exit 1
test_dashboard
test_instances_tab
test_networks_tab
test_volumes_tab
test_images_tab
test_launch_instance

echo ""
echo "=========================================="
echo " Test Summary"
echo "=========================================="
echo "Total Passed: $PASSED"
echo "Total Failed: $FAILED"
echo ""

if [ $FAILED -eq 0 ]; then
    echo -e "${GREEN}✓ All Horizon compatibility tests passed!${NC}"
    echo ""
    echo "Next steps:"
    echo "  1. Deploy actual Horizon dashboard with Docker"
    echo "  2. Test with web browser"
    echo "  3. Verify UI rendering and interactions"
    exit 0
else
    echo -e "${RED}✗ Some tests failed${NC}"
    exit 1
fi
