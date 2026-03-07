# Alternative: Access O3K via OpenStack CLI

Since Docker Hub has restrictions on Horizon images, here's an alternative way to interact with your O3K cloud:

## Option 1: OpenStack CLI (Recommended)

The OpenStack command-line client provides full access to all O3K features:

### Install OpenStack CLI

```bash
# Using pip
pip3 install python-openstackclient python-novaclient python-neutronclient python-cinderclient python-glanceclient

# Or using brew (macOS)
brew install openstackclient
```

### Configure Environment

Create `~/openstack-o3k.sh`:
```bash
export OS_AUTH_URL=http://localhost:35357/v3
export OS_PROJECT_NAME=default
export OS_PROJECT_DOMAIN_NAME=Default
export OS_USERNAME=admin
export OS_PASSWORD=secret
export OS_USER_DOMAIN_NAME=Default
export OS_IDENTITY_API_VERSION=3
export OS_IMAGE_API_VERSION=2
export OS_VOLUME_API_VERSION=3
export OS_COMPUTE_API_VERSION=2.1
```

### Use It

```bash
# Source the environment
source ~/openstack-o3k.sh

# List instances
openstack server list

# Launch an instance
openstack server create \
  --flavor m1.tiny \
  --image cirros \
  --network private \
  myvm

# List networks
openstack network list

# Create a volume
openstack volume create --size 10 myvolume

# List images
openstack image list

# Show quotas
openstack quota show

# Create security group rule
openstack security group rule create \
  --protocol tcp \
  --dst-port 22 \
  default
```

## Option 2: Build Horizon from Source

If you really want the web UI, you can build Horizon locally:

### Step 1: Clone Horizon

```bash
cd ~/
git clone https://opendev.org/openstack/horizon.git --branch stable/2024.1 --depth 1
cd horizon
```

### Step 2: Install Dependencies

```bash
pip3 install -r requirements.txt
pip3 install django
```

### Step 3: Configure Horizon

Create `openstack_dashboard/local/local_settings.py`:
```python
from horizon.settings import *

DEBUG = True
ALLOWED_HOSTS = ['*']

OPENSTACK_HOST = "127.0.0.1"
OPENSTACK_KEYSTONE_URL = "http://%s:35357/v3" % OPENSTACK_HOST

OPENSTACK_API_VERSIONS = {
    "identity": 3,
    "image": 2,
    "volume": 3,
    "compute": 2,
}

OPENSTACK_KEYSTONE_MULTIDOMAIN_SUPPORT = True
OPENSTACK_KEYSTONE_DEFAULT_DOMAIN = 'Default'
```

### Step 4: Run Horizon

```bash
python3 manage.py runserver 0.0.0.0:8000
```

Then access: http://localhost:8000/dashboard

## Option 3: Use Terraform

Terraform OpenStack provider works great with O3K:

```hcl
terraform {
  required_providers {
    openstack = {
      source = "terraform-provider-openstack/openstack"
    }
  }
}

provider "openstack" {
  auth_url    = "http://localhost:35357/v3"
  user_name   = "admin"
  password    = "secret"
  tenant_name = "default"
  domain_name = "Default"
}

resource "openstack_compute_instance_v2" "myvm" {
  name = "terraform-vm"
  flavor_name = "m1.tiny"
  image_name = "cirros"

  network {
    name = "private"
  }
}
```

## Option 4: Direct API Access

You can also use the APIs directly:

### Get Token

```bash
curl -i -X POST http://localhost:35357/v3/auth/tokens \
  -H "Content-Type: application/json" \
  -d '{
    "auth": {
      "identity": {
        "methods": ["password"],
        "password": {
          "user": {
            "name": "admin",
            "domain": {"name": "Default"},
            "password": "secret"
          }
        }
      },
      "scope": {
        "project": {
          "name": "default",
          "domain": {"name": "Default"}
        }
      }
    }
  }'
```

Save the `X-Subject-Token` header value.

### List Instances

```bash
TOKEN="your-token-here"
curl -H "X-Auth-Token: $TOKEN" http://localhost:8774/v2.1/servers
```

### Create Instance

```bash
curl -X POST http://localhost:8774/v2.1/servers \
  -H "X-Auth-Token: $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "server": {
      "name": "myvm",
      "flavorRef": "00000000-0000-0000-0000-000000000010",
      "imageRef": "your-image-id",
      "networks": [{"uuid": "your-network-id"}]
    }
  }'
```

## Recommendation

**I recommend using the OpenStack CLI (Option 1)** - it's:
- ✅ Easy to install
- ✅ Full-featured
- ✅ Well-documented
- ✅ No Docker issues
- ✅ Script-friendly

You'll have the same power as Horizon, just via command-line instead of web UI.

## Quick Demo with CLI

```bash
# Install
pip3 install python-openstackclient

# Configure
export OS_AUTH_URL=http://localhost:35357/v3
export OS_PROJECT_NAME=default
export OS_PROJECT_DOMAIN_NAME=Default
export OS_USERNAME=admin
export OS_PASSWORD=secret
export OS_USER_DOMAIN_NAME=Default
export OS_IDENTITY_API_VERSION=3

# Use it!
openstack server list
openstack network list
openstack volume list
openstack image list

# Launch a VM
openstack server create --flavor m1.tiny --image cirros --network private my-test-vm

# Watch it build
watch openstack server list
```

Much simpler than Docker! 🚀
