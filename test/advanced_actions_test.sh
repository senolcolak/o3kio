#!/bin/bash
# Advanced VM Actions Test (Simplified)

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

# Create instance
INSTANCE_ID=$(curl -s -X POST "http://localhost:8774/v2.1/servers" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{
        "server": {
            "name": "test-advanced-actions-vm",
            "flavorRef": "00000000-0000-0000-0000-000000000010"
        }
    }' | jq -r '.server.id')

if [ -z "$INSTANCE_ID" ] || [ "$INSTANCE_ID" == "null" ]; then
    echo "✗ Failed to create instance"
    exit 1
fi

echo "✓ Created instance: $INSTANCE_ID"
sleep 2

# Test 1: Suspend instance
echo ""
echo "Test 1: Suspend instance"
HTTP_CODE=$(curl -s -w "%{http_code}" -o /dev/null -X POST "http://localhost:8774/v2.1/servers/$INSTANCE_ID/action" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"suspend": null}')

if [ "$HTTP_CODE" == "202" ]; then
    echo -e "${GREEN}✓${NC} Suspend accepted (HTTP 202)"
    ((TESTS_PASSED++))
else
    echo -e "${RED}✗${NC} Suspend failed (HTTP $HTTP_CODE)"
    ((TESTS_FAILED++))
fi

sleep 1

# Test 2: Resume instance
echo ""
echo "Test 2: Resume instance"
HTTP_CODE=$(curl -s -w "%{http_code}" -o /dev/null -X POST "http://localhost:8774/v2.1/servers/$INSTANCE_ID/action" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"resume": null}')

if [ "$HTTP_CODE" == "202" ]; then
    echo -e "${GREEN}✓${NC} Resume accepted (HTTP 202)"
    ((TESTS_PASSED++))
else
    echo -e "${RED}✗${NC} Resume failed (HTTP $HTTP_CODE)"
    ((TESTS_FAILED++))
fi

sleep 1

# Test 3: Shelve instance
echo ""
echo "Test 3: Shelve instance"
HTTP_CODE=$(curl -s -w "%{http_code}" -o /dev/null -X POST "http://localhost:8774/v2.1/servers/$INSTANCE_ID/action" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"shelve": null}')

if [ "$HTTP_CODE" == "202" ]; then
    echo -e "${GREEN}✓${NC} Shelve accepted (HTTP 202)"
    ((TESTS_PASSED++))
else
    echo -e "${RED}✗${NC} Shelve failed (HTTP $HTTP_CODE)"
    ((TESTS_FAILED++))
fi

sleep 1

# Test 4: Unshelve instance
echo ""
echo "Test 4: Unshelve instance"
HTTP_CODE=$(curl -s -w "%{http_code}" -o /dev/null -X POST "http://localhost:8774/v2.1/servers/$INSTANCE_ID/action" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"unshelve": null}')

if [ "$HTTP_CODE" == "202" ]; then
    echo -e "${GREEN}✓${NC} Unshelve accepted (HTTP 202)"
    ((TESTS_PASSED++))
else
    echo -e "${RED}✗${NC} Unshelve failed (HTTP $HTTP_CODE)"
    ((TESTS_FAILED++))
fi

sleep 1

# Test 5: Resize instance
echo ""
echo "Test 5: Resize instance"

# Get list of flavors
FLAVORS=$(curl -s -X GET "http://localhost:8774/v2.1/flavors" \
    -H "X-Auth-Token: $TOKEN" | jq -r '.flavors[].id' | head -2)

# Pick two different flavors
FLAVOR_ARRAY=($FLAVORS)
if [ ${#FLAVOR_ARRAY[@]} -ge 2 ]; then
    NEW_FLAVOR="${FLAVOR_ARRAY[1]}"

    HTTP_CODE=$(curl -s -w "%{http_code}" -o /dev/null -X POST "http://localhost:8774/v2.1/servers/$INSTANCE_ID/action" \
        -H "X-Auth-Token: $TOKEN" \
        -H "Content-Type: application/json" \
        -d "{\"resize\": {\"flavorRef\": \"$NEW_FLAVOR\"}}")

    if [ "$HTTP_CODE" == "202" ]; then
        echo -e "${GREEN}✓${NC} Resize accepted (HTTP 202)"
        ((TESTS_PASSED++))
    else
        echo -e "${RED}✗${NC} Resize failed (HTTP $HTTP_CODE)"
        ((TESTS_FAILED++))
    fi
else
    echo "Skipping resize test (need at least 2 flavors)"
fi

# Cleanup
echo ""
echo "Cleaning up..."
curl -s -X DELETE "http://localhost:8774/v2.1/servers/$INSTANCE_ID" \
    -H "X-Auth-Token: $TOKEN" > /dev/null

echo "✓ Cleanup complete"
echo ""
echo "========================================"
echo -e "Tests passed: ${GREEN}$TESTS_PASSED${NC}"
echo -e "Tests failed: ${RED}$TESTS_FAILED${NC}"
if [ $TESTS_FAILED -eq 0 ]; then
    echo -e "${GREEN}✅ All advanced actions tests passed${NC}"
else
    echo -e "${RED}❌ Some tests failed${NC}"
fi
echo "========================================"
