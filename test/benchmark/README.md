# O3K Performance Benchmarking Suite

Sprint 69 Performance Optimization validation suite with Go benchmarks and k6 load tests.

## Prerequisites

- **O3K running**: `docker compose -f deployments/docker-compose.yml up -d`
- **Redis enabled**: Set `cache.enabled: true` in `config/o3k.yaml`
- **k6 installed**: https://k6.io/docs/getting-started/installation/

## Quick Start

Run all benchmarks:
```bash
./test/benchmark/run_benchmarks.sh
```

## Go Benchmarks

### Cache Performance
```bash
go test -bench=BenchmarkCache -benchmem ./test/benchmark/
```

Tests Redis cache operations:
- `BenchmarkCacheGet`: Read performance (cache hits)
- `BenchmarkCacheSet`: Write performance
- `BenchmarkCacheMiss`: Miss performance
- `BenchmarkCacheHitVsMiss`: Hit vs miss comparison

### Database Performance
```bash
go test -bench=BenchmarkDatabase -benchmem ./test/benchmark/
```

Tests PostgreSQL query performance:
- `BenchmarkDatabaseQuery`: Simple queries
- `BenchmarkDatabaseJoinQuery`: JOIN query performance
- `BenchmarkConnectionPoolUtilization`: Concurrent load (10 vs 100 goroutines)
- `BenchmarkDatabaseInsert`: Write performance

## k6 Load Tests

### Keystone Authentication
```bash
k6 run test/load/keystone_auth.js
```

Target: **1000 req/s** (10x from 100 req/s baseline)
- Tests password authentication flow
- Measures service catalog generation time
- Validates JWT token issuance

### Nova Flavors
```bash
k6 run test/load/nova_flavors.js
```

Target: **500 req/s** (10x from 50 req/s baseline)
- Tests flavor list and detail endpoints
- Validates Redis cache effectiveness (24h TTL)

### Glance Images
```bash
k6 run test/load/glance_images.js
```

Target: **300 req/s** (10x from 30 req/s baseline)
- Tests image list and detail endpoints
- Validates Redis cache effectiveness (1h TTL)

### Neutron Networks
```bash
k6 run test/load/neutron_networks.js
```

Target: **400 req/s** baseline
- Tests network list and detail endpoints
- Validates Redis cache effectiveness (30min TTL)

## Sprint 69 Performance Targets

| Metric | Before | After | Target | Status |
|--------|--------|-------|--------|--------|
| Keystone auth | 100 req/s | ? | 1000 req/s | 🔄 |
| Nova flavors | 50 req/s | ? | 500 req/s | 🔄 |
| Glance images | 30 req/s | ? | 300 req/s | 🔄 |
| Query P95 latency | 50ms | ? | <5ms | 🔄 |
| Connection pool | 80% | ? | 95% | 🔄 |

## Custom Test Scenarios

### Environment Variables
```bash
export O3K_URL=http://localhost:8774
export KEYSTONE_URL=http://localhost:35357
k6 run test/load/nova_flavors.js
```

### Custom Load Profile
Edit `options.stages` in k6 scripts:
```javascript
export const options = {
  stages: [
    { duration: '1m', target: 100 },  // Custom ramp-up
    { duration: '5m', target: 100 },  // Sustained load
    { duration: '30s', target: 0 },   // Ramp-down
  ],
};
```

## Interpreting Results

### Go Benchmarks
```
BenchmarkCacheGet-8    1000000    1234 ns/op    128 B/op    2 allocs/op
```
- `1000000`: Number of iterations
- `1234 ns/op`: Time per operation (lower is better)
- `128 B/op`: Memory allocated per operation
- `2 allocs/op`: Number of allocations

### k6 Load Tests
Key metrics to watch:
- **http_req_duration (p95)**: 95th percentile latency (target: <5ms)
- **http_reqs**: Total requests per second
- **http_req_failed**: Failed request rate (target: <1%)
- **Custom metrics**: `auth_success_rate`, `catalog_size`, etc.

## Troubleshooting

**Cache benchmarks fail**:
```bash
# Verify Redis is running
redis-cli ping
# Should return: PONG
```

**Database benchmarks fail**:
```bash
# Verify PostgreSQL is accessible
pg_isready -h localhost -p 5432
# Should return: localhost:5432 - accepting connections
```

**k6 tests fail with 401**:
- Check O3K is running: `curl http://localhost:35357/v3`
- Verify credentials: admin/secret with default project

**Low throughput**:
- Ensure cache is enabled in config
- Check Redis cache hit rates
- Verify connection pool settings (50 max connections)

## Continuous Benchmarking

Add to CI/CD pipeline:
```bash
# Run smoke test (minimal load)
k6 run --vus 1 --duration 10s test/load/keystone_auth.js

# Fail if P95 > 10ms
k6 run --vus 100 --duration 1m \
  --threshold 'http_req_duration{p(95)}<10' \
  test/load/nova_flavors.js
```
