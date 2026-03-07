# Error Handling Standardization

## Overview

O3K implements OpenStack-compatible error responses across all services. This document describes the standardized error format and how to use it.

## Error Format

### OpenStack Standard Format

All errors follow the OpenStack API error response format:

```json
{
  "<errorType>": {
    "message": "Human-readable error message",
    "code": 404
  }
}
```

### Example Error Responses

**404 Not Found:**
```json
{
  "itemNotFound": {
    "message": "Instance could not be found.",
    "code": 404
  }
}
```

**400 Bad Request:**
```json
{
  "badRequest": {
    "message": "Invalid request body",
    "code": 400,
    "details": "Field 'name': must not be empty"
  }
}
```

**413 Quota Exceeded:**
```json
{
  "overLimit": {
    "message": "Quota exceeded for resource: instances",
    "code": 413
  }
}
```

**500 Internal Server Error:**
```json
{
  "computeFault": {
    "message": "Internal server error occurred",
    "code": 500
  }
}
```

## Error Types

| HTTP Code | Error Type | Usage |
|-----------|------------|-------|
| 400 | badRequest | Invalid input, validation errors |
| 401 | unauthorized | Missing or invalid authentication |
| 403 | forbidden | Insufficient permissions |
| 404 | itemNotFound | Resource not found |
| 405 | badMethod | HTTP method not allowed |
| 409 | conflict / conflictingRequest | Resource conflict, invalid state |
| 413 | overLimit | Quota exceeded, rate limit |
| 429 | overLimit | Too many requests |
| 500 | computeFault | Internal server error |
| 503 | serviceUnavailable | Service temporarily unavailable |

## Using Standardized Errors

### Import the Package

```go
import "github.com/cobaltcore-dev/o3k/internal/common"
```

### Basic Usage

```go
// Not found error
if instance == nil {
    err := common.NewNotFoundError("Instance")
    common.SendError(c, err)
    return
}

// Bad request error
if req.Name == "" {
    err := common.NewBadRequestError("Instance name is required")
    common.SendError(c, err)
    return
}

// Quota exceeded
if quotaExceeded {
    err := common.NewQuotaExceededError("instances")
    common.SendError(c, err)
    return
}
```

### Error Response Helper

For cleaner code, use the ErrorResponse helper:

```go
resp := common.NewErrorResponse(c)

// Not found
if instance == nil {
    resp.NotFound("Instance")
    return
}

// Bad request
if req.Name == "" {
    resp.BadRequest("Instance name is required")
    return
}

// Quota exceeded
if quotaExceeded {
    resp.QuotaExceeded("instances")
    return
}
```

### Database Errors

Use the database error helper to automatically convert database errors:

```go
err := database.DB.QueryRow(ctx, query, instanceID).Scan(&instance)
if err != nil {
    common.HandleDatabaseError(c, err, "Instance")
    return
}
```

This automatically:
- Converts `pgx.ErrNoRows` to 404 Not Found
- Logs other database errors
- Returns 500 Internal Server Error for unexpected errors

### Validation Errors

```go
if len(req.Name) > 255 {
    common.HandleValidationError(c, "name", "must be less than 255 characters")
    return
}
```

### Binding Errors

```go
var req CreateServerRequest
if err := c.ShouldBindJSON(&req); err != nil {
    common.HandleBindingError(c, err)
    return
}
```

## Available Error Constructors

### Basic Errors

```go
// 400 Bad Request
NewBadRequestError(message string) *OpenStackError
NewBadRequestErrorWithDetails(message, details string) *OpenStackError

// 401 Unauthorized
NewUnauthorizedError(message string) *OpenStackError

// 403 Forbidden
NewForbiddenError(message string) *OpenStackError

// 404 Not Found
NewNotFoundError(resource string) *OpenStackError

// 405 Method Not Allowed
NewMethodNotAllowedError(method string) *OpenStackError

// 409 Conflict
NewConflictError(message string) *OpenStackError
NewInvalidStateError(message string) *OpenStackError
NewBuildInProgressError() *OpenStackError

// 413 Quota Exceeded
NewQuotaExceededError(resource string) *OpenStackError

// 429 Rate Limit
NewRateLimitError(message string) *OpenStackError

// 500 Internal Server Error
NewInternalServerError(message string) *OpenStackError

// 503 Service Unavailable
NewServiceUnavailableError(message string) *OpenStackError
```

### Helper Functions

```go
// Send error response
SendError(c *gin.Context, err *OpenStackError)

// Abort request with error
AbortWithError(c *gin.Context, err *OpenStackError)

// Wrap Go error
WrapError(err error, statusCode int, code string) *OpenStackError

// Check error type
IsNotFound(err error) bool
IsConflict(err error) bool
```

## Migration Guide

### Before (Non-Standard)

```go
func GetServer(c *gin.Context) {
    instanceID := c.Param("id")

    var instance Instance
    err := database.DB.QueryRow(ctx, query, instanceID).Scan(&instance)
    if err != nil {
        if err == pgx.ErrNoRows {
            c.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
            return
        }
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{"server": instance})
}
```

