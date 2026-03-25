#!/bin/bash
# Comprehensive O3K API Contract Testing Script
# Tests all services against OpenStack 2025.2 compatibility

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
export OS_AUTH_URL=${OS_AUTH_URL:-http://localhost:35357/v3}
export OS_USERNAME=${OS_USERNAME:-admin}
export OS_PASSWORD=${OS_PASSWORD:-secret}
export OS_PROJECT_NAME=${OS_PROJECT_NAME:-default}
export OS_USER_DOMAIN_NAME=${OS_USER_DOMAIN_NAME:-Default}
export OS_PROJECT_DOMAIN_NAME=${OS_PROJECT_DOMAIN_NAME:-Default}

echo "============================="
echo "O3K API Contract Test Suite"
echo "============================="
echo "Auth URL: $OS_AUTH_URL"
echo ""

# Check if O3K is running
echo "Checking O3K availability..."
if ! curl -sf "$OS_AUTH_URL" > /dev/null 2>&1; then
    echo -e "${RED}ERROR: O3K is not running at $OS_AUTH_URL${NC}"
    echo "Start O3K with: docker compose -f deployments/docker-compose.yml up -d"
    exit 1
fi
echo -e "${GREEN}✓ O3K is running${NC}"
echo ""

# Test authentication
echo "Testing authentication..."
if ! openstack token issue > /dev/null 2>&1; then
    echo -e "${RED}ERROR: Authentication failed${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Authentication successful${NC}"
echo ""

# Run contract tests for each service
SERVICES=("keystone" "cinder" "neutron" "glance")
TOTAL_PASS=0
TOTAL_FAIL=0
TOTAL_TESTS=0

for SERVICE in "${SERVICES[@]}"; do
    echo "============================="
    echo "Testing $SERVICE"
    echo "============================="

    cd "/Users/I761222/git/o3k/test/contract/$SERVICE"

    # Run tests with timeout and JSON output
    TEST_OUTPUT=$(go test -json -count=1 -timeout=3m 2>&1)

    # Count results
    PASS_COUNT=$(echo "$TEST_OUTPUT" | jq -r 'select(.Action == "pass" and .Test != null) | .Test' | wc -l | tr -d ' ')
    FAIL_COUNT=$(echo "$TEST_OUTPUT" | jq -r 'select(.Action == "fail" and .Test != null) | .Test' | wc -l | tr -d ' ')

    TOTAL_PASS=$((TOTAL_PASS + PASS_COUNT))
    TOTAL_FAIL=$((TOTAL_FAIL + FAIL_COUNT))
    TOTAL_TESTS=$((TOTAL_TESTS + PASS_COUNT + FAIL_COUNT))

    if [ "$FAIL_COUNT" -eq 0 ]; then
        echo -e "${GREEN}✓ All $PASS_COUNT tests passed${NC}"
    else
        echo -e "${YELLOW}⚠ $PASS_COUNT passed, $FAIL_COUNT failed${NC}"
        echo ""
        echo "Failed tests:"
        echo "$TEST_OUTPUT" | jq -r 'select(.Action == "fail" and .Test != null) | .Test'
    fi
    echo ""
done

# Summary
echo "============================="
echo "Test Summary"
echo "============================="
echo "Total Tests: $TOTAL_TESTS"
echo -e "Passed: ${GREEN}$TOTAL_PASS${NC}"
echo -e "Failed: ${RED}$TOTAL_FAIL${NC}"

PASS_RATE=$(awk "BEGIN {printf \"%.1f\", ($TOTAL_PASS/$TOTAL_TESTS)*100}")
echo "Pass Rate: $PASS_RATE%"

if [ "$TOTAL_FAIL" -eq 0 ]; then
    echo -e "${GREEN}✓ All tests passed!${NC}"
    exit 0
else
    echo -e "${YELLOW}⚠ Some tests failed${NC}"
    exit 1
fi
