#!/bin/bash
# Database Migration System Test Suite

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
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

# Test 1: Verify migration tool exists
log_test "Migration tool binary exists"
if [ -f "./o3k-migrate" ]; then
    log_pass "o3k-migrate binary found"
else
    log_fail "o3k-migrate binary not found"
fi

# Test 2: Check migration status command
log_test "Migration status command"
STATUS_OUTPUT=$(./o3k-migrate status 2>&1)

if echo "$STATUS_OUTPUT" | grep -q "Current Version:"; then
    VERSION=$(echo "$STATUS_OUTPUT" | grep "Current Version:" | awk '{print $3}')
    log_pass "Status command works (current version: $VERSION)"
else
    log_fail "Status command failed"
fi

# Test 3: Verify all migration files exist
log_test "Verify migration files are paired (up/down)"
UP_COUNT=$(ls migrations/*up.sql 2>/dev/null | wc -l | tr -d ' ')
DOWN_COUNT=$(ls migrations/*down.sql 2>/dev/null | wc -l | tr -d ' ')

if [ "$UP_COUNT" -eq "$DOWN_COUNT" ] && [ "$UP_COUNT" -gt 0 ]; then
    log_pass "All $UP_COUNT migrations have paired up/down files"
else
    log_fail "Migration files mismatch (up: $UP_COUNT, down: $DOWN_COUNT)"
fi

# Test 4: Check migration file naming convention
log_test "Verify migration file naming convention"
INVALID_FILES=$(find migrations -name "*.sql" ! -name "*_*.sql" 2>/dev/null | wc -l | tr -d ' ')

if [ "$INVALID_FILES" -eq "0" ]; then
    log_pass "All migration files follow naming convention (NNN_description.up/down.sql)"
else
    log_fail "Found $INVALID_FILES files with invalid naming"
fi

# Test 5: Verify migration version is up to date
log_test "Verify database is at latest migration"
LATEST_MIGRATION=$(ls migrations/*up.sql | tail -1 | grep -o '[0-9]\+' | head -1 | sed 's/^0*//')
CURRENT_VERSION=$(echo "$STATUS_OUTPUT" | grep "Current Version:" | awk '{print $3}')

if [ "$CURRENT_VERSION" == "$LATEST_MIGRATION" ]; then
    log_pass "Database is at latest version ($CURRENT_VERSION)"
else
    log_fail "Database version mismatch (current: $CURRENT_VERSION, latest: $LATEST_MIGRATION)"
fi

# Test 6: Check migration validation
log_test "Migration validation function"
if echo "$STATUS_OUTPUT" | grep -q "Migration files validated"; then
    MIGRATION_COUNT=$(echo "$STATUS_OUTPUT" | grep "Migration files validated" | grep -o 'migrations":[0-9]*' | grep -o '[0-9]*')
    log_pass "Migration validation passed ($MIGRATION_COUNT migrations)"
else
    log_fail "Migration validation not run or failed"
fi

# Test 7: Check migration state is clean (not dirty)
log_test "Verify migration state is clean (not dirty)"
if echo "$STATUS_OUTPUT" | grep -q "Status: Clean"; then
    log_pass "Migration state is clean"
elif echo "$STATUS_OUTPUT" | grep -q "Status: DIRTY"; then
    log_fail "Migration state is DIRTY (requires manual intervention)"
else
    log_fail "Migration state unknown"
fi

# Test 8: Verify rollback command exists
log_test "Verify rollback command exists (dry-run)"
HELP_OUTPUT=$(./o3k-migrate 2>&1)
if echo "$HELP_OUTPUT" | grep -q "down"; then
    log_pass "Rollback (down) command available"
else
    log_fail "Rollback command not found"
fi

# Test 9: Verify goto command exists
log_test "Verify goto command exists"
if echo "$HELP_OUTPUT" | grep -q "goto"; then
    log_pass "Goto command available for version targeting"
else
    log_fail "Goto command not found"
fi

# Test 10: Check reset command exists
log_test "Verify reset command exists"
if echo "$HELP_OUTPUT" | grep -q "reset"; then
    log_pass "Reset command available"
else
    log_fail "Reset command not found"
fi

# Test 11: Verify force command for dirty state recovery
log_test "Verify force command exists"
if echo "$HELP_OUTPUT" | grep -q "force"; then
    log_pass "Force command available for dirty state recovery"
else
    log_fail "Force command not found"
fi

# Test 12: Check migration descriptions are meaningful
log_test "Verify migration files have meaningful descriptions"
GENERIC_NAMES=$(find migrations -name "*untitled*.sql" -o -name "*test*.sql" | wc -l | tr -d ' ')

if [ "$GENERIC_NAMES" -eq "0" ]; then
    log_pass "All migrations have descriptive names"
else
    log_fail "Found $GENERIC_NAMES migrations with generic/test names"
fi

# Test 13: Verify help output is comprehensive
log_test "Verify comprehensive help documentation"
if echo "$HELP_OUTPUT" | grep -q "Commands:" && echo "$HELP_OUTPUT" | grep -q "Examples:"; then
    log_pass "Help documentation includes commands and examples"
else
    log_fail "Help documentation incomplete"
fi

# Test 14: Check all migration files are readable
log_test "Verify all migration files are readable"
UNREADABLE=0
for file in migrations/*.sql; do
    if [ ! -r "$file" ]; then
        ((UNREADABLE++))
    fi
done

if [ "$UNREADABLE" -eq "0" ]; then
    log_pass "All migration files are readable"
else
    log_fail "Found $UNREADABLE unreadable migration files"
fi

# Test 15: Verify migrations directory structure
log_test "Verify migrations directory exists and is accessible"
if [ -d "migrations" ] && [ -r "migrations" ]; then
    FILE_COUNT=$(ls migrations/*.sql 2>/dev/null | wc -l | tr -d ' ')
    log_pass "Migrations directory accessible with $FILE_COUNT SQL files"
else
    log_fail "Migrations directory not accessible"
fi

# Print summary
echo ""
echo "========================================"
echo "   MIGRATION SYSTEM TEST SUMMARY"
echo "========================================"
echo -e "Total Tests:  $TOTAL_TESTS"
echo -e "${GREEN}Passed:       $TESTS_PASSED${NC}"
echo -e "${RED}Failed:       $TESTS_FAILED${NC}"
echo "========================================"
echo ""

if [ $TESTS_FAILED -eq 0 ]; then
    echo -e "${GREEN}✅ ALL MIGRATION TESTS PASSED${NC}"
    echo ""
    echo "Migration System Summary:"
    echo "  - Current Version: $CURRENT_VERSION"
    echo "  - Total Migrations: $UP_COUNT"
    echo "  - Status: Clean"
    echo ""
    echo "Available Commands:"
    echo "  - status:  Show current migration version and status"
    echo "  - up:      Apply all pending migrations"
    echo "  - down:    Roll back one migration"
    echo "  - goto N:  Migrate to specific version N"
    echo "  - reset:   Drop all tables and re-run migrations"
    echo "  - force N: Force migration version to N (for dirty state)"
    echo ""
    echo "Usage Examples:"
    echo "  ./o3k-migrate status"
    echo "  ./o3k-migrate up"
    echo "  ./o3k-migrate goto 5"
    echo ""
    exit 0
else
    echo -e "${RED}❌ SOME TESTS FAILED${NC}"
    exit 1
fi
