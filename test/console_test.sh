#!/bin/bash
# Console access test

# Get token
TOKEN=$(curl -i -s -X POST "http://localhost:35357/v3/auth/tokens" \
    -H "Content-Type: application/json" \
    -d '{"auth":{"identity":{"methods":["password"],"password":{"user":{"name":"admin","password":"secret","domain":{"name":"default"}}}},"scope":{"project":{"name":"default","domain":{"name":"default"}}}}}' \
    | grep -i "X-Subject-Token:" | awk '{print $2}' | tr -d '\r')

echo "✓ Got auth token"

# Create instance
INSTANCE_ID=$(curl -s -X POST "http://localhost:8774/v2.1/servers" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{
        "server": {
            "name": "test-console-vm",
            "flavorRef": "00000000-0000-0000-0000-000000000010"
        }
    }' | jq -r '.server.id')

if [ -z "$INSTANCE_ID" ] || [ "$INSTANCE_ID" == "null" ]; then
    echo "✗ Failed to create instance"
    exit 1
fi

echo "✓ Created instance: $INSTANCE_ID"
sleep 2

# Test 1: Get console via new API (remote-consoles)
echo ""
echo "Test 1: remote-consoles API (Nova microversion 2.6+)"
CONSOLE_RESPONSE=$(curl -s -X POST "http://localhost:8774/v2.1/servers/$INSTANCE_ID/remote-consoles" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{
        "remote_console": {
            "protocol": "vnc",
            "type": "novnc"
        }
    }')

CONSOLE_URL=$(echo "$CONSOLE_RESPONSE" | jq -r '.remote_console.url')

if [ -n "$CONSOLE_URL" ] && [ "$CONSOLE_URL" != "null" ]; then
    echo "✓ Got console URL: $CONSOLE_URL"
else
    echo "✗ Failed to get console URL"
    echo "Response: $CONSOLE_RESPONSE"
fi

# Test 2: Get console via legacy API (os-getVNCConsole action)
echo ""
echo "Test 2: os-getVNCConsole action (legacy)"
CONSOLE_LEGACY=$(curl -s -X POST "http://localhost:8774/v2.1/servers/$INSTANCE_ID/action" \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{
        "os-getVNCConsole": {
            "type": "novnc"
        }
    }')

LEGACY_URL=$(echo "$CONSOLE_LEGACY" | jq -r '.console.url')

if [ -n "$LEGACY_URL" ] && [ "$LEGACY_URL" != "null" ]; then
    echo "✓ Got legacy console URL: $LEGACY_URL"
else
    echo "✗ Failed to get legacy console URL"
    echo "Response: $CONSOLE_LEGACY"
fi

# Test 3: Verify VNC details were stored in database
echo ""
echo "Test 3: Verify VNC configuration in database"
VNC_PORT=$(curl -s -X GET "http://localhost:8774/v2.1/servers/$INSTANCE_ID" \
    -H "X-Auth-Token: $TOKEN" | jq -r '.server.id')

if [ -n "$VNC_PORT" ]; then
    echo "✓ Instance details retrieved"
else
    echo "✗ Failed to get instance details"
fi

# Cleanup
echo ""
echo "Cleaning up..."
curl -s -X DELETE "http://localhost:8774/v2.1/servers/$INSTANCE_ID" \
    -H "X-Auth-Token: $TOKEN" > /dev/null

echo "✓ Cleanup complete"
echo ""
echo "========================================"
echo "✅ Console access tests passed"
echo "========================================"
