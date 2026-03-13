# Contract: Nova Console Access API

**Feature**: OpenStack Horizon 100% Compatibility
**Service**: Nova (Compute)
**Purpose**: VNC/SPICE/Serial console access for Horizon dashboard

## Endpoint 1: Get Remote Console (Modern API)

### Request

```http
POST /v2.1/servers/{server_id}/remote-consoles HTTP/1.1
Host: o3k-host:8774
X-Auth-Token: {token}
Content-Type: application/json

{
  "remote_console": {
    "protocol": "vnc",
    "type": "novnc"
  }
}
```

**Path Parameters**:
- `server_id`: UUID of the instance

**Request Body**:
```json
{
  "remote_console": {
    "protocol": "vnc|spice|serial|rdp",  // Required
    "type": "novnc|spice-html5|serial|rdp-html5"  // Required
  }
}
```

**Supported Combinations**:
- `protocol: vnc, type: novnc` - VNC via noVNC HTML5 viewer
- `protocol: spice, type: spice-html5` - SPICE via HTML5 viewer
- `protocol: serial, type: serial` - Serial console via WebSocket
- `protocol: rdp, type: rdp-html5` - RDP via HTML5 viewer

### Successful Response

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "remote_console": {
    "protocol": "vnc",
    "type": "novnc",
    "url": "http://o3k-host:6080/vnc_auto.html?token=eyJhbGciOiJIUzI1NiIs..."
  }
}
```

**Response Fields**:
- `protocol`: Echo of request protocol
- `type`: Echo of request type
- `url`: Console access URL with embedded token (valid for 1 hour)

**URL Format**:
- noVNC: `http://{proxy_host}:6080/vnc_auto.html?token={jwt}`
- SPICE: `http://{proxy_host}:6082/spice_auto.html?token={jwt}`
- Serial: `ws://{proxy_host}:6083/?token={jwt}`
- RDP: `http://{proxy_host}:6084/rdp_auto.html?token={jwt}`

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

#### 409 Conflict - Console Not Available

```http
HTTP/1.1 409 Conflict
Content-Type: application/json

{
  "conflictingRequest": {
    "message": "Instance must be in ACTIVE state to access console. Current state: SHUTOFF",
    "code": 409
  }
}
```

**Conditions for 409**:
- Instance not in ACTIVE state
- Instance has no host assignment (not scheduled)
- Hypervisor does not support requested console type

#### 400 Bad Request - Invalid Console Type

```http
HTTP/1.1 400 Bad Request
Content-Type: application/json

{
  "badRequest": {
    "message": "Unsupported console protocol/type combination",
    "code": 400
  }
}
```

---

## Endpoint 2: Get VNC Console (Legacy API)

### Request

```http
POST /v2.1/servers/{server_id}/action HTTP/1.1
Host: o3k-host:8774
X-Auth-Token: {token}
Content-Type: application/json

{
  "os-getVNCConsole": {
    "type": "novnc"
  }
}
```

**Request Body**:
```json
{
  "os-getVNCConsole": {
    "type": "novnc|xvpvnc"  // Required
  }
}
```

### Successful Response

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "console": {
    "type": "novnc",
    "url": "http://o3k-host:6080/vnc_auto.html?token=eyJhbGciOiJIUzI1NiIs..."
  }
}
```

**Response Fields**:
- `type`: Echo of request type
- `url`: Console access URL with embedded token

---

## Endpoint 3: Get SPICE Console (Legacy API)

### Request

```http
POST /v2.1/servers/{server_id}/action HTTP/1.1
Host: o3k-host:8774
X-Auth-Token: {token}
Content-Type: application/json

{
  "os-getSPICEConsole": {
    "type": "spice-html5"
  }
}
```

**Request Body**:
```json
{
  "os-getSPICEConsole": {
    "type": "spice-html5"  // Required
  }
}
```

### Successful Response

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "console": {
    "type": "spice-html5",
    "url": "http://o3k-host:6082/spice_auto.html?token=eyJhbGciOiJIUzI1NiIs..."
  }
}
```

---

## Endpoint 4: Get Serial Console (Legacy API)

### Request

```http
POST /v2.1/servers/{server_id}/action HTTP/1.1
Host: o3k-host:8774
X-Auth-Token: {token}
Content-Type: application/json

{
  "os-getSerialConsole": {
    "type": "serial"
  }
}
```

**Request Body**:
```json
{
  "os-getSerialConsole": {
    "type": "serial"  // Required
  }
}
```

### Successful Response

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "console": {
    "type": "serial",
    "url": "ws://o3k-host:6083/?token=eyJhbGciOiJIUzI1NiIs..."
  }
}
```

**Note**: Serial console returns WebSocket URL, not HTTP.

---

## Endpoint 5: Get RDP Console (Legacy API)

### Request

```http
POST /v2.1/servers/{server_id}/action HTTP/1.1
Host: o3k-host:8774
X-Auth-Token: {token}
Content-Type: application/json

{
  "os-getRDPConsole": {
    "type": "rdp-html5"
  }
}
```

**Request Body**:
```json
{
  "os-getRDPConsole": {
    "type": "rdp-html5"  // Required
  }
}
```

### Successful Response

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "console": {
    "type": "rdp-html5",
    "url": "http://o3k-host:6084/rdp_auto.html?token=eyJhbGciOiJIUzI1NiIs..."
  }
}
```

---

## Endpoint 6: Get Console Output

### Request

```http
POST /v2.1/servers/{server_id}/action HTTP/1.1
Host: o3k-host:8774
X-Auth-Token: {token}
Content-Type: application/json

{
  "os-getConsoleOutput": {
    "length": 100
  }
}
```

