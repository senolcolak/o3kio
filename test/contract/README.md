# Contract Tests

This directory contains **contract tests** for O3K's OpenStack API compliance. Contract tests verify that O3K implements the OpenStack API specification correctly by using real OpenStack client libraries (gophercloud).

## Purpose

Contract tests ensure **100% API compatibility** with OpenStack:
- Use official OpenStack SDKs (gophercloud for Go)
- Test actual API contracts (request/response format, error codes, behavior)
- Validate against OpenStack specifications, not internal implementation details

## TDD Workflow (Constitution Article III)

**MANDATORY**: All new endpoints MUST follow this test-first workflow:

1. **Write contract test** using gophercloud SDK (see `template_test.go`)
2. **Run test** → Confirm RED (fails)
3. **Get approval** from reviewer on test strategy
4. **Implement endpoint** in `internal/{service}/handlers.go`
5. **Run test** → Confirm GREEN (passes)
6. **Run full suite** → All tests still pass
7. **Update GAP_ANALYSIS.md** → Mark endpoint as ✅

## Running Contract Tests

### Prerequisites

O3K must be running:

```bash
# Start O3K with Docker Compose
docker compose up -d

# Or run directly
make run
```

Set environment variables:

```bash
# Source the environment file
source ~/.o3k-env

# Or set manually
export OS_AUTH_URL=http://localhost:35357/v3
export OS_USERNAME=admin
export OS_PASSWORD=secret
export OS_PROJECT_NAME=default
export OS_USER_DOMAIN_NAME=default
```

### Run All Contract Tests

```bash
# Run all contract tests
go test ./test/contract/... -v

# Run specific service tests
go test ./test/contract/keystone/... -v
go test ./test/contract/nova/... -v
go test ./test/contract/neutron/... -v
go test ./test/contract/cinder/... -v
go test ./test/contract/glance/... -v

# Run specific endpoint test
go test ./test/contract/keystone -run TestKeystoneCreateUser_Contract -v
```

### CI Integration

Contract tests are part of the CI pipeline:

```yaml
# .github/workflows/contract-tests.yml
- name: Run Contract Tests
  run: |
    docker compose up -d
    sleep 5  # Wait for O3K to start
    go test ./test/contract/... -v
```

## Writing Contract Tests

### Use the Template

Copy `template_test.go` and replace placeholders:

```go
package contract_test

import (
    "testing"
    "github.com/gophercloud/gophercloud/openstack/identity/v3/users"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestKeystoneCreateUser_Contract(t *testing.T) {
    SkipIfO3KNotRunning(t)

    client := SetupIdentityV3Client(t)

    // Test: Create user using gophercloud SDK
    user, err := users.Create(client, users.CreateOpts{
        Name:     "test-user-123",
        Password: "secret123",
    }).Extract()

    // Assertions: Verify OpenStack API contract
    require.NoError(t, err)
    assert.NotEmpty(t, user.ID)
    assert.Equal(t, "test-user-123", user.Name)

    // Cleanup
    defer users.Delete(client, user.ID)
}
```

### Key Principles

1. **Use gophercloud SDK** - Never use raw HTTP calls. Use `gophercloud/gophercloud/openstack/*` packages.
2. **Test OpenStack contract** - Verify response format, fields, error codes match OpenStack spec.
3. **Cleanup resources** - Use `defer` to delete created resources, even on test failure.
4. **Skip if not running** - Use `SkipIfO3KNotRunning(t)` to avoid false CI failures.
5. **Descriptive names** - Use `Test{Service}{Action}_Contract` naming (e.g., `TestKeystoneCreateUser_Contract`).

### Gophercloud Packages

- **Keystone**: `github.com/gophercloud/gophercloud/openstack/identity/v3/{users,projects,roles}`
- **Nova**: `github.com/gophercloud/gophercloud/openstack/compute/v2/{servers,flavors}`
- **Neutron**: `github.com/gophercloud/gophercloud/openstack/networking/v2/{networks,subnets,ports}`
- **Cinder**: `github.com/gophercloud/gophercloud/openstack/blockstorage/v3/{volumes,snapshots}`
- **Glance**: `github.com/gophercloud/gophercloud/openstack/imageservice/v2/{images,members}`

## Test Coverage Goals

- **Phase 1** (Weeks 1-8): 60+ contract tests (33% → 52% endpoint coverage)
- **Phase 2** (Weeks 9-16): 75+ contract tests (52% → 70% endpoint coverage)
- **Phase 3** (Weeks 17-24): 80+ contract tests (70% → 85% endpoint coverage)
- **Phase 4** (Weeks 25-32): 30+ contract tests (85% → 95%+ endpoint coverage)

**Total**: 245+ contract tests for 95%+ OpenStack API compliance.

## Validation Gates

Every PR that adds/modifies an endpoint must:
1. ✅ Include contract test using gophercloud
2. ✅ Confirm RED (test fails before implementation)
3. ✅ Confirm GREEN (test passes after implementation)
4. ✅ Pass schema validation
5. ✅ Pass integration tests (no regressions)

## Integration with TDD at Scale

Contract tests are **Tier 1** validation (per Implementation Plan):
- **Tier 1**: Contract tests (gophercloud) - mandatory per endpoint
- **Tier 2**: Integration tests (bash scripts) - batch validation per wave
- **Tier 3**: Schema validation (automated) - runs on all tests

Timeline per endpoint: ~2 hours (write test 30min, confirm RED 2min, review 10min, implement 60-90min, confirm GREEN 2min).

## Troubleshooting

**Test fails with "connection refused"**:
- Check O3K is running: `curl http://localhost:35357/v3`
- Check environment variables: `echo $OS_AUTH_URL`

**Test fails with "401 Unauthorized"**:
- Check credentials: `openstack token issue`
- Verify JWT secret matches in config

**Test fails with "404 Not Found"**:
- Expected for new endpoints (RED state)
- Implement endpoint, then re-run test (should turn GREEN)

**Test passes but real OpenStack SDK fails**:
- This indicates an API compatibility bug
- File an issue and add contract test to reproduce

## Resources

- OpenStack API Reference: https://docs.openstack.org/api-ref/
- Gophercloud Documentation: https://github.com/gophercloud/gophercloud
- SPEC-000 Compliance: `/Users/I761222/git/o3k/specs/000-compliance/SPEC.md`
- Implementation Plan: `/Users/I761222/git/o3k/DEVELOPMENT_PLAN_SUMMARY.md`
