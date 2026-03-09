# SPEC-000: OpenStack API Compliance Framework

**Status**: Draft
**Version**: 1.0
**Created**: 2026-03-09
**Priority**: CRITICAL - PRIORITY ZERO (Supersedes all other specifications)

## Mission Statement

> **O3K must be 100% OpenStack API compliant. Consumers—whether Terraform providers, OpenStack CLI, SDKs, or Horizon dashboard—cannot distinguish O3K from canonical OpenStack.**

This is not a goal. This is a **requirement**. This is **non-negotiable**.

## Overview

This specification defines the compliance framework, testing methodology, validation criteria, and enforcement mechanisms that ensure O3K achieves and maintains 100% OpenStack API compatibility across all services.

**Scope**: All O3K services (Keystone, Nova, Neutron, Cinder, Glance, Barbican, Designate, Octavia) and all future services.

## Goals

1. **API Parity**: Every OpenStack API endpoint, parameter, response field matches specification
2. **Client Compatibility**: All OpenStack clients work without modification
3. **Terraform Provider Support**: terraform-provider-openstack works unchanged
4. **SDK Compatibility**: python-openstackclient, gophercloud, openstacksdk, etc. work unchanged
5. **Horizon Compatibility**: OpenStack Horizon dashboard works unmodified
6. **Behavioral Equivalence**: Error codes, status transitions, validation rules match OpenStack
7. **Version Support**: Microversion negotiation and version discovery work correctly

## Non-Goals

- Custom extensions beyond OpenStack APIs (may be added separately, but never break compatibility)
- Performance optimization that breaks API contracts
- "Close enough" compatibility (100% or nothing)

## The Compliance Principle

```
IF a request is valid for OpenStack
THEN it must be valid for O3K and produce equivalent results

IF a request is invalid for OpenStack
THEN it must be invalid for O3K and produce equivalent errors

IF OpenStack client/SDK/tool works with OpenStack
THEN it must work identically with O3K
```

**Zero tolerance for incompatibility.**

---

## OpenStack API Versions

O3K must support these OpenStack API versions:

| Service | API Version | Microversions | Priority |
|---------|-------------|---------------|----------|
| Keystone | v3.14 | N/A | Critical |
| Nova | v2.1 | 2.1 - 2.90 | Critical |
| Neutron | v2.0 | Extensions | Critical |
| Cinder | v3 | 3.0 - 3.70 | Critical |
| Glance | v2 | 2.0 - 2.18 | Critical |
| Barbican | v1 | N/A | High |
| Designate | v2 | N/A | High |
| Octavia | v2 | N/A | High |

### Version Discovery

All services MUST implement version discovery endpoints:

```bash
# Version discovery
GET /
GET /v3  # Keystone
GET /v2.1  # Nova
GET /v2.0  # Neutron
GET /v3  # Cinder
GET /v2  # Glance
GET /v1  # Barbican
GET /v2  # Designate
GET /v2  # Octavia
```

**Response format must match OpenStack exactly**, including:
- `versions` array structure
- `status` field (CURRENT, SUPPORTED, DEPRECATED)
- `min_version` / `max_version` for microversions
- `links` array with proper hrefs

---

## Compliance Validation Framework

### Level 1: API Contract Tests

**Definition**: Every API endpoint must have contract tests using official OpenStack clients.

**Implementation**:
```go
// test/compliance/keystone_v3_test.go
func TestKeystoneV3Compliance(t *testing.T) {
    // Use python-keystoneclient or gophercloud
    provider, err := openstack.AuthenticatedClient(openstack.AuthOptions{
        IdentityEndpoint: "http://localhost:35357/v3",
        Username:         "admin",
        Password:         "secret",
        DomainName:      "default",
    })
    require.NoError(t, err)

    // Test token structure
    token := provider.TokenID
    assert.NotEmpty(t, token)

    // Test service catalog structure
    catalog, err := tokens.Get(provider, token).ExtractServiceCatalog()
    require.NoError(t, err)
    assert.Contains(t, catalog.Entries, "nova")
    assert.Contains(t, catalog.Entries, "neutron")
}
```