**Request Body**:
```json
{
  "os-getConsoleOutput": {
    "length": 100  // Optional: number of lines (default: all)
  }
}
```

### Successful Response

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "output": "[   0.000000] Linux version 5.10.0...\n[   0.001234] Command line: BOOT_IMAGE=/boot/vmlinuz...\n"
}
```

**Response Fields**:
- `output`: Text content of console output (last N lines if length specified)

---

## Token Format

### JWT Token Structure

**Header**:
```json
{
  "alg": "HS256",
  "typ": "JWT"
}
```

**Payload**:
```json
{
  "server_id": "uuid",
  "console_type": "novnc|spice|serial|rdp",
  "iat": 1710328800,
  "exp": 1710332400
}
```

**Signature**: HMAC-SHA256 using O3K JWT secret

**Token Validity**: 1 hour from issuance

**Validation** (by noVNC proxy):
1. Verify JWT signature using shared secret
2. Check expiration (exp > current_time)
3. Extract server_id and console_type
4. Query O3K for instance details
5. Connect to libvirt VNC/SPICE port for that instance

---

## Implementation Status

✅ **COMPLETE** - Implemented in Sprint 58-59

**Current Files**:
- `/Users/I761222/git/o3k/internal/nova/console.go` - Console endpoint handlers
- `/Users/I761222/git/o3k/pkg/hypervisor/console.go` - libvirt console access

**Contract Test**:
- `/Users/I761222/git/o3k/test/contract/nova/console_test.go` - Validate API compliance

---

## Horizon Integration

### Horizon Configuration (local_settings.py)

```python
# Console access configuration
CONSOLE_TYPE = 'novnc'

# noVNC proxy URL (must match O3K console URL)
OPENSTACK_API_VERSIONS = {
    "compute": "2.53",  # Modern console API version
}
```

### User Workflow

1. User clicks "Console" button on instance details page in Horizon
2. Horizon calls `POST /v2.1/servers/{id}/remote-consoles`
3. O3K generates console URL with JWT token
4. Horizon opens popup window with console URL
5. Browser loads noVNC viewer HTML page
6. noVNC JavaScript validates token and connects to proxy
7. Proxy streams VNC/SPICE data to browser via WebSocket

### Required Infrastructure

**noVNC Proxy Deployment** (separate from O3K):
```yaml
# docker-compose.yml
novnc:
  image: novnc/noVNC:latest
  ports:
    - "6080:6080"
  environment:
    - VNC_HOST=o3k-compute-node
    - TOKEN_VALIDATION_URL=http://o3k:35357/v3/auth/tokens
  volumes:
    - ./novnc-config:/etc/novnc
```

**Security**: Token validation ensures only authenticated users with valid tokens can access consoles. Tokens expire after 1 hour to limit exposure window.

---

## Testing

### Contract Test (gophercloud)

```go
func TestGetRemoteConsole_Contract(t *testing.T) {
    client := setupNovaClient(t)

    // Create test instance
    server := createTestServer(t, client)
    defer deleteTestServer(t, client, server.ID)

    // Get VNC console
    reqBody := map[string]interface{}{
        "remote_console": map[string]interface{}{
            "protocol": "vnc",
            "type":     "novnc",
        },
    }

    var result struct {
        RemoteConsole struct {
            Type     string `json:"type"`
            Protocol string `json:"protocol"`
            URL      string `json:"url"`
        } `json:"remote_console"`
    }

    err := client.Post(
        client.ServiceURL("servers", server.ID, "remote-consoles"),
        reqBody,
        &result,
        nil,
    ).ExtractErr()

    assert.NoError(t, err)
    assert.Equal(t, "novnc", result.RemoteConsole.Type)
    assert.Equal(t, "vnc", result.RemoteConsole.Protocol)
    assert.Contains(t, result.RemoteConsole.URL, "token=")
}

func TestGetConsoleInvalidState_Contract(t *testing.T) {
    client := setupNovaClient(t)

    // Create and stop instance
    server := createTestServer(t, client)
    defer deleteTestServer(t, client, server.ID)
    stopServer(t, client, server.ID)

    // Attempt to get console (should fail with 409)
    reqBody := map[string]interface{}{
        "remote_console": map[string]interface{}{
            "protocol": "vnc",
            "type":     "novnc",
        },
    }

    err := client.Post(
        client.ServiceURL("servers", server.ID, "remote-consoles"),
        reqBody,
        nil,
        nil,
    ).ExtractErr()

    assert.Error(t, err)
    assert.Contains(t, err.Error(), "409")
}
```

### Integration Test (bash)

```bash
# test/console_access_test.sh
#!/bin/bash

# Authenticate
TOKEN=$(openstack token issue -f value -c id)

# Create instance
SERVER_ID=$(openstack server create --flavor m1.small --image cirros test-vm -f value -c id)
sleep 5

# Get console URL
CONSOLE_URL=$(curl -s -X POST http://localhost:8774/v2.1/servers/$SERVER_ID/remote-consoles \
    -H "X-Auth-Token: $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"remote_console":{"protocol":"vnc","type":"novnc"}}' \
    | jq -r '.remote_console.url')

# Verify URL format
if [[ $CONSOLE_URL == *"token="* ]]; then
    echo "✓ Console URL generated with token"
else
    echo "✗ Console URL missing token"
    exit 1
fi

# Cleanup
openstack server delete $SERVER_ID
```

---

## Notes

- All endpoints implemented and tested
- Horizon compatibility validated
- Token-based security ensures console access control
- 1-hour token TTL balances security and usability
- Requires separate noVNC proxy deployment (not part of O3K binary)
