#!/bin/bash
# Nova server actions test
# Tests rebuild, rescue, and createImage actions

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m' # No Color

# Base URLs
AUTH_URL="${OS_AUTH_URL:-http://localhost:5001/v3}"
NOVA_URL="${OS_COMPUTE_URL:-http://localhost:8774}"
PROJECT_ID="00000000-0000-0000-0000-000000000002"

echo "=== Nova Server Actions Test ==="

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

# Create test server
echo "Creating test server..."
SERVER_ID=$(curl -s -X POST "$NOVA_URL/v2.1/servers" \
  -H "X-Auth-Token: $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "server": {
      "name": "test-server-actions",
      "flavorRef": "00000000-0000-0000-0000-000000000010",
      "imageRef": "00000000-0000-0000-0000-000000000001"
    }
  }' | jq -r '.server.id')

if [ "$SERVER_ID" = "null" ] || [ -z "$SERVER_ID" ]; then
  echo -e "${RED}FAIL${NC}: Server creation failed"
  exit 1
fi
echo -e "${GREEN}OK${NC}: Server created: $SERVER_ID"

# Test 1: Rebuild server
echo "Test 1: Rebuild server"
RESPONSE=$(curl -s -X POST "$NOVA_URL/v2.1/servers/$SERVER_ID/action" \
  -H "X-Auth-Token: $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "rebuild": {
      "imageRef": "00000000-0000-0000-0000-000000000001",
      "name": "rebuilt-server"
    }
  }')

echo "$RESPONSE" | jq -e '.server.id' > /dev/null 2>&1
if [ $? -eq 0 ]; then
  echo -e "${GREEN}PASS${NC}: Rebuild server"
else
  echo -e "${RED}FAIL${NC}: Rebuild server - $RESPONSE"
fi

# Test 2: Rescue server
echo "Test 2: Rescue server"
curl -s -X POST "$NOVA_URL/v2.1/servers/$SERVER_ID/action" \
  -H "X-Auth-Token: $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"rescue": {"rescue_image_ref": "00000000-0000-0000-0000-000000000001"}}' \
  -w "%{http_code}" | grep -q "200"
if [ $? -eq 0 ]; then
  echo -e "${GREEN}PASS${NC}: Rescue server"
else
  echo -e "${RED}FAIL${NC}: Rescue server"
fi

# Test 3: Create image from server
echo "Test 3: Create image from server"
IMAGE_LOCATION=$(curl -s -i -X POST "$NOVA_URL/v2.1/servers/$SERVER_ID/action" \
  -H "X-Auth-Token: $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "createImage": {
      "name": "test-snapshot-image",
      "metadata": {
        "source": "server"
      }
    }
  }' | grep -i '^location' | cut -d' ' -f2 | tr -d '\r\n')

if [ -n "$IMAGE_LOCATION" ]; then
  echo -e "${GREEN}PASS${NC}: Create image (Location: $IMAGE_LOCATION)"
else
  echo -e "${RED}FAIL${NC}: Create image"
fi

# Cleanup
echo "Cleaning up..."
curl -s -X DELETE "$NOVA_URL/v2.1/servers/$SERVER_ID" \
  -H "X-Auth-Token: $TOKEN" > /dev/null
echo -e "${GREEN}OK${NC}: Cleanup complete"

echo "=== Test Summary ==="
echo "All Nova server action tests completed"
