# SPEC-003: Barbican - Key Management Service

**Status**: Draft
**Version**: 1.0
**Created**: 2026-03-09
**Priority**: High
**Depends On**: SPEC-001 (Modular Architecture)

## Overview

Implement Barbican, OpenStack's key management service, as an independent library-first module. Barbican manages encryption keys, X.509 certificates, and arbitrary secrets with HSM backend support.

## Goals

1. **Secret Management**: Store/retrieve encrypted secrets (passwords, API keys, certificates)
2. **Key Management**: Generate, store, rotate encryption keys
3. **Certificate Management**: Certificate authority operations
4. **HSM Support**: Hardware security module backends (SoftHSM initially, real HSM later)
5. **OpenStack API Compatible**: 100% Barbican v1 API compatibility
6. **Library-First**: Standalone library with CLI tool

## Non-Goals

- Custom encryption algorithms (use industry standards)
- Key escrow/recovery (future phase)
- Quantum-safe encryption (future phase)
- Multi-region key replication (future phase)

## Use Cases

### 1. Volume Encryption (Cinder Integration)
```
1. User creates encrypted volume
2. Cinder requests encryption key from Barbican
3. Barbican generates DEK (Data Encryption Key)
4. Barbican encrypts DEK with KEK (Key Encryption Key)
5. Barbican returns key reference to Cinder
6. Cinder uses key to encrypt volume
```

### 2. Certificate Storage (Nova/Neutron TLS)
```
1. Admin uploads TLS certificate
2. Barbican stores certificate + private key
3. Load balancer references certificate by ID
4. Barbican provides certificate for TLS termination
```

### 3. Application Secrets (Cloud-Init)
```
1. Developer stores API key in Barbican
2. Instance references secret in user-data
3. Cloud-init retrieves secret via metadata service
4. Application uses secret without hardcoding
```

## Architecture

### Components

```
pkg/barbican/
├── service.go           # Service interface
├── secrets.go           # Secret CRUD operations
├── keys.go              # Encryption key management
├── certificates.go      # Certificate operations
├── containers.go        # Secret containers
├── orders.go            # Asynchronous operations
├── acl.go               # Access control lists
├── backend/
│   ├── interface.go     # Backend interface
│   ├── database.go      # Database backend (encrypted at rest)
│   ├── softhsm.go       # SoftHSM backend
│   ├── pkcs11.go        # Hardware HSM via PKCS#11
│   └── vault.go         # HashiCorp Vault backend (optional)
└── cli/
    └── main.go          # CLI tool

cmd/o3k-barbican/
└── main.go              # Standalone binary

internal/barbican/
└── crypto/
    ├── aes.go           # AES-256-GCM encryption
    ├── rsa.go           # RSA key operations
    ├── x509.go          # Certificate operations
    └── kek.go           # Key encryption key management
```

### Secret Types

| Type | Description | Example |
|------|-------------|---------|
| `opaque` | Arbitrary binary data | Passwords, API keys |
| `symmetric` | Symmetric encryption key | AES-256 keys |
| `asymmetric` | Public/private key pair | RSA, ECDSA keys |
| `certificate` | X.509 certificate | TLS certificates |
| `passphrase` | Text passphrase | Database passwords |

### Backend Storage

**Option 1: Database (Encrypted at Rest)**
```
Secrets stored in PostgreSQL, encrypted with KEK
KEK stored in separate secure location (HSM, KMS)
Fast, simple, suitable for most deployments
```

