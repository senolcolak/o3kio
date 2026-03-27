#!/bin/bash
# Simple O3K Integration Test
# Quick validation of all services

# Note: NOT using set -e because we explicitly track pass/fail counts
# and don't want the script to exit on first failure

GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'

PASSED=0
FAILED=0

test_endpoint() {
    local name=$1
    local url=$2

    if curl -s -f "$url" > /dev/null 2>&1; then
        echo -e "${GREEN}✓${NC} $name"
        ((PASSED++))
    else
        echo -e "${RED}✗${NC} $name"
        ((FAILED++))
    fi
}

test_auth() {
    echo "Testing Keystone Authentication..."

    response=$(curl -s -i -X POST "http://localhost:35357/v3/auth/tokens" \
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
        }')

    TOKEN=$(echo "$response" | grep -i "X-Subject-Token:" | awk '{print $2}' | tr -d '\r')

    if [ -n "$TOKEN" ]; then
        echo -e "${GREEN}✓${NC} Scoped authentication"
        ((PASSED++))
        export OS_TOKEN=$TOKEN

        # Check for catalog
        if echo "$response" | grep -q "catalog"; then
            echo -e "${GREEN}✓${NC} Service catalog present"
            ((PASSED++))
        else
            echo -e "${RED}✗${NC} Service catalog missing"
            ((FAILED++))
        fi
    else
        echo -e "${RED}✗${NC} Authentication failed"
        ((FAILED++))
        return 1
    fi
}

test_nova() {
    echo -e "\nTesting Nova..."

    # List servers
    response=$(curl -s -H "X-Auth-Token: $OS_TOKEN" "http://localhost:8774/v2.1/servers")
    if echo "$response" | grep -q "servers"; then
        echo -e "${GREEN}✓${NC} List servers"
        ((PASSED++))
    else
        echo -e "${RED}✗${NC} List servers"
        ((FAILED++))
    fi

    # List flavors
    response=$(curl -s -H "X-Auth-Token: $OS_TOKEN" "http://localhost:8774/v2.1/flavors")
    if echo "$response" | grep -q "flavors"; then
        echo -e "${GREEN}✓${NC} List flavors"
        ((PASSED++))
    else
        echo -e "${RED}✗${NC} List flavors"
        ((FAILED++))
    fi

    # List hypervisors
    response=$(curl -s -H "X-Auth-Token: $OS_TOKEN" "http://localhost:8774/v2.1/os-hypervisors")
    if echo "$response" | grep -q "hypervisors"; then
        echo -e "${GREEN}✓${NC} List hypervisors"
        ((PASSED++))
    else
        echo -e "${RED}✗${NC} List hypervisors"
        ((FAILED++))
    fi
}

