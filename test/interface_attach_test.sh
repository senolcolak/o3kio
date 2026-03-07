#!/bin/bash
# Network Interface Hot-Plug Test

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'

TESTS_PASSED=0
TESTS_FAILED=0

# Get token
TOKEN=$(curl -i -s -X POST "http://localhost:35357/v3/auth/tokens" \
    -H "Content-Type: application/json" \
    -d '{"auth":{"identity":{"methods":["password"],"password":{"user":{"name":"admin","password":"secret","domain":{"name":"default"}}}},"scope":{"project":{"name":"default","domain":{"name":"default"}}}}}' \
    | grep -i "X-Subject-Token:" | awk '{print $2}' | tr -d '\r')

echo "✓ Got auth token"

# Get project ID
PROJECT_ID=$(curl -s -X POST "http://localhost:35357/v3/auth/tokens" \
    -H "Content-Type: application/json" \
    -d '{"auth":{"identity":{"methods":["password"],"password":{"user":{"name":"admin","password":"secret","domain":{"name":"default"}}}},"scope":{"project":{"name":"default","domain":{"name":"default"}}}}}' \
    | jq -r '.token.project.id')

# Create network
NETWORK_ID=$(curl -s -X POST "http://localhost:9696/v2.0/networks" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{
        "network": {
            "name": "test-interface-network"
        }
    }' | jq -r '.network.id')

echo "✓ Created network: $NETWORK_ID"

# Create subnet
SUBNET_ID=$(curl -s -X POST "http://localhost:9696/v2.0/subnets" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d "{
        \"subnet\": {
            \"name\": \"test-interface-subnet\",
            \"network_id\": \"$NETWORK_ID\",
            \"cidr\": \"192.168.100.0/24\",
            \"ip_version\": 4,
            \"enable_dhcp\": false
        }
    }" | jq -r '.subnet.id')

echo "✓ Created subnet: $SUBNET_ID"

# Create instance
INSTANCE_ID=$(curl -s -X POST "http://localhost:8774/v2.1/servers" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{
        "server": {
            "name": "test-interface-vm",
            "flavorRef": "00000000-0000-0000-0000-000000000010"
        }
    }' | jq -r '.server.id')

if [ -z "$INSTANCE_ID" ] || [ "$INSTANCE_ID" == "null" ]; then
    echo "✗ Failed to create instance"
    exit 1
fi

echo "✓ Created instance: $INSTANCE_ID"
sleep 2

# Test 1: List interfaces (should be empty initially)
echo ""
echo "Test 1: List interface attachments (initially empty)"
ATTACHMENTS=$(curl -s -X GET "http://localhost:8774/v2.1/servers/$INSTANCE_ID/os-interface" \
    -H "X-Auth-Token: $TOKEN" | jq '.interfaceAttachments | length')

if [ "$ATTACHMENTS" -eq 0 ]; then
    echo -e "${GREEN}✓${NC} No interfaces attached (as expected)"
    ((TESTS_PASSED++))
else
    echo -e "${RED}✗${NC} Expected 0 interfaces, found $ATTACHMENTS"
    ((TESTS_FAILED++))
fi

# Test 2: Attach interface (auto-create port)
echo ""
echo "Test 2: Attach interface (auto-create port)"
ATTACH_RESPONSE=$(curl -s -X POST "http://localhost:8774/v2.1/servers/$INSTANCE_ID/os-interface" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d "{
        \"interfaceAttachment\": {
            \"net_id\": \"$NETWORK_ID\"
        }
    }")

PORT_ID=$(echo "$ATTACH_RESPONSE" | jq -r '.interfaceAttachment.port_id')
FIXED_IP=$(echo "$ATTACH_RESPONSE" | jq -r '.interfaceAttachment.fixed_ips[0].ip_address')
MAC_ADDR=$(echo "$ATTACH_RESPONSE" | jq -r '.interfaceAttachment.mac_addr')

if [ -n "$PORT_ID" ] && [ "$PORT_ID" != "null" ]; then
    echo -e "${GREEN}✓${NC} Interface attached successfully"
    echo "   Port ID: $PORT_ID"
    echo "   Fixed IP: $FIXED_IP"
    echo "   MAC Address: $MAC_ADDR"
    ((TESTS_PASSED++))
else
    echo -e "${RED}✗${NC} Failed to attach interface"
    echo "Response: $ATTACH_RESPONSE"
    ((TESTS_FAILED++))
fi

# Test 3: List interfaces (should show 1)
echo ""
echo "Test 3: List interface attachments (should show 1)"
sleep 1
ATTACHMENTS=$(curl -s -X GET "http://localhost:8774/v2.1/servers/$INSTANCE_ID/os-interface" \
    -H "X-Auth-Token: $TOKEN")

ATTACHMENTS_COUNT=$(echo "$ATTACHMENTS" | jq '.interfaceAttachments | length')