**Requirements**:
- Every endpoint has at least one contract test
- Tests use **real** OpenStack clients (no custom test clients)
- Tests validate both success and error cases
- Tests validate response schemas

### Level 2: Terraform Provider Tests

**Definition**: terraform-provider-openstack must work unchanged against O3K.

**Test Suite**:
```hcl
# test/compliance/terraform/main.tf
terraform {
  required_providers {
    openstack = {
      source = "terraform-provider-openstack/openstack"
      version = "~> 1.54.1"  # Latest stable
    }
  }
}

provider "openstack" {
  auth_url    = "http://localhost:35357/v3"
  user_name   = "admin"
  password    = "secret"
  tenant_name = "default"
  domain_name = "default"
}

# Test compute instance
resource "openstack_compute_instance_v2" "test" {
  name            = "terraform-test"
  image_name      = "cirros"
  flavor_name     = "m1.small"
  security_groups = ["default"]

  network {
    name = "private"
  }
}

# Test network
resource "openstack_networking_network_v2" "test" {
  name           = "terraform-network"
  admin_state_up = true
}

# Test volume
resource "openstack_blockstorage_volume_v3" "test" {
  name = "terraform-volume"
  size = 10
}

# Test load balancer
resource "openstack_lb_loadbalancer_v2" "test" {
  name          = "terraform-lb"
  vip_subnet_id = openstack_networking_subnet_v2.test.id
}
```

**Validation**:
```bash
#!/bin/bash
# test/compliance/terraform_test.sh

# Run terraform plan
terraform init
terraform plan -out=plan.tfplan

# Apply
terraform apply plan.tfplan

# Validate resources created
terraform show

# Destroy
terraform destroy -auto-approve

# Exit code 0 = compliance pass
```

**Requirements**:
- All terraform-provider-openstack resources work
- terraform plan/apply/destroy succeed
- No custom workarounds needed
- State file compatible

### Level 3: OpenStack CLI Tests

**Definition**: python-openstackclient must work unchanged.

**Test Coverage**:
```bash
#!/bin/bash
# test/compliance/openstackclient_test.sh

# Install official OpenStack CLI
pip install python-openstackclient python-novaclient python-neutronclient \
    python-cinderclient python-glanceclient python-barbicanclient \
    python-designateclient python-octaviaclient

# Configure environment
export OS_AUTH_URL=http://localhost:35357/v3
export OS_USERNAME=admin
export OS_PASSWORD=secret
export OS_PROJECT_NAME=default
export OS_USER_DOMAIN_NAME=default
export OS_PROJECT_DOMAIN_NAME=default

# Test Keystone
openstack token issue
openstack user list
openstack project list
openstack role list

# Test Nova
openstack server create --flavor m1.small --image cirros test-vm
openstack server list
openstack server show test-vm
openstack server delete test-vm

# Test Neutron
openstack network create test-net
openstack subnet create --network test-net --subnet-range 10.0.0.0/24 test-subnet
openstack port create --network test-net test-port
openstack security group create test-sg
openstack security group rule create --ingress --protocol tcp test-sg

# Test Cinder
openstack volume create --size 10 test-vol
openstack volume list
openstack volume delete test-vol

# Test Glance
openstack image create --file cirros.img --disk-format qcow2 test-image
openstack image list
openstack image delete test-image

# Test Barbican
openstack secret store --name test-secret --payload "secret123"
openstack secret list
openstack secret delete test-secret

# Test Designate
openstack zone create --email admin@example.com example.com.
openstack recordset create example.com. www --type A --records 192.168.1.10
openstack zone delete example.com.

# Test Octavia
openstack loadbalancer create --name test-lb --vip-subnet-id <subnet-id>
openstack loadbalancer list
openstack loadbalancer delete test-lb

# ALL commands must succeed with exit code 0
```

**Requirements**:
- Every `openstack` command works
- Output format matches OpenStack
- JSON output parseable
- Table output readable
- Exit codes correct

### Level 4: SDK Compatibility Tests

**Definition**: All major OpenStack SDKs work unchanged.

**SDKs to Test**:

