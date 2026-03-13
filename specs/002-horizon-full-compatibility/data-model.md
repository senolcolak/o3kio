# Data Model: Horizon Full Compatibility

**Feature**: OpenStack Horizon 100% Compatibility
**Date**: 2026-03-13
**Plan**: [plan.md](./plan.md)
**Research**: [research.md](./research.md)

## Overview

Data model additions/modifications for Horizon compatibility. Most features use existing database schema. Only console session tracking and migration records require consideration.

## Entity Definitions

### Entity 1: Console Session (Optional - In-Memory)

**Purpose**: Track temporary VNC/SPICE console access tokens

**Implementation Decision**: Use JWT tokens (no database table needed)

**JWT Payload**:
```json
{
  "server_id": "uuid",
  "console_type": "novnc|spice|serial|rdp",
  "issued_at": "2026-03-13T10:00:00Z",
  "expires_at": "2026-03-13T11:00:00Z"
}
```

**Rationale**: Console access is temporary (1-hour tokens). JWT provides stateless, time-limited tokens without database overhead. Aligns with existing O3K token infrastructure.

**Validation Rules**:
- Token must be valid JWT signature (HMAC-SHA256)
- Token must not be expired
- server_id must reference existing instance
- console_type must be one of: novnc, spice, serial, rdp

**No Database Table**: Tokens validated via JWT verification, no persistence needed.

---

### Entity 2: Migration Record (Existing Table - Verification Needed)

**Purpose**: Track instance migrations for `migrate` and `evacuate` actions

**Expected Existing Schema** (verify in database):
```sql
CREATE TABLE IF NOT EXISTS migrations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    instance_id UUID NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    source_host VARCHAR(255) NOT NULL,
    dest_host VARCHAR(255),
    status VARCHAR(50) NOT NULL DEFAULT 'pending',  -- pending, running, completed, failed
    migration_type VARCHAR(50) NOT NULL,  -- migration, evacuation, resize
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_migrations_instance_id ON migrations(instance_id);
CREATE INDEX idx_migrations_status ON migrations(status);
```

**Fields**:
- `id`: UUID, primary key
- `instance_id`: UUID, foreign key to instances table
- `source_host`: VARCHAR(255), hostname of source compute node
- `dest_host`: VARCHAR(255), hostname of destination compute node (nullable during host selection)
- `status`: VARCHAR(50), migration status (pending → running → completed/failed)
- `migration_type`: VARCHAR(50), type of migration (migration, evacuation, resize)
- `created_at`: TIMESTAMP, migration initiation time
- `updated_at`: TIMESTAMP, last status update time

**Relationships**:
- Belongs to: `instances` (one instance can have multiple migrations)
- Cascade: ON DELETE CASCADE (delete migration records if instance deleted)

**Validation Rules**:
- `instance_id` must reference existing instance
- `status` must be one of: pending, running, completed, failed
- `migration_type` must be one of: migration, evacuation, resize
- `source_host` must not be empty
- `dest_host` can be NULL during initial creation (host selection pending)

**State Transitions**:
```
pending → running → completed
              └──→ failed
```

**Lifecycle**:
1. User calls `POST /servers/{id}/action` with `migrate` or `evacuate`
2. Create migration record with status=pending
3. Select destination host (if not provided)
4. Update status=running, start migration
5. Update status=completed/failed based on result
6. Update instance.host to dest_host if completed

**Query Patterns**:
```sql
-- Get migrations for an instance
SELECT * FROM migrations
WHERE instance_id = $1
ORDER BY created_at DESC;

-- Get active migrations
SELECT * FROM migrations
WHERE status IN ('pending', 'running');

-- Get failed migrations
SELECT * FROM migrations
WHERE status = 'failed'
AND created_at > NOW() - INTERVAL '24 hours';
```

**No Migration Needed**: Table likely exists from previous migrations. Verify with:
```sql
SELECT column_name, data_type
FROM information_schema.columns
WHERE table_name = 'migrations';
```

If table missing, create with migration 048 or 049.

---

### Entity 3: Server Security Group Association (Existing Table)

**Purpose**: Many-to-many relationship between instances and security groups

**Expected Existing Schema** (verify in database):
```sql
CREATE TABLE IF NOT EXISTS server_security_groups (
    server_id UUID NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    security_group_id UUID NOT NULL REFERENCES security_groups(id) ON DELETE CASCADE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (server_id, security_group_id)
);

CREATE INDEX idx_server_security_groups_server_id ON server_security_groups(server_id);
CREATE INDEX idx_server_security_groups_sg_id ON server_security_groups(security_group_id);
```

