#!/bin/bash
# Keypair Test Suite

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

# Test 1: List keypairs (should be empty initially)
log_test "List keypairs (initial)"
KEYPAIRS=$(curl -s -X GET "http://localhost:8774/v2.1/os-keypairs" \
    -H "X-Auth-Token: $TOKEN")

KEYPAIR_COUNT=$(echo "$KEYPAIRS" | jq '.keypairs | length')

if [ "$KEYPAIR_COUNT" == "0" ] || [ "$KEYPAIR_COUNT" == "null" ]; then
    log_pass "Initial keypair list is empty or null"
else
    log_fail "Expected 0 keypairs initially, found $KEYPAIR_COUNT"
fi

# Test 2: Create keypair with auto-generated keys
log_test "Create keypair with auto-generated keys"
KEYPAIR_RESPONSE=$(curl -s -X POST "http://localhost:8774/v2.1/os-keypairs" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{
        "keypair": {
            "name": "test-key-generated"
        }
    }')

KEYPAIR_NAME=$(echo "$KEYPAIR_RESPONSE" | jq -r '.keypair.name')
KEYPAIR_FINGERPRINT=$(echo "$KEYPAIR_RESPONSE" | jq -r '.keypair.fingerprint')
KEYPAIR_PUBLIC=$(echo "$KEYPAIR_RESPONSE" | jq -r '.keypair.public_key')
KEYPAIR_PRIVATE=$(echo "$KEYPAIR_RESPONSE" | jq -r '.keypair.private_key')

if [ "$KEYPAIR_NAME" == "test-key-generated" ] && [ -n "$KEYPAIR_FINGERPRINT" ] && [ "$KEYPAIR_FINGERPRINT" != "null" ]; then
    log_pass "Created keypair with auto-generated keys"
else
    log_fail "Failed to create keypair"
    echo "Response: $KEYPAIR_RESPONSE"
fi

# Test 3: Verify private key was returned (only on creation)
log_test "Verify private key is returned on creation"
if [ -n "$KEYPAIR_PRIVATE" ] && [ "$KEYPAIR_PRIVATE" != "null" ]; then
    log_pass "Private key returned (length: ${#KEYPAIR_PRIVATE} chars)"
else
    log_fail "Private key not returned"
fi

# Test 4: Verify fingerprint format
log_test "Verify fingerprint format (XX:XX:XX:...)"
if [[ "$KEYPAIR_FINGERPRINT" =~ ^[0-9a-f]{2}(:[0-9a-f]{2}){15}$ ]]; then
    log_pass "Fingerprint format is valid: $KEYPAIR_FINGERPRINT"
else
    log_fail "Invalid fingerprint format: $KEYPAIR_FINGERPRINT"
fi

# Test 5: Create keypair with user-provided public key
log_test "Create keypair with user-provided public key"

# Use a real SSH public key
TEST_PUBLIC_KEY="ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC0dqVavIkTaBZC99p3FphxQbiM58NotmLstjh4Mv32Afp1VfBkaX5ahp610XyTghLGfYKta5m+gOykHHFx2NF6vaeQ1w+kCM9+4VxvGJydbe+1qHoxrn11/gyHg0i4un53bCFMZ6o0BFsX2lxZbkKw0ihCHuA94FcMcgtmg1qzgKPm4VUcrokGFC3AdYEvqQJ1df2VMChHH223LQVFzaqnarAtDZjWwXDPD7+vuADWmsDf53+Qj3cxHkhu/5YqJd2VDBRTOJhqOnfQkqKbW9LFqiNOjQ9PuuPR7eRDW0i6e80IB7zuUYroJM96+h4Ik2EkNoXFR6eGnwkFxfT0u8dh test@example.com"

KEYPAIR2_RESPONSE=$(curl -s -X POST "http://localhost:8774/v2.1/os-keypairs" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d "{
        \"keypair\": {
            \"name\": \"test-key-imported\",
            \"public_key\": \"$TEST_PUBLIC_KEY\"
        }
    }")