**Option 2: SoftHSM (PKCS#11)**
```
Keys stored in SoftHSM token
Software HSM, FIPS 140-2 Level 1
Better security than database
```

**Option 3: Hardware HSM (PKCS#11)**
```
Keys stored in hardware HSM (Thales, Utimaco)
FIPS 140-2 Level 3
Production-grade security
```

**Option 4: HashiCorp Vault**
```
Delegate to external Vault instance
Enterprise key management
Requires Vault deployment
```

## Database Schema

```sql
-- Secrets metadata
CREATE TABLE barbican_secrets (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL,
    name VARCHAR(255),
    secret_type VARCHAR(50) NOT NULL,  -- opaque, symmetric, certificate, etc.
    algorithm VARCHAR(50),              -- aes, rsa, etc.
    bit_length INTEGER,
    mode VARCHAR(50),                   -- cbc, gcm, etc.
    status VARCHAR(50) DEFAULT 'ACTIVE',
    expiration TIMESTAMP,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    creator_id UUID,
    INDEX idx_project_secrets (project_id, status)
);

-- Secret data (encrypted)
CREATE TABLE barbican_secret_data (
    secret_id UUID PRIMARY KEY REFERENCES barbican_secrets(id) ON DELETE CASCADE,
    encrypted_data BYTEA NOT NULL,      -- Encrypted with KEK
    kek_id VARCHAR(255) NOT NULL,       -- Key encryption key identifier
    encryption_algorithm VARCHAR(50),   -- Algorithm used for encryption
    iv BYTEA,                           -- Initialization vector
    tag BYTEA                           -- Authentication tag (GCM)
);

-- Containers (group related secrets)
CREATE TABLE barbican_containers (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL,
    name VARCHAR(255),
    type VARCHAR(50),                   -- generic, rsa, certificate
    status VARCHAR(50) DEFAULT 'ACTIVE',
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE barbican_container_secrets (
    container_id UUID REFERENCES barbican_containers(id) ON DELETE CASCADE,
    secret_id UUID REFERENCES barbican_secrets(id) ON DELETE CASCADE,
    name VARCHAR(255),                  -- e.g., "certificate", "private_key"
    PRIMARY KEY (container_id, name)
);

-- Orders (asynchronous operations)
CREATE TABLE barbican_orders (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL,
    type VARCHAR(50) NOT NULL,          -- key, certificate
    status VARCHAR(50) DEFAULT 'PENDING',
    meta JSONB,                         -- Order-specific metadata
    secret_id UUID REFERENCES barbican_secrets(id),
    container_id UUID REFERENCES barbican_containers(id),
    error_status_code VARCHAR(10),
    error_reason TEXT,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- ACLs (access control)
CREATE TABLE barbican_secret_acls (
    id UUID PRIMARY KEY,
    secret_id UUID REFERENCES barbican_secrets(id) ON DELETE CASCADE,
    operation VARCHAR(50) NOT NULL,     -- read, write, delete
    project_access BOOLEAN DEFAULT true,
    users TEXT[],                       -- Array of user IDs
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(secret_id, operation)
);

-- Key rotation history
CREATE TABLE barbican_key_rotations (
    id UUID PRIMARY KEY,
    secret_id UUID REFERENCES barbican_secrets(id) ON DELETE CASCADE,
    old_kek_id VARCHAR(255),
    new_kek_id VARCHAR(255),
    rotated_at TIMESTAMP DEFAULT NOW(),
    rotated_by UUID
);
```

## API Endpoints

### Secrets
- `GET /v1/secrets` - List secrets (project scoped)
- `POST /v1/secrets` - Create secret (payload + metadata)
- `GET /v1/secrets/{id}` - Get secret metadata
- `GET /v1/secrets/{id}/payload` - Get secret data (decrypted)
- `PUT /v1/secrets/{id}` - Update secret (two-step creation)
- `DELETE /v1/secrets/{id}` - Delete secret
- `GET /v1/secrets/{id}/metadata` - Get all metadata

### Containers
- `GET /v1/containers` - List containers
- `POST /v1/containers` - Create container
- `GET /v1/containers/{id}` - Get container
- `DELETE /v1/containers/{id}` - Delete container

### Orders
- `GET /v1/orders` - List orders
- `POST /v1/orders` - Create order (async key generation)
- `GET /v1/orders/{id}` - Get order status
- `DELETE /v1/orders/{id}` - Cancel order

### ACLs
- `GET /v1/secrets/{id}/acl` - Get secret ACL
- `PUT /v1/secrets/{id}/acl` - Update secret ACL
- `PATCH /v1/secrets/{id}/acl` - Partial ACL update
- `DELETE /v1/secrets/{id}/acl` - Remove ACL

## Encryption Design

### Key Hierarchy

```
Master Key (KEK) - Stored in HSM or secure config
    ↓
Data Encryption Keys (DEK) - Per-secret encryption
    ↓
Secret Payload - Actual user data
```

### Encryption Flow

```go
// Store secret
func (s *Service) StoreSecret(payload []byte) (string, error) {
    // 1. Generate random DEK
    dek := generateRandomKey(32) // AES-256

    // 2. Encrypt payload with DEK
    iv := generateIV()
    ciphertext, tag := aesGCMEncrypt(payload, dek, iv)

    // 3. Encrypt DEK with KEK (from HSM)
    encryptedDEK := s.backend.EncryptKey(dek, s.kekID)

    // 4. Store encrypted payload + encrypted DEK
    s.db.Insert(SecretData{
        EncryptedData: ciphertext,
        EncryptedDEK: encryptedDEK,
        IV: iv,
        Tag: tag,
    })
}

// Retrieve secret
func (s *Service) GetSecret(id string) ([]byte, error) {
    // 1. Fetch encrypted data
    data := s.db.GetSecretData(id)

    // 2. Decrypt DEK with KEK (from HSM)
    dek := s.backend.DecryptKey(data.EncryptedDEK, s.kekID)

    // 3. Decrypt payload with DEK
    payload := aesGCMDecrypt(data.EncryptedData, dek, data.IV, data.Tag)

    return payload, nil
}
```

### KEK Management

**Configuration**:
```yaml
barbican:
  kek:
    mode: softhsm  # database, softhsm, pkcs11, vault
    rotation_period: 90d

  # Database mode
  database:
    kek_secret: base64-encoded-key  # 32 bytes for AES-256

  # SoftHSM mode
  softhsm:
    token_label: o3k-barbican
    user_pin: encrypted-pin
    key_label: master-kek

  # Hardware HSM mode
  pkcs11:
    library_path: /usr/lib/libpkcs11.so
    token_label: barbican-token
    user_pin: encrypted-pin
    key_label: master-kek

  # Vault mode
  vault:
    address: http://vault:8200
    token: vault-token
    transit_key: barbican-kek
```

## Fail-Early Strategy

All HSM operations have 1-second timeout:

```go
func (h *HSMBackend) EncryptKey(key []byte, kekID string) ([]byte, error) {
    ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
    defer cancel()

    result, err := h.pkcs11Encrypt(ctx, key, kekID)
    if err != nil {
        return nil, fmt.Errorf("HSM encryption failed (fail-early): %w", err)
    }
    return result, nil
}
```

If HSM is down:
- Secret creation fails immediately with 503 Service Unavailable
- Secret retrieval fails immediately with 503 Service Unavailable
- No queuing, no retries, operator alerted immediately

## Integration Points

### Cinder (Volume Encryption)
```go
// Cinder requests encryption key
volumeKey := barbicanClient.CreateSecret(SecretOpts{
    Name: "cinder-volume-" + volumeID,
    SecretType: "symmetric",
    Algorithm: "aes",
    BitLength: 256,
    Payload: generateVolumeKey(256),
})

// Store key reference in volume metadata
volume.EncryptionKeyID = volumeKey.ID

// Later, attach volume
key := barbicanClient.GetSecret(volume.EncryptionKeyID)
libvirt.AttachEncryptedVolume(volume, key)
```

### Glance (Image Signing)
```go
// Sign image
privateKey := barbicanClient.GetSecret(imageKeyID)
signature := signImage(imageData, privateKey)
image.Properties["signature"] = signature
image.Properties["signature_key_id"] = imageKeyID

// Verify image
publicKey := barbicanClient.GetSecret(imageKeyID)
valid := verifyImageSignature(imageData, signature, publicKey)
```

### Nova (Instance Secrets)
```go
// Inject secret into instance
secretPayload := barbicanClient.GetSecret(secretID)
cloudInit := generateCloudInit(map[string]string{
    "db_password": string(secretPayload),
})
```

## CLI Tool

```bash
# Create secret
o3k-barbican-cli secret store --name db-password --payload "secret123"

# Retrieve secret
o3k-barbican-cli secret get --id <uuid> --payload

# List secrets
o3k-barbican-cli secret list --project-id <uuid>

# Create key order
o3k-barbican-cli order create --type key --algorithm aes --bit-length 256

# Create certificate container
o3k-barbican-cli container create --type certificate \
  --name "web-cert" \
  --secret-ref certificate=<cert-id> \
  --secret-ref private_key=<key-id>
```

## Testing Strategy

### Unit Tests
- AES-GCM encryption/decryption
- KEK wrapping/unwrapping
- Secret metadata validation
- ACL enforcement
- Certificate parsing

### Integration Tests
```bash
#!/bin/bash
# test/integration/barbican_test.sh

# Start Barbican with SoftHSM
o3k-barbican --config test-config.yaml &

# Create secret
SECRET_ID=$(openstack secret store --name test --payload "secret" -f value -c "Secret href")

# Retrieve secret
PAYLOAD=$(openstack secret get $SECRET_ID --payload -f value)
assert_equals "$PAYLOAD" "secret"

# Test encryption
# (Verify secret is encrypted in database)
DB_DATA=$(psql -c "SELECT encrypted_data FROM barbican_secret_data WHERE secret_id='$SECRET_ID'")
assert_not_contains "$DB_DATA" "secret"

# Delete secret
openstack secret delete $SECRET_ID
```

### Contract Tests
```go
func TestBarbicanOpenStackCompatibility(t *testing.T) {
    // Use python-barbicanclient
    client := NewBarbicanClient("http://localhost:9311")

    // Store secret
    secret := client.CreateSecret(SecretCreateRequest{
        Name: "test-secret",
        Payload: "sensitive-data",
        PayloadContentType: "text/plain",
    })
    assert.NotEmpty(t, secret.SecretRef)

    // Retrieve secret
    retrieved := client.GetSecret(secret.SecretRef)
    assert.Equal(t, "sensitive-data", retrieved.Payload)
}
```

## Migration Path

### Phase 1: Core Secret Management (Weeks 1-2)
- Database schema
- Secret CRUD API
- Database backend (encrypted at rest)
- Basic CLI tool
- Unit tests

### Phase 2: SoftHSM Integration (Week 3)
- PKCS#11 integration
- SoftHSM backend
- KEK management
- Integration tests

### Phase 3: Containers & Orders (Week 4)
- Container API
- Asynchronous orders
- Certificate containers

### Phase 4: Cinder Integration (Week 5)
- Volume encryption support
- Key reference passing
- End-to-end volume encryption test

### Phase 5: Advanced Features (Week 6)
- ACLs
- Key rotation
- Hardware HSM support
- Audit logging

## Success Criteria

- [ ] python-barbicanclient works with all endpoints
- [ ] Secrets encrypted at rest (verify in database)
- [ ] SoftHSM backend functional
- [ ] Cinder volume encryption uses Barbican keys
- [ ] CLI tool works for common operations
- [ ] Fail-early: HSM failures return < 1s
- [ ] Contract tests pass
- [ ] OpenStack CLI integration works
- [ ] Documentation complete
- [ ] Security audit passes

## Security Considerations

1. **KEK Protection**: Master key never stored in plain text
2. **Key Rotation**: Automated KEK rotation every 90 days
3. **Access Logging**: All secret access audited
4. **Secure Deletion**: Cryptographic shredding on delete
5. **Memory Wiping**: Zero out secrets after use
6. **Rate Limiting**: Prevent brute-force attacks
7. **HSM Failover**: Detect HSM failures immediately

## Performance Targets

- Secret storage: < 50ms
- Secret retrieval: < 20ms (with HSM)
- Secret listing: < 100ms (1000 secrets)
- HSM operation timeout: 1s (fail-early)

## References

- OpenStack Barbican API v1
- NIST SP 800-57 (Key Management)
- PKCS#11 Cryptographic Token Interface
- AES-GCM (RFC 5116)
- FIPS 140-2 Standard
