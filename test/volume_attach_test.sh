#!/bin/bash
# O3K Volume Attach/Detach Test Suite
# Tests volume lifecycle: create → attach → detach → delete

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Test configuration
OS_AUTH_URL="http://localhost:35357/v3"
OS_USERNAME="admin"
OS_PASSWORD="secret"
OS_PROJECT_NAME="default"
OS_USER_DOMAIN_NAME="default"
OS_PROJECT_DOMAIN_NAME="default"

# Test counters
TESTS_PASSED=0
TESTS_FAILED=0
TOTAL_TESTS=0

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[PASS]${NC} $1"
    ((TESTS_PASSED++))
}

log_error() {
    echo -e "${RED}[FAIL]${NC} $1"
    ((TESTS_FAILED++))
}

log_warning() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_test() {
    ((TOTAL_TESTS++))
    echo -e "${YELLOW}[TEST $TOTAL_TESTS]${NC} $1"
}

# Cleanup function
cleanup() {
    log_info "Cleaning up test resources..."

    if [ -n "$INSTANCE_ID" ]; then
        curl -s -X DELETE "http://localhost:8774/v2.1/servers/$INSTANCE_ID" \
            -H "X-Auth-Token: $TOKEN" > /dev/null 2>&1 || true
    fi

    if [ -n "$VOLUME_ID" ]; then
        curl -s -X DELETE "http://localhost:8776/v3/$PROJECT_ID/volumes/$VOLUME_ID" \
            -H "X-Auth-Token: $TOKEN" > /dev/null 2>&1 || true
    fi

    if [ -n "$VOLUME2_ID" ]; then
        curl -s -X DELETE "http://localhost:8776/v3/$PROJECT_ID/volumes/$VOLUME2_ID" \
            -H "X-Auth-Token: $TOKEN" > /dev/null 2>&1 || true
    fi

    if [ -n "$NETWORK_ID" ]; then
        curl -s -X DELETE "http://localhost:9696/v2.0/networks/$NETWORK_ID" \
            -H "X-Auth-Token: $TOKEN" > /dev/null 2>&1 || true
    fi
}

trap cleanup EXIT

# Authenticate
log_test "Authenticate with Keystone"
AUTH_RESPONSE=$(curl -i -s -X POST "$OS_AUTH_URL/auth/tokens" \
    -H "Content-Type: application/json" \
    -d "{
        \"auth\": {
            \"identity\": {
                \"methods\": [\"password\"],
                \"password\": {
                    \"user\": {
                        \"name\": \"$OS_USERNAME\",
                        \"password\": \"$OS_PASSWORD\",
                        \"domain\": {\"name\": \"$OS_USER_DOMAIN_NAME\"}
                    }
                }
            },
            \"scope\": {
                \"project\": {
                    \"name\": \"$OS_PROJECT_NAME\",
                    \"domain\": {\"name\": \"$OS_PROJECT_DOMAIN_NAME\"}
                }
            }
        }
    }")

TOKEN=$(echo "$AUTH_RESPONSE" | grep -i "X-Subject-Token:" | awk '{print $2}' | tr -d '\r')
BODY=$(echo "$AUTH_RESPONSE" | tail -1)
PROJECT_ID=$(echo "$BODY" | jq -r '.token.project.id')

if [ -z "$TOKEN" ] || [ "$TOKEN" == "null" ]; then
    log_error "Failed to get authentication token"
    exit 1
fi

log_success "Authenticated successfully"
log_info "Token: ${TOKEN:0:20}..."
log_info "Project ID: $PROJECT_ID"

# Create network for instance
log_test "Create test network"
NETWORK_RESPONSE=$(curl -s -X POST "http://localhost:9696/v2.0/networks" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{
        "network": {
            "name": "test-volume-network"
        }
    }')

NETWORK_ID=$(echo "$NETWORK_RESPONSE" | jq -r '.network.id')

if [ -z "$NETWORK_ID" ] || [ "$NETWORK_ID" == "null" ]; then
    log_error "Failed to create network"
    exit 1
fi

log_success "Created network: $NETWORK_ID"

# Create subnet
log_test "Create test subnet"
SUBNET_RESPONSE=$(curl -s -X POST "http://localhost:9696/v2.0/subnets" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d "{
        \"subnet\": {
            \"name\": \"test-volume-subnet\",
            \"network_id\": \"$NETWORK_ID\",
            \"cidr\": \"10.20.0.0/24\",
            \"ip_version\": 4,
            \"enable_dhcp\": false
        }
    }")

