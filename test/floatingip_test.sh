#!/bin/bash
# Floating IP Test Suite

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

TESTS_PASSED=0
TESTS_FAILED=0
TOTAL_TESTS=0

log_test() {
    ((TOTAL_TESTS++))
    echo -e "${YELLOW}[TEST $TOTAL_TESTS]${NC} $1"
}

log_pass() {
    echo -e "${GREEN}✓${NC} $1"
    ((TESTS_PASSED++))
}

log_fail() {
    echo -e "${RED}✗${NC} $1"
    ((TESTS_FAILED++))
}

# Get token
TOKEN=$(curl -i -s -X POST "http://localhost:35357/v3/auth/tokens" \
    -H "Content-Type: application/json" \
    -d '{"auth":{"identity":{"methods":["password"],"password":{"user":{"name":"admin","password":"secret","domain":{"name":"default"}}}},"scope":{"project":{"name":"default","domain":{"name":"default"}}}}}' \
    | grep -i "X-Subject-Token:" | awk '{print $2}' | tr -d '\r')

log_pass "Got authentication token"

# Test 1: Create external network
log_test "Create external network"
EXT_NET_RESPONSE=$(curl -s -X POST "http://localhost:9696/v2.0/networks" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{
        "network": {
            "name": "external-network",
            "admin_state_up": true,
            "shared": true
        }
    }')

EXT_NET_ID=$(echo "$EXT_NET_RESPONSE" | jq -r '.network.id')

if [ -n "$EXT_NET_ID" ] && [ "$EXT_NET_ID" != "null" ]; then
    log_pass "Created external network: $EXT_NET_ID"
else
    log_fail "Failed to create external network"
    echo "Response: $EXT_NET_RESPONSE"
fi

# Test 2: Create external subnet
log_test "Create external subnet (10.0.0.0/24)"
EXT_SUBNET_RESPONSE=$(curl -s -X POST "http://localhost:9696/v2.0/subnets" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d "{
        \"subnet\": {
            \"name\": \"external-subnet\",
            \"network_id\": \"$EXT_NET_ID\",
            \"cidr\": \"10.0.0.0/24\",
            \"ip_version\": 4,
            \"enable_dhcp\": false
        }
    }")

EXT_SUBNET_ID=$(echo "$EXT_SUBNET_RESPONSE" | jq -r '.subnet.id')

if [ -n "$EXT_SUBNET_ID" ] && [ "$EXT_SUBNET_ID" != "null" ]; then
    log_pass "Created external subnet: $EXT_SUBNET_ID"
else
    log_fail "Failed to create external subnet"
    echo "Response: $EXT_SUBNET_RESPONSE"
fi

# Test 3: Create private network
log_test "Create private network"
PRIV_NET_RESPONSE=$(curl -s -X POST "http://localhost:9696/v2.0/networks" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{
        "network": {
            "name": "private-network",
            "admin_state_up": true
        }
    }')

PRIV_NET_ID=$(echo "$PRIV_NET_RESPONSE" | jq -r '.network.id')

if [ -n "$PRIV_NET_ID" ] && [ "$PRIV_NET_ID" != "null" ]; then
    log_pass "Created private network: $PRIV_NET_ID"
else
    log_fail "Failed to create private network"
fi

# Test 4: Create private subnet
log_test "Create private subnet (192.168.1.0/24)"
PRIV_SUBNET_RESPONSE=$(curl -s -X POST "http://localhost:9696/v2.0/subnets" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d "{
        \"subnet\": {
            \"name\": \"private-subnet\",
            \"network_id\": \"$PRIV_NET_ID\",
            \"cidr\": \"192.168.1.0/24\",
            \"ip_version\": 4,
            \"gateway_ip\": \"192.168.1.1\"
        }
    }")

PRIV_SUBNET_ID=$(echo "$PRIV_SUBNET_RESPONSE" | jq -r '.subnet.id')

if [ -n "$PRIV_SUBNET_ID" ] && [ "$PRIV_SUBNET_ID" != "null" ]; then
    log_pass "Created private subnet: $PRIV_SUBNET_ID"
else
    log_fail "Failed to create private subnet"
fi

# Test 5: Create router
log_test "Create router with external gateway"
ROUTER_RESPONSE=$(curl -s -X POST "http://localhost:9696/v2.0/routers" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d "{
        \"router\": {
            \"name\": \"test-router\",
            \"admin_state_up\": true,
            \"external_gateway_info\": {
                \"network_id\": \"$EXT_NET_ID\",
                \"enable_snat\": true
            }
        }
    }")

ROUTER_ID=$(echo "$ROUTER_RESPONSE" | jq -r '.router.id')

if [ -n "$ROUTER_ID" ] && [ "$ROUTER_ID" != "null" ]; then
    log_pass "Created router: $ROUTER_ID"
else
    log_fail "Failed to create router"
    echo "Response: $ROUTER_RESPONSE"
fi

