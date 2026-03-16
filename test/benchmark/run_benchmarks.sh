#!/bin/bash
# Performance benchmarking runner script for Sprint 69 validation
# Usage: ./test/benchmark/run_benchmarks.sh

set -e

echo "==================================================================="
echo "O3K Performance Benchmark Suite - Sprint 69 Validation"
echo "==================================================================="
echo ""

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Check prerequisites
echo "Checking prerequisites..."

# Check if O3K is running
if ! curl -s http://localhost:35357/v3 > /dev/null 2>&1; then
    echo -e "${RED}ERROR: O3K is not running on localhost:35357${NC}"
    echo "Start O3K with: docker compose -f deployments/docker-compose.yml up -d"
    exit 1
fi

# Check if Redis is running
if ! redis-cli ping > /dev/null 2>&1; then
    echo -e "${YELLOW}WARNING: Redis is not running. Cache benchmarks will be skipped.${NC}"
    REDIS_AVAILABLE=false
else
    echo -e "${GREEN}✓ Redis is running${NC}"
    REDIS_AVAILABLE=true
fi

# Check if PostgreSQL is running
if ! pg_isready -h localhost -p 5432 > /dev/null 2>&1; then
    echo -e "${YELLOW}WARNING: PostgreSQL is not accessible. Database benchmarks will be skipped.${NC}"
    POSTGRES_AVAILABLE=false
else
    echo -e "${GREEN}✓ PostgreSQL is running${NC}"
    POSTGRES_AVAILABLE=true
fi

# Check if k6 is installed
if ! command -v k6 &> /dev/null; then
    echo -e "${YELLOW}WARNING: k6 is not installed. Load tests will be skipped.${NC}"
    echo "Install k6: https://k6.io/docs/getting-started/installation/"
    K6_AVAILABLE=false
else
    echo -e "${GREEN}✓ k6 is installed${NC}"
    K6_AVAILABLE=true
fi

echo ""
echo "==================================================================="
echo "Running Go Benchmarks"
echo "==================================================================="
echo ""

# Run Go benchmarks
echo "Running cache benchmarks..."
if [ "$REDIS_AVAILABLE" = true ]; then
    go test -bench=BenchmarkCache -benchmem -benchtime=10s ./test/benchmark/ | tee benchmark_cache.txt
else
    echo -e "${YELLOW}Skipped (Redis not available)${NC}"
fi

echo ""
echo "Running database benchmarks..."
if [ "$POSTGRES_AVAILABLE" = true ]; then
    go test -bench=BenchmarkDatabase -benchmem -benchtime=10s ./test/benchmark/ | tee benchmark_database.txt
else
    echo -e "${YELLOW}Skipped (PostgreSQL not available)${NC}"
fi

echo ""
echo "==================================================================="
echo "Running k6 Load Tests"
echo "==================================================================="
echo ""

if [ "$K6_AVAILABLE" = true ]; then
    # Run k6 tests
    echo "Running Keystone authentication load test..."
    k6 run --summary-export=load_keystone.json test/load/keystone_auth.js

    echo ""
    echo "Running Nova flavors load test..."
    k6 run --summary-export=load_nova_flavors.json test/load/nova_flavors.js

    echo ""
    echo "Running Glance images load test..."
    k6 run --summary-export=load_glance_images.json test/load/glance_images.js

    echo ""
    echo "Running Neutron networks load test..."
    k6 run --summary-export=load_neutron_networks.json test/load/neutron_networks.js
else
    echo -e "${YELLOW}Skipped (k6 not available)${NC}"
fi

echo ""
echo "==================================================================="
echo "Benchmark Results Summary"
echo "==================================================================="
echo ""

# Parse and display key metrics
if [ "$K6_AVAILABLE" = true ] && [ -f load_keystone.json ]; then
    echo "Keystone Authentication:"
    echo "  - P95 latency: $(jq -r '.metrics.http_req_duration.values.["p(95)"]' load_keystone.json)ms"
    echo "  - Requests/sec: $(jq -r '.metrics.http_reqs.values.rate' load_keystone.json)"
    echo "  - Success rate: $(jq -r '.metrics.auth_success_rate.values.rate' load_keystone.json)"
    echo ""
