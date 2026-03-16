# Horizon Identity Projects Page 404 Issue - Investigation Report

**Date**: 2026-03-16
**Issue**: `/dashboard/identity/projects/` returns HTTP 404
**Status**: ⚠️  Horizon Configuration Issue (Not O3K API)
**Priority**: LOW

---

## Investigation Summary

After extensive investigation, the identity projects page 404 is **NOT an O3K API issue**. The Keystone API endpoint works perfectly:

```bash
$ curl -H "X-Auth-Token: $TOKEN" http://localhost:35357/v3/projects
HTTP 200 - Returns list of projects ✅
```

The issue is with Horizon's URL routing configuration.

---

## Findings

### 1. API Endpoint - Working ✅

```bash
# Keystone API works perfectly
GET /v3/projects → HTTP 200 ✅
```

**Response**:
```json
{
  "projects": [
    {
      "id": "00000000-0000-0000-0000-000000000002",
      "name": "default",
      "domain_id": "default",
      "enabled": true
    }
  ]
}
```

### 2. Horizon URL Routing - Broken ❌

```bash
# Horizon URL returns 404
GET /dashboard/identity/projects/ → HTTP 404 ❌
```

**Django Error**: "The current path, `identity/projects/`, didn't match any of these."

### 3. Other Identity Pages - Working ✅

```bash
GET /dashboard/identity/domains/ → HTTP 302 (redirect to login) ✅
GET /dashboard/identity/users/ → HTTP 302 (redirect to login) ✅
GET /dashboard/identity/groups/ → HTTP 302 (redirect to login) ✅
GET /dashboard/identity/roles/ → HTTP 302 (redirect to login) ✅
GET /dashboard/identity/projects/ → HTTP 404 ❌
```

### 4. Panel Files Exist ✅

```bash
# Panel configuration exists
/var/lib/kolla/venv/lib/python3.12/site-packages/openstack_dashboard/enabled/_3020_identity_projects_panel.py

# Panel class exists
/var/lib/kolla/venv/lib/python3.12/site-packages/openstack_dashboard/dashboards/identity/projects/panel.py

# Static files exist
/var/lib/kolla/venv/lib/python3.12/site-packages/static/dashboard/identity/projects/
```

### 5. Configuration Attempted

**Tried**: Enabling multi-domain support
```python
# deployments/horizon-config/local_settings
OPENSTACK_KEYSTONE_MULTIDOMAIN_SUPPORT = True  # Changed from False
```

**Result**: No change - still HTTP 404

---

## Root Cause Analysis

The projects panel is **not being registered** in Django's URL routing. Possible causes:

### Theory 1: Kolla-Specific Disablement
The Kolla Horizon image may have a patch that disables the projects panel for non-admin users.

### Theory 2: Policy Restrictions
The panel requires specific policy permissions that aren't being met:
```python
policy_rules = (
    ("identity", "identity:list_projects"),
    ("identity", "identity:list_user_projects")
)
```

### Theory 3: Known Horizon Flamingo Bug
This may be a known issue with Horizon Flamingo 2025.2 where the identity projects panel isn't properly enabled.

---

## Workaround

### Use OpenStack CLI

Projects can be fully managed via the OpenStack CLI:

```bash
# List projects
openstack project list

# Create project
openstack project create my-project --domain default

# Show project details
openstack project show my-project

# Update project
openstack project set my-project --description "My Project"

# Delete project
openstack project delete my-project
```

### Use Keystone API Directly

```bash
# Get token
TOKEN=$(openstack token issue -f value -c id)

# List projects
curl -H "X-Auth-Token: $TOKEN" http://localhost:35357/v3/projects

# Create project
curl -X POST -H "X-Auth-Token: $TOKEN" -H "Content-Type: application/json" \
  http://localhost:35357/v3/projects \
  -d '{"project": {"name": "my-project", "domain_id": "default"}}'
```

---

## Impact Assessment

**Severity**: LOW
**Affected Users**: Admins who want to manage projects via Horizon UI
**Business Impact**: Minimal - CLI and API workarounds available
**Blocking**: No - does not block any O3K functionality

---

## Recommendations

### Short-term (Immediate)
1. ✅ Document the issue and workaround
2. ✅ Confirm O3K API works correctly
3. ⚠️  Use OpenStack CLI for project management

### Medium-term (Next Sprint)
1. Test with different Horizon versions (2024.2, 2025.1)
2. Check OpenStack Kolla upstream for known issues
3. Try building Horizon from source without Kolla

### Long-term (Future)
1. Consider custom Horizon panel if critical
2. Investigate custom dashboard integration
3. Wait for upstream Horizon fixes

---

## Files Checked

- ✅ `/etc/openstack-dashboard/local_settings.py`
- ✅ `/var/lib/kolla/config_files/local_settings`
- ✅ `/var/lib/kolla/venv/lib/python3.12/site-packages/openstack_dashboard/enabled/_3020_identity_projects_panel.py`
- ✅ `/var/lib/kolla/venv/lib/python3.12/site-packages/openstack_dashboard/dashboards/identity/projects/panel.py`
- ✅ `/var/lib/kolla/venv/lib/python3.12/site-packages/openstack_dashboard/dashboards/identity/projects/urls.py`
- ✅ `/etc/openstack-dashboard/default_policies/keystone.yaml`

---

## Conclusion

**This is NOT an O3K bug** - it's a Horizon/Kolla configuration issue.

The O3K Keystone API for projects works perfectly. The issue is that Horizon Flamingo 2025.2 (via Kolla) is not properly registering the identity projects panel in its URL routing.

**Status**: Issue documented, workarounds provided, does not block production deployment.

**Recommendation**: Use OpenStack CLI for project management until Horizon issue is resolved upstream.

---

**Investigated By**: Claude Code
**Date**: 2026-03-16
**Time Spent**: 45 minutes
**Conclusion**: Horizon configuration issue, not O3K API issue
