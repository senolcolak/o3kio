#!/bin/bash
# O3K Validation Startup Script
# Run this script once Docker/OrbStack is running

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}╔══════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║           O3K Real-World Validation - Startup                 ║${NC}"
echo -e "${BLUE}╚══════════════════════════════════════════════════════════════╝${NC}"
echo ""

# Check Docker is running
echo -e "${YELLOW}[1/5]${NC} Checking Docker daemon..."
if ! docker ps &> /dev/null; then
    echo -e "${RED}✗ Docker is not running${NC}"
    echo ""
    echo "Please start Docker/OrbStack and run this script again"
    exit 1
fi
echo -e "${GREEN}✓ Docker is running${NC}"
echo ""

# Start O3K services
echo -e "${YELLOW}[2/5]${NC} Starting O3K services..."
docker compose down &> /dev/null || true
docker compose up -d --build
echo -e "${GREEN}✓ O3K services started${NC}"
echo ""

# Wait for O3K to be ready
echo -e "${YELLOW}[3/5]${NC} Waiting for O3K to be healthy..."
for i in {1..30}; do
    if curl -s -f http://localhost:35357/v3 &> /dev/null; then
        echo -e "${GREEN}✓ O3K is healthy${NC}"
        break
    fi
    if [ $i -eq 30 ]; then
        echo -e "${RED}✗ O3K failed to start${NC}"
        echo "Check logs with: docker logs o3k"
        exit 1
    fi
    sleep 2
    echo -n "."
done
echo ""

# Run validation suite
echo -e "${YELLOW}[4/5]${NC} Running CLI validation suite..."
echo ""
./test/validation_suite.sh | tee validation_results_$(date +%Y%m%d_%H%M%S).txt
VALIDATION_EXIT=$?
echo ""

# Start Horizon
echo -e "${YELLOW}[5/5]${NC} Starting Horizon dashboard..."
docker compose -f docker-compose.horizon.yml up -d
echo -e "${GREEN}✓ Horizon started${NC}"
echo ""

# Wait for Horizon to be ready
echo "Waiting for Horizon to be healthy..."
for i in {1..30}; do
    if curl -s -f http://localhost:8080/ &> /dev/null; then
        echo -e "${GREEN}✓ Horizon is healthy${NC}"
        break
    fi
    if [ $i -eq 30 ]; then
        echo -e "${YELLOW}⚠ Horizon may not be ready yet${NC}"
        echo "Check logs with: docker logs o3k-horizon"
    fi
    sleep 2
    echo -n "."
done
echo ""

# Summary
echo -e "${BLUE}╔══════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║                    VALIDATION READY                           ║${NC}"
echo -e "${BLUE}╚══════════════════════════════════════════════════════════════╝${NC}"
echo ""
echo -e "✓ O3K Services:     ${GREEN}http://localhost:35357/v3${NC} (Keystone)"
echo -e "                    http://localhost:8774/v2.1 (Nova)"
echo -e "                    http://localhost:9696/v2.0 (Neutron)"
echo -e "                    http://localhost:8776/v3 (Cinder)"
echo -e "                    http://localhost:9292/v2 (Glance)"
echo ""
echo -e "✓ Horizon UI:       ${GREEN}http://localhost:8080${NC}"
echo -e "  Login:            admin / secret"
echo ""
echo -e "✓ Validation Log:   validation_results_*.txt"
echo ""

if [ $VALIDATION_EXIT -eq 0 ]; then
    echo -e "${GREEN}✓ CLI Validation: ALL TESTS PASSED${NC}"
    echo ""
    echo -e "${BLUE}Next Steps:${NC}"
    echo "1. Open Horizon in browser: http://localhost:8080"
    echo "2. Login with: admin / secret"
    echo "3. Test these workflows:"
    echo "   - Launch an instance"
    echo "   - Create a network"
    echo "   - Create a volume"
    echo "   - Upload an image"
    echo "4. Document any errors (check browser console F12)"
else
    echo -e "${YELLOW}⚠ CLI Validation: SOME TESTS FAILED${NC}"
    echo ""
    echo -e "${BLUE}Next Steps:${NC}"
    echo "1. Review validation_results_*.txt"
    echo "2. Identify missing APIs"
    echo "3. Prioritize by impact"
    echo "4. Open Horizon (http://localhost:8080) to discover UI-specific gaps"
fi
echo ""
echo -e "${BLUE}To stop services:${NC}"
echo "  docker compose down"
echo "  docker compose -f docker-compose.horizon.yml down"
echo ""
