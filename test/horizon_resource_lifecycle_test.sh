#!/bin/bash
# Integration test for complete resource lifecycle through Horizon-compatible APIs
# Tests: Instance, Network, Volume, Image lifecycle with cleanup

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

PASSED=0
FAILED=0

echo "=========================================="
echo " O3K Horizon Resource Lifecycle Test"
echo "=========================================="
echo ""

# Authenticate
echo -e "${YELLOW}[TEST]${NC} Authentication"
TOKEN=$(openstack token issue -f value -c id 2>/dev/null)
if [ -z "$TOKEN" ]; then
    echo -e "${RED}[FAIL]${NC} Authentication failed"
    exit 1
fi
echo -e "${GREEN}[PASS]${NC} Authenticated successfully"
((PASSED++))

# Get project ID
PROJECT_ID=$(openstack token issue -f json | jq -r '.project_id')
echo "  ℹ  Project ID: $PROJECT_ID"

echo ""
echo -e "${YELLOW}[TEST]${NC} Complete Resource Lifecycle"

# 1. Create Network
echo -e "${YELLOW}[STEP 1]${NC} Create network"
NETWORK_ID=$(openstack network create lifecycle-test-net -f value -c id 2>/dev/null)
if [ -z "$NETWORK_ID" ]; then
    echo -e "${RED}[FAIL]${NC} Network creation failed"
    ((FAILED++))
else
    echo -e "${GREEN}[PASS]${NC} Network created: $NETWORK_ID"
    ((PASSED++))
fi

# 2. Create Subnet
echo -e "${YELLOW}[STEP 2]${NC} Create subnet"
SUBNET_ID=$(openstack subnet create lifecycle-test-subnet \
    --network lifecycle-test-net \
    --subnet-range 192.168.200.0/24 \
    --gateway 192.168.200.1 \
    --dhcp \
    -f value -c id 2>/dev/null)
if [ -z "$SUBNET_ID" ]; then
    echo -e "${RED}[FAIL]${NC} Subnet creation failed"
    ((FAILED++))
else
    echo -e "${GREEN}[PASS]${NC} Subnet created: $SUBNET_ID"
    ((PASSED++))
fi

# 3. Create Volume
echo -e "${YELLOW}[STEP 3]${NC} Create volume"
VOLUME_ID=$(openstack volume create lifecycle-test-vol --size 1 -f value -c id 2>/dev/null)
if [ -z "$VOLUME_ID" ]; then
    echo -e "${RED}[FAIL]${NC} Volume creation failed"
    ((FAILED++))
else
    echo -e "${GREEN}[PASS]${NC} Volume created: $VOLUME_ID"
    ((PASSED++))
fi

# Wait for volume to be available
echo "  ℹ  Waiting for volume to be available..."
for i in {1..30}; do
    VOL_STATUS=$(openstack volume show $VOLUME_ID -f value -c status 2>/dev/null || echo "error")
    if [ "$VOL_STATUS" = "available" ]; then
        break
    fi
    if [ $i -eq 30 ]; then
        echo -e "${RED}[FAIL]${NC} Volume did not become available"
        ((FAILED++))
    fi
    sleep 1
done

# 4. Create Instance
echo -e "${YELLOW}[STEP 4]${NC} Create instance"
INSTANCE_ID=$(openstack server create lifecycle-test-vm \
    --flavor m1.small \
    --image 00000000-0000-0000-0000-000000000001 \
    --network lifecycle-test-net \
    -f value -c id 2>/dev/null)
if [ -z "$INSTANCE_ID" ]; then
    echo -e "${RED}[FAIL]${NC} Instance creation failed"
    ((FAILED++))
else
    echo -e "${GREEN}[PASS]${NC} Instance created: $INSTANCE_ID"
    ((PASSED++))
fi

# Wait for instance to be ACTIVE
echo "  ℹ  Waiting for instance to be ACTIVE..."
for i in {1..60}; do
    INSTANCE_STATUS=$(openstack server show $INSTANCE_ID -f value -c status 2>/dev/null || echo "error")
    if [ "$INSTANCE_STATUS" = "ACTIVE" ]; then
        break
    fi
    if [ $i -eq 60 ]; then
        echo -e "${RED}[FAIL]${NC} Instance did not become ACTIVE"
        ((FAILED++))
    fi
    sleep 1
done

# 5. Attach Volume to Instance
echo -e "${YELLOW}[STEP 5]${NC} Attach volume to instance"
openstack server add volume $INSTANCE_ID $VOLUME_ID 2>/dev/null
if [ $? -eq 0 ]; then
    echo -e "${GREEN}[PASS]${NC} Volume attached to instance"
    ((PASSED++))
else
    echo -e "${RED}[FAIL]${NC} Volume attach failed"
    ((FAILED++))
fi

# Wait for volume to be in-use
sleep 2
VOL_STATUS=$(openstack volume show $VOLUME_ID -f value -c status 2>/dev/null || echo "error")
if [ "$VOL_STATUS" = "in-use" ]; then
    echo -e "${GREEN}[PASS]${NC} Volume status is in-use"
    ((PASSED++))
else
    echo -e "${RED}[FAIL]${NC} Volume status is $VOL_STATUS (expected in-use)"
    ((FAILED++))
fi

# 6. Create Volume Snapshot
echo -e "${YELLOW}[STEP 6]${NC} Create volume snapshot"
SNAPSHOT_ID=$(openstack volume snapshot create lifecycle-test-snap --volume $VOLUME_ID -f value -c id 2>/dev/null)
if [ -z "$SNAPSHOT_ID" ]; then
    echo -e "${RED}[FAIL]${NC} Snapshot creation failed"
    ((FAILED++))
