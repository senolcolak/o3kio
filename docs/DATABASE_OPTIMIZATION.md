# Database Optimization Guide

**O3K Version**: v0.4.1+
**Last Updated**: March 13, 2026

This guide covers database connection pooling, query optimization, and performance tuning for O3K deployments.

---

## Connection Pool Configuration

### Default Settings

```yaml
database:
  url: "postgres://user:pass@localhost/o3k?sslmode=disable"
  max_connections: 20        # Maximum concurrent connections
  min_connections: 2         # Minimum idle connections to maintain
  max_conn_lifetime: 1h      # Recycle connections after this duration
  max_conn_idle_time: 15m    # Close idle connections after this duration
  health_check_period: 1m    # How often to check connection health
```

### Recommended Settings by Deployment Size

#### Small Deployment (< 100 VMs)
```yaml
max_connections: 20
min_connections: 2
max_conn_lifetime: 1h
max_conn_idle_time: 15m
```

**Rationale**: Small deployments with occasional API calls don't need many connections. Keep overhead low.

#### Medium Deployment (100-1000 VMs)
```yaml
max_connections: 50
min_connections: 5
max_conn_lifetime: 30m
max_conn_idle_time: 10m
```

**Rationale**: More frequent operations require higher connection count. Recycle connections more frequently under load.

#### Large Deployment (> 1000 VMs)
```yaml
max_connections: 100
min_connections: 10
max_conn_lifetime: 15m
max_conn_idle_time: 5m
```

**Rationale**: High-traffic environments benefit from large pool. Aggressive recycling prevents connection leaks.

---

## Query Performance Monitoring

### Enable Slow Query Logging

O3K includes built-in slow query detection. Queries taking longer than a threshold are logged with full context.

**Usage in Code**:
```go
import "github.com/cobaltcore-dev/o3k/internal/database"

// Create query logger with 100ms threshold
queryLogger := database.NewQueryLogger(100 * time.Millisecond)

// Use instead of direct DB calls
rows, err := queryLogger.Query(ctx, "SELECT * FROM instances WHERE project_id = $1", projectID)
```

**Log Output** (when query is slow):
```json
{
  "level": "warn",
  "duration": 250,
  "query": "SELECT * FROM instances WHERE project_id = $1",
  "args": ["abc-123"],
  "msg": "Slow query detected"
}
```

### PostgreSQL Configuration

Enable PostgreSQL's slow query log for server-side analysis:

```ini
# postgresql.conf
log_min_duration_statement = 100  # Log queries taking > 100ms
log_statement = 'all'             # Log all statements (careful in production)
log_line_prefix = '%t [%p]: '     # Timestamp and process ID
```

---

## Common Query Patterns and Optimization

### 1. List Operations with Project Filtering

**Pattern**: Fetching resources for a project
```sql
SELECT * FROM instances WHERE project_id = $1
```

**Optimization**: Ensure index exists
```sql
CREATE INDEX idx_instances_project_id ON instances(project_id);
```

**O3K Status**: ✅ Index exists (migration 001)

### 2. Status Filtering

**Pattern**: Fetching active/available resources
```sql
SELECT * FROM volumes WHERE project_id = $1 AND status = 'available'
```

**Optimization**: Composite index for frequently combined filters
```sql
CREATE INDEX idx_volumes_project_status ON volumes(project_id, status);
```

**O3K Status**: 📝 Suggested (see Index Recommendations below)

### 3. Joins with Foreign Keys

**Pattern**: Fetching ports with network details
```sql
SELECT p.*, n.name
FROM ports p
JOIN networks n ON p.network_id = n.id
WHERE p.project_id = $1
```

**Optimization**: Index foreign key columns
```sql
CREATE INDEX idx_ports_network_id ON ports(network_id);
```

**O3K Status**: ✅ Index exists (migration 004)

### 4. Service Catalog Queries

**Pattern**: Building service catalog on every auth
```sql
SELECT s.id, s.type, s.name, e.id, e.interface, e.url, e.region
FROM services s
LEFT JOIN endpoints e ON s.id = e.service_id
WHERE s.enabled = true
ORDER BY s.type
```

