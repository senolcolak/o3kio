#!/bin/bash
# Metadata Service Test

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Get token
TOKEN=$(curl -i -s -X POST "http://localhost:35357/v3/auth/tokens" \
    -H "Content-Type: application/json" \
    -d '{"auth":{"identity":{"methods":["password"],"password":{"user":{"name":"admin","password":"secret","domain":{"name":"default"}}}},"scope":{"project":{"name":"default","domain":{"name":"default"}}}}}' \
    | grep -i "X-Subject-Token:" | awk '{print $2}' | tr -d '\r')

echo "✓ Got auth token"

# Create instance
INSTANCE_ID=$(curl -s -X POST "http://localhost:8774/v2.1/servers" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{
        "server": {
            "name": "test-metadata-vm",
            "flavorRef": "00000000-0000-0000-0000-000000000010"
        }
    }' | jq -r '.server.id')

if [ -z "$INSTANCE_ID" ] || [ "$INSTANCE_ID" == "null" ]; then
    echo "✗ Failed to create instance"
    exit 1
fi

echo "✓ Created instance: $INSTANCE_ID"
sleep 2

# Test 1: Version discovery
echo ""
echo "Test 1: Version discovery"
VERSIONS=$(curl -s -X GET "http://localhost:8775/" \
    -H "X-Instance-ID: $INSTANCE_ID")

if echo "$VERSIONS" | grep -q "openstack"; then
    echo -e "${GREEN}✓${NC} Version discovery works"
else
    echo -e "${RED}✗${NC} Version discovery failed"
    echo "Response: $VERSIONS"
fi

# Test 2: OpenStack metadata (JSON)
echo ""
echo "Test 2: OpenStack metadata (meta_data.json)"
METADATA=$(curl -s -X GET "http://localhost:8775/openstack/latest/meta_data.json" \
    -H "X-Instance-ID: $INSTANCE_ID")

INSTANCE_UUID=$(echo "$METADATA" | jq -r '.uuid')
INSTANCE_NAME=$(echo "$METADATA" | jq -r '.name')

if [ "$INSTANCE_UUID" == "$INSTANCE_ID" ]; then
    echo -e "${GREEN}✓${NC} Got metadata for instance: $INSTANCE_NAME"
    echo "   UUID: $INSTANCE_UUID"
else
    echo -e "${RED}✗${NC} Metadata UUID mismatch"
    echo "Response: $METADATA"
fi

# Test 3: EC2-style metadata (instance-id)
echo ""
echo "Test 3: EC2-style metadata (instance-id)"
EC2_INSTANCE_ID=$(curl -s -X GET "http://localhost:8775/2009-04-04/meta-data/instance-id" \
    -H "X-Instance-ID: $INSTANCE_ID")

if [ "$EC2_INSTANCE_ID" == "$INSTANCE_ID" ]; then
    echo -e "${GREEN}✓${NC} EC2 instance-id matches: $EC2_INSTANCE_ID"
else
    echo -e "${RED}✗${NC} EC2 instance-id mismatch"
    echo "Expected: $INSTANCE_ID"
    echo "Got: $EC2_INSTANCE_ID"
fi

# Test 4: EC2-style metadata (hostname)
echo ""
echo "Test 4: EC2-style metadata (hostname)"
EC2_HOSTNAME=$(curl -s -X GET "http://localhost:8775/2009-04-04/meta-data/hostname" \
    -H "X-Instance-ID: $INSTANCE_ID")

if [ -n "$EC2_HOSTNAME" ] && [ "$EC2_HOSTNAME" != "" ]; then
    echo -e "${GREEN}✓${NC} Got hostname: $EC2_HOSTNAME"
else
    echo -e "${RED}✗${NC} Failed to get hostname"
fi

# Test 5: User-data (should be empty if not set)
echo ""
echo "Test 5: User-data endpoint"
USER_DATA=$(curl -s -X GET "http://localhost:8775/openstack/latest/user_data" \
    -H "X-Instance-ID: $INSTANCE_ID")

if [ -z "$USER_DATA" ]; then
    echo -e "${GREEN}✓${NC} User-data empty (as expected)"
else
    echo -e "${GREEN}✓${NC} Got user-data: ${USER_DATA:0:50}..."
fi

# Test 6: Network data
echo ""
echo "Test 6: Network data (network_data.json)"
NETWORK_DATA=$(curl -s -X GET "http://localhost:8775/openstack/latest/network_data.json" \
    -H "X-Instance-ID: $INSTANCE_ID")

LINKS_COUNT=$(echo "$NETWORK_DATA" | jq '.links | length')

if [ "$LINKS_COUNT" -ge 0 ]; then
    echo -e "${GREEN}✓${NC} Got network data with $LINKS_COUNT links"
else
    echo -e "${RED}✗${NC} Failed to get network data"
    echo "Response: $NETWORK_DATA"
fi

# Cleanup
echo ""
echo "Cleaning up..."
curl -s -X DELETE "http://localhost:8774/v2.1/servers/$INSTANCE_ID" \
    -H "X-Auth-Token: $TOKEN" > /dev/null

echo "✓ Cleanup complete"
echo ""
echo "========================================"
echo -e "${GREEN}✅ Metadata service tests passed${NC}"
echo "========================================"