# Test 6: Attach router to private network
log_test "Attach router to private network"
ROUTER_IFACE_RESPONSE=$(curl -s -X PUT "http://localhost:9696/v2.0/routers/$ROUTER_ID/add_router_interface" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d "{
        \"subnet_id\": \"$PRIV_SUBNET_ID\"
    }")

ROUTER_PORT_ID=$(echo "$ROUTER_IFACE_RESPONSE" | jq -r '.port_id')

if [ -n "$ROUTER_PORT_ID" ] && [ "$ROUTER_PORT_ID" != "null" ]; then
    log_pass "Attached router to private network"
else
    log_fail "Failed to attach router"
    echo "Response: $ROUTER_IFACE_RESPONSE"
fi

# Test 7: Create port on private network
log_test "Create port on private network"
PORT_RESPONSE=$(curl -s -X POST "http://localhost:9696/v2.0/ports" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d "{
        \"port\": {
            \"name\": \"test-port\",
            \"network_id\": \"$PRIV_NET_ID\",
            \"fixed_ips\": [{
                \"subnet_id\": \"$PRIV_SUBNET_ID\",
                \"ip_address\": \"192.168.1.10\"
            }]
        }
    }")

PORT_ID=$(echo "$PORT_RESPONSE" | jq -r '.port.id')
FIXED_IP=$(echo "$PORT_RESPONSE" | jq -r '.port.fixed_ips[0].ip_address')

if [ -n "$PORT_ID" ] && [ "$PORT_ID" != "null" ]; then
    log_pass "Created port: $PORT_ID (IP: $FIXED_IP)"
else
    log_fail "Failed to create port"
    echo "Response: $PORT_RESPONSE"
fi

# Test 8: Allocate floating IP (unassociated)
log_test "Allocate floating IP from external network"
FIP_RESPONSE=$(curl -s -X POST "http://localhost:9696/v2.0/floatingips" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d "{
        \"floatingip\": {
            \"floating_network_id\": \"$EXT_NET_ID\"
        }
    }")

FIP_ID=$(echo "$FIP_RESPONSE" | jq -r '.floatingip.id')
FIP_ADDRESS=$(echo "$FIP_RESPONSE" | jq -r '.floatingip.floating_ip_address')
FIP_STATUS=$(echo "$FIP_RESPONSE" | jq -r '.floatingip.status')

if [ -n "$FIP_ID" ] && [ "$FIP_ID" != "null" ]; then
    log_pass "Allocated floating IP: $FIP_ADDRESS (status: $FIP_STATUS)"
else
    log_fail "Failed to allocate floating IP"
    echo "Response: $FIP_RESPONSE"
fi

# Test 9: Verify floating IP is DOWN when unassociated
log_test "Verify floating IP status is DOWN"
if [ "$FIP_STATUS" == "DOWN" ]; then
    log_pass "Floating IP status is DOWN (unassociated)"
else
    log_fail "Expected status DOWN, got: $FIP_STATUS"
fi

# Test 10: List floating IPs
log_test "List floating IPs"
FIP_LIST=$(curl -s -X GET "http://localhost:9696/v2.0/floatingips" \
    -H "X-Auth-Token: $TOKEN")

FIP_COUNT=$(echo "$FIP_LIST" | jq '.floatingips | length')

if [ "$FIP_COUNT" -ge 1 ]; then
    log_pass "Found $FIP_COUNT floating IP(s)"
else
    log_fail "Expected at least 1 floating IP, found $FIP_COUNT"
fi

# Test 11: Get floating IP details
log_test "Get floating IP details"
FIP_DETAILS=$(curl -s -X GET "http://localhost:9696/v2.0/floatingips/$FIP_ID" \
    -H "X-Auth-Token: $TOKEN")

FIP_DETAIL_ID=$(echo "$FIP_DETAILS" | jq -r '.floatingip.id')

if [ "$FIP_DETAIL_ID" == "$FIP_ID" ]; then
    log_pass "Retrieved floating IP details"
else
    log_fail "Failed to get floating IP details"
fi

# Test 12: Associate floating IP with port
log_test "Associate floating IP with port"
ASSOC_RESPONSE=$(curl -s -X PUT "http://localhost:9696/v2.0/floatingips/$FIP_ID" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d "{
        \"floatingip\": {
            \"port_id\": \"$PORT_ID\",
            \"fixed_ip_address\": \"$FIXED_IP\"
        }
    }")

ASSOC_PORT_ID=$(echo "$ASSOC_RESPONSE" | jq -r '.floatingip.port_id')
ASSOC_STATUS=$(echo "$ASSOC_RESPONSE" | jq -r '.floatingip.status')

if [ "$ASSOC_PORT_ID" == "$PORT_ID" ]; then
    log_pass "Associated floating IP with port (status: $ASSOC_STATUS)"
else
    log_fail "Failed to associate floating IP"
    echo "Response: $ASSOC_RESPONSE"
fi

