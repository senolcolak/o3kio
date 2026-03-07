# Horizon Custom Settings for O3K
# This file configures Horizon to connect to O3K running on the host

import os

# Debug mode (set to False in production)
DEBUG = os.environ.get('DEBUG', 'True') == 'True'

# Allow all hosts (configure properly in production)
ALLOWED_HOSTS = ['*']

# OpenStack settings - connect to O3K container
# Use "o3k" when running in Docker Compose, or "host.docker.internal" for host-based O3K
OPENSTACK_HOST = os.environ.get('OPENSTACK_HOST', 'o3k')
OPENSTACK_KEYSTONE_URL = "http://%s:5000/v3" % OPENSTACK_HOST

# Default region
OPENSTACK_KEYSTONE_DEFAULT_REGION = "RegionOne"

# API versions
OPENSTACK_API_VERSIONS = {
    "identity": 3,
    "image": 2,
    "volume": 3,
    "compute": 2,
}

# Keystone v3 settings
OPENSTACK_KEYSTONE_MULTIDOMAIN_SUPPORT = True
OPENSTACK_KEYSTONE_DEFAULT_DOMAIN = 'Default'

# Service endpoints
OPENSTACK_ENDPOINT_TYPE = "publicURL"

# Available regions
AVAILABLE_REGIONS = [
    ('http://%s:5000/v3' % os.environ.get('OPENSTACK_HOST', 'o3k'), 'RegionOne'),
]

# Console type
CONSOLE_TYPE = "AUTO"

# Enable Neutron
OPENSTACK_NEUTRON_NETWORK = {
    'enable_router': True,
    'enable_quotas': True,
    'enable_ipv6': False,
    'enable_distributed_router': False,
    'enable_ha_router': False,
    'enable_fip_topology_check': True,
    'enable_security_group': True,
    'enable_firewall': False,
    'enable_vpn': False,
    'profile_support': None,
    'supported_vnic_types': ['*'],
    'supported_provider_types': ['*'],
    'segmentation_id_range': {},
    'extra_provider_types': {},
    'enable_lb': False,
}

# Cinder features
OPENSTACK_CINDER_FEATURES = {
    'enable_backup': False,
}

# Hypervisor features
OPENSTACK_HYPERVISOR_FEATURES = {
    'can_set_mount_point': False,
    'can_set_password': False,
    'requires_keypair': False,
    'enable_quotas': True,
}

# Nova features
LAUNCH_INSTANCE_DEFAULTS = {
    'config_drive': False,
    'enable_scheduler_hints': False,
    'disable_image': False,
    'disable_instance_snapshot': False,
    'disable_volume': False,
    'disable_volume_snapshot': False,
}

# Image features
IMAGE_CUSTOM_PROPERTY_TITLES = {
    "architecture": "Architecture",
    "kernel_id": "Kernel ID",
    "ramdisk_id": "Ramdisk ID",
    "image_state": "Euca2ools state",
    "project_id": "Project ID",
    "image_type": "Image Type",
}

# Create default networks on first login
CREATE_DEFAULT_NETWORK = False

# Session configuration
SESSION_TIMEOUT = 3600  # 1 hour

# Password validation rules (optional)
# HORIZON_CONFIG = {
#     'password_autocomplete': 'on',
# }

# Logging
LOGGING = {
    'version': 1,
    'disable_existing_loggers': False,
    'formatters': {
        'verbose': {
            'format': '%(asctime)s %(levelname)s %(name)s %(message)s'
        },
    },
    'handlers': {
        'console': {
            'level': 'DEBUG' if DEBUG else 'INFO',
            'class': 'logging.StreamHandler',
            'formatter': 'verbose'
        },
    },
    'loggers': {
        'django': {
            'handlers': ['console'],
            'level': 'INFO',
            'propagate': False,
        },
        'horizon': {
            'handlers': ['console'],
            'level': 'DEBUG' if DEBUG else 'INFO',
            'propagate': False,
        },
        'openstack_dashboard': {
            'handlers': ['console'],
            'level': 'DEBUG' if DEBUG else 'INFO',
            'propagate': False,
        },
    },
}

# Security settings for production
if not DEBUG:
    CSRF_COOKIE_SECURE = True
    SESSION_COOKIE_SECURE = True
    SESSION_COOKIE_HTTPONLY = True

# Custom branding (optional)
SITE_BRANDING = "O3K OpenStack Dashboard"

# Available themes
AVAILABLE_THEMES = [
    ('default', 'Default', 'themes/default'),
    ('material', 'Material', 'themes/material'),
]

# Default theme
DEFAULT_THEME = 'default'

# Dashboard customization
HORIZON_CONFIG = {
    "password_autocomplete": "on",
    "help_url": "https://docs.openstack.org",
    "bug_url": "https://github.com/cobaltcore-dev/o3k/issues",
}

print("O3K Horizon configuration loaded successfully")
print(f"Connecting to Keystone at: {OPENSTACK_KEYSTONE_URL}")
