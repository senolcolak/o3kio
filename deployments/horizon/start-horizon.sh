#!/bin/bash
# Quick setup script for Horizon Dashboard with O3K

set -e

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo "=========================================="
echo "  O3K Horizon Dashboard Setup"
echo "=========================================="
echo ""

# Check if O3K is running
echo -e "${YELLOW}[1/5]${NC} Checking O3K status..."
if curl -s http://localhost:35357/v3 > /dev/null 2>&1; then
    echo -e "${GREEN}✓${NC} O3K is running"
else
    echo -e "${RED}✗${NC} O3K is not running on localhost:35357"
    echo ""
    echo "Please start O3K first:"
    echo "  cd /Users/I761222/git/lightstack"
    echo "  ./o3k --config config/o3k.yaml"
    echo ""
    exit 1
fi

# Check Docker
echo -e "${YELLOW}[2/5]${NC} Checking Docker..."
if command -v docker &> /dev/null; then
    echo -e "${GREEN}✓${NC} Docker is installed"
else
    echo -e "${RED}✗${NC} Docker is not installed"
    echo "Please install Docker from https://www.docker.com/get-started"
    exit 1
fi

# Check Docker Compose
echo -e "${YELLOW}[3/5]${NC} Checking Docker Compose..."
if docker compose version &> /dev/null; then
    echo -e "${GREEN}✓${NC} Docker Compose is installed"
elif command -v docker-compose &> /dev/null; then
    echo -e "${GREEN}✓${NC} Docker Compose is installed (legacy)"
    COMPOSE_CMD="docker-compose"
else
    echo -e "${RED}✗${NC} Docker Compose is not installed"
    exit 1
fi

# Use appropriate compose command
COMPOSE_CMD="${COMPOSE_CMD:-docker compose}"

# Start Horizon
echo -e "${YELLOW}[4/5]${NC} Starting Horizon Dashboard..."
cd "$(dirname "$0")"

$COMPOSE_CMD up -d

echo ""
echo -e "${YELLOW}[5/5]${NC} Waiting for Horizon to be ready..."
echo "This may take 30-60 seconds on first start..."

# Wait for Horizon to be healthy
MAX_ATTEMPTS=60
ATTEMPT=0
while [ $ATTEMPT -lt $MAX_ATTEMPTS ]; do
    if curl -sf http://localhost/dashboard/ > /dev/null 2>&1; then
        echo -e "${GREEN}✓${NC} Horizon is ready!"
        break
    fi
    ATTEMPT=$((ATTEMPT + 1))
    echo -n "."
    sleep 2
done

if [ $ATTEMPT -eq $MAX_ATTEMPTS ]; then
    echo ""
    echo -e "${RED}✗${NC} Horizon did not start in time"
    echo ""
    echo "Check logs with:"
    echo "  $COMPOSE_CMD logs horizon"
    exit 1
fi

echo ""
echo "=========================================="
echo "  Horizon Dashboard Ready!"
echo "=========================================="
echo ""
echo -e "${GREEN}Dashboard URL:${NC} http://localhost/dashboard"
echo ""
echo -e "${GREEN}Login Credentials:${NC}"
echo "  Domain:   Default"
echo "  Username: admin"
echo "  Password: secret"
echo ""
echo -e "${GREEN}Quick Commands:${NC}"
echo "  View logs:    $COMPOSE_CMD logs -f horizon"
echo "  Restart:      $COMPOSE_CMD restart horizon"
echo "  Stop:         $COMPOSE_CMD down"
echo ""
echo "Opening browser..."

# Try to open browser (works on macOS and Linux)
if command -v open &> /dev/null; then
    open "http://localhost/dashboard"
elif command -v xdg-open &> /dev/null; then
    xdg-open "http://localhost/dashboard"
else
    echo "Please open http://localhost/dashboard in your browser"
fi

echo ""
echo "Happy cloud computing! 🚀"
