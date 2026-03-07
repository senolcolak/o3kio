#!/bin/bash
# Phase 3 Comprehensive Test Suite
# Tests: Volume attach/detach, Console access, Metadata service, Advanced actions, Interface hot-plug

set -e

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo ""
echo "========================================"
echo "  Phase 3 Comprehensive Test Suite"
echo "========================================"
echo ""

TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

# Function to run a test suite
run_test_suite() {
    local test_name="$1"
    local test_script="$2"

    echo -e "${BLUE}Running: $test_name${NC}"
    echo "----------------------------------------"

    if [ ! -f "$SCRIPT_DIR/$test_script" ]; then
        echo -e "${RED}✗ Test script not found: $test_script${NC}"
        ((FAILED_TESTS++))
        return
    fi

    if bash "$SCRIPT_DIR/$test_script" > /tmp/test_output.log 2>&1; then
        # Test passed
        echo -e "${GREEN}✓ $test_name PASSED${NC}"
        ((PASSED_TESTS++))

        # Show summary from output
        grep -E "Tests passed:|✅" /tmp/test_output.log | tail -3
    else
        # Test failed
        echo -e "${RED}✗ $test_name FAILED${NC}"
        ((FAILED_TESTS++))

        # Show failure details
        echo ""
        echo "Last 20 lines of output:"
        tail -20 /tmp/test_output.log
    fi

    ((TOTAL_TESTS++))
    echo ""
}

# Run all Phase 3 test suites
echo "Starting test execution..."
echo ""

run_test_suite "Volume Attach/Detach" "volume_attach_test.sh"
run_test_suite "Console Access (VNC)" "console_test.sh"
run_test_suite "Metadata Service" "metadata_test.sh"
run_test_suite "Advanced VM Actions" "advanced_actions_test.sh"
run_test_suite "Network Interface Hot-Plug" "interface_attach_test.sh"

# Print final summary
echo ""
echo "========================================"
echo "       PHASE 3 TEST SUMMARY"
echo "========================================"
echo -e "Total Test Suites: $TOTAL_TESTS"
echo -e "${GREEN}Passed:            $PASSED_TESTS${NC}"
echo -e "${RED}Failed:            $FAILED_TESTS${NC}"
echo "========================================"
echo ""

if [ $FAILED_TESTS -eq 0 ]; then
    echo -e "${GREEN}✅ ALL PHASE 3 TESTS PASSED${NC}"
    echo ""
    echo "Phase 3 Features Verified:"
    echo "  ✓ Volume attach/detach with libvirt"
    echo "  ✓ VM console access (VNC)"
    echo "  ✓ Metadata service for cloud-init"
    echo "  ✓ Advanced VM actions (suspend/resume/shelve/resize)"
    echo "  ✓ Network interface hot-plug/unplug"
    echo ""
    exit 0
else
    echo -e "${RED}❌ SOME TESTS FAILED${NC}"
    echo ""
    echo "Review the output above for failure details."
    echo ""
    exit 1
fi
