# Horizon Dashboard Issues - Fix Summary

## Date: 2026-03-18

## Issues Fixed

### 1. Nova Instances Page - AttributeError: addresses
**Symptom**: Instances page showed "Something went wrong!" error
**Root Cause**: Nova API was returning `addresses: null` in server responses, causing `AttributeError` when Horizon tried to iterate over `instance.addresses.items()`
**Fix**: Added `getInstanceAddresses()` helper function to query ports table and build proper addresses dictionary
- Modified `GetServer()` handler in `internal/nova/handlers.go`
- Modified `ListServersDetail()` handler
- Addresses now returned as: `{"addresses": {"network-name": [{"addr": "10.0.0.5", "version": 4, "OS-EXT-IPS:type": "fixed"}]}}`

**Files Changed**:
- `internal/nova/handlers.go` (lines 661, 566, added getInstanceAddresses function)

### 2. Neutron Endpoint - Double /v2.0/ in URLs
**Symptom**: Network topology and other Neutron endpoints failing with 404
**Root Cause**: Service catalog had endpoint URL as `http://o3k:9696/v2.0`, but neutronclient automatically adds `/v2.0/` prefix, resulting in `/v2.0/v2.0/...` double prefix
**Fix**: Updated Neutron endpoint in database to `http://o3k:9696` (without `/v2.0` suffix)

**SQL Fix**:
```sql
UPDATE endpoints SET url = 'http://o3k:9696' WHERE url LIKE '%:9696/v2.0';
```

**Verification**:
```bash
curl -s -X POST http://10.2.199.101:35357/v3/auth/tokens ... | jq '.token.catalog[] | select(.type == "network")'
# Should show: {"url": "http://o3k:9696"} (no /v2.0 suffix)
```

### 3. Added Debug Logging
**Added**: Request logging middleware to Neutron service for troubleshooting
**Files Changed**: `internal/neutron/network.go`

## Remaining Issues

### Network Topology Page - 500 Internal Server Error
**Status**: Not resolved yet
**Symptoms**:
- HTTP 500 error when accessing `/dashboard/project/network_topology/`
- No requests reaching O3K backend
- No Python tracebacks in Apache error.log

**Possible Causes**:
1. Horizon unable to resolve `o3k` hostname (Docker service name)
2. Python exception in Horizon before API calls are made
3. Cached session data in Memcached (already flushed with `flush_all`)
4. Missing or misconfigured Neutron extensions

### Instances Page - "Unable to connect to Neutron"
**Status**: Not resolved yet
**Note**: This might be related to the network topology issue

## Configuration Changes Needed

### Service Catalog Endpoint Format
All OpenStack endpoints should NOT include the API version in the base URL:
- ✅ Correct: `http://o3k:9696` (neutronclient adds `/v2.0/`)
- ❌ Wrong: `http://o3k:9696/v2.0` (results in double prefix)

This applies to:
- Neutron: `http://o3k:9696` (not `/v2.0`)
- Nova: `http://o3k:8774/v2.1/{project_id}` (version in path is ok here)
- Glance: `http://o3k:9292` (not `/v2`)
- Cinder: `http://o3k:8776/v3/{project_id}` (version in path is ok here)

## Testing Commands

```bash
# Clear Memcached cache
docker exec o3k-memcached sh -c "echo 'flush_all' | nc localhost 11211"

# Check Neutron endpoint in service catalog
docker exec o3k-postgres psql -U o3k -d o3k -c "SELECT name, type, url FROM endpoints JOIN services ON endpoints.service_id = services.id WHERE type = 'network';"

# Test quota details endpoint directly
TOKEN=$(openstack token issue -f value -c id)
PROJECT_ID=$(openstack token issue -f value -c project_id)
curl -H "X-Auth-Token: $TOKEN" http://10.2.199.101:9696/v2.0/quotas/$PROJECT_ID/details

# Monitor O3K logs for incoming requests
docker logs o3k --follow | grep -E "(GET|POST|PUT|DELETE)"
```

## Next Steps

1. Check if issue persists after clean redeploy
2. If error persists, capture full 500 error response HTML from browser DevTools
3. Enable Django debug output in Horizon to see Python tracebacks
4. Check if `o3k` hostname resolves correctly from Horizon container
5. Verify all service endpoints in catalog match recommended format

## Commit Reference

Commit: f41ef1d
Message: "fix(horizon): fix Neutron endpoint and Nova addresses field"
