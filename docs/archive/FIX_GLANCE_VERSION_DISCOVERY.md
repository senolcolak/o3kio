# Fix: Glance Version Discovery Endpoint

**Issue**: SPEC-000 Compliance Bug
**Status**: ✅ FIXED
**Date**: 2026-03-09

## Problem

Glance's version discovery endpoint (`GET /v2`) required authentication, returning 401 Unauthorized for unauthenticated requests.

**OpenStack Specification**: Version discovery endpoints MUST be accessible without authentication to allow clients to detect API capabilities before authenticating.

**Impact**: Minor SPEC-000 compliance violation that prevents proper API version discovery.

## Root Cause

In `cmd/o3k/main.go`, the `createGlanceServer()` function applied the auth middleware globally to all routes:

```go
// BEFORE (incorrect)
func createGlanceServer(cfg *common.Config, svc *glance.Service, authService *keystone.AuthService) *http.Server {
	r := gin.New()
	r.Use(middleware.LoggingMiddleware())
	r.Use(middleware.RecoveryMiddleware())
	r.Use(middleware.CORSMiddleware())
	r.Use(middleware.AuthMiddleware(authService))  // ❌ Applied to ALL routes

	svc.RegisterRoutes(r.Group(""))

	return &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Glance.Port),
		Handler: r,
	}
}
```

This caused version discovery endpoints to require authentication.

## Solution

Separated version discovery endpoints from authenticated routes:

### 1. Updated `cmd/o3k/main.go`

```go
// AFTER (correct)
func createGlanceServer(cfg *common.Config, svc *glance.Service, authService *keystone.AuthService) *http.Server {
	r := gin.New()
	r.Use(middleware.LoggingMiddleware())
	r.Use(middleware.RecoveryMiddleware())
	r.Use(middleware.CORSMiddleware())

	// Version discovery endpoints (no auth required per OpenStack spec)
	root := r.Group("")
	root.GET("/", svc.GetVersions)
	root.GET("/v2", svc.GetVersionV2)

	// All other routes require authentication
	authGroup := r.Group("")
	authGroup.Use(middleware.AuthMiddleware(authService))  // ✅ Applied only to data routes
	svc.RegisterRoutes(authGroup)

	return &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Glance.Port),
		Handler: r,
	}
}
```

### 2. Updated `internal/glance/images.go`

Removed version discovery registration from `RegisterRoutes()` since it's now handled separately:

```go
// RegisterRoutes registers Glance routes (excluding version discovery which is handled separately)
func (svc *Service) RegisterRoutes(r *gin.RouterGroup) {
	// Note: Version discovery (GET / and GET /v2) are registered separately
	// in main.go without auth middleware to comply with OpenStack spec

	v2 := r.Group("/v2")
	{
		// Images
		v2.GET("/images", svc.ListImages)
		v2.POST("/images", svc.CreateImage)
		// ... rest of routes
	}
}
```

## Testing

### Before Fix
```bash
$ curl -s http://localhost:9292/v2
{"error":{"code":401,"message":"authentication required","title":"Unauthorized"}}
❌ FAIL
```

### After Fix
```bash
$ curl -s http://localhost:9292/v2
{
  "version": {
    "id": "v2.9",
    "links": [
      {
        "href": "http://localhost:9292/v2/",
        "rel": "self"
      }
    ],
    "status": "CURRENT"
  }
}
✅ PASS
```

## Verification

Ran complete version discovery test suite:

```
Version Discovery:
  ✓ Keystone /v3
  ✓ Nova /v2.1
  ✓ Neutron /v2.0
  ✓ Glance /v2          ← FIXED!

Summary: 4 passed, 0 failed
✅ All version discovery tests passed!
```

## Files Changed

1. **cmd/o3k/main.go**
   - Modified `createGlanceServer()` function
   - Separated version discovery from authenticated routes

2. **internal/glance/images.go**
   - Updated `RegisterRoutes()` function
   - Removed duplicate version discovery registration
   - Added documentation comment

## SPEC-000 Compliance Impact

**Before**: 12/13 tests passing (92%)
**After**: 13/13 tests passing (100%) ✅

This fix resolves the only failed test in the quick integration test suite.

## OpenStack Compatibility

This change aligns O3K with OpenStack specification requirements:
- ✅ Version discovery works without authentication
- ✅ Clients can detect API capabilities before authenticating
- ✅ Compatible with OpenStack clients that probe version endpoints
- ✅ Follows OpenStack Identity API v3 patterns

## Related Specifications

- [SPEC-000: OpenStack API Compliance Framework](../specs/000-api-compliance/README.md)
- OpenStack Identity API v3 specification
- Glance v2 API specification

## Deployment

To deploy this fix:

```bash
# Build Linux binary
GOOS=linux GOARCH=amd64 go build -o bin/o3k-linux ./cmd/o3k

# Copy to container
docker cp bin/o3k-linux o3k:/app/o3k

# Restart service
docker restart o3k
```

## Commit Message

```
fix(glance): allow unauthenticated version discovery

Glance version discovery endpoints (GET / and GET /v2) now
work without authentication, compliant with OpenStack spec.

Version discovery endpoints must be accessible before
authentication to allow clients to detect API capabilities.

Fixes SPEC-000 compliance issue.

Changes:
- Separate version discovery routes from authenticated routes
- Apply auth middleware only to data endpoints
- Update route registration to avoid duplication

Test results: 13/13 tests passing (was 12/13)

Related: SPEC-000 OpenStack API Compliance Framework
```

## Lessons Learned

1. **Auth Middleware Scope**: Be careful when applying auth middleware globally. Some endpoints (version discovery, health checks) should be public.

2. **OpenStack Patterns**: Version discovery is always unauthenticated across all OpenStack services (Keystone, Nova, Neutron, Cinder, Glance).

3. **Testing Importance**: Integration tests caught this compliance issue before it reached production.

4. **Quick Iteration**: Being able to build, deploy, and test quickly (< 5 minutes) enables rapid bug fixes.

## Next Steps

- [x] Fix implemented
- [x] Tests passing
- [ ] Update integration tests to verify unauthenticated access
- [ ] Apply similar pattern review to other services
- [ ] Document version discovery requirements in developer guide

---

**Status**: ✅ Complete
**Test Results**: All tests passing
**SPEC-000 Compliance**: Improved from 92% to 100% (quick test suite)