# Test 13: Verify floating IP is ACTIVE after association
log_test "Verify floating IP status is ACTIVE"
if [ "$ASSOC_STATUS" == "ACTIVE" ]; then
    log_pass "Floating IP status is ACTIVE"
else
    log_fail "Expected status ACTIVE, got: $ASSOC_STATUS"
fi

# Test 14: Verify router ID is set after association
log_test "Verify router ID is set"
ASSOC_ROUTER_ID=$(echo "$ASSOC_RESPONSE" | jq -r '.floatingip.router_id')

if [ -n "$ASSOC_ROUTER_ID" ] && [ "$ASSOC_ROUTER_ID" != "null" ]; then
    log_pass "Router ID is set: $ASSOC_ROUTER_ID"
else
    log_fail "Router ID not set after association"
fi

# Test 15: Disassociate floating IP
log_test "Disassociate floating IP from port"
DISASSOC_RESPONSE=$(curl -s -X PUT "http://localhost:9696/v2.0/floatingips/$FIP_ID" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{
        "floatingip": {
            "port_id": null
        }
    }')

DISASSOC_PORT_ID=$(echo "$DISASSOC_RESPONSE" | jq -r '.floatingip.port_id')
DISASSOC_STATUS=$(echo "$DISASSOC_RESPONSE" | jq -r '.floatingip.status')

if [ "$DISASSOC_PORT_ID" == "null" ]; then
    log_pass "Disassociated floating IP (status: $DISASSOC_STATUS)"
else
    log_fail "Failed to disassociate floating IP"
fi

# Test 16: Verify status is DOWN after disassociation
log_test "Verify floating IP status is DOWN after disassociation"
if [ "$DISASSOC_STATUS" == "DOWN" ]; then
    log_pass "Floating IP status is DOWN"
else
    log_fail "Expected status DOWN, got: $DISASSOC_STATUS"
fi

# Test 17: Delete floating IP
log_test "Delete floating IP"
HTTP_CODE=$(curl -s -w "%{http_code}" -o /dev/null -X DELETE "http://localhost:9696/v2.0/floatingips/$FIP_ID" \
    -H "X-Auth-Token: $TOKEN")

if [ "$HTTP_CODE" == "204" ]; then
    log_pass "Deleted floating IP (HTTP 204)"
else
    log_fail "Failed to delete floating IP (HTTP $HTTP_CODE)"
fi

# Test 18: Verify floating IP was deleted
log_test "Verify floating IP was deleted"
sleep 1
DELETED_FIP=$(curl -s -X GET "http://localhost:9696/v2.0/floatingips/$FIP_ID" \
    -H "X-Auth-Token: $TOKEN" \
    -w "\n%{http_code}")

HTTP_CODE=$(echo "$DELETED_FIP" | tail -1)

if [ "$HTTP_CODE" == "404" ]; then
    log_pass "Floating IP no longer exists (HTTP 404)"
else
    log_fail "Floating IP still exists (HTTP $HTTP_CODE)"
fi

# Cleanup
echo ""
echo "Cleaning up test resources..."
curl -s -X DELETE "http://localhost:9696/v2.0/ports/$PORT_ID" -H "X-Auth-Token: $TOKEN" > /dev/null
curl -s -X PUT "http://localhost:9696/v2.0/routers/$ROUTER_ID/remove_router_interface" \
    -H "X-Auth-Token: $TOKEN" -H "Content-Type: application/json" \
    -d "{\"subnet_id\": \"$PRIV_SUBNET_ID\"}" > /dev/null
curl -s -X DELETE "http://localhost:9696/v2.0/routers/$ROUTER_ID" -H "X-Auth-Token: $TOKEN" > /dev/null
curl -s -X DELETE "http://localhost:9696/v2.0/subnets/$PRIV_SUBNET_ID" -H "X-Auth-Token: $TOKEN" > /dev/null
curl -s -X DELETE "http://localhost:9696/v2.0/networks/$PRIV_NET_ID" -H "X-Auth-Token: $TOKEN" > /dev/null
curl -s -X DELETE "http://localhost:9696/v2.0/subnets/$EXT_SUBNET_ID" -H "X-Auth-Token: $TOKEN" > /dev/null
curl -s -X DELETE "http://localhost:9696/v2.0/networks/$EXT_NET_ID" -H "X-Auth-Token: $TOKEN" > /dev/null

# Print summary
echo ""
echo "========================================"
echo "     FLOATING IP TEST SUMMARY"
echo "========================================"
echo -e "Total Tests:  $TOTAL_TESTS"
echo -e "${GREEN}Passed:       $TESTS_PASSED${NC}"
echo -e "${RED}Failed:       $TESTS_FAILED${NC}"
echo "========================================"
echo ""

if [ $TESTS_FAILED -eq 0 ]; then
    echo -e "${GREEN}✅ ALL FLOATING IP TESTS PASSED${NC}"
    exit 0
else
    echo -e "${RED}❌ SOME TESTS FAILED${NC}"
    exit 1
fi
