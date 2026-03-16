#!/bin/bash
# Comprehensive endpoint audit - verify actual coverage
# Counts implemented endpoints across all services

echo "=== O3K Endpoint Coverage Audit ==="
echo ""

# Helper function to count endpoints
count_endpoints() {
    local file=$1
    local service=$2
    local count=$(grep -c "v[0-9]\+\.\(GET\|POST\|PUT\|PATCH\|DELETE\)" "$file" 2>/dev/null || echo 0)
    echo "$service: $count endpoints"
}

# Nova endpoints
echo "## Nova (Compute)"
NOVA_COUNT=0
for file in /Users/I761222/git/o3k/internal/nova/*.go; do
    count=$(grep -c "v[0-9]\+\.\(GET\|POST\|PUT\|PATCH\|DELETE\)" "$file" 2>/dev/null || echo 0)
    NOVA_COUNT=$((NOVA_COUNT + count))
done
echo "Total: $NOVA_COUNT endpoints"
echo ""

# Neutron endpoints
echo "## Neutron (Network)"
NEUTRON_COUNT=0
for file in /Users/I761222/git/o3k/internal/neutron/*.go; do
    count=$(grep -c "v[0-9]\+\.\(GET\|POST\|PUT\|PATCH\|DELETE\)" "$file" 2>/dev/null || echo 0)
    NEUTRON_COUNT=$((NEUTRON_COUNT + count))
done
echo "Total: $NEUTRON_COUNT endpoints"
echo ""

# Cinder endpoints
echo "## Cinder (Volume)"
CINDER_COUNT=0
for file in /Users/I761222/git/o3k/internal/cinder/*.go; do
    count=$(grep -c "v[0-9]\+\.\(GET\|POST\|PUT\|PATCH\|DELETE\)" "$file" 2>/dev/null || echo 0)
    CINDER_COUNT=$((CINDER_COUNT + count))
done
echo "Total: $CINDER_COUNT endpoints"
echo ""

# Glance endpoints
echo "## Glance (Image)"
GLANCE_COUNT=0
for file in /Users/I761222/git/o3k/internal/glance/*.go; do
    count=$(grep -c "v[0-9]\+\.\(GET\|POST\|PUT\|PATCH\|DELETE\)" "$file" 2>/dev/null || echo 0)
    GLANCE_COUNT=$((GLANCE_COUNT + count))
done
echo "Total: $GLANCE_COUNT endpoints"
echo ""

# Keystone endpoints
echo "## Keystone (Identity)"
KEYSTONE_COUNT=0
for file in /Users/I761222/git/o3k/internal/keystone/*.go; do
    count=$(grep -c "v[0-9]\+\.\(GET\|POST\|PUT\|PATCH\|DELETE\)" "$file" 2>/dev/null || echo 0)
    KEYSTONE_COUNT=$((KEYSTONE_COUNT + count))
done
echo "Total: $KEYSTONE_COUNT endpoints"
echo ""

# Calculate total
TOTAL=$((NOVA_COUNT + NEUTRON_COUNT + CINDER_COUNT + GLANCE_COUNT + KEYSTONE_COUNT))
PERCENTAGE=$((TOTAL * 100 / 330))

echo "=================================="
echo "Total Endpoints: $TOTAL / 330"
echo "Coverage: ${PERCENTAGE}%"
echo "Remaining: $((330 - TOTAL)) endpoints"
echo "=================================="
echo ""

# List missing high-value endpoints
echo "## Known Missing Endpoints (LOW Priority)"
echo ""
echo "1. Keystone Federation/SAML (~10 endpoints)"
echo "   - OS-FEDERATION identity providers"
echo "   - OS-FEDERATION mappings"
echo "   - OS-FEDERATION protocols"
echo ""
echo "2. Neutron Advanced (~8 endpoints)"
echo "   - Metering labels/rules"
echo "   - Auto-allocated topology"
echo "   - DVR support"
echo ""
echo "3. Nova Microversions (~variable)"
echo "   - Version-gated features"
echo "   - Enhanced metadata fields"
echo ""

exit 0
