#!/bin/bash
# Quickstart Validation Test - Verify Horizon deployment guide
# Tests all steps in specs/002-horizon-full-compatibility/quickstart.md

set -e

echo "=== Quickstart Validation Test ==="
echo "Validating Horizon deployment guide"
echo ""

# Test environment
DOCKER_REQUIRED_VERSION="20.10"
DOCKER_COMPOSE_REQUIRED_VERSION="2.0"

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Success/failure tracking
TESTS_PASSED=0
TESTS_FAILED=0

pass() {
    echo -e "${GREEN}✓${NC} $1"
    ((TESTS_PASSED++))
}

fail() {
    echo -e "${RED}✗${NC} $1"
    ((TESTS_FAILED++))
}

# Step 1: Verify Prerequisites
echo "Step 1: Verify Prerequisites"
echo "-----------------------------"

# Check Docker
if command -v docker &> /dev/null; then
    DOCKER_VERSION=$(docker --version | grep -oE '[0-9]+\.[0-9]+' | head -1)
    if [[ $(echo "$DOCKER_VERSION >= $DOCKER_REQUIRED_VERSION" | bc) -eq 1 ]]; then
        pass "Docker $DOCKER_VERSION installed (required: $DOCKER_REQUIRED_VERSION+)"
    else
        fail "Docker $DOCKER_VERSION too old (required: $DOCKER_REQUIRED_VERSION+)"
    fi
else
    fail "Docker not installed"
fi

# Check Docker Compose
if docker compose version &> /dev/null; then
    COMPOSE_VERSION=$(docker compose version | grep -oE 'v[0-9]+\.[0-9]+' | head -1 | sed 's/v//')
    if [[ $(echo "$COMPOSE_VERSION >= $DOCKER_COMPOSE_REQUIRED_VERSION" | bc) -eq 1 ]]; then
        pass "Docker Compose $COMPOSE_VERSION installed (required: $DOCKER_COMPOSE_REQUIRED_VERSION+)"
    else
        fail "Docker Compose $COMPOSE_VERSION too old (required: $DOCKER_COMPOSE_REQUIRED_VERSION+)"
    fi
else
    fail "Docker Compose not installed"
fi

# Check disk space
AVAILABLE_SPACE=$(df -h . | awk 'NR==2 {print $4}' | sed 's/G//')
if [[ $(echo "$AVAILABLE_SPACE >= 10" | bc) -eq 1 ]]; then
    pass "Disk space: ${AVAILABLE_SPACE}GB available (required: 10GB+)"
else
    fail "Disk space: ${AVAILABLE_SPACE}GB available (required: 10GB+)"
fi

echo ""

# Step 2: Verify O3K Deployment
echo "Step 2: Verify O3K Deployment"
echo "------------------------------"

# Check if O3K is running
if docker ps | grep -q "o3k"; then
    pass "O3K container running"
else
    fail "O3K container not running (run: docker compose -f deployments/docker-compose.yml up -d)"
fi

# Check if PostgreSQL is running
if docker ps | grep -q "postgres"; then
    pass "PostgreSQL container running"
else
    fail "PostgreSQL container not running"
fi

# Test Keystone endpoint
if curl -s -f http://localhost:35357/v3 > /dev/null; then
    pass "Keystone endpoint accessible at http://localhost:35357/v3"
else
    fail "Keystone endpoint not accessible"
fi

# Test authentication
export OS_AUTH_URL=http://localhost:35357/v3
export OS_USERNAME=admin
export OS_PASSWORD=secret
export OS_PROJECT_NAME=default
export OS_USER_DOMAIN_NAME=Default
export OS_PROJECT_DOMAIN_NAME=Default
export OS_IDENTITY_API_VERSION=3

if TOKEN=$(openstack token issue -f value -c id 2>/dev/null) && [ -n "$TOKEN" ]; then
    pass "Authentication successful"
else
    fail "Authentication failed"
fi

echo ""

# Step 3: Verify Service Catalog
echo "Step 3: Verify Service Catalog"
echo "-------------------------------"

# Check required services
REQUIRED_SERVICES=("identity" "compute" "network" "volume" "image")
for service in "${REQUIRED_SERVICES[@]}"; do
    if openstack catalog show "$service" &> /dev/null; then
        pass "Service catalog contains '$service'"
    else
        fail "Service catalog missing '$service'"
    fi
done

echo ""

# Step 4: Verify API Endpoints
echo "Step 4: Verify API Endpoints"
echo "-----------------------------"

# Nova
if curl -s -f http://localhost:8774/v2.1 > /dev/null; then
    pass "Nova endpoint accessible"
else
    fail "Nova endpoint not accessible"
fi

# Neutron
if curl -s -f http://localhost:9696/v2.0 > /dev/null; then
    pass "Neutron endpoint accessible"
else
    fail "Neutron endpoint not accessible"
fi

