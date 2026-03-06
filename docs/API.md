# O3K API Documentation

## Keystone Identity Service (v3)

Base URL: `http://localhost:5000`

### Version Discovery

```bash
# Get API version
curl http://localhost:5000/
curl http://localhost:5000/v3
```

### Authentication

#### Unscoped Authentication (Password)

Get a token without project scope:

```bash
curl -X POST http://localhost:5000/v3/auth/tokens \
  -H "Content-Type: application/json" \
  -d '{
    "auth": {
      "identity": {
        "methods": ["password"],
        "password": {
          "user": {
            "name": "admin",
            "password": "secret"
          }
        }
      }
    }
  }' -i
```

The token will be in the `X-Subject-Token` response header.

#### Scoped Authentication (Project-scoped)

Get a project-scoped token with service catalog:

```bash
curl -X POST http://localhost:5000/v3/auth/tokens \
  -H "Content-Type: application/json" \
  -d '{
    "auth": {
      "identity": {
        "methods": ["password"],
        "password": {
          "user": {
            "name": "admin",
            "password": "secret"
          }
        }
      },
      "scope": {
        "project": {
          "name": "default"
        }
      }
    }
  }' -i
```

### Token Validation

```bash
# Validate a token
curl -X GET http://localhost:5000/v3/auth/tokens \
  -H "X-Auth-Token: YOUR_TOKEN" \
  -H "X-Subject-Token: TOKEN_TO_VALIDATE"
```

### Token Revocation

```bash
# Revoke a token (JWT tokens expire naturally, this is a no-op)
curl -X DELETE http://localhost:5000/v3/auth/tokens \
  -H "X-Auth-Token: YOUR_TOKEN" \
  -H "X-Subject-Token: TOKEN_TO_REVOKE"
```

### Users

```bash
# List users
curl http://localhost:5000/v3/users \
  -H "X-Auth-Token: YOUR_TOKEN"

# Get user details
curl http://localhost:5000/v3/users/USER_ID \
  -H "X-Auth-Token: YOUR_TOKEN"
```

### Projects

```bash
# List projects
curl http://localhost:5000/v3/projects \
  -H "X-Auth-Token: YOUR_TOKEN"

# Get project details
curl http://localhost:5000/v3/projects/PROJECT_ID \
  -H "X-Auth-Token: YOUR_TOKEN"
```

### Roles

```bash
# List roles
curl http://localhost:5000/v3/roles \
  -H "X-Auth-Token: YOUR_TOKEN"
```

## OpenStack CLI Testing

### Setup

```bash
# Install OpenStack client
pip install python-openstackclient

# Set environment variables
export OS_AUTH_URL=http://localhost:5000/v3
export OS_USERNAME=admin
export OS_PASSWORD=secret
export OS_PROJECT_NAME=default
export OS_USER_DOMAIN_NAME=default
export OS_PROJECT_DOMAIN_NAME=default
export OS_IDENTITY_API_VERSION=3
```

### Commands

```bash
# Test authentication
openstack token issue

# List users
openstack user list

# List projects
openstack project list

# List roles
openstack role list

# List endpoints (service catalog)
openstack catalog list

# List servers (when Nova is implemented)
openstack server list

# List networks (when Neutron is implemented)
openstack network list

# List volumes (when Cinder is implemented)
openstack volume list

# List images (when Glance is implemented)
openstack image list
```

## Service Catalog

After project-scoped authentication, the token response includes a service catalog:

```json
{
  "token": {
    "catalog": [
      {
        "type": "identity",
        "name": "keystone",
        "endpoints": [
          {
            "interface": "public",
            "region": "RegionOne",
            "url": "http://localhost:5000/v3"
          }
        ]
      },
      {
        "type": "compute",
        "name": "nova",
        "endpoints": [
          {
            "interface": "public",
            "region": "RegionOne",
            "url": "http://localhost:8774/v2.1"
          }
        ]
      },
      {
        "type": "network",
        "name": "neutron",
        "endpoints": [
          {
            "interface": "public",
            "region": "RegionOne",
            "url": "http://localhost:9696/v2.0"
          }
        ]
      },
      {
        "type": "volumev3",
        "name": "cinderv3",
        "endpoints": [
          {
            "interface": "public",
            "region": "RegionOne",
            "url": "http://localhost:8776/v3/{project_id}"
          }
        ]
      },
      {
        "type": "image",
        "name": "glance",
        "endpoints": [
          {
            "interface": "public",
            "region": "RegionOne",
            "url": "http://localhost:9292"
          }
        ]
      }
    ]
  }
}
```

## Default Credentials

- **Username:** `admin`
- **Password:** `secret`
- **Project:** `default`
- **Roles:** `admin`, `member`, `reader`

## Default Flavors

- `m1.tiny`: 1 vCPU, 512 MB RAM, 1 GB disk
- `m1.small`: 1 vCPU, 2 GB RAM, 20 GB disk
- `m1.medium`: 2 vCPUs, 4 GB RAM, 40 GB disk
- `m1.large`: 4 vCPUs, 8 GB RAM, 80 GB disk
- `m1.xlarge`: 8 vCPUs, 16 GB RAM, 160 GB disk

## Error Responses

O3K follows OpenStack error response format:

```json
{
  "error": {
    "message": "invalid credentials",
    "code": 401,
    "title": "Unauthorized"
  }
}
```

Common HTTP status codes:
- `200` - Success
- `201` - Created
- `204` - No Content (successful deletion)
- `400` - Bad Request
- `401` - Unauthorized (invalid credentials/token)
- `403` - Forbidden (insufficient permissions)
- `404` - Not Found
- `409` - Conflict (duplicate resource)
- `500` - Internal Server Error
- `503` - Service Unavailable (external dependency failure)
