# Contract: Nova Server Actions API

**Feature**: OpenStack Horizon 100% Compatibility
**Service**: Nova (Compute)
**Purpose**: Complete server action implementations for Horizon dashboard

## Endpoint 1: Migrate Server

### Request

```http
POST /v2.1/servers/{server_id}/action HTTP/1.1
Host: o3k-host:8774
X-Auth-Token: {token}
Content-Type: application/json

{
  "migrate": null
}
```

**Path Parameters**:
- `server_id`: UUID of the instance to migrate

**Request Body**:
```json
{
  "migrate": null  // No parameters required
}
```

### Successful Response

```http
HTTP/1.1 202 Accepted
Content-Type: application/json

{}
```

**Response**: Empty body with 202 Accepted status

**Side Effects**:
1. Migration record created in `migrations` table
2. Instance `task_state` set to `migrating`
3. Background task selects destination host
4. Instance migrated to new host
5. Instance `host` field updated
6. Instance `task_state` cleared

### Error Responses

#### 404 Not Found - Server Does Not Exist

```http
HTTP/1.1 404 Not Found
Content-Type: application/json

{
  "itemNotFound": {
    "message": "Instance {server_id} could not be found.",
    "code": 404
  }
}
```

#### 409 Conflict - Invalid State

```http
HTTP/1.1 409 Conflict
Content-Type: application/json

{
  "conflictingRequest": {
    "message": "Cannot migrate instance in SHUTOFF state. Must be ACTIVE.",
    "code": 409
  }
}
```

**Conditions for 409**:
- Instance not in ACTIVE state
- Instance already has migration in progress
- No suitable destination host available

---

## Endpoint 2: Evacuate Server

### Request

```http
POST /v2.1/servers/{server_id}/action HTTP/1.1
Host: o3k-host:8774
X-Auth-Token: {token}
Content-Type: application/json

{
  "evacuate": {
    "host": "compute-node-2",
    "onSharedStorage": false,
    "adminPass": "newPassword123"
  }
}
```

**Request Body**:
```json
{
  "evacuate": {
    "host": "compute-node-2",        // Optional: destination host
    "onSharedStorage": false,         // Optional: whether storage is shared
    "adminPass": "newPassword123"     // Optional: new admin password
  }
}
```

**Field Descriptions**:
- `host`: Target compute node (optional, auto-selected if omitted)
- `onSharedStorage`: Whether instance storage is on shared storage (default: false)
- `adminPass`: New admin password after evacuation (optional, auto-generated if omitted)

### Successful Response

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "adminPass": "newPassword123"
}
```

**Response Fields**:
- `adminPass`: The admin password (echoed if provided, generated if not)

**Side Effects**:
1. Migration record created with type=`evacuation`
2. Instance recreated on destination host
3. If `onSharedStorage=false`, storage copied
4. Admin password updated if provided/generated
5. Instance `host` field updated

### Error Responses

#### 404 Not Found

```http
HTTP/1.1 404 Not Found
Content-Type: application/json

{
  "itemNotFound": {
    "message": "Instance {server_id} could not be found.",
    "code": 404
  }
}
```

#### 400 Bad Request - Invalid Host

```http
HTTP/1.1 400 Bad Request
Content-Type: application/json

{
  "badRequest": {
    "message": "Compute host compute-node-2 could not be found.",
    "code": 400
  }
}
```

---

## Endpoint 3: Change Admin Password

### Request

```http
POST /v2.1/servers/{server_id}/action HTTP/1.1
Host: o3k-host:8774
X-Auth-Token: {token}
Content-Type: application/json

{
  "changePassword": {
    "adminPass": "newSecurePassword456"
  }
}
```

**Request Body**:
```json
{
  "changePassword": {
    "adminPass": "newSecurePassword456"  // Required: new admin password
  }
}
```

**Password Requirements**:
- Minimum length: 8 characters
- Maximum length: 255 characters
- No special character requirements (configurable)

### Successful Response

```http
HTTP/1.1 202 Accepted
Content-Type: application/json

{}
```

**Response**: Empty body with 202 Accepted status

**Side Effects**:
1. Password hashed using bcrypt (cost 10)
2. Instance `admin_password_hash` field updated
3. Instance `updated_at` timestamp updated

**Security Note**: Password is never stored in plaintext, only bcrypt hash persisted.

### Error Responses

#### 404 Not Found

```http
HTTP/1.1 404 Not Found
Content-Type: application/json

{
  "itemNotFound": {
    "message": "Instance {server_id} could not be found.",
    "code": 404
  }
}
```

#### 400 Bad Request - Invalid Password

```http
HTTP/1.1 400 Bad Request
Content-Type: application/json

{
  "badRequest": {
    "message": "Admin password must be between 8 and 255 characters.",
    "code": 400
  }
}
```

---

## Endpoint 4: Create Backup

### Request

```http
POST /v2.1/servers/{server_id}/action HTTP/1.1
Host: o3k-host:8774
X-Auth-Token: {token}
Content-Type: application/json