if [ "$ATTACHMENTS_COUNT" -eq 1 ]; then
    echo -e "${GREEN}✓${NC} Found 1 interface attachment"
    ATTACHED_PORT=$(echo "$ATTACHMENTS" | jq -r '.interfaceAttachments[0].port_id')
    echo "   Attached port: $ATTACHED_PORT"
    ((TESTS_PASSED++))
else
    echo -e "${RED}✗${NC} Expected 1 interface, found $ATTACHMENTS_COUNT"
    ((TESTS_FAILED++))
fi

# Test 4: Attach second interface with specific IP
echo ""
echo "Test 4: Attach second interface with specific IP"
ATTACH2_RESPONSE=$(curl -s -X POST "http://localhost:8774/v2.1/servers/$INSTANCE_ID/os-interface" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d "{
        \"interfaceAttachment\": {
            \"net_id\": \"$NETWORK_ID\",
            \"fixed_ip\": \"192.168.100.50\"
        }
    }")

PORT2_ID=$(echo "$ATTACH2_RESPONSE" | jq -r '.interfaceAttachment.port_id')
FIXED_IP2=$(echo "$ATTACH2_RESPONSE" | jq -r '.interfaceAttachment.fixed_ips[0].ip_address')

if [ "$FIXED_IP2" == "192.168.100.50" ]; then
    echo -e "${GREEN}✓${NC} Second interface attached with requested IP: $FIXED_IP2"
    echo "   Port ID: $PORT2_ID"
    ((TESTS_PASSED++))
else
    echo -e "${RED}✗${NC} Failed to attach with specific IP"
    echo "Expected: 192.168.100.50, Got: $FIXED_IP2"
    ((TESTS_FAILED++))
fi

# Test 5: List interfaces (should show 2)
echo ""
echo "Test 5: List interface attachments (should show 2)"
sleep 1
ATTACHMENTS_COUNT=$(curl -s -X GET "http://localhost:8774/v2.1/servers/$INSTANCE_ID/os-interface" \
    -H "X-Auth-Token: $TOKEN" | jq '.interfaceAttachments | length')

if [ "$ATTACHMENTS_COUNT" -eq 2 ]; then
    echo -e "${GREEN}✓${NC} Found 2 interface attachments"
    ((TESTS_PASSED++))
else
    echo -e "${RED}✗${NC} Expected 2 interfaces, found $ATTACHMENTS_COUNT"
    ((TESTS_FAILED++))
fi

# Test 6: Detach first interface
echo ""
echo "Test 6: Detach first interface"
HTTP_CODE=$(curl -s -w "%{http_code}" -o /dev/null -X DELETE "http://localhost:8774/v2.1/servers/$INSTANCE_ID/os-interface/$PORT_ID" \
    -H "X-Auth-Token: $TOKEN")

if [ "$HTTP_CODE" == "202" ]; then
    echo -e "${GREEN}✓${NC} Interface detached successfully (HTTP 202)"
    ((TESTS_PASSED++))
else
    echo -e "${RED}✗${NC} Failed to detach interface (HTTP $HTTP_CODE)"
    ((TESTS_FAILED++))
fi

# Test 7: List interfaces (should show 1 remaining)
echo ""
echo "Test 7: List interface attachments (should show 1 remaining)"
sleep 1
ATTACHMENTS=$(curl -s -X GET "http://localhost:8774/v2.1/servers/$INSTANCE_ID/os-interface" \
    -H "X-Auth-Token: $TOKEN")

ATTACHMENTS_COUNT=$(echo "$ATTACHMENTS" | jq '.interfaceAttachments | length')

if [ "$ATTACHMENTS_COUNT" -eq 1 ]; then
    echo -e "${GREEN}✓${NC} Found 1 remaining interface attachment"
    REMAINING_PORT=$(echo "$ATTACHMENTS" | jq -r '.interfaceAttachments[0].port_id')
    if [ "$REMAINING_PORT" == "$PORT2_ID" ]; then
        echo "   Correctly shows second interface still attached"
    else
        echo -e "${RED}✗${NC} Wrong interface remaining"
    fi
    ((TESTS_PASSED++))
else
    echo -e "${RED}✗${NC} Expected 1 interface, found $ATTACHMENTS_COUNT"
    ((TESTS_FAILED++))
fi

# Cleanup
echo ""
echo "Cleaning up..."
curl -s -X DELETE "http://localhost:8774/v2.1/servers/$INSTANCE_ID" \
    -H "X-Auth-Token: $TOKEN" > /dev/null

curl -s -X DELETE "http://localhost:9696/v2.0/networks/$NETWORK_ID" \
    -H "X-Auth-Token: $TOKEN" > /dev/null

echo "✓ Cleanup complete"
echo ""
echo "========================================"
echo -e "Tests passed: ${GREEN}$TESTS_PASSED${NC}"
echo -e "Tests failed: ${RED}$TESTS_FAILED${NC}"
if [ $TESTS_FAILED -eq 0 ]; then
    echo -e "${GREEN}✅ All interface attachment tests passed${NC}"
else
    echo -e "${RED}❌ Some tests failed${NC}"
fi
echo "========================================"