**Fields**:
- `server_id`: UUID, foreign key to instances table
- `security_group_id`: UUID, foreign key to security_groups table
- `created_at`: TIMESTAMP, association creation time

**Relationships**:
- Belongs to: `instances` (many servers)
- Belongs to: `security_groups` (many security groups)
- Cascade: ON DELETE CASCADE (delete associations if server or SG deleted)

**Validation Rules**:
- `server_id` must reference existing instance
- `security_group_id` must reference existing security group
- Primary key enforces uniqueness (no duplicate associations)
- At least one security group must remain associated (enforced in application logic)

**Lifecycle**:
1. Instance created with default security group (INSERT during instance creation)
2. User calls `addSecurityGroup` (INSERT new association)
3. User calls `removeSecurityGroup` (DELETE association, prevent if last SG)
4. Instance deleted (CASCADE DELETE all associations)

**Query Patterns**:
```sql
-- Get security groups for a server
SELECT sg.*
FROM security_groups sg
JOIN server_security_groups ssg ON sg.id = ssg.security_group_id
WHERE ssg.server_id = $1;

-- Check if association exists
SELECT EXISTS(
    SELECT 1 FROM server_security_groups
    WHERE server_id = $1 AND security_group_id = $2
);

-- Count security groups for server
SELECT COUNT(*)
FROM server_security_groups
WHERE server_id = $1;
```

**No Migration Needed**: Table likely exists. Verify in database.

---

### Entity 4: Network Topology (Computed - No Database Table)

**Purpose**: Network topology visualization data for Horizon

**Implementation Decision**: Compute on-demand from existing tables (networks, subnets, ports, routers)

**Data Sources**:
- `networks` table: Network nodes
- `subnets` table: Subnet details
- `ports` table: Instance connections, router interfaces
- `routers` table: Router nodes
- `instances` table: Instance nodes

**Computed Structure** (in-memory):
```go
type TopologyGraph struct {
    Networks  []NetworkNode
    Routers   []RouterNode
    Instances []InstanceNode
    Edges     []TopologyEdge
}

type NetworkNode struct {
    ID      string
    Name    string
    Subnets []SubnetNode
}

type SubnetNode struct {
    ID       string
    Name     string
    CIDR     string
    Gateway  string
}

type RouterNode struct {
    ID   string
    Name string
    Interfaces []RouterInterface
}

type RouterInterface struct {
    NetworkID string
    SubnetID  string
    IPAddress string
}

type InstanceNode struct {
    ID        string
    Name      string
    NetworkID string
    IPAddress string
}

type TopologyEdge struct {
    Source     string  // network/router/instance ID
    Target     string
    EdgeType   string  // "subnet", "router_interface", "instance_port"
}
```

**Query Logic**:
```sql
-- Get all networks with subnets
SELECT n.id, n.name, s.id, s.name, s.cidr, s.gateway_ip
FROM networks n
LEFT JOIN subnets s ON n.id = s.network_id
WHERE n.project_id = $1;

-- Get router interfaces (ports with device_owner=network:router_interface)
SELECT p.device_id AS router_id, p.network_id, p.fixed_ips
FROM ports p
WHERE p.project_id = $1
AND p.device_owner = 'network:router_interface';

-- Get instance connections
SELECT p.device_id AS instance_id, p.network_id, p.fixed_ips
FROM ports p
WHERE p.project_id = $1
AND p.device_owner LIKE 'compute:%';
```

**Rationale**: Horizon builds topology client-side. No custom endpoint needed. Existing Neutron APIs provide all required data.

**No Database Table**: Computed on-demand, not persisted.

---

### Entity 5: Instance Backups (Glance Images)

**Purpose**: Track instance backups created by `createBackup` action

**Implementation Decision**: Use existing Glance `images` table with special properties

**Existing Schema** (no changes needed):
```sql
-- images table already exists
-- Backups distinguished by properties JSON field
```

**Backup Image Properties**:
```json
{
  "backup_type": "daily|weekly|monthly",
  "instance_uuid": "source-instance-uuid",
  "backup_name": "user-provided-name",
  "created_by_action": "createBackup"
}
```

