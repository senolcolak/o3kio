#!/bin/bash
# Quota Management Test Suite

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

PROJECT_ID="00000000-0000-0000-0000-000000000002"

# Test 1: Get quota set
log_test "Get quota set for project"
QUOTA_RESPONSE=$(curl -s -X GET "http://localhost:8774/v2.1/os-quota-sets/$PROJECT_ID" \
    -H "X-Auth-Token: $TOKEN")

INSTANCES_LIMIT=$(echo "$QUOTA_RESPONSE" | jq -r '.quota_set.instances')
CORES_LIMIT=$(echo "$QUOTA_RESPONSE" | jq -r '.quota_set.cores')
RAM_LIMIT=$(echo "$QUOTA_RESPONSE" | jq -r '.quota_set.ram')

if [ "$INSTANCES_LIMIT" != "null" ] && [ -n "$INSTANCES_LIMIT" ]; then
    log_pass "Retrieved quota set (instances: $INSTANCES_LIMIT, cores: $CORES_LIMIT, ram: $RAM_LIMIT)"
else
    log_fail "Failed to retrieve quota set"
    echo "Response: $QUOTA_RESPONSE"
fi

# Test 2: Verify usage information is included
log_test "Verify usage information in quota set"
INSTANCES_USED=$(echo "$QUOTA_RESPONSE" | jq -r '.quota_set.instances_used')
CORES_USED=$(echo "$QUOTA_RESPONSE" | jq -r '.quota_set.cores_used')

if [ "$INSTANCES_USED" != "null" ]; then
    log_pass "Usage information included (instances_used: $INSTANCES_USED, cores_used: $CORES_USED)"
else
    log_fail "Usage information missing"
fi

# Test 3: Get default quotas
log_test "Get default quota set"
DEFAULTS_RESPONSE=$(curl -s -X GET "http://localhost:8774/v2.1/os-quota-sets/$PROJECT_ID/defaults" \
    -H "X-Auth-Token: $TOKEN")

DEFAULT_INSTANCES=$(echo "$DEFAULTS_RESPONSE" | jq -r '.quota_set.instances')

if [ "$DEFAULT_INSTANCES" == "10" ]; then
    log_pass "Retrieved default quotas (instances: $DEFAULT_INSTANCES)"
else
    log_fail "Failed to retrieve default quotas"
fi

# Test 4: Update quota limits
log_test "Update quota limits (set instances to 3)"
UPDATE_RESPONSE=$(curl -s -X PUT "http://localhost:8774/v2.1/os-quota-sets/$PROJECT_ID" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{
        "quota_set": {
            "instances": 3,
            "cores": 6
        }
    }')

NEW_INSTANCES_LIMIT=$(echo "$UPDATE_RESPONSE" | jq -r '.quota_set.instances')

if [ "$NEW_INSTANCES_LIMIT" == "3" ]; then
    log_pass "Updated quota limits (instances: $NEW_INSTANCES_LIMIT)"
else
    log_fail "Failed to update quota limits"
    echo "Response: $UPDATE_RESPONSE"
fi

# Test 5: Create instances up to quota limit
log_test "Create instances up to quota limit (3)"
CREATED_INSTANCES=()

for i in {1..3}; do
    INSTANCE_RESPONSE=$(curl -s -X POST "http://localhost:8774/v2.1/servers" \
        -H "X-Auth-Token: $TOKEN" \
        -H "Content-Type: application/json" \
        -d "{
            \"server\": {
                \"name\": \"quota-test-vm-$i\",
                \"flavorRef\": \"00000000-0000-0000-0000-000000000010\",
                \"imageRef\": \"00000000-0000-0000-0000-000000000001\"
            }
        }")

    INSTANCE_ID=$(echo "$INSTANCE_RESPONSE" | jq -r '.server.id')

    if [ -n "$INSTANCE_ID" ] && [ "$INSTANCE_ID" != "null" ]; then
        CREATED_INSTANCES+=("$INSTANCE_ID")
    fi
done