SUBNET_ID=$(echo "$SUBNET_RESPONSE" | jq -r '.subnet.id')

if [ -z "$SUBNET_ID" ] || [ "$SUBNET_ID" == "null" ]; then
    log_error "Failed to create subnet"
    exit 1
fi

log_success "Created subnet: $SUBNET_ID"

# Get flavor ID
log_test "Get flavor for instance"
FLAVOR_RESPONSE=$(curl -s -X GET "http://localhost:8774/v2.1/flavors" \
    -H "X-Auth-Token: $TOKEN")

FLAVOR_ID=$(echo "$FLAVOR_RESPONSE" | jq -r '.flavors[0].id')

if [ -z "$FLAVOR_ID" ] || [ "$FLAVOR_ID" == "null" ]; then
    log_error "No flavors found"
    exit 1
fi

log_success "Using flavor: $FLAVOR_ID"

# Create instance (without volume)
log_test "Create test instance"
INSTANCE_RESPONSE=$(curl -s -X POST "http://localhost:8774/v2.1/servers" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d "{
        \"server\": {
            \"name\": \"test-volume-instance\",
            \"flavorRef\": \"$FLAVOR_ID\",
            \"networks\": [{\"uuid\": \"$NETWORK_ID\"}]
        }
    }")

INSTANCE_ID=$(echo "$INSTANCE_RESPONSE" | jq -r '.server.id')

if [ -z "$INSTANCE_ID" ] || [ "$INSTANCE_ID" == "null" ]; then
    log_error "Failed to create instance"
    echo "Response: $INSTANCE_RESPONSE"
    exit 1
fi

log_success "Created instance: $INSTANCE_ID"
sleep 2

# Create volume
log_test "Create volume (10 GB)"
VOLUME_RESPONSE=$(curl -s -X POST "http://localhost:8776/v3/$PROJECT_ID/volumes" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{
        "volume": {
            "name": "test-volume-1",
            "size": 10,
            "description": "Test volume for attach/detach"
        }
    }')

VOLUME_ID=$(echo "$VOLUME_RESPONSE" | jq -r '.volume.id')

if [ -z "$VOLUME_ID" ] || [ "$VOLUME_ID" == "null" ]; then
    log_error "Failed to create volume"
    echo "Response: $VOLUME_RESPONSE"
    exit 1
fi

log_success "Created volume: $VOLUME_ID"
log_info "Waiting for volume to become available..."
sleep 5

# Verify volume status is available
log_test "Verify volume status is 'available'"
VOLUME_STATUS=$(curl -s -X GET "http://localhost:8776/v3/$PROJECT_ID/volumes/$VOLUME_ID" \
    -H "X-Auth-Token: $TOKEN" | jq -r '.volume.status')

if [ "$VOLUME_STATUS" != "available" ]; then
    log_error "Volume status is '$VOLUME_STATUS', expected 'available'"
else
    log_success "Volume status is 'available'"
fi

# List volume attachments (should be empty)
log_test "List volume attachments (should be empty)"
ATTACHMENTS_RESPONSE=$(curl -s -X GET "http://localhost:8774/v2.1/servers/$INSTANCE_ID/os-volume_attachments" \
    -H "X-Auth-Token: $TOKEN")

ATTACHMENTS_COUNT=$(echo "$ATTACHMENTS_RESPONSE" | jq '.volumeAttachments | length')

if [ "$ATTACHMENTS_COUNT" -eq 0 ]; then
    log_success "No attachments found (as expected)"
else
    log_error "Expected 0 attachments, found $ATTACHMENTS_COUNT"
fi

# Attach volume to instance
log_test "Attach volume to instance"
ATTACH_RESPONSE=$(curl -s -X POST "http://localhost:8774/v2.1/servers/$INSTANCE_ID/os-volume_attachments" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d "{
        \"volumeAttachment\": {
            \"volumeId\": \"$VOLUME_ID\"
        }
    }")

ATTACHMENT_ID=$(echo "$ATTACH_RESPONSE" | jq -r '.volumeAttachment.id')
DEVICE=$(echo "$ATTACH_RESPONSE" | jq -r '.volumeAttachment.device')

if [ -z "$ATTACHMENT_ID" ] || [ "$ATTACHMENT_ID" == "null" ]; then
    log_error "Failed to attach volume"
    echo "Response: $ATTACH_RESPONSE"
