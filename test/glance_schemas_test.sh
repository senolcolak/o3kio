#!/bin/bash
# Glance schema endpoints test
# Tests member and members schema endpoints

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m' # No Color

# Base URLs
AUTH_URL="${OS_AUTH_URL:-http://localhost:5001/v3}"
GLANCE_URL="${OS_IMAGE_URL:-http://localhost:9292}"

echo "=== Glance Schema Endpoints Test ==="

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

# Test 1: GET /v2/schemas/member
echo "Test 1: GET member schema"
RESPONSE=$(curl -s -X GET "$GLANCE_URL/v2/schemas/member" \
  -H "X-Auth-Token: $TOKEN")

echo "$RESPONSE" | jq -e '.name == "member"' > /dev/null 2>&1
if [ $? -eq 0 ]; then
  echo -e "${GREEN}PASS${NC}: Member schema has correct name"
else
  echo -e "${RED}FAIL${NC}: Member schema name incorrect"
fi

echo "$RESPONSE" | jq -e '.properties' > /dev/null 2>&1
if [ $? -eq 0 ]; then
  echo -e "${GREEN}PASS${NC}: Member schema has properties"
else
  echo -e "${RED}FAIL${NC}: Member schema missing properties"
fi

# Test 2: GET /v2/schemas/members
echo "Test 2: GET members schema"
RESPONSE=$(curl -s -X GET "$GLANCE_URL/v2/schemas/members" \
  -H "X-Auth-Token: $TOKEN")

echo "$RESPONSE" | jq -e '.name == "members"' > /dev/null 2>&1
if [ $? -eq 0 ]; then
  echo -e "${GREEN}PASS${NC}: Members schema has correct name"
else
  echo -e "${RED}FAIL${NC}: Members schema name incorrect"
fi

echo "$RESPONSE" | jq -e '.properties' > /dev/null 2>&1
if [ $? -eq 0 ]; then
  echo -e "${GREEN}PASS${NC}: Members schema has properties"
else
  echo -e "${RED}FAIL${NC}: Members schema missing properties"
fi

echo "=== Test Summary ==="
echo "All Glance schema tests completed"
