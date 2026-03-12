# O3K Validation Quick Reference

## 🚀 Starting Validation

```bash
# When Docker is ready, run:
./start_validation.sh

# This will:
# 1. Start O3K services
# 2. Run CLI validation suite
# 3. Start Horizon dashboard
# 4. Generate results report
```

## 📊 Access Points

| Service | URL | Credentials |
|---------|-----|-------------|
| **Horizon UI** | http://localhost:8080 | admin / secret |
| Keystone API | http://localhost:35357/v3 | - |
| Nova API | http://localhost:8774/v2.1 | - |
| Neutron API | http://localhost:9696/v2.0 | - |
| Cinder API | http://localhost:8776/v3 | - |
| Glance API | http://localhost:9292/v2 | - |

## 🧪 Manual Testing Commands

### Quick Health Check
```bash
# Test authentication
openstack token issue

# List resources
openstack server list
openstack network list
openstack volume list
openstack image list
```

### Full Workflow Test
```bash
# Create network
openstack network create test-net
openstack subnet create test-subnet --network test-net --subnet-range 10.0.0.0/24

# Create volume
openstack volume create test-vol --size 1

# Launch instance
openstack server create test-vm \
  --flavor m1.tiny \
  --image cirros \
  --network test-net

# Cleanup
openstack server delete test-vm
openstack volume delete test-vol
openstack subnet delete test-subnet
openstack network delete test-net
```

## 🐛 Debugging Tips

### Check O3K Logs
```bash
docker logs o3k --tail 100 -f
```

### Check Horizon Logs
```bash
docker logs o3k-horizon --tail 100 -f
```

### Check Database
```bash
docker exec -it o3k-postgres psql -U postgres -d o3k
```

### Test Specific Endpoint
```bash
# Get token
TOKEN=$(openstack token issue -f value -c id)

# Test endpoint directly
curl -H "X-Auth-Token: $TOKEN" http://localhost:8774/v2.1/os-hypervisors
```

## 📝 Documenting Gaps

When you find a missing API:

1. **Note the exact error**
   - Browser console (F12)
   - Docker logs
   - CLI error message

2. **Identify the endpoint**
   - Method: GET/POST/PUT/DELETE
   - Path: /v2.1/os-hypervisors
   - Service: Nova/Neutron/Cinder/Glance/Keystone

3. **Record in GAP_DISCOVERY_REPORT.md**
   - Use the template provided
   - Set priority (🔴/🟡/🟢/⚪)
   - Estimate effort

4. **Screenshot if UI issue**
   - Save to `docs/validation_screenshots/`

## ✅ Horizon Workflow Checklist

Copy this to track your testing:

**Identity:**
- [ ] Login works
- [ ] User list displays
- [ ] Create user works
- [ ] Edit user works
- [ ] Delete user works
- [ ] Project list displays
- [ ] Create project works

**Compute:**
- [ ] Instance list displays
- [ ] Launch instance wizard opens
- [ ] Flavor selection works
- [ ] Image selection works
- [ ] Network selection works
- [ ] Instance launches successfully
- [ ] Instance actions (stop/start) work
- [ ] Console access works
- [ ] Delete instance works

**Network:**
- [ ] Network list displays
- [ ] Create network works
- [ ] Subnet creation works
- [ ] Router list displays
- [ ] Create router works
- [ ] Router interface attachment works
- [ ] Security group management works

**Volume:**
- [ ] Volume list displays
- [ ] Create volume works
- [ ] Attach volume to instance works
- [ ] Detach volume works
- [ ] Create snapshot works
- [ ] Delete volume works

**Image:**
- [ ] Image list displays
- [ ] Upload image works
- [ ] Image details show correctly
- [ ] Launch instance from image works
- [ ] Delete image works

**Overview:**
- [ ] Dashboard loads without errors
- [ ] Usage charts display
- [ ] Quota information shows

## 🎯 Success Criteria

**Minimum (70%):**
- Basic CRUD works for all resource types
- Can launch an instance end-to-end
- No critical Horizon errors

**Target (85%):**
- All standard workflows functional
- Minor features may be missing
- <5 high-priority gaps remaining

**Ideal (95%+):**
- Full Horizon functionality
- All CLI operations work
- Ready for production testing

## 📋 Priority Definitions

| Priority | Meaning | Action |
|----------|---------|--------|
| 🔴 **CRITICAL** | Blocks core workflow | Implement immediately |
| 🟡 **HIGH** | Degrades experience | Implement in Sprint 2 |
| 🟢 **MEDIUM** | Nice to have | Defer to later |
| ⚪ **LOW** | Advanced feature | Future enhancement |

## 🔄 Iteration Process

1. **Run validation** → Collect gaps
2. **Prioritize gaps** → Sort by impact
3. **Implement critical** → TDD workflow
4. **Re-validate** → Confirm fixes
5. **Repeat** until success criteria met

## 💡 Quick Fixes Reference

### Add Simple GET Endpoint
```go
// 1. Add route
v2.GET("/path", svc.HandlerName)

// 2. Add handler
func (svc *Service) HandlerName(c *gin.Context) {
    // Query database or return stub data
    c.JSON(http.StatusOK, gin.H{"data": []})
}
```

### Add Database Query
```go
rows, err := database.DB.Query(c.Request.Context(),
    "SELECT * FROM table WHERE condition = $1",
    param,
)
```

### Return Stub Data (Fast)
```go
// For quick unblocking
c.JSON(http.StatusOK, gin.H{
    "items": []gin.H{
        {"id": "stub-1", "name": "Stub Resource"},
    },
})
```

## 📞 Getting Help

- Check existing test scripts in `test/`
- Review CLAUDE.md for patterns
- Look at similar endpoints in codebase
- Check OpenStack API docs

## 🎬 Next Steps After Validation

1. **Generate report**
   ```bash
   cat validation_results_*.txt
   ```

2. **Fill in GAP_DISCOVERY_REPORT.md**
   - Document all gaps found
   - Set priorities
   - Estimate effort

3. **Create implementation plan**
   - Sprint 1: Critical gaps (2-3 days)
   - Sprint 2: High priority (1-2 days)
   - Sprint 3: Polish (1 day)

4. **Begin gap filling**
   - Use TDD workflow
   - Commit per feature
   - Re-test after each fix

---

**Status:** Ready to start validation when Docker is available