else
    log_success "Volume attached successfully"
    log_info "  Attachment ID: $ATTACHMENT_ID"
    log_info "  Device: $DEVICE"
fi

# Verify volume status changed to 'in-use'
log_test "Verify volume status changed to 'in-use'"
sleep 1
VOLUME_STATUS=$(curl -s -X GET "http://localhost:8776/v3/$PROJECT_ID/volumes/$VOLUME_ID" \
    -H "X-Auth-Token: $TOKEN" | jq -r '.volume.status')

if [ "$VOLUME_STATUS" != "in-use" ]; then
    log_error "Volume status is '$VOLUME_STATUS', expected 'in-use'"
else
    log_success "Volume status is 'in-use'"
fi

# List volume attachments (should show 1)
log_test "List volume attachments (should show 1)"
ATTACHMENTS_RESPONSE=$(curl -s -X GET "http://localhost:8774/v2.1/servers/$INSTANCE_ID/os-volume_attachments" \
    -H "X-Auth-Token: $TOKEN")

ATTACHMENTS_COUNT=$(echo "$ATTACHMENTS_RESPONSE" | jq '.volumeAttachments | length')

if [ "$ATTACHMENTS_COUNT" -eq 1 ]; then
    log_success "Found 1 attachment"
    ATTACHED_VOLUME_ID=$(echo "$ATTACHMENTS_RESPONSE" | jq -r '.volumeAttachments[0].volumeId')
    ATTACHED_DEVICE=$(echo "$ATTACHMENTS_RESPONSE" | jq -r '.volumeAttachments[0].device')
    log_info "  Volume ID: $ATTACHED_VOLUME_ID"
    log_info "  Device: $ATTACHED_DEVICE"
else
    log_error "Expected 1 attachment, found $ATTACHMENTS_COUNT"
fi

# Try to attach the same volume again (should fail)
log_test "Try to attach already-attached volume (should fail)"
ATTACH_FAIL_RESPONSE=$(curl -s -X POST "http://localhost:8774/v2.1/servers/$INSTANCE_ID/os-volume_attachments" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d "{
        \"volumeAttachment\": {
            \"volumeId\": \"$VOLUME_ID\"
        }
    }")

ERROR_MESSAGE=$(echo "$ATTACH_FAIL_RESPONSE" | jq -r '.error.message')

if [[ "$ERROR_MESSAGE" == *"already attached"* ]]; then
    log_success "Correctly rejected double-attach"
else
    log_error "Should have rejected already-attached volume"
    echo "Response: $ATTACH_FAIL_RESPONSE"
fi

# Create second volume
log_test "Create second volume"
VOLUME2_RESPONSE=$(curl -s -X POST "http://localhost:8776/v3/$PROJECT_ID/volumes" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{
        "volume": {
            "name": "test-volume-2",
            "size": 5,
            "description": "Second test volume"
        }
    }')

VOLUME2_ID=$(echo "$VOLUME2_RESPONSE" | jq -r '.volume.id')

if [ -z "$VOLUME2_ID" ] || [ "$VOLUME2_ID" == "null" ]; then
    log_error "Failed to create second volume"
else
    log_success "Created second volume: $VOLUME2_ID"
fi

sleep 2

# Attach second volume (device should be auto-assigned)
log_test "Attach second volume (auto-assign device)"
ATTACH2_RESPONSE=$(curl -s -X POST "http://localhost:8774/v2.1/servers/$INSTANCE_ID/os-volume_attachments" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d "{
        \"volumeAttachment\": {
            \"volumeId\": \"$VOLUME2_ID\"
        }
    }")

DEVICE2=$(echo "$ATTACH2_RESPONSE" | jq -r '.volumeAttachment.device')

if [ "$DEVICE2" == "/dev/vdc" ]; then
    log_success "Second volume attached to $DEVICE2 (auto-assigned correctly)"
else
    log_error "Expected device /dev/vdc, got $DEVICE2"
fi

# List attachments (should show 2)
log_test "List volume attachments (should show 2)"
ATTACHMENTS_RESPONSE=$(curl -s -X GET "http://localhost:8774/v2.1/servers/$INSTANCE_ID/os-volume_attachments" \
    -H "X-Auth-Token: $TOKEN")

ATTACHMENTS_COUNT=$(echo "$ATTACHMENTS_RESPONSE" | jq '.volumeAttachments | length')

