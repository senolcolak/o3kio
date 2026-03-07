#!/bin/bash
# Security Groups Test Suite

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

# Test 1: List security groups (should include default)
log_test "List security groups"
GROUPS=$(curl -s -X GET "http://localhost:9696/v2.0/security-groups" \
    -H "X-Auth-Token: $TOKEN")

DEFAULT_EXISTS=$(echo "$GROUPS" | jq -r '.security_groups[] | select(.name=="default") | .id' 2>/dev/null)

if [ -n "$DEFAULT_EXISTS" ]; then
    log_pass "Default security group exists"
else
    log_fail "Default security group not found"
fi

# Test 2: Create security group
log_test "Create security group"
SG_RESPONSE=$(curl -s -X POST "http://localhost:9696/v2.0/security-groups" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{
        "security_group": {
            "name": "test-sg",
            "description": "Test security group"
        }
    }')

SG_ID=$(echo "$SG_RESPONSE" | jq -r '.security_group.id')

if [ -n "$SG_ID" ] && [ "$SG_ID" != "null" ]; then
    log_pass "Created security group: $SG_ID"
else
    log_fail "Failed to create security group"
    echo "Response: $SG_RESPONSE"
fi

# Test 3: Get security group details
log_test "Get security group details"
SG_DETAILS=$(curl -s -X GET "http://localhost:9696/v2.0/security-groups/$SG_ID" \
    -H "X-Auth-Token: $TOKEN")

SG_NAME=$(echo "$SG_DETAILS" | jq -r '.security_group.name')

if [ "$SG_NAME" == "test-sg" ]; then
    log_pass "Retrieved security group details"
else
    log_fail "Failed to get security group details"
fi

# Test 4: Create security group rule (allow SSH)
log_test "Create security group rule (allow SSH ingress)"
RULE_RESPONSE=$(curl -s -X POST "http://localhost:9696/v2.0/security-group-rules" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d "{
        \"security_group_rule\": {
            \"security_group_id\": \"$SG_ID\",
            \"direction\": \"ingress\",
            \"ethertype\": \"IPv4\",
            \"protocol\": \"tcp\",
            \"port_range_min\": 22,
            \"port_range_max\": 22,
            \"remote_ip_prefix\": \"0.0.0.0/0\"
        }
    }")

RULE_ID=$(echo "$RULE_RESPONSE" | jq -r '.security_group_rule.id')

if [ -n "$RULE_ID" ] && [ "$RULE_ID" != "null" ]; then
    log_pass "Created SSH ingress rule: $RULE_ID"
else
    log_fail "Failed to create security group rule"
    echo "Response: $RULE_RESPONSE"
fi

# Test 5: Create another rule (allow HTTP)
log_test "Create security group rule (allow HTTP ingress)"
RULE2_RESPONSE=$(curl -s -X POST "http://localhost:9696/v2.0/security-group-rules" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d "{
        \"security_group_rule\": {
            \"security_group_id\": \"$SG_ID\",
            \"direction\": \"ingress\",
            \"ethertype\": \"IPv4\",
            \"protocol\": \"tcp\",
            \"port_range_min\": 80,
            \"port_range_max\": 80
        }
    }")

RULE2_ID=$(echo "$RULE2_RESPONSE" | jq -r '.security_group_rule.id')

if [ -n "$RULE2_ID" ] && [ "$RULE2_ID" != "null" ]; then
    log_pass "Created HTTP ingress rule: $RULE2_ID"
else
    log_fail "Failed to create HTTP rule"
fi

# Test 6: Create egress rule (allow all)
log_test "Create security group rule (allow all egress)"
RULE3_RESPONSE=$(curl -s -X POST "http://localhost:9696/v2.0/security-group-rules" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d "{
        \"security_group_rule\": {
            \"security_group_id\": \"$SG_ID\",
            \"direction\": \"egress\",
            \"ethertype\": \"IPv4\"
        }
    }")

RULE3_ID=$(echo "$RULE3_RESPONSE" | jq -r '.security_group_rule.id')

if [ -n "$RULE3_ID" ] && [ "$RULE3_ID" != "null" ]; then
    log_pass "Created egress rule: $RULE3_ID"
else
    log_fail "Failed to create egress rule"
fi

# Test 7: List security group rules
log_test "List security group rules"
RULES=$(curl -s -X GET "http://localhost:9696/v2.0/security-group-rules" \
    -H "X-Auth-Token: $TOKEN")

RULE_COUNT=$(echo "$RULES" | jq "[.security_group_rules[] | select(.security_group_id==\"$SG_ID\")] | length")