else
    echo -e "${GREEN}[PASS]${NC} Snapshot created: $SNAPSHOT_ID"
    ((PASSED++))
fi

# 7. Stop Instance
echo -e "${YELLOW}[STEP 7]${NC} Stop instance"
openstack server stop $INSTANCE_ID 2>/dev/null
sleep 2
INSTANCE_STATUS=$(openstack server show $INSTANCE_ID -f value -c status 2>/dev/null)
if [ "$INSTANCE_STATUS" = "SHUTOFF" ]; then
    echo -e "${GREEN}[PASS]${NC} Instance stopped"
    ((PASSED++))
else
    echo -e "${RED}[FAIL]${NC} Instance status is $INSTANCE_STATUS (expected SHUTOFF)"
    ((FAILED++))
fi

# 8. Start Instance
echo -e "${YELLOW}[STEP 8]${NC} Start instance"
openstack server start $INSTANCE_ID 2>/dev/null
sleep 3
INSTANCE_STATUS=$(openstack server show $INSTANCE_ID -f value -c status 2>/dev/null)
if [ "$INSTANCE_STATUS" = "ACTIVE" ]; then
    echo -e "${GREEN}[PASS]${NC} Instance started"
    ((PASSED++))
else
    echo -e "${RED}[FAIL]${NC} Instance status is $INSTANCE_STATUS (expected ACTIVE)"
    ((FAILED++))
fi

# 9. Create Instance Backup (Image)
echo -e "${YELLOW}[STEP 9]${NC} Create instance backup"
BACKUP_ID=$(openstack server image create $INSTANCE_ID --name lifecycle-test-backup -f value -c id 2>/dev/null)
if [ -z "$BACKUP_ID" ]; then
    echo -e "${RED}[FAIL]${NC} Backup creation failed"
    ((FAILED++))
else
    echo -e "${GREEN}[PASS]${NC} Backup image created: $BACKUP_ID"
    ((PASSED++))
fi

# 10. Cleanup - Detach Volume
echo -e "${YELLOW}[STEP 10]${NC} Detach volume"
openstack server remove volume $INSTANCE_ID $VOLUME_ID 2>/dev/null
sleep 2
VOL_STATUS=$(openstack volume show $VOLUME_ID -f value -c status 2>/dev/null || echo "error")
if [ "$VOL_STATUS" = "available" ]; then
    echo -e "${GREEN}[PASS]${NC} Volume detached (status: available)"
    ((PASSED++))
else
    echo -e "${RED}[FAIL]${NC} Volume status is $VOL_STATUS (expected available)"
    ((FAILED++))
fi

# 11. Delete Instance
echo -e "${YELLOW}[STEP 11]${NC} Delete instance"
openstack server delete $INSTANCE_ID --wait 2>/dev/null
if [ $? -eq 0 ]; then
    echo -e "${GREEN}[PASS]${NC} Instance deleted"
    ((PASSED++))
else
    echo -e "${RED}[FAIL]${NC} Instance deletion failed"
    ((FAILED++))
fi

# 12. Delete Volume Snapshot
echo -e "${YELLOW}[STEP 12]${NC} Delete snapshot"
openstack volume snapshot delete $SNAPSHOT_ID 2>/dev/null
if [ $? -eq 0 ]; then
    echo -e "${GREEN}[PASS]${NC} Snapshot deleted"
    ((PASSED++))
else
    echo -e "${RED}[FAIL]${NC} Snapshot deletion failed"
    ((FAILED++))
fi

# 13. Delete Volume
echo -e "${YELLOW}[STEP 13]${NC} Delete volume"
openstack volume delete $VOLUME_ID 2>/dev/null
if [ $? -eq 0 ]; then
    echo -e "${GREEN}[PASS]${NC} Volume deleted"
    ((PASSED++))
else
    echo -e "${RED}[FAIL]${NC} Volume deletion failed"
    ((FAILED++))
fi

# 14. Delete Backup Image
echo -e "${YELLOW}[STEP 14]${NC} Delete backup image"
openstack image delete $BACKUP_ID 2>/dev/null
if [ $? -eq 0 ]; then
    echo -e "${GREEN}[PASS]${NC} Backup image deleted"
    ((PASSED++))
else
    echo -e "${RED}[FAIL]${NC} Backup image deletion failed"
    ((FAILED++))
fi

# 15. Delete Network (will cascade delete subnet)
echo -e "${YELLOW}[STEP 15]${NC} Delete network"
openstack network delete $NETWORK_ID 2>/dev/null
if [ $? -eq 0 ]; then
    echo -e "${GREEN}[PASS]${NC} Network deleted (subnet cascade deleted)"
    ((PASSED++))
else
    echo -e "${RED}[FAIL]${NC} Network deletion failed"
    ((FAILED++))
fi

echo ""
echo "=========================================="
echo " Test Summary"
echo "=========================================="
echo "Total Passed: $PASSED"
echo "Total Failed: $FAILED"
echo ""

if [ $FAILED -eq 0 ]; then
    echo -e "${GREEN}✓ All lifecycle tests passed!${NC}"
    echo ""
    echo "Resource lifecycle validated:"
    echo "  - Network/Subnet: create → delete (cascade)"
    echo "  - Volume: create → attach → detach → snapshot → delete"
    echo "  - Instance: create → stop → start → backup → delete"
    echo "  - Image: backup created → delete"
    exit 0
else
    echo -e "${RED}✗ $FAILED test(s) failed${NC}"
    exit 1
fi