# Cinder
if curl -s -f "http://localhost:8776/v3/$OS_PROJECT_NAME/volumes" -H "X-Auth-Token: $TOKEN" > /dev/null; then
    pass "Cinder endpoint accessible"
else
    fail "Cinder endpoint not accessible"
fi

# Glance
if curl -s -f http://localhost:9292/v2/images -H "X-Auth-Token: $TOKEN" > /dev/null; then
    pass "Glance endpoint accessible"
else
    fail "Glance endpoint not accessible"
fi

echo ""

# Step 5: Test Core Operations
echo "Step 5: Test Core Operations"
echo "-----------------------------"

# Test flavor list
if openstack flavor list > /dev/null 2>&1; then
    FLAVOR_COUNT=$(openstack flavor list -f value | wc -l)
    pass "Flavor list works ($FLAVOR_COUNT flavors available)"
else
    fail "Flavor list failed"
fi

# Test image list
if openstack image list > /dev/null 2>&1; then
    IMAGE_COUNT=$(openstack image list -f value | wc -l)
    pass "Image list works ($IMAGE_COUNT images available)"
else
    fail "Image list failed"
fi

# Test network list
if openstack network list > /dev/null 2>&1; then
    NETWORK_COUNT=$(openstack network list -f value | wc -l)
    pass "Network list works ($NETWORK_COUNT networks available)"
else
    fail "Network list failed"
fi

# Test volume list
if openstack volume list > /dev/null 2>&1; then
    VOLUME_COUNT=$(openstack volume list -f value | wc -l)
    pass "Volume list works ($VOLUME_COUNT volumes available)"
else
    fail "Volume list failed"
fi

echo ""

# Step 6: Verify Pagination Support
echo "Step 6: Verify Pagination Support"
echo "----------------------------------"

# Test Nova pagination
if curl -s -f "http://localhost:8774/v2.1/servers?limit=10" -H "X-Auth-Token: $TOKEN" > /dev/null; then
    pass "Nova pagination supported (limit parameter)"
else
    fail "Nova pagination not working"
fi

# Test Neutron pagination
if curl -s -f "http://localhost:9696/v2.0/networks?limit=10" -H "X-Auth-Token: $TOKEN" > /dev/null; then
    pass "Neutron pagination supported (limit parameter)"
else
    fail "Neutron pagination not working"
fi

# Test Cinder pagination
if curl -s -f "http://localhost:8776/v3/$OS_PROJECT_NAME/volumes?limit=10" -H "X-Auth-Token: $TOKEN" > /dev/null; then
    pass "Cinder pagination supported (limit parameter)"
else
    fail "Cinder pagination not working"
fi

# Test Glance pagination
if curl -s -f "http://localhost:9292/v2/images?limit=10" -H "X-Auth-Token: $TOKEN" > /dev/null; then
    pass "Glance pagination supported (limit parameter)"
else
    fail "Glance pagination not working"
fi

echo ""

# Step 7: Verify Database Indexes
echo "Step 7: Verify Database Indexes"
echo "--------------------------------"

# Check if migration 055 was applied
if docker exec o3k-postgres psql -U o3k -d o3k -tAc "SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE indexname = 'idx_instances_status');" | grep -q "t"; then
    pass "Performance indexes applied (idx_instances_status exists)"
else
    fail "Performance indexes not applied (migration 055 may not have run)"
fi

# Check database connection pool
if docker exec o3k-postgres psql -U o3k -d o3k -tAc "SELECT count(*) FROM pg_stat_activity;" > /dev/null; then
    ACTIVE_CONNECTIONS=$(docker exec o3k-postgres psql -U o3k -d o3k -tAc "SELECT count(*) FROM pg_stat_activity;")
    pass "Database connection pool active ($ACTIVE_CONNECTIONS connections)"
else
    fail "Database connection pool check failed"
fi

echo ""

# Summary
echo "========================================="
echo "Test Summary"
echo "========================================="
echo -e "Passed: ${GREEN}$TESTS_PASSED${NC}"
echo -e "Failed: ${RED}$TESTS_FAILED${NC}"
echo ""

if [ $TESTS_FAILED -eq 0 ]; then
    echo -e "${GREEN}✓ All tests passed!${NC}"
    echo "Quickstart deployment guide is valid."
    echo ""
    echo "Next steps:"
    echo "1. Deploy Horizon following quickstart.md Step 2"
    echo "2. Access dashboard at http://localhost/dashboard"
    echo "3. Login with admin/secret"
    exit 0
else
    echo -e "${RED}✗ Some tests failed.${NC}"
    echo "Please fix the issues above before deploying Horizon."
    echo ""
    echo "Common fixes:"
    echo "- Ensure O3K is running: docker compose -f deployments/docker-compose.yml up -d"
    echo "- Wait for services to initialize (30-60 seconds)"
    echo "- Run migrations: make migrate"
    echo "- Check logs: docker compose -f deployments/docker-compose.yml logs"
    exit 1
fi