{
  "createBackup": {
    "name": "daily-backup-2026-03-13",
    "backup_type": "daily",
    "rotation": 7
  }
}
```

**Request Body**:
```json
{
  "createBackup": {
    "name": "daily-backup-2026-03-13",  // Required: backup image name
    "backup_type": "daily",              // Required: daily, weekly, or monthly
    "rotation": 7                        // Required: max backups to keep (≥1)
  }
}
```

**Field Descriptions**:
- `name`: User-friendly backup name
- `backup_type`: Backup frequency category (daily/weekly/monthly)
- `rotation`: Maximum number of backups of this type to retain

### Successful Response

```http
HTTP/1.1 202 Accepted
Location: http://o3k-host:9292/v2/images/12345678-abcd-ef12-3456-0123456789ab
Content-Type: application/json

{
  "image_id": "12345678-abcd-ef12-3456-0123456789ab"
}
```

**Response Headers**:
- `Location`: URL of the created backup image

**Response Fields**:
- `image_id`: UUID of the created backup image

**Side Effects**:
1. Snapshot of instance created in Glance
2. Image properties set: `backup_type`, `instance_uuid`, `backup_name`
3. Rotation applied: oldest backups deleted if count > rotation
4. Instance `task_state` set to `image_backup` during creation

### Error Responses

#### 404 Not Found

```http
HTTP/1.1 404 Not Found
Content-Type: application/json

{
  "itemNotFound": {
    "message": "Instance {server_id} could not be found.",
    "code": 404
  }
}
```

#### 400 Bad Request - Invalid Rotation

```http
HTTP/1.1 400 Bad Request
Content-Type: application/json

{
  "badRequest": {
    "message": "Rotation must be greater than or equal to 1.",
    "code": 400
  }
}
```

#### 400 Bad Request - Invalid Backup Type

```http
HTTP/1.1 400 Bad Request
Content-Type: application/json

{
  "badRequest": {
    "message": "backup_type must be one of: daily, weekly, monthly",
    "code": 400
  }
}
```

---

## Endpoint 5: Add Security Group

### Request

```http
POST /v2.1/servers/{server_id}/action HTTP/1.1
Host: o3k-host:8774
X-Auth-Token: {token}
Content-Type: application/json

{
  "addSecurityGroup": {
    "name": "web-server"
  }
}
```

**Request Body**:
```json
{
  "addSecurityGroup": {
    "name": "web-server"  // Required: security group name
  }
}
```

### Successful Response

```http
HTTP/1.1 202 Accepted
Content-Type: application/json

{}
```

**Response**: Empty body with 202 Accepted status

**Side Effects**:
1. Association created in `server_security_groups` table
2. Neutron port security groups updated (real mode)
3. iptables rules applied (real mode)

### Error Responses

#### 404 Not Found - Server Does Not Exist

```http
HTTP/1.1 404 Not Found
Content-Type: application/json

{
  "itemNotFound": {
    "message": "Instance {server_id} could not be found.",
    "code": 404
  }
}
```

#### 404 Not Found - Security Group Does Not Exist

```http
HTTP/1.1 404 Not Found
Content-Type: application/json

{
  "itemNotFound": {
    "message": "Security group web-server could not be found.",
    "code": 404
  }
}
```

#### 409 Conflict - Already Associated

```http
HTTP/1.1 409 Conflict
Content-Type: application/json

{
  "conflictingRequest": {
    "message": "Security group web-server is already associated with this instance.",
    "code": 409
  }
}
```

---

## Endpoint 6: Remove Security Group

### Request

```http
POST /v2.1/servers/{server_id}/action HTTP/1.1
Host: o3k-host:8774
X-Auth-Token: {token}
Content-Type: application/json

{
  "removeSecurityGroup": {
    "name": "web-server"
  }
}
```

**Request Body**:
```json
{
  "removeSecurityGroup": {
    "name": "web-server"  // Required: security group name
  }
}
```

### Successful Response

```http
HTTP/1.1 202 Accepted
Content-Type: application/json

{}
```

**Response**: Empty body with 202 Accepted status

**Side Effects**:
1. Association deleted from `server_security_groups` table
2. Neutron port security groups updated (real mode)
3. iptables rules removed (real mode)

### Error Responses

#### 404 Not Found - Server Does Not Exist

```http
HTTP/1.1 404 Not Found
Content-Type: application/json

{
  "itemNotFound": {
    "message": "Instance {server_id} could not be found.",
    "code": 404
  }
}
```

#### 404 Not Found - Security Group Does Not Exist

```http
HTTP/1.1 404 Not Found
Content-Type: application/json

{
  "itemNotFound": {
    "message": "Security group web-server could not be found.",
    "code": 404
  }
}
```

#### 400 Bad Request - Last Security Group

```http
HTTP/1.1 400 Bad Request
Content-Type: application/json

{
  "badRequest": {
    "message": "Cannot remove last security group from instance. At least one security group must remain.",
    "code": 400
  }
}
```

#### 400 Bad Request - Not Associated

```http
HTTP/1.1 400 Bad Request
Content-Type: application/json