#### Python (openstacksdk)
```python
# test/compliance/sdk/python_test.py
import openstack

# Connect
conn = openstack.connect(
    auth_url='http://localhost:35357/v3',
    project_name='default',
    username='admin',
    password='secret',
    user_domain_name='default',
    project_domain_name='default',
)

# Test compute
server = conn.compute.create_server(
    name='sdk-test',
    image='cirros',
    flavor='m1.small',
)
assert server.id is not None

# Test network
network = conn.network.create_network(name='sdk-network')
assert network.id is not None

# Test volume
volume = conn.block_storage.create_volume(size=10, name='sdk-volume')
assert volume.id is not None
```

#### Go (gophercloud)
```go
// test/compliance/sdk/go_test.go
package compliance_test

import (
    "github.com/gophercloud/gophercloud"
    "github.com/gophercloud/gophercloud/openstack"
    "github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
)

func TestGophercloudCompliance(t *testing.T) {
    provider, err := openstack.AuthenticatedClient(gophercloud.AuthOptions{
        IdentityEndpoint: "http://localhost:35357/v3",
        Username:         "admin",
        Password:         "secret",
        DomainName:      "default",
        TenantName:      "default",
    })
    require.NoError(t, err)

    // Create server
    server, err := servers.Create(client, servers.CreateOpts{
        Name:      "gophercloud-test",
        ImageRef:  imageID,
        FlavorRef: flavorID,
    }).Extract()
    require.NoError(t, err)
    assert.NotEmpty(t, server.ID)
}
```

#### Java (openstack4j)
```java
// test/compliance/sdk/JavaTest.java
import org.openstack4j.api.OSClient.OSClientV3;
import org.openstack4j.openstack.OSFactory;

public class JavaComplianceTest {
    @Test
    public void testOpenStack4j() {
        OSClientV3 os = OSFactory.builderV3()
            .endpoint("http://localhost:35357/v3")
            .credentials("admin", "secret", Identifier.byName("default"))
            .scopeToProject(Identifier.byName("default"), Identifier.byName("default"))
            .authenticate();

        // Create server
        Server server = os.compute().servers()
            .boot(Builders.server()
                .name("java-test")
                .flavor("m1.small")
                .image(imageId)
                .build());

        assertNotNull(server.getId());
    }
}
```

**Requirements**:
- All SDK methods work
- Return types match expectations
- Error handling compatible
- Authentication flows work

### Level 5: Horizon Dashboard Tests

**Definition**: OpenStack Horizon 2025.2 must work unmodified.

**Test Workflow**:
```python
# test/compliance/horizon/selenium_test.py
from selenium import webdriver
from selenium.webdriver.common.by import By

def test_horizon_login():
    driver = webdriver.Chrome()
    driver.get("http://localhost/horizon")

    # Login
    driver.find_element(By.ID, "id_username").send_keys("admin")
    driver.find_element(By.ID, "id_password").send_keys("secret")
    driver.find_element(By.ID, "loginBtn").click()

    # Verify dashboard loaded
    assert "Overview" in driver.page_source

def test_horizon_instance_create():
    # Navigate to Instances
    driver.find_element(By.LINK_TEXT, "Instances").click()

    # Launch Instance
    driver.find_element(By.ID, "instances__action_launch-ng").click()

    # Fill form (follows Horizon workflow exactly)
    driver.find_element(By.ID, "id_name").send_keys("horizon-test")
    # ... select flavor, image, network

    # Launch
    driver.find_element(By.ID, "launch-instance-btn").click()

    # Verify instance appears in list
    assert "horizon-test" in driver.page_source
```

**Manual Test Checklist**:
- [ ] Login works
- [ ] Project overview shows metrics
- [ ] Instance create workflow works
- [ ] Instance actions (start, stop, reboot) work
- [ ] Network topology visualization renders
- [ ] Volume create/attach works
- [ ] Image upload works
- [ ] Security group rules can be added
- [ ] Floating IP assignment works
- [ ] Load balancer panel works (if Octavia enabled)
- [ ] DNS zones panel works (if Designate enabled)

**Requirements**:
- Zero JavaScript errors in browser console
- All API calls succeed
- No custom Horizon modifications needed
- Upstream Horizon 2025.2 Docker image works

