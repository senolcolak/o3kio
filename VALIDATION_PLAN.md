# O3K Real-World Validation Plan
## Phase 2: Option B - Horizon & Real Tools Testing

### Overview
This document outlines the validation strategy for testing O3K with real OpenStack clients and tools to discover critical API gaps before continuing systematic API coverage.

---

## Testing Infrastructure Setup

### 1. Horizon Dashboard Integration

**Docker Setup Created:**
- `docker-compose.horizon.yml` - Horizon container configuration
- `config/horizon/local_settings.py` - Horizon configuration for O3K

**To Start Horizon:**
```bash
# Start O3K services
docker compose up -d

# Start Horizon dashboard
docker compose -f docker-compose.horizon.yml up -d

# Access Horizon at: http://localhost:8080
# Login: admin / secret
```

**Horizon Test Workflows:**
1. **Identity Management**
   - Login with admin user
   - Create new users
   - Create new projects
   - Assign roles

2. **Compute (Instances)**
   - Launch instance wizard
   - List instances
   - Instance actions (stop, start, reboot, delete)
   - Console access
   - Resize instance

3. **Networking**
   - Create network
   - Create subnet
   - Create router
   - Attach router interface
   - Create floating IP
   - Security groups

4. **Storage**
   - Create volume
   - Attach volume to instance
   - Create volume snapshot
   - Create volume from snapshot
   - Volume actions (extend, retype)

5. **Images**
   - List images
   - Upload image
   - Create instance from image
   - Share image (members)
   - Add tags to image

### 2. OpenStack CLI Comprehensive Testing

**Test Script Created:**
- `test/validation_suite.sh` - Comprehensive CLI validation

**To Run:**
```bash
# Ensure O3K is running
docker compose up -d

# Run validation suite
./test/validation_suite.sh
```

**Coverage:**
- ✓ Identity: users, projects, roles, tokens
- ✓ Compute: servers, flavors, actions
- ✓ Network: networks, subnets, ports
- ✓ Volume: volumes, snapshots, metadata
- ✓ Image: images, properties, tags

### 3. Terraform Provider Testing

**Test Plan:**
```hcl
# Create test-infrastructure.tf with:
# - Network + subnet
# - Security group with rules
# - Volume
# - Instance with attached volume
# - Floating IP assignment
```

**To Test:**
```bash
cd test/terraform
terraform init
terraform plan
terraform apply
terraform destroy
```

---

## Current Validation Status

### ✅ Phase 1 Complete (49 endpoints)
All basic CRUD operations and core workflows implemented and tested.

### 🔄 Phase 2 Ready
Validation infrastructure is in place. Next steps:

1. **Start Docker** and run O3K services
2. **Run validation_suite.sh** to get baseline
3. **Start Horizon** and test UI workflows
4. **Document gaps** discovered during testing
5. **Prioritize missing APIs** based on impact
6. **Implement critical gaps** (estimated 10-20 endpoints)
7. **Re-validate** until Horizon works smoothly

---

## Expected Gaps to Discover

### High Priority (Horizon Blockers)
These are likely to be discovered as missing:

**Compute:**
- [ ] GET /v2.1/os-hypervisors (hypervisor list)
- [ ] GET /v2.1/os-availability-zone (AZ list)
- [ ] GET /v2.1/limits (quota/limits)
- [ ] GET /v2.1/os-services (service list)
- [ ] GET /v2.1/os-keypairs (keypair list - may exist)

**Network:**
- [ ] GET /v2.0/extensions (API extensions)
- [ ] GET /v2.0/quotas (network quotas)
- [ ] POST /v2.0/routers (router CRUD - may partial)

**Volume:**
- [ ] GET /v3/{project_id}/limits (volume quotas)
- [ ] GET /v3/{project_id}/os-services (service status)

**Image:**
- [ ] GET /v2/schemas (other schema types)

**Identity:**
- [ ] GET /v3/domains (domain list)
- [ ] GET /v3/regions (region list)
- [ ] GET /v3/services (service catalog management)

### Medium Priority (CLI Enhancements)
- Volume backup/restore
- Network QoS policies
- Aggregate/host management
- Server groups
- Flavor extra specs

### Low Priority (Advanced Features)
- Heat orchestration (not implemented)
- Swift object storage (not implemented)
- Sahara data processing (not implemented)

---

## Validation Metrics

### Success Criteria
- ✅ **95%+ Horizon workflows functional** (some gaps acceptable)
- ✅ **100% CLI CRUD operations working** for core resources
- ✅ **80%+ Terraform provider compatibility** (common resources)

### Gap Tracking
Document discovered gaps in this format:

```markdown
## Gap: Hypervisor List API

**Discovery Method:** Horizon instance launch page
**Error:** 404 on GET /v2.1/os-hypervisors
**Impact:** HIGH - Horizon cannot show compute hosts
**Priority:** 1
**Estimated Effort:** 2 hours
**Implementation:** Add ListHypervisors handler to Nova
```

---

## Next Session Actions

1. **Start Services:**
   ```bash
   # Start Docker/OrbStack
   docker compose up -d
   docker compose -f docker-compose.horizon.yml up -d
   ```

2. **Run Validation:**
   ```bash
   ./test/validation_suite.sh | tee results.txt
   ```

3. **Test Horizon:**
   - Open http://localhost:8080
   - Login as admin/secret
   - Try each workflow
   - Document errors in browser console (F12)

4. **Analyze & Prioritize:**
   - Review validation results
   - Check Horizon error logs: `docker logs o3k-horizon`
   - Create gap list with priorities

5. **Implement Critical Gaps:**
   - Use same TDD workflow (RED → GREEN → commit)
   - Focus on HIGH priority Horizon blockers first
   - Target: 10-20 new endpoints in 2-3 days

---

## Timeline Estimate

**Week 1: Validation & Discovery**
- Day 1: Run all validation tests, document gaps
- Day 2: Prioritize gaps, create implementation plan

**Week 2: Gap Filling**
- Day 3-4: Implement high-priority endpoints (Horizon blockers)
- Day 5: Re-test Horizon, verify workflows work

**Week 3: Production Features** (if validation successful)
- Add metrics/monitoring
- Add health checks
- Improve error handling

---

## Documentation Links

- Horizon installation: https://docs.openstack.org/horizon/latest/install/
- OpenStack CLI: https://docs.openstack.org/python-openstackclient/latest/
- Terraform provider: https://registry.terraform.io/providers/terraform-provider-openstack/openstack/latest/docs

---

## Status: READY TO START VALIDATION

All infrastructure is in place. Next step: Start Docker and run validation suite.
