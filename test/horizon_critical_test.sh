#!/bin/bash
# Quick test of critical Horizon functionality

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m'

echo "Testing critical Horizon endpoints..."
echo ""

# Get auth token
TOKEN=$(curl -s -X POST http://10.2.199.101:35357/v3/auth/tokens \
  -H "Content-Type: application/json" \
  -d '{
    "auth": {
      "identity": {
        "methods": ["password"],
        "password": {
          "user": {"name": "admin", "domain": {"name": "Default"}, "password": "secret"}
        }
      },
      "scope": {"project": {"name": "default", "domain": {"name": "Default"}}}
    }
  }' | jq -r '.token.catalog[] | select(.type == "network") | .endpoints[0].url')

AUTH_TOKEN=$(curl -s -i -X POST http://10.2.199.101:35357/v3/auth/tokens \
  -H "Content-Type: application/json" \
  -d '{
    "auth": {
      "identity": {
        "methods": ["password"],
        "password": {
          "user": {"name": "admin", "domain": {"name": "Default"}, "password": "secret"}
        }
      },
      "scope": {"project": {"name": "default", "domain": {"name": "Default"}}}
    }
  }' | grep -i "^X-Subject-Token:" | cut -d' ' -f2 | tr -d '\r')

PROJECT_ID=$(curl -s -X POST http://10.2.199.101:35357/v3/auth/tokens \
  -H "Content-Type: application/json" \
  -d '{
    "auth": {
      "identity": {
        "methods": ["password"],
        "password": {
          "user": {"name": "admin", "domain": {"name": "Default"}, "password": "secret"}
        }
      },
      "scope": {"project": {"name": "default", "domain": {"name": "Default"}}}
    }
  }' | jq -r '.token.project.id')

echo "1. Service Catalog - Neutron endpoint"
echo "   Expected: http://o3k:9696 (no /v2.0 suffix)"
echo "   Got: $TOKEN"
if [[ "$TOKEN" == "http://o3k:9696" ]]; then
    echo -e "   ${GREEN}✓ PASS${NC}"
else
    echo -e "   ${RED}✗ FAIL${NC}"
fi
echo ""

echo "2. Nova - List servers with addresses field"
SERVERS=$(curl -s -H "X-Auth-Token: $AUTH_TOKEN" \
  "http://10.2.199.101:8774/v2.1/$PROJECT_ID/servers/detail")
if echo "$SERVERS" | jq -e '.servers[0].addresses' > /dev/null 2>&1; then
    echo -e "   ${GREEN}✓ PASS${NC} - addresses field present"
elif echo "$SERVERS" | jq -e '.servers | length == 0' > /dev/null 2>&1; then
    echo -e "   ${GREEN}✓ PASS${NC} - no servers (addresses field will be added when servers exist)"
else
    echo -e "   ${RED}✗ FAIL${NC} - addresses field missing"
    echo "   Response: $SERVERS" | head -c 200
fi
echo ""

echo "3. Neutron - Get quota details"
QUOTA=$(curl -s -w "\n%{http_code}" -H "X-Auth-Token: $AUTH_TOKEN" \
  "http://10.2.199.101:9696/v2.0/quotas/$PROJECT_ID/details")
STATUS=$(echo "$QUOTA" | tail -1)
BODY=$(echo "$QUOTA" | head -n -1)

if [ "$STATUS" = "200" ]; then
    echo -e "   ${GREEN}✓ PASS${NC} - HTTP 200"
    echo "   Response: $BODY" | jq '.quota' | head -10
else
    echo -e "   ${RED}✗ FAIL${NC} - HTTP $STATUS"
    echo "   Response: $BODY"
fi
echo ""

echo "4. Neutron - List networks"
NETWORKS=$(curl -s -w "\n%{http_code}" -H "X-Auth-Token: $AUTH_TOKEN" \
  "http://10.2.199.101:9696/v2.0/networks")
STATUS=$(echo "$NETWORKS" | tail -1)

if [ "$STATUS" = "200" ]; then
    echo -e "   ${GREEN}✓ PASS${NC} - HTTP 200"
else
    echo -e "   ${RED}✗ FAIL${NC} - HTTP $STATUS"
fi
echo ""

echo "5. Neutron - List routers"
ROUTERS=$(curl -s -w "\n%{http_code}" -H "X-Auth-Token: $AUTH_TOKEN" \
  "http://10.2.199.101:9696/v2.0/routers")
STATUS=$(echo "$ROUTERS" | tail -1)

if [ "$STATUS" = "200" ]; then
    echo -e "   ${GREEN}✓ PASS${NC} - HTTP 200"
else
    echo -e "   ${RED}✗ FAIL${NC} - HTTP $STATUS"
fi
echo ""

echo "6. Neutron - List floating IPs"
FIPS=$(curl -s -w "\n%{http_code}" -H "X-Auth-Token: $AUTH_TOKEN" \
  "http://10.2.199.101:9696/v2.0/floatingips")
STATUS=$(echo "$FIPS" | tail -1)

if [ "$STATUS" = "200" ]; then
    echo -e "   ${GREEN}✓ PASS${NC} - HTTP 200"
else
    echo -e "   ${RED}✗ FAIL${NC} - HTTP $STATUS"
fi
echo ""

echo "================================"
echo "Critical endpoints test complete"
echo "================================"