KEYPAIR2_NAME=$(echo "$KEYPAIR2_RESPONSE" | jq -r '.keypair.name')
KEYPAIR2_FINGERPRINT=$(echo "$KEYPAIR2_RESPONSE" | jq -r '.keypair.fingerprint')
KEYPAIR2_PRIVATE=$(echo "$KEYPAIR2_RESPONSE" | jq -r '.keypair.private_key')

if [ "$KEYPAIR2_NAME" == "test-key-imported" ] && [ -n "$KEYPAIR2_FINGERPRINT" ] && [ "$KEYPAIR2_FINGERPRINT" != "null" ]; then
    log_pass "Created keypair with imported public key"
else
    log_fail "Failed to create keypair with imported key"
    echo "Response: $KEYPAIR2_RESPONSE"
fi

# Test 6: Verify no private key returned for imported key
log_test "Verify no private key for imported public key"
if [ "$KEYPAIR2_PRIVATE" == "null" ] || [ -z "$KEYPAIR2_PRIVATE" ]; then
    log_pass "Private key not returned for imported key"
else
    log_fail "Private key should not be returned for imported key"
fi

# Test 7: List keypairs (should have 2 now)
log_test "List keypairs (should have 2)"
KEYPAIRS=$(curl -s -X GET "http://localhost:8774/v2.1/os-keypairs" \
    -H "X-Auth-Token: $TOKEN")

KEYPAIR_COUNT=$(echo "$KEYPAIRS" | jq '.keypairs | length')

if [ "$KEYPAIR_COUNT" == "2" ]; then
    log_pass "Found 2 keypairs"
else
    log_fail "Expected 2 keypairs, found $KEYPAIR_COUNT"
fi

# Test 8: Get specific keypair details
log_test "Get keypair details by name"
KEYPAIR_DETAILS=$(curl -s -X GET "http://localhost:8774/v2.1/os-keypairs/test-key-generated" \
    -H "X-Auth-Token: $TOKEN")

DETAIL_NAME=$(echo "$KEYPAIR_DETAILS" | jq -r '.keypair.name')
DETAIL_FINGERPRINT=$(echo "$KEYPAIR_DETAILS" | jq -r '.keypair.fingerprint')

if [ "$DETAIL_NAME" == "test-key-generated" ] && [ "$DETAIL_FINGERPRINT" == "$KEYPAIR_FINGERPRINT" ]; then
    log_pass "Retrieved keypair details correctly"
else
    log_fail "Failed to get keypair details"
fi

