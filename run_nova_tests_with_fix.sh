#!/bin/bash
# Run Nova tests with hostname workaround
#
# This script requires sudo to add hostname to /etc/hosts
# Run with: ./run_nova_tests_with_fix.sh

set -e

echo "Adding o3k hostname to /etc/hosts..."
if ! grep -q "^127.0.0.1.*o3k" /etc/hosts; then
    echo "127.0.0.1 o3k" | sudo tee -a /etc/hosts
    echo "✓ Added o3k to /etc/hosts"
else
    echo "✓ o3k already in /etc/hosts"
fi

echo ""
echo "Running Nova lifecycle tests..."
cd /Users/I761222/git/o3k/test/contract/nova

export OS_AUTH_URL=http://localhost:35357/v3
export OS_USERNAME=admin
export OS_PASSWORD=secret
export OS_PROJECT_NAME=default
export OS_USER_DOMAIN_NAME=Default
export OS_PROJECT_DOMAIN_NAME=Default

echo ""
echo "Test 1: Server Create"
go test -v -run 'TestNovaServerCreate_Contract$' -count=1 -timeout=2m

echo ""
echo "Test 2: Server List"
go test -v -run 'TestNovaServerList_Contract$' -count=1 -timeout=2m

echo ""
echo "Test 3: Server Get"
go test -v -run 'TestNovaServerGet_Contract$' -count=1 -timeout=2m

echo ""
echo "Test 4: Server Delete"
go test -v -run 'TestNovaServerDelete_Contract$' -count=1 -timeout=2m

echo ""
echo "Test 5: Server Reboot"
go test -v -run 'TestNovaServerReboot_Contract$' -count=1 -timeout=2m

echo ""
echo "Test 6: Server Update"
go test -v -run 'TestNovaServerUpdate_Contract$' -count=1 -timeout=2m

echo ""
echo "Test 7: Server Lifecycle (Full)"
go test -v -run 'TestNovaServerLifecycle_Contract$' -count=1 -timeout=3m

echo ""
echo "=========================================="
echo "All Nova tests completed!"
echo "=========================================="