---

## Response Schema Validation

Every API response must match OpenStack schema **exactly**.

### Schema Validation Framework

```go
// pkg/compliance/validator.go
package compliance

import (
    "encoding/json"
    "github.com/xeipuuv/gojsonschema"
)

type SchemaValidator struct {
    schemas map[string]*gojsonschema.Schema
}

func (v *SchemaValidator) ValidateResponse(apiPath, method string, response []byte) error {
    // Load OpenStack JSON schema for this endpoint
    schemaKey := fmt.Sprintf("%s:%s", method, apiPath)
    schema, exists := v.schemas[schemaKey]
    if !exists {
        return fmt.Errorf("no schema defined for %s", schemaKey)
    }

    // Validate
    docLoader := gojsonschema.NewBytesLoader(response)
    result, err := schema.Validate(docLoader)
    if err != nil {
        return fmt.Errorf("validation error: %w", err)
    }

    if !result.Valid() {
        var errs []string
        for _, desc := range result.Errors() {
            errs = append(errs, desc.String())
        }
        return fmt.Errorf("schema validation failed: %v", errs)
    }

    return nil
}
```

### OpenStack Schema Repository

Maintain schemas for all endpoints:

```
test/compliance/schemas/
├── keystone/
│   ├── v3/
│   │   ├── POST_auth_tokens_request.json
│   │   ├── POST_auth_tokens_response.json
│   │   ├── GET_users_response.json
│   │   └── ...
├── nova/
│   ├── v2.1/
│   │   ├── POST_servers_request.json
│   │   ├── POST_servers_response.json
│   │   ├── GET_servers_detail_response.json
│   │   └── ...
├── neutron/
│   ├── v2.0/
│   │   └── ...
└── ...
```

**Schema Source**: Extract from OpenStack official repositories or Tempest test suite.

---

## Error Response Compatibility

Error responses must match OpenStack format **exactly**.

### OpenStack Error Format

```json
{
  "error": {
    "message": "Invalid input for field/attribute name",
    "code": 400,
    "title": "Bad Request"
  }
}
```

**Or** (older format):
```json
{
  "badRequest": {
    "message": "Invalid input for field/attribute name",
    "code": 400
  }
}
```

### Error Code Mapping

| Scenario | HTTP Code | Error Type | OpenStack Behavior |
|----------|-----------|------------|-------------------|
| Resource not found | 404 | itemNotFound | `{"itemNotFound": {"message": "...", "code": 404}}` |
| Unauthorized | 401 | unauthorized | `{"error": {"message": "...", "code": 401}}` |
| Forbidden | 403 | forbidden | `{"forbidden": {"message": "...", "code": 403}}` |
| Bad request | 400 | badRequest | `{"badRequest": {"message": "...", "code": 400}}` |
| Conflict | 409 | conflict | `{"conflict": {"message": "...", "code": 409}}` |
| Service unavailable | 503 | serviceUnavailable | `{"serviceUnavailable": {"message": "...", "code": 503}}` |

**Critical**: Error message text should also match OpenStack patterns.

### Error Testing

```bash
# Test error responses
curl -X GET http://localhost:8774/v2.1/servers/nonexistent-id \
    -H "X-Auth-Token: $TOKEN" | jq .

# Expected (exactly):
{
  "itemNotFound": {
    "message": "Instance could not be found",
    "code": 404
  }
}
```

---

## Microversion Support (Nova)

Nova uses microversions for API evolution. O3K must implement microversion negotiation correctly.

### Microversion Headers

```
# Client requests specific version
X-OpenStack-Nova-API-Version: 2.79

# Server responds with actual version used
X-OpenStack-Nova-API-Version: 2.79

# Version discovery
GET /v2.1
{
  "versions": [{
    "id": "v2.1",
    "status": "CURRENT",
    "version": "2.90",
    "min_version": "2.1",
    "links": [...]
  }]
}
```

### Version-Specific Behavior