if [ "$ATTACHMENTS_COUNT" -eq 2 ]; then
    log_success "Found 2 attachments"
else
    log_error "Expected 2 attachments, found $ATTACHMENTS_COUNT"
fi

# Detach first volume
log_test "Detach first volume"
DETACH_RESPONSE=$(curl -s -X DELETE "http://localhost:8774/v2.1/servers/$INSTANCE_ID/os-volume_attachments/$VOLUME_ID" \
    -H "X-Auth-Token: $TOKEN" \
    -w "\n%{http_code}")

HTTP_CODE=$(echo "$DETACH_RESPONSE" | tail -1)

if [ "$HTTP_CODE" == "202" ]; then
    log_success "Volume detached successfully"
else
    log_error "Failed to detach volume (HTTP $HTTP_CODE)"
fi

# Verify volume status changed back to 'available'
log_test "Verify volume status changed back to 'available'"
sleep 1
VOLUME_STATUS=$(curl -s -X GET "http://localhost:8776/v3/$PROJECT_ID/volumes/$VOLUME_ID" \
    -H "X-Auth-Token: $TOKEN" | jq -r '.volume.status')

if [ "$VOLUME_STATUS" != "available" ]; then
    log_error "Volume status is '$VOLUME_STATUS', expected 'available'"
else
    log_success "Volume status is 'available'"
fi

# List attachments (should show 1 remaining)
log_test "List volume attachments (should show 1 remaining)"
ATTACHMENTS_RESPONSE=$(curl -s -X GET "http://localhost:8774/v2.1/servers/$INSTANCE_ID/os-volume_attachments" \
    -H "X-Auth-Token: $TOKEN")

ATTACHMENTS_COUNT=$(echo "$ATTACHMENTS_RESPONSE" | jq '.volumeAttachments | length')

if [ "$ATTACHMENTS_COUNT" -eq 1 ]; then
    log_success "Found 1 remaining attachment"
    REMAINING_VOLUME=$(echo "$ATTACHMENTS_RESPONSE" | jq -r '.volumeAttachments[0].volumeId')
    if [ "$REMAINING_VOLUME" == "$VOLUME2_ID" ]; then
        log_info "  Correctly shows second volume still attached"
    else
        log_error "  Wrong volume remaining"
    fi
else
    log_error "Expected 1 attachment, found $ATTACHMENTS_COUNT"
fi

# Try to delete attached volume (should fail)
log_test "Try to delete attached volume (should fail)"
DELETE_FAIL_RESPONSE=$(curl -s -X DELETE "http://localhost:8776/v3/$PROJECT_ID/volumes/$VOLUME2_ID" \
    -H "X-Auth-Token: $TOKEN" \
    -w "\n%{http_code}")

HTTP_CODE=$(echo "$DELETE_FAIL_RESPONSE" | tail -1)

if [ "$HTTP_CODE" == "400" ]; then
    log_success "Correctly rejected deletion of attached volume"
else
    log_error "Should have rejected deletion (got HTTP $HTTP_CODE)"
fi

# Detach second volume
log_test "Detach second volume"
curl -s -X DELETE "http://localhost:8774/v2.1/servers/$INSTANCE_ID/os-volume_attachments/$VOLUME2_ID" \
    -H "X-Auth-Token: $TOKEN" > /dev/null

sleep 1
log_success "Second volume detached"

# Delete both volumes
log_test "Delete first volume"
curl -s -X DELETE "http://localhost:8776/v3/$PROJECT_ID/volumes/$VOLUME_ID" \
    -H "X-Auth-Token: $TOKEN" > /dev/null
log_success "First volume deleted"

log_test "Delete second volume"
curl -s -X DELETE "http://localhost:8776/v3/$PROJECT_ID/volumes/$VOLUME2_ID" \
    -H "X-Auth-Token: $TOKEN" > /dev/null
log_success "Second volume deleted"

# Print summary
echo ""
echo "========================================"
echo "        TEST SUMMARY"
echo "========================================"
echo -e "Total Tests:  $TOTAL_TESTS"
echo -e "${GREEN}Passed:       $TESTS_PASSED${NC}"
echo -e "${RED}Failed:       $TESTS_FAILED${NC}"
echo "========================================"

if [ $TESTS_FAILED -eq 0 ]; then
    echo -e "${GREEN}✅ ALL TESTS PASSED${NC}"
    exit 0
else
    echo -e "${RED}❌ SOME TESTS FAILED${NC}"
    exit 1
fi