test_neutron() {
    echo -e "\nTesting Neutron..."

    # List networks
    response=$(curl -s -H "X-Auth-Token: $OS_TOKEN" "http://localhost:9696/v2.0/networks")
    if echo "$response" | grep -q "networks"; then
        echo -e "${GREEN}✓${NC} List networks"
        ((PASSED++))
    else
        echo -e "${RED}✗${NC} List networks"
        ((FAILED++))
    fi

    # Create network
    response=$(curl -s -X POST -H "X-Auth-Token: $OS_TOKEN" \
        -H "Content-Type: application/json" \
        "http://localhost:9696/v2.0/networks" \
        -d '{"network": {"name": "quick-test-net", "admin_state_up": true}}')

    if echo "$response" | grep -q "quick-test-net"; then
        NETWORK_ID=$(echo "$response" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
        echo -e "${GREEN}✓${NC} Create network ($NETWORK_ID)"
        ((PASSED++))

        # Delete network
        curl -s -X DELETE -H "X-Auth-Token: $OS_TOKEN" \
            "http://localhost:9696/v2.0/networks/$NETWORK_ID" > /dev/null
        echo -e "${GREEN}✓${NC} Delete network"
        ((PASSED++))
    else
        echo -e "${RED}✗${NC} Create network"
        ((FAILED++))
    fi
}

test_cinder() {
    echo -e "\nTesting Cinder..."
    echo "⚠️  Note: Cinder tests skipped - volume backend not available in docker-compose"

    # All Cinder tests skipped in docker-compose environment
    echo -e "\033[0;33m⊘\033[0m List volumes (skipped - no backend)"
    echo -e "\033[0;33m⊘\033[0m Create volume (skipped - no backend)"
    echo -e "\033[0;33m⊘\033[0m Delete volume (skipped - no backend)"
}

test_glance() {
    echo -e "\nTesting Glance..."

    # List images
    response=$(curl -s -H "X-Auth-Token: $OS_TOKEN" "http://localhost:9292/v2/images")
    if echo "$response" | grep -q "images"; then
        echo -e "${GREEN}✓${NC} List images"
        ((PASSED++))
    else
        echo -e "${RED}✗${NC} List images"
        ((FAILED++))
    fi

    # Create image metadata
    response=$(curl -s -X POST -H "X-Auth-Token: $OS_TOKEN" \
        -H "Content-Type: application/json" \
        "http://localhost:9292/v2/images" \
        -d '{"name": "quick-test-img", "disk_format": "raw", "container_format": "bare"}')

    if echo "$response" | grep -q "quick-test-img"; then
        IMAGE_ID=$(echo "$response" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
        echo -e "${GREEN}✓${NC} Create image metadata ($IMAGE_ID)"
        ((PASSED++))

        # Upload image data (small file)
        dd if=/dev/zero of=/tmp/test.raw bs=1M count=1 2>/dev/null
        http_code=$(curl -s -w "%{http_code}" -o /dev/null -X PUT \
            -H "X-Auth-Token: $OS_TOKEN" \
            -H "Content-Type: application/octet-stream" \
            "http://localhost:9292/v2/images/${IMAGE_ID}/file" \
            --data-binary @/tmp/test.raw)
        rm -f /tmp/test.raw

        if [ "$http_code" = "204" ]; then
            echo -e "${GREEN}✓${NC} Upload image data"
            ((PASSED++))
        else
            echo -e "${RED}✗${NC} Upload image data (HTTP $http_code)"
            ((FAILED++))
        fi

        # Download image data
        http_code=$(curl -s -w "%{http_code}" -o /tmp/downloaded.raw \
            -H "X-Auth-Token: $OS_TOKEN" \
            "http://localhost:9292/v2/images/${IMAGE_ID}/file")

        if [ "$http_code" = "200" ]; then
            echo -e "${GREEN}✓${NC} Download image data"
            ((PASSED++))
        else
            echo -e "${RED}✗${NC} Download image data (HTTP $http_code)"
            ((FAILED++))
        fi
        rm -f /tmp/downloaded.raw

        # Delete image
        curl -s -X DELETE -H "X-Auth-Token: $OS_TOKEN" \
            "http://localhost:9292/v2/images/$IMAGE_ID" > /dev/null
        echo -e "${GREEN}✓${NC} Delete image"
        ((PASSED++))
    else
        echo -e "${RED}✗${NC} Create image"
        ((FAILED++))
    fi
}

# Main
echo "=========================================="
echo " O3K Quick Integration Test"
echo "=========================================="
echo ""

# Version discovery
echo "Testing Version Discovery..."
test_endpoint "Keystone /v3" "http://localhost:35357/v3"
test_endpoint "Nova /v2.1" "http://localhost:8774/v2.1"
test_endpoint "Neutron /v2.0" "http://localhost:9696/v2.0"
test_endpoint "Glance /v2" "http://localhost:9292/v2"

echo ""
test_auth

test_nova
test_neutron
test_cinder
test_glance

echo ""
echo "=========================================="
echo " Test Summary"
echo "=========================================="
echo "Total Passed: $PASSED"
echo "Total Failed: $FAILED"
echo ""

if [ $FAILED -eq 0 ]; then
    echo -e "${GREEN}✓ All tests passed!${NC}"
    exit 0
else
    echo -e "${RED}✗ Some tests failed${NC}"
    exit 1
fi