```go
func (s *NovaService) CreateServer(c *gin.Context) {
    requestedVersion := c.GetHeader("X-OpenStack-Nova-API-Version")
    if requestedVersion == "" {
        requestedVersion = c.GetHeader("OpenStack-API-Version")
    }

    // Parse version
    version := parseVersion(requestedVersion) // e.g., 2.79

    // Version-specific behavior
    var response map[string]interface{}
    if version >= 2.63 {
        // Include trusted_image_certificates in response
        response["server"]["trusted_image_certificates"] = []string{}
    }
    if version >= 2.75 {
        // Include availability_zone in server list
        response["server"]["availability_zone"] = "nova"
    }

    // Set response header
    c.Header("X-OpenStack-Nova-API-Version", fmt.Sprintf("%.2f", version))
    c.JSON(200, response)
}
```

**Requirements**:
- Min version: 2.1
- Max version: 2.90 (as of 2025.2 release)
- Version negotiation per OpenStack spec
- Unsupported version returns 406 Not Acceptable

---

## Compliance Test Automation

### CI/CD Pipeline

```yaml
# .github/workflows/compliance.yml
name: OpenStack API Compliance

on: [push, pull_request]

jobs:
  contract-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Start O3K services
        run: docker compose up -d
      - name: Run contract tests
        run: make test-compliance-contracts
      - name: Validate schemas
        run: make test-compliance-schemas

  terraform-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: hashicorp/setup-terraform@v2
      - name: Start O3K
        run: docker compose up -d
      - name: Run Terraform tests
        run: make test-compliance-terraform

  cli-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Install OpenStack CLI
        run: pip install python-openstackclient
      - name: Start O3K
        run: docker compose up -d
      - name: Run CLI tests
        run: make test-compliance-cli

  sdk-tests:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        sdk: [python, go, java]
    steps:
      - uses: actions/checkout@v3
      - name: Setup SDK (${{ matrix.sdk }})
        run: make setup-sdk-${{ matrix.sdk }}
      - name: Start O3K
        run: docker compose up -d
      - name: Run SDK tests
        run: make test-compliance-sdk-${{ matrix.sdk }}

  horizon-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Start O3K + Horizon
        run: docker compose -f docker-compose.yml -f docker-compose.horizon.yml up -d
      - name: Run Selenium tests
        run: make test-compliance-horizon
```

### Compliance Gates

**Merging to main branch requires**:
- [ ] All contract tests pass (100%)
- [ ] All Terraform tests pass (100%)
- [ ] All CLI tests pass (100%)
- [ ] All SDK tests pass (100%)
- [ ] Horizon manual smoke test passed
- [ ] Schema validation passes (100%)
- [ ] No regressions in error responses

**Zero failures allowed. No exceptions.**

---

## Compliance Enforcement

### Development Workflow

```
1. Feature Implementation
   ↓
2. Write Contract Test (using OpenStack client)
   ↓
3. Test FAILS (RED) - Expected!
   ↓
4. Implement Feature
   ↓
5. Test PASSES (GREEN)
   ↓
6. Validate against OpenStack schema
   ↓
7. Run full compliance suite
   ↓
8. IF all pass → Merge
   IF any fail → Fix and repeat
```

### Definition of Done

A feature is **NOT DONE** until:
- [ ] OpenStack client works
- [ ] Terraform provider works (if applicable)
- [ ] CLI command works
- [ ] Response schema validated
- [ ] Error responses validated
- [ ] Microversion handled (if Nova)
- [ ] Documentation matches OpenStack docs
- [ ] No custom workarounds required

### Compliance Monitoring

```bash
# Daily compliance report
make compliance-report

# Output:
# ====================================
# O3K OpenStack API Compliance Report
# ====================================
# Date: 2026-03-09
#
# Contract Tests:      524/524 PASS (100%)
# Terraform Tests:      47/47 PASS (100%)
# CLI Tests:           156/156 PASS (100%)
# SDK Tests (Python):   89/89 PASS (100%)
# SDK Tests (Go):       67/67 PASS (100%)
# Schema Validation:   412/412 PASS (100%)
#
# Overall Compliance: 100% ✅
#
# Ready for Production: YES
```

---

## Breaking Changes Policy

**NEVER break API compatibility. Period.**