if [ ${#CREATED_INSTANCES[@]} -eq 3 ]; then
    log_pass "Created 3 instances successfully"
else
    log_fail "Failed to create 3 instances (created ${#CREATED_INSTANCES[@]})"
fi

# Test 6: Try to exceed quota (should fail)
log_test "Try to create instance exceeding quota (should fail with HTTP 413)"
EXCEEDED_RESPONSE=$(curl -s -w "\nHTTP_CODE:%{http_code}" -X POST "http://localhost:8774/v2.1/servers" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{
        "server": {
            "name": "quota-test-vm-exceed",
            "flavorRef": "00000000-0000-0000-0000-000000000010",
            "imageRef": "00000000-0000-0000-0000-000000000001"
        }
    }')

HTTP_CODE=$(echo "$EXCEEDED_RESPONSE" | grep "HTTP_CODE:" | sed 's/HTTP_CODE://')
ERROR_MSG=$(echo "$EXCEEDED_RESPONSE" | grep -v "HTTP_CODE:" | jq -r '.error.message' 2>/dev/null)

if [ "$HTTP_CODE" == "413" ]; then
    log_pass "Correctly rejected with HTTP 413 (Quota exceeded)"
else
    log_fail "Expected HTTP 413, got HTTP $HTTP_CODE"
    echo "Error message: $ERROR_MSG"
fi

# Test 7: Verify usage equals limit
log_test "Verify usage equals limit after creating 3 instances"
QUOTA_RESPONSE=$(curl -s -X GET "http://localhost:8774/v2.1/os-quota-sets/$PROJECT_ID" \
    -H "X-Auth-Token: $TOKEN")

INSTANCES_USED=$(echo "$QUOTA_RESPONSE" | jq -r '.quota_set.instances_used')
INSTANCES_LIMIT=$(echo "$QUOTA_RESPONSE" | jq -r '.quota_set.instances')

if [ "$INSTANCES_USED" == "$INSTANCES_LIMIT" ]; then
    log_pass "Usage equals limit (used: $INSTANCES_USED, limit: $INSTANCES_LIMIT)"
else
    log_fail "Usage does not equal limit (used: $INSTANCES_USED, limit: $INSTANCES_LIMIT)"
fi

# Test 8: Delete one instance
log_test "Delete one instance to free quota"
if [ ${#CREATED_INSTANCES[@]} -gt 0 ]; then
    DELETE_ID="${CREATED_INSTANCES[0]}"
    curl -s -X DELETE "http://localhost:8774/v2.1/servers/$DELETE_ID" -H "X-Auth-Token: $TOKEN" > /dev/null
    sleep 1
    log_pass "Deleted instance: $DELETE_ID"
else
    log_fail "No instances to delete"
fi

# Test 9: Create instance after freeing quota (should succeed)
log_test "Create instance after freeing quota (should succeed)"
NEW_INSTANCE_RESPONSE=$(curl -s -X POST "http://localhost:8774/v2.1/servers" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{
        "server": {
            "name": "quota-test-vm-after-delete",
            "flavorRef": "00000000-0000-0000-0000-000000000010",
            "imageRef": "00000000-0000-0000-0000-000000000001"
        }
    }')

NEW_INSTANCE_ID=$(echo "$NEW_INSTANCE_RESPONSE" | jq -r '.server.id')

if [ -n "$NEW_INSTANCE_ID" ] && [ "$NEW_INSTANCE_ID" != "null" ]; then
    log_pass "Created instance after freeing quota: $NEW_INSTANCE_ID"
    CREATED_INSTANCES+=("$NEW_INSTANCE_ID")
else
    log_fail "Failed to create instance after freeing quota"
fi

# Test 10: Test cores quota
log_test "Test cores quota enforcement"

# Get current cores usage
QUOTA_RESPONSE=$(curl -s -X GET "http://localhost:8774/v2.1/os-quota-sets/$PROJECT_ID" \
    -H "X-Auth-Token: $TOKEN")
CORES_LIMIT=$(echo "$QUOTA_RESPONSE" | jq -r '.quota_set.cores')

if [ "$CORES_LIMIT" == "6" ]; then
    log_pass "Cores quota is set to 6"
else
    log_fail "Cores quota not set correctly"
fi

# Cleanup
echo ""
echo "Cleaning up test resources..."
for instance_id in "${CREATED_INSTANCES[@]}"; do
    curl -s -X DELETE "http://localhost:8774/v2.1/servers/$instance_id" -H "X-Auth-Token: $TOKEN" > /dev/null
done

# Reset quotas to defaults
curl -s -X PUT "http://localhost:8774/v2.1/os-quota-sets/$PROJECT_ID" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{
        "quota_set": {
            "instances": 10,
            "cores": 20,
            "ram": 51200
        }
    }' > /dev/null

# Print summary
echo ""
echo "========================================"
echo "      QUOTA MANAGEMENT TEST SUMMARY"
echo "========================================"
echo -e "Total Tests:  $TOTAL_TESTS"
echo -e "${GREEN}Passed:       $TESTS_PASSED${NC}"
echo -e "${RED}Failed:       $TESTS_FAILED${NC}"
echo "========================================"
echo ""

if [ $TESTS_FAILED -eq 0 ]; then
    echo -e "${GREEN}✅ ALL QUOTA TESTS PASSED${NC}"
    exit 0
else
    echo -e "${RED}❌ SOME TESTS FAILED${NC}"
    exit 1
fi
