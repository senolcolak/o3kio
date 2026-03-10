#!/bin/bash
# Glance image activation test
# Tests deactivating and reactivating images

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m' # No Color

# Base URLs
AUTH_URL="${OS_AUTH_URL:-http://localhost:5001/v3}"
GLANCE_URL="${OS_IMAGE_URL:-http://localhost:9292}"

echo "=== Glance Image Activation Test ==="

# Get auth token
echo "Authenticating..."
TOKEN=$(curl -s -i -X POST "$AUTH_URL/auth/tokens" \
  -H "Content-Type: application/json" \
  -d '{
    "auth": {
      "identity": {
        "methods": ["password"],
        "password": {
          "user": {
            "name": "admin",
            "domain": {"name": "Default"},
            "password": "secret"
          }
        }
      },
      "scope": {
        "project": {
          "name": "default",
          "domain": {"name": "Default"}
        }
      }
    }
  }' | grep -i '^x-subject-token' | cut -d' ' -f2 | tr -d '\r\n')

if [ -z "$TOKEN" ]; then
  echo -e "${RED}FAIL${NC}: Authentication failed"
  exit 1
fi
echo -e "${GREEN}OK${NC}: Authenticated"

# Create test image
echo "Creating test image..."
IMAGE_ID=$(curl -s -X POST "$GLANCE_URL/v2/images" \
  -H "X-Auth-Token: $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "test-image-activation",
    "container_format": "bare",
    "disk_format": "qcow2"
  }' | jq -r '.id')

if [ "$IMAGE_ID" = "null" ] || [ -z "$IMAGE_ID" ]; then
  echo -e "${RED}FAIL${NC}: Image creation failed"
  exit 1
fi
echo -e "${GREEN}OK${NC}: Image created: $IMAGE_ID"

# Verify initial status is active
STATUS=$(curl -s -X GET "$GLANCE_URL/v2/images/$IMAGE_ID" \
  -H "X-Auth-Token: $TOKEN" | jq -r '.status')
if [ "$STATUS" = "queued" ] || [ "$STATUS" = "active" ]; then
  echo -e "${GREEN}OK${NC}: Initial status: $STATUS"
else
  echo -e "${RED}FAIL${NC}: Unexpected initial status: $STATUS"
fi

# Test 1: Deactivate image
echo "Test 1: Deactivate image"
curl -s -X POST "$GLANCE_URL/v2/images/$IMAGE_ID/actions/deactivate" \
  -H "X-Auth-Token: $TOKEN" \
  -w "%{http_code}" | grep -q "204\|200"
if [ $? -eq 0 ]; then
  echo -e "${GREEN}PASS${NC}: Deactivate image"
else
  echo -e "${RED}FAIL${NC}: Deactivate image"
fi

# Verify status is deactivated
STATUS=$(curl -s -X GET "$GLANCE_URL/v2/images/$IMAGE_ID" \
  -H "X-Auth-Token: $TOKEN" | jq -r '.status')
if [ "$STATUS" = "deactivated" ]; then
  echo -e "${GREEN}PASS${NC}: Image status is deactivated"
else
  echo -e "${RED}FAIL${NC}: Expected deactivated status, got: $STATUS"
fi

# Test 2: Reactivate image
echo "Test 2: Reactivate image"
curl -s -X POST "$GLANCE_URL/v2/images/$IMAGE_ID/actions/reactivate" \
  -H "X-Auth-Token: $TOKEN" \
  -w "%{http_code}" | grep -q "204\|200"
if [ $? -eq 0 ]; then
  echo -e "${GREEN}PASS${NC}: Reactivate image"
else
  echo -e "${RED}FAIL${NC}: Reactivate image"
fi

# Verify status is back to active
STATUS=$(curl -s -X GET "$GLANCE_URL/v2/images/$IMAGE_ID" \
  -H "X-Auth-Token: $TOKEN" | jq -r '.status')
if [ "$STATUS" = "active" ] || [ "$STATUS" = "queued" ]; then
  echo -e "${GREEN}PASS${NC}: Image status is active/queued"
else
  echo -e "${RED}FAIL${NC}: Expected active status, got: $STATUS"
fi

# Cleanup
echo "Cleaning up..."
curl -s -X DELETE "$GLANCE_URL/v2/images/$IMAGE_ID" \
  -H "X-Auth-Token: $TOKEN" > /dev/null
echo -e "${GREEN}OK${NC}: Cleanup complete"

echo "=== Test Summary ==="
echo "All Glance image activation tests completed"