### After (Standardized)

```go
func GetServer(c *gin.Context) {
    instanceID := c.Param("id")

    var instance Instance
    err := database.DB.QueryRow(ctx, query, instanceID).Scan(&instance)
    if err != nil {
        common.HandleDatabaseError(c, err, "Instance")
        return
    }

    c.JSON(http.StatusOK, gin.H{"server": instance})
}
```

### Complex Example

**Before:**
```go
func CreateServer(c *gin.Context) {
    var req CreateServerRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
        return
    }

    if req.Name == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
        return
    }

    // Check quota
    if quotaExceeded {
        c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": gin.H{
            "message": "Quota exceeded for resource: instances",
            "code": 413,
        }})
        return
    }

    // Create instance...
}
```

**After:**
```go
func CreateServer(c *gin.Context) {
    var req CreateServerRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        common.HandleBindingError(c, err)
        return
    }

    if req.Name == "" {
        common.HandleValidationError(c, "name", "is required")
        return
    }

    // Check quota
    if quotaExceeded {
        common.NewErrorResponse(c).QuotaExceeded("instances")
        return
    }

    // Create instance...
}
```

## Error Handling Middleware

The error handling middleware catches panics and converts errors to proper responses:

```go
// In server setup
r.Use(middleware.ErrorHandlingMiddleware())
r.NoRoute(middleware.NotFoundHandler())
r.NoMethod(middleware.MethodNotAllowedHandler())
```

This provides:
- Panic recovery
- Automatic error conversion
- 404 handling for undefined routes
- 405 handling for wrong HTTP methods

## Best Practices

### 1. Use Specific Error Types

**Good:**
```go
if instance.Status == "BUILD" {
    err := common.NewBuildInProgressError()
    common.SendError(c, err)
    return
}
```

**Bad:**
```go
c.JSON(409, gin.H{"error": "cannot perform action"})
```

### 2. Provide Context in Error Messages

**Good:**
```go
err := common.NewNotFoundError("Instance")
// Produces: "Instance could not be found."
```

**Bad:**
```go
err := common.NewNotFoundError("")
// Produces: " could not be found."
```

### 3. Use Details for Validation Errors

**Good:**
```go
err := common.NewBadRequestErrorWithDetails(
    "Validation failed",
    "Field 'vcpus': must be between 1 and 128",
)
```

**Bad:**
```go
err := common.NewBadRequestError("invalid vcpus")
```

### 4. Don't Expose Internal Details

**Good:**
```go
err := common.NewInternalServerError("Failed to create instance")
logger.Error().Err(dbErr).Msg("Database error")
```

**Bad:**
```go
err := common.NewInternalServerError(fmt.Sprintf("Database error: %v", dbErr))
// Exposes internal database structure to users
```

### 5. Log Before Returning Errors

```go
if err := createVM(); err != nil {
    logger.Error().
        Err(err).
        Str("instance_id", instanceID).
        Msg("Failed to create VM")

    common.NewErrorResponse(c).InternalError("Failed to create instance")
    return
}
```

## Testing

Run error handling tests:

```bash
./test/error_handling_test.sh
```

Tests verify:
- Correct error format (itemNotFound, badRequest, etc.)
- HTTP status codes match error body codes
- Error messages are user-friendly
- No sensitive information exposed
- Consistent format across all services

## OpenStack Compatibility

Our error format is compatible with:
- OpenStack Nova API
- OpenStack Neutron API
- OpenStack Cinder API
- OpenStack Keystone API
- Horizon Dashboard
- python-openstackclient
- gophercloud SDK

## Error Format Examples

### Nova (Compute)

```json
// Instance not found
{
  "itemNotFound": {
    "message": "Instance could not be found.",
    "code": 404
  }
}

// Invalid flavor
{
  "badRequest": {
    "message": "Invalid flavor specified",
    "code": 400
  }
}

// Instance building
{
  "conflictingRequest": {
    "message": "Cannot perform action, instance is building.",
    "code": 409
  }
}
```

### Neutron (Network)

```json
// Network not found
{
  "itemNotFound": {
    "message": "Network could not be found.",
    "code": 404
  }
}

// Invalid CIDR
{
  "badRequest": {
    "message": "Invalid CIDR format",
    "code": 400,
    "details": "Field 'cidr': must be valid IPv4 or IPv6 CIDR"
  }
}
```

### Cinder (Block Storage)

```json
// Volume not found
{
  "itemNotFound": {
    "message": "Volume could not be found.",
    "code": 404
  }
}

// Volume in use
{
  "conflict": {
    "message": "Volume is attached to an instance",
    "code": 409
  }
}
```

## References

- [OpenStack API Error Handling](https://docs.openstack.org/api-ref/compute/#errors)
- [HTTP Status Codes](https://developer.mozilla.org/en-US/docs/Web/HTTP/Status)
- [O3K Error Testing](test/error_handling_test.sh)