**Query Pattern**:
```sql
-- Get backups for an instance
SELECT id, name, created_at, properties
FROM images
WHERE properties->>'instance_uuid' = $1
AND properties->>'backup_type' IS NOT NULL
ORDER BY created_at DESC;

-- Apply rotation (delete oldest)
DELETE FROM images
WHERE properties->>'instance_uuid' = $1
AND properties->>'backup_type' = $2
AND id NOT IN (
    SELECT id FROM images
    WHERE properties->>'instance_uuid' = $1
    AND properties->>'backup_type' = $2
    ORDER BY created_at DESC
    LIMIT $3  -- rotation count
);
```

**No Migration Needed**: Uses existing images table.

---

## Database Migrations

### Migration Status

**Required Migrations**: NONE (all tables already exist or not needed)

**Verification Checklist**:
- [ ] Verify `migrations` table exists with expected schema
- [ ] Verify `server_security_groups` table exists with expected schema
- [ ] Verify `images` table has JSONB `properties` column

**If Tables Missing** (create migration):
```sql
-- migration 048_server_actions_support.up.sql (if needed)
CREATE TABLE IF NOT EXISTS migrations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    instance_id UUID NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    source_host VARCHAR(255) NOT NULL,
    dest_host VARCHAR(255),
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    migration_type VARCHAR(50) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_migrations_instance_id ON migrations(instance_id);
CREATE INDEX idx_migrations_status ON migrations(status);

CREATE TABLE IF NOT EXISTS server_security_groups (
    server_id UUID NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    security_group_id UUID NOT NULL REFERENCES security_groups(id) ON DELETE CASCADE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (server_id, security_group_id)
);

CREATE INDEX idx_server_security_groups_server_id ON server_security_groups(server_id);
CREATE INDEX idx_server_security_groups_sg_id ON server_security_groups(security_group_id);
```

---

## Entity Relationships

### ER Diagram (Text Format)

```
instances (existing)
    ├─── migrations (1:N) - track instance migrations
    ├─── server_security_groups (N:M) - SG associations
    └─── ports (1:N) - network connections (existing)

security_groups (existing)
    └─── server_security_groups (N:M) - instance associations

images (existing)
    └─── backups (1:N) - instances have many backups (via properties)

networks (existing)
    ├─── subnets (1:N)
    └─── ports (1:N)

routers (existing)
    └─── router_interfaces via ports (1:N)
```

### Key Relationships Summary

| Parent Entity | Child Entity | Type | Cascade | Purpose |
|---------------|--------------|------|---------|---------|
| instances | migrations | 1:N | CASCADE | Track migrations |
| instances | server_security_groups | N:M | CASCADE | SG associations |
| security_groups | server_security_groups | N:M | CASCADE | SG associations |
| images | backups | 1:N (logical) | - | Instance backups |

---

## Indexes

### Required Indexes

**Existing** (verify):
- `idx_migrations_instance_id` on migrations(instance_id)
- `idx_migrations_status` on migrations(status)
- `idx_server_security_groups_server_id` on server_security_groups(server_id)
- `idx_server_security_groups_sg_id` on server_security_groups(security_group_id)

**Performance Considerations**:
- Instance ID lookups: Fast (indexed)
- Security group membership checks: Fast (indexed primary key)
- Migration history queries: Fast (indexed on instance_id)
- Backup queries: Moderate (JSONB property scan, consider GIN index if slow)

**Optional Performance Index** (if backup queries slow):
```sql
-- GIN index on images.properties JSONB column
CREATE INDEX idx_images_properties ON images USING GIN (properties);
```

---

## Data Validation

### Application-Level Validation

**Console Tokens**:
- JWT signature validation (cryptographic)
- Expiration check (time.Now() < token.ExpiresAt)
- Server existence check (SELECT FROM instances WHERE id = token.ServerID)

**Migrations**:
- Instance must exist and be in valid state for migration
- Source host must be current instance.host
- Dest host must be different from source host
- Status transitions must follow valid path

**Security Groups**:
- Both server and security group must exist
- Duplicate association check (primary key enforcement)
- Last SG protection (COUNT > 1 before delete)

**Backups**:
- Backup name length ≤ 255 characters
- Backup type must be: daily, weekly, monthly
- Rotation count must be ≥ 1

---

## Summary

**New Tables**: 0 (all required tables likely already exist)

**Modified Tables**: 0 (no schema changes needed)

**Verification Required**: Check `migrations` and `server_security_groups` tables exist

**Implementation Focus**: Application logic using existing database schema

**Performance**: All critical queries covered by existing indexes
