#!/bin/bash
# Test Nova Server Actions (Sprint 56-57)
# Validates 8 operational action endpoints

set -e

echo "=== Nova Server Actions Test ==="

# Configuration
export OS_AUTH_URL=http://localhost:35357/v3
export OS_USERNAME=admin
export OS_PASSWORD=secret
export OS_PROJECT_NAME=default
export OS_DOMAIN_NAME=Default

# Authenticate
echo "1. Authenticating..."
TOKEN=$(openstack token issue -f value -c id)
if [ -z "$TOKEN" ]; then
    echo "✗ Authentication failed"
    exit 1
fi
echo "✓ Authentication successful"

# Create test instance
echo "2. Creating test instance..."
FLAVOR_ID=$(openstack flavor list -f value -c ID | head -1)
SERVER_ID=$(openstack server create test-actions-server \
    --flavor "$FLAVOR_ID" \
    --image test-image \
    -f value -c id 2>/dev/null)

if [ -z "$SERVER_ID" ]; then
    echo "✗ Failed to create test server"
    exit 1
fi
echo "✓ Test server created: $SERVER_ID"

# Wait for ACTIVE state
sleep 2

# Test 1: os-resetState (admin only)
echo ""
echo "Test 1: Reset State Action"
RESPONSE=$(curl -s -X POST \
    "http://localhost:8774/v2.1/servers/$SERVER_ID/action" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"os-resetState": {"state": "error"}}' \
    -w "%{http_code}")

HTTP_CODE="${RESPONSE: -3}"
if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "202" ]; then
    echo "✓ os-resetState: PASS (HTTP $HTTP_CODE)"
else
    echo "✗ os-resetState: FAIL (HTTP $HTTP_CODE)"
fi

# Test 2: os-resetNetwork
echo ""
echo "Test 2: Reset Network Action"
RESPONSE=$(curl -s -X POST \
    "http://localhost:8774/v2.1/servers/$SERVER_ID/action" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"os-resetNetwork": null}' \
    -w "%{http_code}")

HTTP_CODE="${RESPONSE: -3}"
if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "202" ]; then
    echo "✓ os-resetNetwork: PASS (HTTP $HTTP_CODE)"
else
    echo "✗ os-resetNetwork: FAIL (HTTP $HTTP_CODE)"
fi

# Test 3: migrate (cold migration)
echo ""
echo "Test 3: Migrate (Cold Migration)"
RESPONSE=$(curl -s -X POST \
    "http://localhost:8774/v2.1/servers/$SERVER_ID/action" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"migrate": null}' \
    -w "%{http_code}")

HTTP_CODE="${RESPONSE: -3}"
if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "202" ]; then
    echo "✓ migrate: PASS (HTTP $HTTP_CODE)"
else
    echo "✗ migrate: FAIL (HTTP $HTTP_CODE)"
fi

# Test 4: evacuate (admin only)
echo ""
echo "Test 4: Evacuate Action"
RESPONSE=$(curl -s -X POST \
    "http://localhost:8774/v2.1/servers/$SERVER_ID/action" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"evacuate": {}}' \
    -w "%{http_code}")

HTTP_CODE="${RESPONSE: -3}"
if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "202" ]; then
    echo "✓ evacuate: PASS (HTTP $HTTP_CODE)"
else
    echo "✗ evacuate: FAIL (HTTP $HTTP_CODE)"
fi

# Test 5: changePassword
echo ""
echo "Test 5: Change Password Action"
RESPONSE=$(curl -s -X POST \
    "http://localhost:8774/v2.1/servers/$SERVER_ID/action" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"changePassword": {"adminPass": "newpassword123"}}' \
    -w "%{http_code}")

HTTP_CODE="${RESPONSE: -3}"
if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "202" ]; then
    echo "✓ changePassword: PASS (HTTP $HTTP_CODE)"
else
    echo "✗ changePassword: FAIL (HTTP $HTTP_CODE)"
fi

# Test 6: createBackup
echo ""
echo "Test 6: Create Backup Action"
RESPONSE=$(curl -s -X POST \
    "http://localhost:8774/v2.1/servers/$SERVER_ID/action" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"createBackup": {"name": "test-backup", "backup_type": "daily", "rotation": 3}}' \
    -w "%{http_code}")

HTTP_CODE="${RESPONSE: -3}"
if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "202" ]; then
    echo "✓ createBackup: PASS (HTTP $HTTP_CODE)"
else
    echo "✗ createBackup: FAIL (HTTP $HTTP_CODE)"
fi

# Test 7: addSecurityGroup
echo ""
echo "Test 7: Add Security Group Action"
# First create a security group
SG_ID=$(openstack security group create test-sg -f value -c id 2>/dev/null)
RESPONSE=$(curl -s -X POST \
    "http://localhost:8774/v2.1/servers/$SERVER_ID/action" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d "{\"addSecurityGroup\": {\"name\": \"$SG_ID\"}}" \
    -w "%{http_code}")

HTTP_CODE="${RESPONSE: -3}"
if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "202" ]; then
    echo "✓ addSecurityGroup: PASS (HTTP $HTTP_CODE)"
else
    echo "✗ addSecurityGroup: FAIL (HTTP $HTTP_CODE)"
fi

# Test 8: removeSecurityGroup
echo ""
echo "Test 8: Remove Security Group Action"
RESPONSE=$(curl -s -X POST \
    "http://localhost:8774/v2.1/servers/$SERVER_ID/action" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d "{\"removeSecurityGroup\": {\"name\": \"$SG_ID\"}}" \
    -w "%{http_code}")

HTTP_CODE="${RESPONSE: -3}"
if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "202" ]; then
    echo "✓ removeSecurityGroup: PASS (HTTP $HTTP_CODE)"
else
    echo "✗ removeSecurityGroup: FAIL (HTTP $HTTP_CODE)"
fi

# Cleanup
echo ""
echo "Cleanup..."
openstack server delete "$SERVER_ID" 2>/dev/null || true
openstack security group delete "$SG_ID" 2>/dev/null || true

echo ""
echo "=== Test Complete ==="
echo "All 8 Nova Server Actions tested"