**Optimization**:
- Add index on `services.enabled` and `services.type`
- Consider caching catalog in memory (rarely changes)

**O3K Status**: 🔧 Partial (enabled index exists, caching not implemented)

---

## Index Recommendations

### Automatically Detected Missing Indexes

Use the query analyzer to detect missing indexes:

```go
import "github.com/cobaltcore-dev/o3k/internal/database"

qa := database.NewQueryAnalyzer()
suggestions, err := qa.CheckMissingIndexes(ctx)

for _, suggestion := range suggestions {
    fmt.Printf("Table: %s, Columns: %v, Reason: %s\n",
        suggestion.Table, suggestion.Columns, suggestion.Reason)
}
```

### Common Suggestions

Based on typical O3K query patterns:

| Table | Columns | Reason |
|-------|---------|--------|
| instances | `(project_id, status)` | Frequently filtered by both |
| volumes | `(project_id, status)` | Frequently filtered by both |
| networks | `(project_id)` | Frequently filtered by project |
| ports | `(network_id)` | Frequently joined with networks |
| security_group_rules | `(security_group_id)` | Frequently joined with security groups |
| floating_ips | `(port_id)` | Frequently looked up by port |
| images | `(project_id, visibility)` | Frequently filtered by both |

### Creating Missing Indexes

If analyzer suggests missing indexes, create them via migration:

```sql
-- migrations/051_performance_indexes.up.sql
CREATE INDEX CONCURRENTLY idx_instances_project_status
ON instances(project_id, status);

CREATE INDEX CONCURRENTLY idx_volumes_project_status
ON volumes(project_id, status);

CREATE INDEX CONCURRENTLY idx_images_project_visibility
ON images(project_id, visibility);
```

**Note**: Use `CREATE INDEX CONCURRENTLY` to avoid locking tables during creation.

---

## Connection Pool Monitoring

### Check Pool Statistics

```go
import "github.com/cobaltcore-dev/o3k/internal/database"

stats := database.GetQueryStats()
fmt.Printf("Total connections: %v\n", stats["total_conns"])
fmt.Printf("Acquired connections: %v\n", stats["acquired_conns"])
fmt.Printf("Idle connections: %v\n", stats["idle_conns"])
fmt.Printf("Acquire duration: %v\n", stats["acquire_duration"])
```

### Key Metrics

- **total_conns**: Total connections in pool (should be ≤ max_connections)
- **acquired_conns**: Currently in-use connections
- **idle_conns**: Available connections (should be ≥ min_connections)
- **acquire_duration**: Average time to get a connection (should be < 10ms)
- **canceled_acquire**: Failed to get connection (should be 0)

### Health Check

```go
import "github.com/cobaltcore-dev/o3k/internal/database"

if err := database.HealthCheck(ctx); err != nil {
    log.Error().Err(err).Msg("Database health check failed")
}
```

---

## Performance Tuning Checklist

### Database Server (PostgreSQL)

- [ ] **shared_buffers**: Set to 25% of RAM (e.g., 4GB for 16GB server)
- [ ] **effective_cache_size**: Set to 50-75% of RAM
- [ ] **work_mem**: Set based on concurrent connections (RAM / max_connections / 2)
- [ ] **maintenance_work_mem**: Set to 256MB-1GB for faster index creation
- [ ] **checkpoint_timeout**: Increase to 15-30min for write-heavy workloads
- [ ] **max_connections**: Set to sum of all O3K instances' max_connections + buffer

### O3K Application

- [ ] Connection pool sized appropriately (20-100 depending on load)
- [ ] Slow query logging enabled (threshold: 100ms)
- [ ] Indexes exist for frequent query patterns
- [ ] Service catalog caching implemented (future optimization)
- [ ] Health checks running periodically

### Network

- [ ] Database and O3K on same subnet (low latency)
- [ ] Connection pooling enabled (not opening new connection per query)
- [ ] Keep-alive enabled on database connections

---