{
  "badRequest": {
    "message": "Security group web-server is not associated with this instance.",
    "code": 400
  }
}
```

---

## Endpoint 7: Reset Server State (Admin Only)

### Request

```http
POST /v2.1/servers/{server_id}/action HTTP/1.1
Host: o3k-host:8774
X-Auth-Token: {admin-token}
Content-Type: application/json

{
  "os-resetState": {
    "state": "error"
  }
}
```

**Request Body**:
```json
{
  "os-resetState": {
    "state": "active|error|stopped"  // Required: target state
  }
}
```

**Valid States**:
- `active`: Set instance to ACTIVE
- `error`: Set instance to ERROR
- `stopped`: Set instance to SHUTOFF

**Authorization**: Requires `admin` role. Returns 403 Forbidden for non-admin users.

### Successful Response

```http
HTTP/1.1 202 Accepted
Content-Type: application/json

{}
```

**Response**: Empty body with 202 Accepted status

**Side Effects**:
1. Instance `vm_state` field updated directly
2. Instance `task_state` cleared
3. Instance `updated_at` timestamp updated

**Security**: Admin role check CRITICAL. Do not allow non-admin users to reset state.

### Error Responses

#### 403 Forbidden - Non-Admin User

```http
HTTP/1.1 403 Forbidden
Content-Type: application/json

{
  "forbidden": {
    "message": "Admin role required for state reset.",
    "code": 403
  }
}
```

#### 404 Not Found

```http
HTTP/1.1 404 Not Found
Content-Type: application/json

{
  "itemNotFound": {
    "message": "Instance {server_id} could not be found.",
    "code": 404
  }
}
```

#### 400 Bad Request - Invalid State

```http
HTTP/1.1 400 Bad Request
Content-Type: application/json

{
  "badRequest": {
    "message": "Invalid state: must be one of active, error, stopped",
    "code": 400
  }
}
```

---

## Implementation Status

⚠️ **INCOMPLETE** - Sprint 56-57 in progress

**Current Implementation**:
- File: `/Users/I761222/git/o3k/internal/nova/advanced_actions.go`
- Status: All endpoints registered, most are incomplete stubs
- Security Issue: `os-resetState` missing admin role check

**Required Work**:
1. ✅ `os-resetNetwork` - Complete
2. ⚠️ `migrate` - 30% complete (needs migration record, host selection)
3. ⚠️ `evacuate` - 20% complete (needs request parsing, host validation)
4. ⚠️ `changePassword` - 40% complete (needs bcrypt hashing, DB update)
5. ⚠️ `createBackup` - 30% complete (needs image creation, rotation)
6. ⚠️ `addSecurityGroup` - 50% complete (needs table INSERT, port updates)
7. ⚠️ `removeSecurityGroup` - 50% complete (needs table DELETE, last SG check)
8. 🔴 `os-resetState` - 70% complete (MISSING ADMIN CHECK - SECURITY ISSUE)

---

## Contract Tests

### Test File: `test/contract/nova/server_actions_test.go`

```go
func TestMigrateServer_Contract(t *testing.T) {
    client := setupNovaClient(t)
    server := createTestServer(t, client)
    defer deleteTestServer(t, client, server.ID)

    // Migrate server
    err := client.Post(
        client.ServiceURL("servers", server.ID, "action"),
        map[string]interface{}{"migrate": nil},
        nil,
        nil,
    ).ExtractErr()

    assert.NoError(t, err)

    // Verify migration record created
    // (requires database query or API to list migrations)
}

func TestChangePassword_Contract(t *testing.T) {
    client := setupNovaClient(t)
    server := createTestServer(t, client)
    defer deleteTestServer(t, client, server.ID)

    // Change password
    err := client.Post(
        client.ServiceURL("servers", server.ID, "action"),
        map[string]interface{}{
            "changePassword": map[string]interface{}{
                "adminPass": "newSecurePassword123",
            },
        },
        nil,
        nil,
    ).ExtractErr()

    assert.NoError(t, err)

    // Verify password updated (requires SSH test or instance console)
}

func TestResetStateUnauthorized_Contract(t *testing.T) {
    client := setupNovaClient(t) // Non-admin client
    server := createTestServer(t, client)
    defer deleteTestServer(t, client, server.ID)

    // Attempt state reset without admin role
    err := client.Post(
        client.ServiceURL("servers", server.ID, "action"),
        map[string]interface{}{
            "os-resetState": map[string]interface{}{
                "state": "error",
            },
        },
        nil,
        nil,
    ).ExtractErr()

    // MUST return 403 Forbidden
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "403")
}
```

---

## Notes

- All 7 actions are Horizon dashboard features used by cloud administrators
- Admin role enforcement CRITICAL for `os-resetState`
- Backup rotation prevents unlimited storage growth
- Security group operations must maintain "at least one SG" invariant
- Password changes use bcrypt with cost 10+ for production security