# Test 9: Try to create duplicate keypair (should fail)
log_test "Try to create duplicate keypair (should fail)"
DUPLICATE_RESPONSE=$(curl -s -X POST "http://localhost:8774/v2.1/os-keypairs" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{
        "keypair": {
            "name": "test-key-generated"
        }
    }')

ERROR_MSG=$(echo "$DUPLICATE_RESPONSE" | jq -r '.error')

if [[ "$ERROR_MSG" == *"already exists"* ]] || [[ "$ERROR_MSG" == *"Conflict"* ]]; then
    log_pass "Correctly rejected duplicate keypair"
else
    log_fail "Should have rejected duplicate keypair"
    echo "Response: $DUPLICATE_RESPONSE"
fi

# Test 10: Create instance with test metadata endpoint
log_test "Create test instance for metadata service test"
INSTANCE_RESPONSE=$(curl -s -X POST "http://localhost:8774/v2.1/servers" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{
        "server": {
            "name": "keypair-test-vm",
            "flavorRef": "00000000-0000-0000-0000-000000000010",
            "imageRef": "00000000-0000-0000-0000-000000000001"
        }
    }')

INSTANCE_ID=$(echo "$INSTANCE_RESPONSE" | jq -r '.server.id')

if [ -n "$INSTANCE_ID" ] && [ "$INSTANCE_ID" != "null" ]; then
    log_pass "Created test instance: $INSTANCE_ID"
else
    log_fail "Failed to create test instance"
fi

# Test 11: Query metadata service for SSH keys
log_test "Query metadata service for SSH keys"
sleep 2  # Wait for instance to be created

METADATA=$(curl -s -X GET "http://localhost:8775/openstack/latest/meta_data.json" \
    -H "X-Instance-ID: $INSTANCE_ID")

PUBLIC_KEYS=$(echo "$METADATA" | jq '.public_keys')

if [ "$PUBLIC_KEYS" != "null" ] && [ "$PUBLIC_KEYS" != "{}" ]; then
    KEY_COUNT=$(echo "$PUBLIC_KEYS" | jq 'keys | length')
    log_pass "Metadata service returned $KEY_COUNT public key(s)"
else
    log_fail "Metadata service did not return public keys"
    echo "Metadata: $METADATA"
fi

# Test 12: Verify correct keys in metadata
log_test "Verify correct keys appear in metadata"
KEY1_IN_META=$(echo "$PUBLIC_KEYS" | jq -r '."test-key-generated"')
KEY2_IN_META=$(echo "$PUBLIC_KEYS" | jq -r '."test-key-imported"')

if [ -n "$KEY1_IN_META" ] && [ "$KEY1_IN_META" != "null" ] && [ -n "$KEY2_IN_META" ] && [ "$KEY2_IN_META" != "null" ]; then
    log_pass "Both keypairs appear in metadata"
else
    log_fail "Not all keypairs found in metadata"
fi

# Test 13: Delete keypair
log_test "Delete keypair"
HTTP_CODE=$(curl -s -w "%{http_code}" -o /dev/null -X DELETE "http://localhost:8774/v2.1/os-keypairs/test-key-imported" \
    -H "X-Auth-Token: $TOKEN")

if [ "$HTTP_CODE" == "202" ] || [ "$HTTP_CODE" == "204" ]; then
    log_pass "Deleted keypair (HTTP $HTTP_CODE)"
else
    log_fail "Failed to delete keypair (HTTP $HTTP_CODE)"
fi

# Test 14: Verify keypair was deleted
log_test "Verify keypair was deleted"
sleep 1
DELETED_KEYPAIR=$(curl -s -X GET "http://localhost:8774/v2.1/os-keypairs/test-key-imported" \
    -H "X-Auth-Token: $TOKEN")

ERROR_MSG=$(echo "$DELETED_KEYPAIR" | jq -r '.error')

if [[ "$ERROR_MSG" == *"not found"* ]] || [[ "$ERROR_MSG" == *"Not"* ]]; then
    log_pass "Keypair no longer exists"
else
    log_fail "Keypair still exists"
fi

# Test 15: List keypairs after deletion (should have 1)
log_test "List keypairs after deletion"
KEYPAIRS=$(curl -s -X GET "http://localhost:8774/v2.1/os-keypairs" \
    -H "X-Auth-Token: $TOKEN")

KEYPAIR_COUNT=$(echo "$KEYPAIRS" | jq '.keypairs | length')

if [ "$KEYPAIR_COUNT" == "1" ]; then
    log_pass "Found 1 keypair after deletion"
else
    log_fail "Expected 1 keypair, found $KEYPAIR_COUNT"
fi

# Cleanup
echo ""
echo "Cleaning up test resources..."
curl -s -X DELETE "http://localhost:8774/v2.1/servers/$INSTANCE_ID" -H "X-Auth-Token: $TOKEN" > /dev/null
curl -s -X DELETE "http://localhost:8774/v2.1/os-keypairs/test-key-generated" -H "X-Auth-Token: $TOKEN" > /dev/null

# Print summary
echo ""
echo "========================================"
echo "       KEYPAIR TEST SUMMARY"
echo "========================================"
echo -e "Total Tests:  $TOTAL_TESTS"
echo -e "${GREEN}Passed:       $TESTS_PASSED${NC}"
echo -e "${RED}Failed:       $TESTS_FAILED${NC}"
echo "========================================"
echo ""

if [ $TESTS_FAILED -eq 0 ]; then
    echo -e "${GREEN}✅ ALL KEYPAIR TESTS PASSED${NC}"
    exit 0
else
    echo -e "${RED}❌ SOME TESTS FAILED${NC}"
    exit 1
fi
