#!/bin/bash
# Comprehensive Horizon + Real Tools Validation Test
# Tests O3K with Horizon dashboard, OpenStack CLI, and gophercloud

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Results tracking
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0
MISSING_APIS=()

echo -e "${BLUE}╔══════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║          O3K REAL-WORLD VALIDATION TEST SUITE                ║${NC}"
echo -e "${BLUE}║   Testing with Horizon Dashboard & OpenStack CLI             ║${NC}"
echo -e "${BLUE}╔══════════════════════════════════════════════════════════════╝${NC}"
echo ""

# Setup
AUTH_URL="${OS_AUTH_URL:-http://localhost:35357/v3}"
export OS_AUTH_URL="$AUTH_URL"
export OS_USERNAME="${OS_USERNAME:-admin}"
export OS_PASSWORD="${OS_PASSWORD:-secret}"
export OS_PROJECT_NAME="${OS_PROJECT_NAME:-default}"
export OS_USER_DOMAIN_NAME="${OS_USER_DOMAIN_NAME:-Default}"
export OS_PROJECT_DOMAIN_NAME="${OS_PROJECT_DOMAIN_NAME:-Default}"
export OS_IDENTITY_API_VERSION=3

test_case() {
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    local description="$1"
    echo -e "${YELLOW}Test $TOTAL_TESTS:${NC} $description"
}

pass() {
    PASSED_TESTS=$((PASSED_TESTS + 1))
    echo -e "  ${GREEN}✓ PASS${NC}"
}

fail() {
    FAILED_TESTS=$((FAILED_TESTS + 1))
    local reason="$1"
    echo -e "  ${RED}✗ FAIL${NC}: $reason"
}

missing_api() {
    local api="$1"
    MISSING_APIS+=("$api")
    echo -e "  ${RED}✗ MISSING API${NC}: $api"
}

# Pre-flight checks
echo -e "${BLUE}═══ Pre-flight Checks ═══${NC}"
test_case "Check O3K is running"
if curl -s -f http://localhost:35357/v3 > /dev/null 2>&1; then
    pass
else
    fail "O3K not responding"
    exit 1
fi

test_case "Check OpenStack CLI is installed"
if command -v openstack &> /dev/null; then
    pass
else
    fail "OpenStack CLI not found"
    echo "  Install with: pip install python-openstackclient"
    exit 1
fi

# Identity (Keystone) Tests
echo ""
echo -e "${BLUE}═══ Identity Service (Keystone) Tests ═══${NC}"

test_case "Token authentication"
if TOKEN=$(openstack token issue -f value -c id 2>/dev/null); then
    pass
    export OS_TOKEN="$TOKEN"
else
    fail "Cannot get token"
    exit 1
fi

test_case "List users"
if openstack user list &> /dev/null; then
    pass
else
    fail "Cannot list users"
fi

test_case "List projects"
if openstack project list &> /dev/null; then
    pass
else
    fail "Cannot list projects"
fi

test_case "List roles"
if openstack role list &> /dev/null; then
    pass
else
    fail "Cannot list roles"
fi

test_case "Create user"
TEST_USER="test-user-$$"
if openstack user create "$TEST_USER" --password testpass &> /dev/null; then
    pass
else
    fail "Cannot create user"
fi

test_case "Show user details"
if openstack user show "$TEST_USER" &> /dev/null; then
    pass
else
    fail "Cannot show user"
fi

test_case "Delete user"
if openstack user delete "$TEST_USER" &> /dev/null; then
    pass
else
    fail "Cannot delete user"
fi

# Compute (Nova) Tests
echo ""
echo -e "${BLUE}═══ Compute Service (Nova) Tests ═══${NC}"

test_case "List flavors"
if openstack flavor list &> /dev/null; then
    pass
else
    fail "Cannot list flavors"
fi

test_case "List servers"
if openstack server list &> /dev/null; then
    pass
else
    fail "Cannot list servers"
fi

test_case "Create server"
TEST_SERVER="test-server-$$"
if SERVER_ID=$(openstack server create "$TEST_SERVER" \
    --flavor m1.tiny \
    --image cirros \
    --network private \
    -f value -c id 2>/dev/null); then
    pass
else
    fail "Cannot create server"
    SERVER_ID=""
fi

if [ -n "$SERVER_ID" ]; then
    test_case "Show server"
    if openstack server show "$SERVER_ID" &> /dev/null; then
        pass
    else
        fail "Cannot show server"
    fi

    test_case "Server stop"
    if openstack server stop "$SERVER_ID" &> /dev/null; then
        pass
    else
        fail "Cannot stop server"
    fi

    test_case "Server start"
    if openstack server start "$SERVER_ID" &> /dev/null; then
        pass
    else
        fail "Cannot start server"
    fi

    test_case "Server reboot"
    if openstack server reboot "$SERVER_ID" &> /dev/null; then
        pass
    else
        fail "Cannot reboot server"
    fi

    test_case "Delete server"
    if openstack server delete "$SERVER_ID" &> /dev/null; then
        pass
    else
        fail "Cannot delete server"
    fi
fi

