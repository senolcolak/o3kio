#!/bin/bash
# Cinder metadata endpoints test
# Tests volume and snapshot metadata operations

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m' # No Color

# Base URLs
AUTH_URL="${OS_AUTH_URL:-http://localhost:5001/v3}"
CINDER_URL="${OS_VOLUME_URL:-http://localhost:8776}"
PROJECT_ID="00000000-0000-0000-0000-000000000002"

echo "=== Cinder Metadata Test ==="

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

# Create test volume
echo "Creating test volume..."
VOLUME_ID=$(curl -s -X POST "$CINDER_URL/v3/$PROJECT_ID/volumes" \
  -H "X-Auth-Token: $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"volume":{"size":1,"name":"test-metadata-volume"}}' \
  | jq -r '.volume.id')

if [ "$VOLUME_ID" = "null" ] || [ -z "$VOLUME_ID" ]; then
  echo -e "${RED}FAIL${NC}: Volume creation failed"
  exit 1
fi
echo -e "${GREEN}OK${NC}: Volume created: $VOLUME_ID"

# Test 1: Set volume metadata
echo "Test 1: Set volume metadata"
curl -s -X POST "$CINDER_URL/v3/$PROJECT_ID/volumes/$VOLUME_ID/metadata" \
  -H "X-Auth-Token: $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"metadata":{"key1":"value1","key2":"value2"}}' \
  | jq -e '.metadata.key1 == "value1"' > /dev/null
if [ $? -eq 0 ]; then
  echo -e "${GREEN}PASS${NC}: Set volume metadata"
else
  echo -e "${RED}FAIL${NC}: Set volume metadata"
fi

# Test 2: Get volume metadata
echo "Test 2: Get volume metadata"
curl -s -X GET "$CINDER_URL/v3/$PROJECT_ID/volumes/$VOLUME_ID/metadata" \
  -H "X-Auth-Token: $TOKEN" \
  | jq -e '.metadata.key1 == "value1" and .metadata.key2 == "value2"' > /dev/null
if [ $? -eq 0 ]; then
  echo -e "${GREEN}PASS${NC}: Get volume metadata"
else
  echo -e "${RED}FAIL${NC}: Get volume metadata"
fi

# Test 3: Get single metadata key
echo "Test 3: Get single metadata key"
curl -s -X GET "$CINDER_URL/v3/$PROJECT_ID/volumes/$VOLUME_ID/metadata/key1" \
  -H "X-Auth-Token: $TOKEN" \
  | jq -e '.meta.key1 == "value1"' > /dev/null
if [ $? -eq 0 ]; then
  echo -e "${GREEN}PASS${NC}: Get single metadata key"
else
  echo -e "${RED}FAIL${NC}: Get single metadata key"
fi

# Test 4: Update metadata key
echo "Test 4: Update metadata key"
curl -s -X PUT "$CINDER_URL/v3/$PROJECT_ID/volumes/$VOLUME_ID/metadata/key1" \
  -H "X-Auth-Token: $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"meta":{"key1":"updated_value"}}' \
  | jq -e '.meta.key1 == "updated_value"' > /dev/null
if [ $? -eq 0 ]; then
  echo -e "${GREEN}PASS${NC}: Update metadata key"
else
  echo -e "${RED}FAIL${NC}: Update metadata key"
fi

# Test 5: Delete metadata key
echo "Test 5: Delete metadata key"
curl -s -X DELETE "$CINDER_URL/v3/$PROJECT_ID/volumes/$VOLUME_ID/metadata/key2" \
  -H "X-Auth-Token: $TOKEN" \
  -w "%{http_code}" | grep -q "204\|200"
if [ $? -eq 0 ]; then
  echo -e "${GREEN}PASS${NC}: Delete metadata key"
else
  echo -e "${RED}FAIL${NC}: Delete metadata key"
fi

# Verify key2 is deleted
curl -s -X GET "$CINDER_URL/v3/$PROJECT_ID/volumes/$VOLUME_ID/metadata" \
  -H "X-Auth-Token: $TOKEN" \
  | jq -e '.metadata | has("key2") | not' > /dev/null
if [ $? -eq 0 ]; then
  echo -e "${GREEN}PASS${NC}: Metadata key deleted successfully"
else
  echo -e "${RED}FAIL${NC}: Metadata key not deleted"
fi

# Cleanup
echo "Cleaning up..."
curl -s -X DELETE "$CINDER_URL/v3/$PROJECT_ID/volumes/$VOLUME_ID" \
  -H "X-Auth-Token: $TOKEN" > /dev/null
echo -e "${GREEN}OK${NC}: Cleanup complete"

echo "=== Test Summary ==="
echo "All Cinder metadata tests completed"
