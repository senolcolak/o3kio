# O3K Horizon Configuration
# Custom settings for OpenStack Horizon to connect to O3K

import os

# Import base settings
from openstack_dashboard.settings import *

# Debug mode for testing
DEBUG = True
ALLOWED_HOSTS = ['*']

# Keystone endpoint
OPENSTACK_HOST = os.environ.get('OPENSTACK_HOST', 'host.docker.internal')
OPENSTACK_KEYSTONE_URL = "http://%s:35357/v3" % OPENSTACK_HOST

# API endpoints
OPENSTACK_API_VERSIONS = {
    "identity": 3,
    "compute": 2,
    "volume": 3,
    "network": 2,
    "image": 2,
}

# Keystone settings
OPENSTACK_KEYSTONE_DEFAULT_ROLE = "member"
OPENSTACK_KEYSTONE_MULTIDOMAIN_SUPPORT = False
OPENSTACK_KEYSTONE_DEFAULT_DOMAIN = "Default"

# Region
OPENSTACK_REGION = "RegionOne"

# Service catalog
OPENSTACK_ENDPOINT_TYPE = "publicURL"

# SSL settings
OPENSTACK_SSL_NO_VERIFY = True
OPENSTACK_SSL_CACERT = None

# Session settings
SECRET_KEY = os.environ.get('SECRET_KEY', 'o3k-horizon-secret-key')
SESSION_ENGINE = 'django.contrib.sessions.backends.cache'
SESSION_TIMEOUT = 3600

# Cache settings
CACHES = {
    'default': {
        'BACKEND': 'django.core.cache.backends.locmem.LocMemCache',
    }
}

# Enable all services
HORIZON_CONFIG = {
    'dashboards': ('project', 'admin', 'identity'),
    'default_dashboard': 'project',
    'user_home': 'openstack_dashboard.views.get_user_home',
    'ajax_queue_limit': 10,
    'auto_fade_alerts': {
        'delay': 3000,
        'fade_duration': 1500,
        'types': ['alert-success', 'alert-info']
    },
    'help_url': "https://docs.openstack.org",
    'exceptions': {'recoverable': [], 'not_found': [], 'unauthorized': []},
    'modal_backdrop': 'static',
    'angular_modules': [],
    'js_files': [],
}

# Enable panels
INSTALLED_APPS = list(INSTALLED_APPS) + [
    'openstack_dashboard.dashboards.project',
    'openstack_dashboard.dashboards.admin',
    'openstack_dashboard.dashboards.identity',
]

# API result limits
API_RESULT_LIMIT = 1000
API_RESULT_PAGE_SIZE = 20

# Image upload settings
IMAGE_CUSTOM_PROPERTY_TITLES = {
    "architecture": "Architecture",
    "kernel_id": "Kernel ID",
    "ramdisk_id": "Ramdisk ID",
    "image_state": "Euca2ools state",
    "project_id": "Project ID",
    "image_type": "Image Type",
}

# Network settings
OPENSTACK_NEUTRON_NETWORK = {
    'enable_router': True,
    'enable_quotas': True,
    'enable_ipv6': False,
    'enable_distributed_router': False,
    'enable_ha_router': False,
    'enable_lb': False,
    'enable_firewall': False,
    'enable_vpn': False,
    'enable_fip_topology_check': False,
    'profile_support': None,
}

# Hypervisor settings
OPENSTACK_HYPERVISOR_FEATURES = {
    'can_set_mount_point': False,
    'can_set_password': False,
    'requires_keypair': False,
    'enable_quotas': True
}

# Console type
CONSOLE_TYPE = "AUTO"

# Password validation (disable for testing)
PASSWORD_VALIDATOR = {
    'regex': '.*',
    'help_text': "Any password is accepted"
}

# Logging
LOGGING = {
    'version': 1,
    'disable_existing_loggers': False,
    'handlers': {
        'console': {
            'level': 'DEBUG',
            'class': 'logging.StreamHandler',
        },
    },
    'loggers': {
        'django': {
            'handlers': ['console'],
            'level': 'INFO',
        },
        'openstack_dashboard': {
            'handlers': ['console'],
            'level': 'DEBUG',
        },
        'openstack_auth': {
            'handlers': ['console'],
            'level': 'DEBUG',
        },
    },
}

print("O3K Horizon configuration loaded")
print(f"Keystone URL: {OPENSTACK_KEYSTONE_URL}")