if [ "$RULE_COUNT" -ge 3 ]; then
    log_pass "Found $RULE_COUNT rules for security group"
else
    log_fail "Expected at least 3 rules, found $RULE_COUNT"
fi

# Test 8: Verify rules in security group details
log_test "Verify rules appear in security group details"
SG_WITH_RULES=$(curl -s -X GET "http://localhost:9696/v2.0/security-groups/$SG_ID" \
    -H "X-Auth-Token: $TOKEN")

EMBEDDED_RULE_COUNT=$(echo "$SG_WITH_RULES" | jq '.security_group.security_group_rules | length')

if [ "$EMBEDDED_RULE_COUNT" -ge 3 ]; then
    log_pass "Security group has $EMBEDDED_RULE_COUNT embedded rules"
else
    log_fail "Expected at least 3 embedded rules, found $EMBEDDED_RULE_COUNT"
fi

# Test 9: Delete a security group rule
log_test "Delete security group rule"
HTTP_CODE=$(curl -s -w "%{http_code}" -o /dev/null -X DELETE "http://localhost:9696/v2.0/security-group-rules/$RULE_ID" \
    -H "X-Auth-Token: $TOKEN")

if [ "$HTTP_CODE" == "204" ]; then
    log_pass "Deleted security group rule (HTTP 204)"
else
    log_fail "Failed to delete rule (HTTP $HTTP_CODE)"
fi

# Test 10: Verify rule was deleted
log_test "Verify rule was deleted"
sleep 1
REMAINING_RULES=$(curl -s -X GET "http://localhost:9696/v2.0/security-group-rules" \
    -H "X-Auth-Token: $TOKEN")

DELETED_RULE_EXISTS=$(echo "$REMAINING_RULES" | jq -r ".security_group_rules[] | select(.id==\"$RULE_ID\") | .id")

if [ -z "$DELETED_RULE_EXISTS" ]; then
    log_pass "Rule successfully deleted"
else
    log_fail "Rule still exists after deletion"
fi

# Test 11: Try to delete default security group (should fail)
log_test "Try to delete default security group (should fail)"
DEFAULT_SG_ID=$(echo "$GROUPS" | jq -r '.security_groups[] | select(.name=="default") | .id' 2>/dev/null)

if [ -n "$DEFAULT_SG_ID" ]; then
    HTTP_CODE=$(curl -s -w "%{http_code}" -o /dev/null -X DELETE "http://localhost:9696/v2.0/security-groups/$DEFAULT_SG_ID" \
        -H "X-Auth-Token: $TOKEN")

    if [ "$HTTP_CODE" == "409" ] || [ "$HTTP_CODE" == "403" ]; then
        log_pass "Correctly rejected deletion of default security group (HTTP $HTTP_CODE)"
    else
        log_fail "Should have rejected default group deletion (got HTTP $HTTP_CODE)"
    fi
fi

# Test 12: Delete custom security group
log_test "Delete custom security group"
HTTP_CODE=$(curl -s -w "%{http_code}" -o /dev/null -X DELETE "http://localhost:9696/v2.0/security-groups/$SG_ID" \
    -H "X-Auth-Token: $TOKEN")

if [ "$HTTP_CODE" == "204" ]; then
    log_pass "Deleted security group (HTTP 204)"
else
    log_fail "Failed to delete security group (HTTP $HTTP_CODE)"
fi

# Test 13: Verify security group was deleted
log_test "Verify security group was deleted"
sleep 1
DELETED_SG=$(curl -s -X GET "http://localhost:9696/v2.0/security-groups/$SG_ID" \
    -H "X-Auth-Token: $TOKEN" \
    -w "\n%{http_code}")

HTTP_CODE=$(echo "$DELETED_SG" | tail -1)

if [ "$HTTP_CODE" == "404" ]; then
    log_pass "Security group no longer exists (HTTP 404)"
else
    log_fail "Security group still exists (HTTP $HTTP_CODE)"
fi

# Print summary
echo ""
echo "========================================"
echo "     SECURITY GROUPS TEST SUMMARY"
echo "========================================"
echo -e "Total Tests:  $TOTAL_TESTS"
echo -e "${GREEN}Passed:       $TESTS_PASSED${NC}"
echo -e "${RED}Failed:       $TESTS_FAILED${NC}"
echo "========================================"
echo ""

if [ $TESTS_FAILED -eq 0 ]; then
    echo -e "${GREEN}✅ ALL SECURITY GROUP TESTS PASSED${NC}"
    exit 0
else
    echo -e "${RED}❌ SOME TESTS FAILED${NC}"
    exit 1
fi
