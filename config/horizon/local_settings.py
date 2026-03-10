import os

from django.utils.translation import gettext_lazy as _

DEBUG = False

# O3K endpoints
OPENSTACK_HOST = os.environ.get('OPENSTACK_HOST', 'localhost')
OPENSTACK_KEYSTONE_URL = "http://%s:35357/v3" % OPENSTACK_HOST
OPENSTACK_KEYSTONE_DEFAULT_ROLE = "member"

# API versions
OPENSTACK_API_VERSIONS = {
    "identity": 3,
    "image": 2,
    "volume": 3,
    "compute": 2,
}

# Features
OPENSTACK_HYPERVISOR_FEATURES = {
    'can_set_mount_point': False,
    'can_set_password': False,
}

OPENSTACK_CINDER_FEATURES = {
    'enable_backup': False,
}

OPENSTACK_NEUTRON_NETWORK = {
    'enable_router': True,
    'enable_quotas': False,
    'enable_ipv6': False,
    'enable_distributed_router': False,
    'enable_ha_router': False,
    'enable_lb': False,
    'enable_firewall': False,
    'enable_vpn': False,
    'enable_fip_topology_check': False,
}

# Allowed hosts
ALLOWED_HOSTS = ['*']

# Security
SECRET_KEY = 'o3k-horizon-secret-key-change-in-production'
SESSION_ENGINE = 'django.contrib.sessions.backends.cache'

# Caching (use local memory cache)
CACHES = {
    'default': {
        'BACKEND': 'django.core.cache.backends.locmem.LocMemCache',
    }
}

# Logging
LOGGING = {
    'version': 1,
    'disable_existing_loggers': False,
    'handlers': {
        'console': {
            'level': 'INFO',
            'class': 'logging.StreamHandler',
        },
    },
    'loggers': {
        'horizon': {
            'handlers': ['console'],
            'level': 'INFO',
            'propagate': False,
        },
        'openstack_dashboard': {
            'handlers': ['console'],
            'level': 'INFO',
            'propagate': False,
        },
        'keystoneclient': {
            'handlers': ['console'],
            'level': 'INFO',
            'propagate': False,
        },
    }
}

# Additional settings
COMPRESS_OFFLINE = True
WEBROOT = '/'
LOGIN_URL = WEBROOT + 'auth/login/'
LOGOUT_URL = WEBROOT + 'auth/logout/'
LOGIN_REDIRECT_URL = WEBROOT