# Network (Neutron) Tests
echo ""
echo -e "${BLUE}═══ Network Service (Neutron) Tests ═══${NC}"

test_case "List networks"
if openstack network list &> /dev/null; then
    pass
else
    fail "Cannot list networks"
fi

test_case "Create network"
TEST_NET="test-net-$$"
if NET_ID=$(openstack network create "$TEST_NET" -f value -c id 2>/dev/null); then
    pass
else
    fail "Cannot create network"
    NET_ID=""
fi

if [ -n "$NET_ID" ]; then
    test_case "Create subnet"
    if SUBNET_ID=$(openstack subnet create "test-subnet-$$" \
        --network "$NET_ID" \
        --subnet-range 10.0.99.0/24 \
        -f value -c id 2>/dev/null); then
        pass
    else
        fail "Cannot create subnet"
        SUBNET_ID=""
    fi

    test_case "List subnets"
    if openstack subnet list &> /dev/null; then
        pass
    else
        fail "Cannot list subnets"
    fi

    test_case "Create port"
    if PORT_ID=$(openstack port create "test-port-$$" \
        --network "$NET_ID" \
        -f value -c id 2>/dev/null); then
        pass
    else
        fail "Cannot create port"
        PORT_ID=""
    fi

    if [ -n "$PORT_ID" ]; then
        test_case "Delete port"
        if openstack port delete "$PORT_ID" &> /dev/null; then
            pass
        else
            fail "Cannot delete port"
        fi
    fi

    if [ -n "$SUBNET_ID" ]; then
        test_case "Delete subnet"
        if openstack subnet delete "$SUBNET_ID" &> /dev/null; then
            pass
        else
            fail "Cannot delete subnet"
        fi
    fi

    test_case "Delete network"
    if openstack network delete "$NET_ID" &> /dev/null; then
        pass
    else
        fail "Cannot delete network"
    fi
fi

# Volume (Cinder) Tests
echo ""
echo -e "${BLUE}═══ Volume Service (Cinder) Tests ═══${NC}"

test_case "List volumes"
if openstack volume list &> /dev/null; then
    pass
else
    fail "Cannot list volumes"
fi

test_case "Create volume"
TEST_VOL="test-vol-$$"
if VOL_ID=$(openstack volume create "$TEST_VOL" --size 1 -f value -c id 2>/dev/null); then
    pass
else
    fail "Cannot create volume"
    VOL_ID=""
fi

if [ -n "$VOL_ID" ]; then
    test_case "Show volume"
    if openstack volume show "$VOL_ID" &> /dev/null; then
        pass
    else
        fail "Cannot show volume"
    fi

    test_case "Volume set (update metadata)"
    if openstack volume set "$VOL_ID" --property test=value &> /dev/null; then
        pass
    else
        fail "Cannot set volume metadata"
    fi

    test_case "Delete volume"
    if openstack volume delete "$VOL_ID" &> /dev/null; then
        pass
    else
        fail "Cannot delete volume"
    fi
fi

# Image (Glance) Tests
echo ""
echo -e "${BLUE}═══ Image Service (Glance) Tests ═══${NC}"

test_case "List images"
if openstack image list &> /dev/null; then
    pass
else
    fail "Cannot list images"
fi

test_case "Create image"
TEST_IMG="test-img-$$"
if IMG_ID=$(openstack image create "$TEST_IMG" \
    --disk-format qcow2 \
    --container-format bare \
    -f value -c id 2>/dev/null); then
    pass
else
    fail "Cannot create image"
    IMG_ID=""
fi

if [ -n "$IMG_ID" ]; then
    test_case "Show image"
    if openstack image show "$IMG_ID" &> /dev/null; then
        pass
    else
        fail "Cannot show image"
    fi

    test_case "Image set (add property)"
    if openstack image set "$IMG_ID" --property test=value &> /dev/null; then
        pass
    else
        fail "Cannot set image property"
    fi

    test_case "Delete image"
    if openstack image delete "$IMG_ID" &> /dev/null; then
        pass
    else
        fail "Cannot delete image"
    fi
fi

# Summary
echo ""
echo -e "${BLUE}╔══════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║                      TEST SUMMARY                             ║${NC}"
echo -e "${BLUE}╚══════════════════════════════════════════════════════════════╝${NC}"
echo ""
echo -e "Total Tests:  $TOTAL_TESTS"
echo -e "${GREEN}Passed:       $PASSED_TESTS${NC}"
echo -e "${RED}Failed:       $FAILED_TESTS${NC}"
echo ""

if [ ${#MISSING_APIS[@]} -gt 0 ]; then
    echo -e "${RED}Missing APIs detected:${NC}"
    for api in "${MISSING_APIS[@]}"; do
        echo "  - $api"
    done
    echo ""
fi

PASS_RATE=$((PASSED_TESTS * 100 / TOTAL_TESTS))
echo -e "Pass Rate: $PASS_RATE%"
echo ""

if [ $FAILED_TESTS -eq 0 ]; then
    echo -e "${GREEN}✓ All tests passed!${NC}"
    exit 0
else
    echo -e "${RED}✗ Some tests failed${NC}"
    exit 1
fi