fi

if [ "$K6_AVAILABLE" = true ] && [ -f load_nova_flavors.json ]; then
    echo "Nova Flavors:"
    echo "  - P95 latency: $(jq -r '.metrics.http_req_duration.values.["p(95)"]' load_nova_flavors.json)ms"
    echo "  - Requests/sec: $(jq -r '.metrics.http_reqs.values.rate' load_nova_flavors.json)"
    echo "  - Success rate: $(jq -r '.metrics.flavor_list_success_rate.values.rate' load_nova_flavors.json)"
    echo ""
fi

if [ "$K6_AVAILABLE" = true ] && [ -f load_glance_images.json ]; then
    echo "Glance Images:"
    echo "  - P95 latency: $(jq -r '.metrics.http_req_duration.values.["p(95)"]' load_glance_images.json)ms"
    echo "  - Requests/sec: $(jq -r '.metrics.http_reqs.values.rate' load_glance_images.json)"
    echo "  - Success rate: $(jq -r '.metrics.image_list_success_rate.values.rate' load_glance_images.json)"
    echo ""
fi

if [ "$K6_AVAILABLE" = true ] && [ -f load_neutron_networks.json ]; then
    echo "Neutron Networks:"
    echo "  - P95 latency: $(jq -r '.metrics.http_req_duration.values.["p(95)"]' load_neutron_networks.json)ms"
    echo "  - Requests/sec: $(jq -r '.metrics.http_reqs.values.rate' load_neutron_networks.json)"
    echo "  - Success rate: $(jq -r '.metrics.network_list_success_rate.values.rate' load_neutron_networks.json)"
    echo ""
fi

echo "==================================================================="
echo "Sprint 69 Target Validation"
echo "==================================================================="
echo ""
echo "Targets:"
echo "  - Keystone auth: 1000 req/s (from 100 req/s)"
echo "  - Nova flavors: 500 req/s (from 50 req/s)"
echo "  - Glance images: 300 req/s (from 30 req/s)"
echo "  - Query latency P95: <5ms (from 50ms)"
echo ""

if [ "$K6_AVAILABLE" = true ]; then
    # Validate targets
    KEYSTONE_RPS=$(jq -r '.metrics.http_reqs.values.rate' load_keystone.json 2>/dev/null || echo "0")
    NOVA_RPS=$(jq -r '.metrics.http_reqs.values.rate' load_nova_flavors.json 2>/dev/null || echo "0")
    GLANCE_RPS=$(jq -r '.metrics.http_reqs.values.rate' load_glance_images.json 2>/dev/null || echo "0")

    echo "Results:"
    if (( $(echo "$KEYSTONE_RPS > 1000" | bc -l) )); then
        echo -e "  - Keystone: ${GREEN}✓ PASS${NC} ($KEYSTONE_RPS req/s)"
    else
        echo -e "  - Keystone: ${RED}✗ FAIL${NC} ($KEYSTONE_RPS req/s)"
    fi

    if (( $(echo "$NOVA_RPS > 500" | bc -l) )); then
        echo -e "  - Nova: ${GREEN}✓ PASS${NC} ($NOVA_RPS req/s)"
    else
        echo -e "  - Nova: ${RED}✗ FAIL${NC} ($NOVA_RPS req/s)"
    fi

    if (( $(echo "$GLANCE_RPS > 300" | bc -l) )); then
        echo -e "  - Glance: ${GREEN}✓ PASS${NC} ($GLANCE_RPS req/s)"
    else
        echo -e "  - Glance: ${RED}✗ FAIL${NC} ($GLANCE_RPS req/s)"
    fi
fi

echo ""
echo "Benchmark complete! Results saved to:"
echo "  - Go benchmarks: benchmark_cache.txt, benchmark_database.txt"
echo "  - k6 load tests: load_*.json"
echo ""
