#!/bin/bash
# Glance image tags test
# Tests adding and removing tags from images

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m' # No Color

# Base URLs
AUTH_URL="${OS_AUTH_URL:-http://localhost:5001/v3}"
GLANCE_URL="${OS_IMAGE_URL:-http://localhost:9292}"

echo "=== Glance Image Tags Test ==="

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
    "name": "test-image-tags",
    "container_format": "bare",
    "disk_format": "qcow2"
  }' | jq -r '.id')

if [ "$IMAGE_ID" = "null" ] || [ -z "$IMAGE_ID" ]; then
  echo -e "${RED}FAIL${NC}: Image creation failed"
  exit 1
fi
echo -e "${GREEN}OK${NC}: Image created: $IMAGE_ID"

# Test 1: Add tag to image
echo "Test 1: Add tag to image"
curl -s -X PUT "$GLANCE_URL/v2/images/$IMAGE_ID/tags/production" \
  -H "X-Auth-Token: $TOKEN" \
  -w "%{http_code}" | grep -q "204\|200"
if [ $? -eq 0 ]; then
  echo -e "${GREEN}PASS${NC}: Add tag"
else
  echo -e "${RED}FAIL${NC}: Add tag"
fi

# Verify tag was added
curl -s -X GET "$GLANCE_URL/v2/images/$IMAGE_ID" \
  -H "X-Auth-Token: $TOKEN" \
  | jq -e '.tags[] | select(. == "production")' > /dev/null
if [ $? -eq 0 ]; then
  echo -e "${GREEN}PASS${NC}: Tag appears in image"
else
  echo -e "${RED}FAIL${NC}: Tag not found in image"
fi

# Test 2: Add another tag
echo "Test 2: Add second tag"
curl -s -X PUT "$GLANCE_URL/v2/images/$IMAGE_ID/tags/stable" \
  -H "X-Auth-Token: $TOKEN" \
  -w "%{http_code}" | grep -q "204\|200"
if [ $? -eq 0 ]; then
  echo -e "${GREEN}PASS${NC}: Add second tag"
else
  echo -e "${RED}FAIL${NC}: Add second tag"
fi

# Verify both tags exist
TAGS=$(curl -s -X GET "$GLANCE_URL/v2/images/$IMAGE_ID" \
  -H "X-Auth-Token: $TOKEN" \
  | jq -r '.tags | length')
if [ "$TAGS" = "2" ]; then
  echo -e "${GREEN}PASS${NC}: Image has 2 tags"
else
  echo -e "${RED}FAIL${NC}: Expected 2 tags, got $TAGS"
fi

# Test 3: Delete tag
echo "Test 3: Delete tag"
curl -s -X DELETE "$GLANCE_URL/v2/images/$IMAGE_ID/tags/production" \
  -H "X-Auth-Token: $TOKEN" \
  -w "%{http_code}" | grep -q "204\|200"
if [ $? -eq 0 ]; then
  echo -e "${GREEN}PASS${NC}: Delete tag"
else
  echo -e "${RED}FAIL${NC}: Delete tag"
fi

# Verify tag was deleted
TAGS=$(curl -s -X GET "$GLANCE_URL/v2/images/$IMAGE_ID" \
  -H "X-Auth-Token: $TOKEN" \
  | jq -r '.tags | length')
if [ "$TAGS" = "1" ]; then
  echo -e "${GREEN}PASS${NC}: Tag deleted successfully (1 tag remaining)"
else
  echo -e "${RED}FAIL${NC}: Expected 1 tag, got $TAGS"
fi

# Cleanup
echo "Cleaning up..."
curl -s -X DELETE "$GLANCE_URL/v2/images/$IMAGE_ID" \
  -H "X-Auth-Token: $TOKEN" > /dev/null
echo -e "${GREEN}OK${NC}: Cleanup complete"

echo "=== Test Summary ==="
echo "All Glance image tags tests completed"