## Troubleshooting

### "Too many connections" Error

**Symptom**: PostgreSQL rejects new connections

**Solutions**:
1. Increase PostgreSQL `max_connections`:
   ```ini
   max_connections = 200  # In postgresql.conf
   ```
2. Reduce O3K `max_connections` per instance
3. Scale horizontally (more O3K instances with load balancer)

### High Connection Acquisition Time

**Symptom**: `acquire_duration` > 50ms in pool stats

**Solutions**:
1. Increase `max_connections` in O3K config
2. Reduce `max_conn_idle_time` to recycle faster
3. Check for connection leaks (queries not closing rows)
4. Check database server CPU/memory usage

### Slow Queries

**Symptom**: Frequent "Slow query detected" warnings

**Solutions**:
1. Run `EXPLAIN ANALYZE` on the query:
   ```go
   plan, _ := queryAnalyzer.Explain(ctx, sql, args...)
   fmt.Println(plan)
   ```
2. Add missing indexes
3. Optimize query (avoid `SELECT *`, use specific columns)
4. Consider query result caching

### Connection Pool Exhaustion

**Symptom**: `canceled_acquire` > 0 in pool stats

**Solutions**:
1. Increase `max_connections`
2. Check for connection leaks (not calling `rows.Close()`)
3. Reduce query duration (add indexes, optimize queries)
4. Scale horizontally

---

## Best Practices

### 1. Always Use Connection Pooling

❌ **Bad**: Opening new connection per request
```go
conn, _ := pgx.Connect(ctx, dbURL)
defer conn.Close()
```

✅ **Good**: Using shared pool
```go
rows, err := database.DB.Query(ctx, sql, args...)
defer rows.Close()
```

### 2. Always Close Rows

❌ **Bad**: Forgetting to close
```go
rows, _ := DB.Query(ctx, sql)
// Missing rows.Close()
```

✅ **Good**: Using defer
```go
rows, err := DB.Query(ctx, sql)
if err != nil {
    return err
}
defer rows.Close()
```

### 3. Use Prepared Statements

❌ **Bad**: String interpolation (SQL injection risk)
```go
sql := fmt.Sprintf("SELECT * FROM instances WHERE id = '%s'", instanceID)
```

✅ **Good**: Parameterized queries
```go
sql := "SELECT * FROM instances WHERE id = $1"
rows, err := DB.Query(ctx, sql, instanceID)
```

### 4. Limit Result Sets

❌ **Bad**: Fetching all rows
```go
SELECT * FROM instances WHERE project_id = $1
```

✅ **Good**: Using LIMIT and OFFSET
```go
SELECT * FROM instances WHERE project_id = $1
ORDER BY created_at DESC
LIMIT 100 OFFSET 0
```

### 5. Use Composite Indexes

❌ **Bad**: Separate indexes
```sql
CREATE INDEX idx_instances_project ON instances(project_id);
CREATE INDEX idx_instances_status ON instances(status);
```

✅ **Good**: Composite index (when filtered together)
```sql
CREATE INDEX idx_instances_project_status ON instances(project_id, status);
```

---

## Future Optimizations (Roadmap)

### v0.5.0
- [ ] Service catalog caching (in-memory, invalidate on update)
- [ ] Query result caching (Redis-backed)
- [ ] Read replicas support (route SELECT to replicas)
- [ ] Connection pool per service (isolate Nova from Neutron)

### v0.6.0
- [ ] Database sharding (by project_id)
- [ ] Time-series data optimization (separate tables for metrics)
- [ ] Materialized views for complex aggregations
- [ ] Query plan caching

---

## References

- [PostgreSQL Performance Tuning](https://wiki.postgresql.org/wiki/Tuning_Your_PostgreSQL_Server)
- [pgx Connection Pooling](https://github.com/jackc/pgx/wiki/Connection-pooling)
- [OpenStack Database Performance](https://docs.openstack.org/operations-guide/ops-lay-of-the-land.html#database)

---

**Maintainer**: O3K Development Team
**License**: Apache License 2.0
