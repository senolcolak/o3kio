#!/bin/bash
set -e

# Horizon Full Functionality Test
# Tests all major Horizon dashboard features against O3K

echo "================================"
echo "Horizon Full Functionality Test"
echo "================================"
echo ""

# Configuration
export OS_AUTH_URL=http://10.2.199.101:35357/v3
export OS_USERNAME=admin
export OS_PASSWORD=secret
export OS_PROJECT_NAME=default
export OS_DOMAIN_NAME=Default
export OS_IDENTITY_API_VERSION=3
export OS_ENDPOINT_OVERRIDE_network=http://10.2.199.101:9696
export OS_ENDPOINT_OVERRIDE_compute=http://10.2.199.101:8774
export OS_ENDPOINT_OVERRIDE_volume=http://10.2.199.101:8776
export OS_ENDPOINT_OVERRIDE_image=http://10.2.199.101:9292

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

PASSED=0
FAILED=0

test_endpoint() {
    local name=$1
    local method=$2
    local path=$3
    local expected_status=$4
    local extra_args=${5:-}

    echo -n "Testing $name... "

    TOKEN=$(openstack token issue -f value -c id 2>/dev/null)

    response=$(curl -s -w "\n%{http_code}" -X $method \
        -H "X-Auth-Token: $TOKEN" \
        -H "Content-Type: application/json" \
        $extra_args \
        "http://10.2.199.101:9696$path" 2>/dev/null || echo "000")

    status_code=$(echo "$response" | tail -1)
    body=$(echo "$response" | head -n -1)

    if [ "$status_code" = "$expected_status" ]; then
        echo -e "${GREEN}✓ PASS${NC} (HTTP $status_code)"
        ((PASSED++))
        return 0
    else
        echo -e "${RED}✗ FAIL${NC} (Expected $expected_status, got $status_code)"
        if [ -n "$body" ]; then
            echo "  Response: $body" | head -c 200
            echo ""
        fi
        ((FAILED++))
        return 1
    fi
}

test_openstack_command() {
    local name=$1
    shift
    local cmd="$@"

    echo -n "Testing $name... "

    if output=$(eval "$cmd" 2>&1); then
        echo -e "${GREEN}✓ PASS${NC}"
        ((PASSED++))
        return 0
    else
        echo -e "${RED}✗ FAIL${NC}"
        echo "  Error: $output" | head -c 200
        echo ""
        ((FAILED++))
        return 1
    fi
}

echo "=== Authentication & Service Catalog ==="
test_openstack_command "Token issue" "openstack token issue -f json > /dev/null"
test_openstack_command "Service catalog" "openstack catalog list -f json > /dev/null"

echo ""
echo "=== Keystone (Identity) ==="
test_openstack_command "List users" "openstack user list -f json > /dev/null"
test_openstack_command "List projects" "openstack project list -f json > /dev/null"
test_openstack_command "List roles" "openstack role list -f json > /dev/null"
test_openstack_command "List domains" "openstack domain list -f json > /dev/null"

echo ""
echo "=== Nova (Compute) ==="
test_openstack_command "List servers" "openstack server list -f json > /dev/null"
test_openstack_command "List flavors" "openstack flavor list -f json > /dev/null"
test_openstack_command "List keypairs" "openstack keypair list -f json > /dev/null"
test_openstack_command "Compute service list" "openstack compute service list -f json > /dev/null"
test_openstack_command "Hypervisor list" "openstack hypervisor list -f json > /dev/null"
test_openstack_command "Hypervisor stats" "openstack hypervisor stats show -f json > /dev/null"

echo ""
echo "=== Neutron (Network) ==="
test_openstack_command "List networks" "openstack network list -f json > /dev/null"
test_openstack_command "List subnets" "openstack subnet list -f json > /dev/null"
test_openstack_command "List ports" "openstack port list -f json > /dev/null"
test_openstack_command "List routers" "openstack router list -f json > /dev/null"
test_openstack_command "List security groups" "openstack security group list -f json > /dev/null"
test_openstack_command "List security group rules" "openstack security group rule list -f json > /dev/null"
test_openstack_command "List floating IPs" "openstack floating ip list -f json > /dev/null"

echo ""
echo "=== Neutron API Endpoints (Direct) ==="
test_endpoint "GET /v2.0" "GET" "/v2.0" "200"
test_endpoint "GET /v2.0/extensions" "GET" "/v2.0/extensions" "200"
test_endpoint "GET /v2.0/networks" "GET" "/v2.0/networks" "200"
test_endpoint "GET /v2.0/subnets" "GET" "/v2.0/subnets" "200"
test_endpoint "GET /v2.0/ports" "GET" "/v2.0/ports" "200"
test_endpoint "GET /v2.0/routers" "GET" "/v2.0/routers" "200"
test_endpoint "GET /v2.0/security-groups" "GET" "/v2.0/security-groups" "200"
test_endpoint "GET /v2.0/floatingips" "GET" "/v2.0/floatingips" "200"

# Get project ID for quota test
PROJECT_ID=$(openstack token issue -f value -c project_id)
test_endpoint "GET /v2.0/quotas/:id" "GET" "/v2.0/quotas/$PROJECT_ID" "200"
test_endpoint "GET /v2.0/quotas/:id/details" "GET" "/v2.0/quotas/$PROJECT_ID/details" "200"

echo ""
echo "=== Cinder (Block Storage) ==="
test_openstack_command "List volumes" "openstack volume list -f json > /dev/null"
test_openstack_command "List volume types" "openstack volume type list -f json > /dev/null"
test_openstack_command "List volume snapshots" "openstack volume snapshot list -f json > /dev/null"

echo ""
echo "=== Glance (Images) ==="
test_openstack_command "List images" "openstack image list -f json > /dev/null"

echo ""
echo "=== Quotas ==="
test_openstack_command "Show Nova quotas" "openstack quota show -f json > /dev/null"

echo ""
echo "==================================="
echo "Test Summary"
echo "==================================="
echo -e "Passed: ${GREEN}$PASSED${NC}"
echo -e "Failed: ${RED}$FAILED${NC}"
echo "Total:  $((PASSED + FAILED))"
echo ""

if [ $FAILED -eq 0 ]; then
    echo -e "${GREEN}All tests passed!${NC}"
    exit 0
else
    echo -e "${RED}Some tests failed.${NC}"
    exit 1
fi