If OpenStack makes a breaking change:
1. Support OLD behavior by default
2. Add NEW behavior with microversion flag
3. Maintain both until OpenStack deprecates old

If we discover an incompatibility:
1. Fix it immediately (P0 bug)
2. Backport fix to all affected versions
3. Update compliance tests to prevent regression

---

## Reference OpenStack Deployment

Maintain a reference OpenStack deployment for validation:

```yaml
# docker-compose.openstack.yml (for comparison testing)
services:
  openstack-devstack:
    image: openstack/devstack:2025.2
    # Full OpenStack deployment
    # Used for comparison testing
```

**Usage**:
```bash
# Test against real OpenStack
./test/compliance/compare.sh o3k openstack-devstack

# Output: Diff report of any behavioral differences
```

---

## Documentation Requirements

All API documentation must link to official OpenStack docs:

```markdown
# Nova API: Create Server

**OpenStack Reference**: https://docs.openstack.org/api-ref/compute/#create-server

O3K implements this endpoint with 100% compatibility.

## Request
...

## Response
...

## Example (using OpenStack CLI)
...
```

---

## Success Criteria

O3K achieves 100% OpenStack API compliance when:

- [x] **All compliance tests pass** (contract, Terraform, CLI, SDK, Horizon)
- [x] **Schema validation: 100%** (every response matches OpenStack schema)
- [x] **Error responses: 100%** (every error matches OpenStack format)
- [x] **Terraform provider works** (unchanged, all resources)
- [x] **OpenStack CLI works** (all commands, no modifications)
- [x] **All SDKs work** (Python, Go, Java unchanged)
- [x] **Horizon works** (2025.2 unmodified, zero console errors)
- [x] **Microversion support** (Nova 2.1-2.90)
- [x] **Consumer cannot tell the difference** (behavioral equivalence)

---

## Relationship to Other Specs

**This spec supersedes all other specifications.**

Every other spec (SPEC-001 through SPEC-005) must include:
- "Compliance with SPEC-000" section
- Contract tests using OpenStack clients
- Schema validation tests
- Error response validation
- Terraform provider tests (if applicable)

**All implementation work must prioritize API compliance over**:
- Performance optimizations
- Feature additions
- Internal refactoring
- New capabilities

**If a choice must be made**: Choose API compliance. Always.

---

## Appendix A: Test Coverage Requirements

Minimum test coverage per service:

| Service | Endpoints | Contract Tests | Terraform Tests | CLI Tests |
|---------|-----------|----------------|-----------------|-----------|
| Keystone | 20+ | 20+ | 5+ | 15+ |
| Nova | 50+ | 50+ | 10+ | 40+ |
| Neutron | 40+ | 40+ | 15+ | 30+ |
| Cinder | 25+ | 25+ | 5+ | 20+ |
| Glance | 15+ | 15+ | 3+ | 12+ |
| Barbican | 20+ | 20+ | 2+ | 10+ |
| Designate | 15+ | 15+ | 3+ | 8+ |
| Octavia | 30+ | 30+ | 5+ | 15+ |

**Total**: 215+ contract tests, 48+ Terraform tests, 150+ CLI tests

---

## Appendix B: OpenStack References

- [OpenStack API Reference](https://docs.openstack.org/api-ref/)
- [Keystone API v3](https://docs.openstack.org/api-ref/identity/v3/)
- [Nova API v2.1](https://docs.openstack.org/api-ref/compute/)
- [Neutron API v2.0](https://docs.openstack.org/api-ref/network/v2/)
- [Cinder API v3](https://docs.openstack.org/api-ref/block-storage/v3/)
- [Glance API v2](https://docs.openstack.org/api-ref/image/v2/)
- [Tempest Test Suite](https://docs.openstack.org/tempest/latest/)
- [terraform-provider-openstack](https://registry.terraform.io/providers/terraform-provider-openstack/openstack/latest/docs)
- [python-openstackclient](https://docs.openstack.org/python-openstackclient/latest/)

---

**Priority**: ZERO (Highest - supersedes all other work)
**Enforcement**: MANDATORY
**Exceptions**: NONE
**Negotiable**: NO

**This is the foundation. Everything builds on this.**
