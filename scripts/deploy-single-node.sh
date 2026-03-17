#!/bin/bash
#
# O3K Single-Node Deployment Script
# Interactively configures and deploys O3K with KVM hypervisor on a single host
#
# Usage: sudo ./scripts/deploy-single-node.sh
#

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Banner
show_banner() {
    cat << "EOF"
╔═══════════════════════════════════════════════════════════╗
║                                                           ║
║   O3K Single-Node Deployment Script                      ║
║   Deploy O3K + KVM Hypervisor on Single Host            ║
║                                                           ║
╚═══════════════════════════════════════════════════════════╝
EOF
    echo ""
}

# Check if running as root
check_root() {
    if [ "$EUID" -ne 0 ]; then
        log_error "This script must be run as root (use sudo)"
        exit 1
    fi
}

# Detect OS
detect_os() {
    if [ -f /etc/os-release ]; then
        . /etc/os-release
        OS=$ID
        OS_VERSION=$VERSION_ID
    else
        log_error "Cannot detect OS. /etc/os-release not found."
        exit 1
    fi

    log_info "Detected OS: $OS $OS_VERSION"

    # Check if supported
    case "$OS" in
        ubuntu)
            if [ "$OS_VERSION" != "24.04" ] && [ "$OS_VERSION" != "22.04" ]; then
                log_warning "OS version $OS_VERSION not officially tested. Recommended: Ubuntu 24.04 LTS"
            fi
            ;;
        debian)
            if [ "$OS_VERSION" != "12" ]; then
                log_warning "OS version $OS_VERSION not officially tested. Recommended: Debian 12"
            fi
            ;;
        *)
            log_error "Unsupported OS: $OS. Supported: Ubuntu 24.04/22.04, Debian 12"
            exit 1
            ;;
    esac
}

# Check hardware requirements
check_hardware() {
    log_info "Checking hardware requirements..."

    # Check CPU cores
    CPU_CORES=$(nproc)
    if [ "$CPU_CORES" -lt 4 ]; then
        log_error "Insufficient CPU cores: $CPU_CORES (minimum: 4)"
        exit 1
    fi
    log_success "CPU cores: $CPU_CORES ✓"

    # Check RAM (in GB)
    TOTAL_RAM=$(free -g | awk '/^Mem:/{print $2}')
    if [ "$TOTAL_RAM" -lt 16 ]; then
        log_error "Insufficient RAM: ${TOTAL_RAM}GB (minimum: 16GB)"
        exit 1
    fi
    log_success "RAM: ${TOTAL_RAM}GB ✓"

    # Check disk space (in GB)
    DISK_SPACE=$(df -BG / | awk 'NR==2{print $4}' | sed 's/G//')
    if [ "$DISK_SPACE" -lt 100 ]; then
        log_error "Insufficient disk space: ${DISK_SPACE}GB (minimum: 100GB)"
        exit 1
    fi
    log_success "Disk space: ${DISK_SPACE}GB ✓"

    # Check virtualization support
    if ! egrep -c '(vmx|svm)' /proc/cpuinfo > /dev/null; then
        log_error "CPU does not support virtualization (VT-x/AMD-V)"
        exit 1
    fi
    log_success "Virtualization support: enabled ✓"
}

# Interactive configuration
get_configuration() {
    log_info "Starting interactive configuration..."
    echo ""

    # Hostname
    read -p "Enter hostname for this node [o3k-demo]: " HOSTNAME
    HOSTNAME=${HOSTNAME:-o3k-demo}

    # Host IP address
    DEFAULT_IP=$(ip route get 8.8.8.8 | awk '{print $7; exit}')
    read -p "Enter host IP address [$DEFAULT_IP]: " HOST_IP
    HOST_IP=${HOST_IP:-$DEFAULT_IP}

    # Network interface
    DEFAULT_IFACE=$(ip route | grep default | awk '{print $5}')
    read -p "Enter primary network interface [$DEFAULT_IFACE]: " NETWORK_IFACE
    NETWORK_IFACE=${NETWORK_IFACE:-$DEFAULT_IFACE}

    # Gateway
    DEFAULT_GATEWAY=$(ip route | grep default | awk '{print $3}')
    read -p "Enter network gateway [$DEFAULT_GATEWAY]: " GATEWAY
    GATEWAY=${GATEWAY:-$DEFAULT_GATEWAY}

    # DNS servers
    read -p "Enter DNS servers [8.8.8.8,8.8.4.4]: " DNS_SERVERS
    DNS_SERVERS=${DNS_SERVERS:-"8.8.8.8,8.8.4.4"}

    # Database password
    read -sp "Enter PostgreSQL password for O3K database: " DB_PASSWORD
    echo ""
    if [ -z "$DB_PASSWORD" ]; then
        DB_PASSWORD=$(openssl rand -base64 32)
        log_warning "No password provided. Generated random password: $DB_PASSWORD"
    fi

    # JWT secret
    JWT_SECRET=$(openssl rand -base64 32)
    log_info "Generated JWT secret for Keystone"

    # Storage paths
    read -p "Enter storage path for volumes [/var/lib/o3k/volumes]: " VOLUME_PATH
    VOLUME_PATH=${VOLUME_PATH:-/var/lib/o3k/volumes}

    read -p "Enter storage path for images [/var/lib/o3k/images]: " IMAGE_PATH
    IMAGE_PATH=${IMAGE_PATH:-/var/lib/o3k/images}

    read -p "Enter storage path for instances [/var/lib/o3k/instances]: " INSTANCE_PATH
    INSTANCE_PATH=${INSTANCE_PATH:-/var/lib/o3k/instances}

    # Horizon deployment
    read -p "Deploy Horizon dashboard? [Y/n]: " DEPLOY_HORIZON
    DEPLOY_HORIZON=${DEPLOY_HORIZON:-Y}

    # Confirmation
    echo ""
    log_info "Configuration Summary:"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "  Hostname:           $HOSTNAME"
    echo "  Host IP:            $HOST_IP"
    echo "  Network Interface:  $NETWORK_IFACE"
    echo "  Gateway:            $GATEWAY"
    echo "  DNS Servers:        $DNS_SERVERS"
    echo "  Volume Path:        $VOLUME_PATH"
    echo "  Image Path:         $IMAGE_PATH"
    echo "  Instance Path:      $INSTANCE_PATH"
    echo "  Deploy Horizon:     $DEPLOY_HORIZON"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""

    read -p "Proceed with installation? [Y/n]: " CONFIRM
    CONFIRM=${CONFIRM:-Y}

    if [[ ! "$CONFIRM" =~ ^[Yy]$ ]]; then
        log_error "Installation cancelled by user"
        exit 1
    fi
}

# Update system
update_system() {
    log_info "Updating system packages..."
    apt-get update -qq
    DEBIAN_FRONTEND=noninteractive apt-get upgrade -y -qq
    log_success "System updated"
}

# Set hostname
configure_hostname() {
    log_info "Configuring hostname: $HOSTNAME"
    hostnamectl set-hostname "$HOSTNAME"

    # Update /etc/hosts
    if ! grep -q "127.0.0.1.*$HOSTNAME" /etc/hosts; then
        echo "127.0.0.1 $HOSTNAME" >> /etc/hosts
    fi

    if ! grep -q "$HOST_IP.*$HOSTNAME" /etc/hosts; then
        echo "$HOST_IP $HOSTNAME" >> /etc/hosts
    fi

    log_success "Hostname configured"
}

# Install KVM and libvirt
install_kvm() {
    log_info "Installing KVM and libvirt..."

    DEBIAN_FRONTEND=noninteractive apt-get install -y -qq \
        qemu-kvm \
        libvirt-daemon-system \
        libvirt-clients \
        bridge-utils \
        virt-manager \
        cpu-checker \
        > /dev/null

    # Verify KVM
    if kvm-ok | grep -q "KVM acceleration can be used"; then
        log_success "KVM installed and acceleration available"
    else
        log_error "KVM acceleration not available. Check BIOS virtualization settings."
        exit 1
    fi

    # Start libvirt
    systemctl enable --now libvirtd
    log_success "libvirt enabled and started"
}

# Configure networking
configure_networking() {
    log_info "Configuring network bridge..."

    # Backup existing netplan config
    mkdir -p /etc/netplan/backup
    cp /etc/netplan/*.yaml /etc/netplan/backup/ 2>/dev/null || true

    # Create bridge configuration
    # Parse DNS servers into proper YAML array format
    DNS_ARRAY=""
    IFS=',' read -ra DNS_LIST <<< "$DNS_SERVERS"
    for dns in "${DNS_LIST[@]}"; do
        DNS_ARRAY="$DNS_ARRAY        - ${dns// /}\n"
    done

    cat > /etc/netplan/01-o3k-bridge.yaml <<EOF
network:
  version: 2
  renderer: networkd
  ethernets:
    $NETWORK_IFACE:
      dhcp4: no
      dhcp6: no
  bridges:
    br-ext:
      interfaces: [$NETWORK_IFACE]
      addresses:
        - $HOST_IP/24
      routes:
        - to: default
          via: $GATEWAY
      nameservers:
        addresses:
$(echo -e "$DNS_ARRAY")      dhcp4: no
      dhcp6: no
      parameters:
        stp: false
        forward-delay: 0
EOF

    # Set proper permissions
    chmod 600 /etc/netplan/01-o3k-bridge.yaml

    # Apply netplan
    log_warning "Applying network configuration. SSH connection may be interrupted briefly..."
    netplan apply

    # Verify bridge
    sleep 2
    if ip addr show br-ext > /dev/null 2>&1; then
        log_success "Bridge br-ext created successfully"
    else
        log_error "Failed to create bridge br-ext"
        exit 1
    fi

    # Enable IP forwarding
    cat >> /etc/sysctl.conf <<EOF

# O3K networking
net.ipv4.ip_forward=1
net.ipv4.conf.all.forwarding=1
net.ipv6.conf.all.forwarding=1
net.bridge.bridge-nf-call-iptables=1
net.bridge.bridge-nf-call-ip6tables=1
EOF

    sysctl -p > /dev/null
    log_success "IP forwarding enabled"
}

# Install PostgreSQL
install_postgresql() {
    log_info "Installing PostgreSQL 18..."

    # Add PostgreSQL repository
    apt-get install -y -qq curl ca-certificates > /dev/null
    curl -fsSL https://www.postgresql.org/media/keys/ACCC4CF8.asc | gpg --dearmor -o /usr/share/keyrings/postgresql-keyring.gpg
    echo "deb [signed-by=/usr/share/keyrings/postgresql-keyring.gpg] http://apt.postgresql.org/pub/repos/apt $(lsb_release -cs)-pgdg main" > /etc/apt/sources.list.d/pgdg.list

    apt-get update -qq
    DEBIAN_FRONTEND=noninteractive apt-get install -y -qq postgresql-18 postgresql-client-18 > /dev/null

    # Start PostgreSQL
    systemctl enable --now postgresql

    # Create database and user
    sudo -u postgres psql <<EOF
CREATE DATABASE o3k;
CREATE USER o3k WITH ENCRYPTED PASSWORD '$DB_PASSWORD';
GRANT ALL PRIVILEGES ON DATABASE o3k TO o3k;
ALTER DATABASE o3k OWNER TO o3k;
EOF

    log_success "PostgreSQL installed and configured"
}

# Install Docker and Docker Compose
install_docker() {
    log_info "Installing Docker and Docker Compose..."

    # Install Docker
    curl -fsSL https://get.docker.com -o /tmp/get-docker.sh
    sh /tmp/get-docker.sh > /dev/null
    rm /tmp/get-docker.sh

    # Install Docker Compose plugin
    apt-get install -y -qq docker-compose-plugin > /dev/null

    systemctl enable --now docker

    log_success "Docker and Docker Compose installed"
}

# Clone O3K repository
clone_o3k() {
    log_info "Cloning O3K repository..."

    apt-get install -y -qq git > /dev/null

    if [ -d /opt/o3k ]; then
        log_warning "/opt/o3k already exists. Pulling latest changes..."
        cd /opt/o3k
        git pull origin main
    else
        git clone https://github.com/cobaltcore-dev/o3k.git /opt/o3k
    fi

    log_success "O3K repository ready"
}

# Build O3K
build_o3k() {
    log_info "Building O3K binary..."

    # Install Go if not present
    if ! command -v go &> /dev/null; then
        log_info "Installing Go 1.22..."
        wget -q https://go.dev/dl/go1.22.0.linux-amd64.tar.gz -O /tmp/go.tar.gz
        rm -rf /usr/local/go
        tar -C /usr/local -xzf /tmp/go.tar.gz
        rm /tmp/go.tar.gz
        export PATH=$PATH:/usr/local/go/bin
        echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile
    fi

    cd /opt/o3k
    make build > /dev/null 2>&1

    log_success "O3K binary built: /opt/o3k/bin/o3k"
}

# Create O3K configuration
create_config() {
    log_info "Creating O3K configuration..."

    mkdir -p /opt/o3k/config

    cat > /opt/o3k/config/o3k.yaml <<EOF
# O3K Single-Node Configuration
# Generated by deploy-single-node.sh on $(date)

database:
  url: "postgres://o3k:$DB_PASSWORD@localhost:5432/o3k?sslmode=disable"

keystone:
  host: "0.0.0.0"
  port: 35357
  jwt_secret: "$JWT_SECRET"
  token_ttl: 24h

nova:
  host: "0.0.0.0"
  port: 8774
  libvirt_mode: real
  libvirt_uri: "qemu:///system"
  instance_storage_path: "$INSTANCE_PATH"
  console_proxy_base_url: "http://$HOST_IP:6080/vnc_auto.html"

neutron:
  host: "0.0.0.0"
  port: 9696
  networking_mode: iptables
  external_bridge: "br-ext"
  vxlan_enabled: false

cinder:
  host: "0.0.0.0"
  port: 8776
  storage_mode: local
  local_storage_path: "$VOLUME_PATH"

glance:
  host: "0.0.0.0"
  port: 9292
  storage_mode: local
  local_storage_path: "$IMAGE_PATH"

metadata:
  host: "0.0.0.0"
  port: 8775

compute:
  node_id: "compute-node-1"
  node_name: "$HOSTNAME"
  tunnel_ip: "$HOST_IP"
EOF

    chmod 600 /opt/o3k/config/o3k.yaml
    log_success "Configuration created: /opt/o3k/config/o3k.yaml"
}

# Create storage directories
create_storage_dirs() {
    log_info "Creating storage directories..."

    mkdir -p "$VOLUME_PATH" "$IMAGE_PATH" "$INSTANCE_PATH"
    chmod 755 "$VOLUME_PATH" "$IMAGE_PATH" "$INSTANCE_PATH"

    log_success "Storage directories created"
}

# Run database migrations
run_migrations() {
    log_info "Running database migrations..."

    cd /opt/o3k
    ./bin/o3k migrate --config config/o3k.yaml

    log_success "Database migrations completed"
}

# Create systemd service
create_systemd_service() {
    log_info "Creating systemd service..."

    cat > /etc/systemd/system/o3k.service <<EOF
[Unit]
Description=O3K OpenStack Services
After=network.target postgresql.service libvirtd.service

[Service]
Type=simple
User=root
WorkingDirectory=/opt/o3k
ExecStart=/opt/o3k/bin/o3k --config /opt/o3k/config/o3k.yaml
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    systemctl enable o3k.service
    systemctl start o3k.service

    # Wait for services to start
    sleep 5

    if systemctl is-active --quiet o3k.service; then
        log_success "O3K service started successfully"
    else
        log_error "O3K service failed to start. Check logs: journalctl -u o3k.service -n 50"
        exit 1
    fi
}

# Deploy Horizon dashboard
deploy_horizon() {
    if [[ ! "$DEPLOY_HORIZON" =~ ^[Yy]$ ]]; then
        log_info "Skipping Horizon deployment (user choice)"
        return
    fi

    log_info "Deploying Horizon dashboard..."

    mkdir -p /opt/horizon

    # Create docker-compose.yml
    cat > /opt/horizon/docker-compose.yml <<EOF
version: '3.8'

services:
  horizon:
    image: quay.io/openstack.kolla/horizon:2025.2-ubuntu-noble
    container_name: horizon
    restart: unless-stopped
    ports:
      - "80:80"
    environment:
      - OPENSTACK_HOST=$HOST_IP
      - OPENSTACK_KEYSTONE_URL=http://$HOST_IP:35357/v3
      - OPENSTACK_KEYSTONE_MULTIDOMAIN_SUPPORT=True
      - OPENSTACK_KEYSTONE_DEFAULT_DOMAIN=Default
      - CONSOLE_TYPE=novnc
      - NOVNC_PROXY_BASE_URL=http://$HOST_IP:6080/vnc_auto.html
    volumes:
      - ./local_settings.py:/etc/openstack-dashboard/local_settings.py:ro
    networks:
      - o3k-net

  novnc:
    image: quay.io/openstack.kolla/nova-novncproxy:2025.2-ubuntu-noble
    container_name: novnc-proxy
    restart: unless-stopped
    ports:
      - "6080:6080"
    environment:
      - NOVA_NOVNCPROXY_BASE_URL=http://$HOST_IP:6080/vnc_auto.html
    command: >
      /usr/bin/nova-novncproxy
      --web /usr/share/novnc
      --novncproxy_host=0.0.0.0
      --novncproxy_port=6080
    networks:
      - o3k-net

networks:
  o3k-net:
    driver: bridge
EOF

    # Create local_settings.py
    cat > /opt/horizon/local_settings.py <<EOF
import os
from django.utils.translation import gettext_lazy as _

WEBROOT = '/'
OPENSTACK_HOST = os.environ.get('OPENSTACK_HOST', '$HOST_IP')
OPENSTACK_KEYSTONE_URL = f"http://{OPENSTACK_HOST}:35357/v3"

OPENSTACK_API_VERSIONS = {
    "identity": 3,
    "image": 2,
    "volume": 3,
    "compute": 2.1,
}

OPENSTACK_KEYSTONE_MULTIDOMAIN_SUPPORT = True
OPENSTACK_KEYSTONE_DEFAULT_DOMAIN = 'Default'
OPENSTACK_KEYSTONE_DEFAULT_ROLE = 'member'

SESSION_TIMEOUT = 14400

CONSOLE_TYPE = 'novnc'
OPENSTACK_CONSOLE_NOVNC_PROXY_URL = f"http://{OPENSTACK_HOST}:6080/vnc_auto.html"

ALLOWED_HOSTS = ['*']
DEBUG = False

CACHES = {
    'default': {
        'BACKEND': 'django.core.cache.backends.locmem.LocMemCache',
    },
}

TIME_ZONE = "UTC"
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
    },
}
EOF

    # Start Horizon
    cd /opt/horizon
    docker compose up -d

    log_success "Horizon dashboard deployed"
}

# Configure firewall
configure_firewall() {
    log_info "Configuring firewall..."

    apt-get install -y -qq ufw > /dev/null

    # Allow SSH
    ufw allow 22/tcp

    # Allow OpenStack services
    ufw allow 35357/tcp  # Keystone
    ufw allow 8774/tcp   # Nova
    ufw allow 8775/tcp   # Metadata
    ufw allow 8776/tcp   # Cinder
    ufw allow 9292/tcp   # Glance
    ufw allow 9696/tcp   # Neutron
    ufw allow 6080/tcp   # noVNC
    ufw allow 80/tcp     # Horizon

    # Enable firewall
    ufw --force enable

    log_success "Firewall configured"
}

# Install OpenStack CLI
install_openstack_cli() {
    log_info "Installing OpenStack CLI..."

    apt-get install -y -qq python3-pip python3-openstackclient > /dev/null

    log_success "OpenStack CLI installed"
}

# Create environment file
create_env_file() {
    log_info "Creating environment file..."

    cat > /root/.o3k-env <<EOF
# O3K Environment Variables
export OS_AUTH_URL=http://$HOST_IP:35357/v3
export OS_PROJECT_NAME=default
export OS_USERNAME=admin
export OS_PASSWORD=secret
export OS_USER_DOMAIN_NAME=Default
export OS_PROJECT_DOMAIN_NAME=Default
export OS_IDENTITY_API_VERSION=3
EOF

    # Add to .bashrc
    if ! grep -q ".o3k-env" /root/.bashrc; then
        echo "source /root/.o3k-env" >> /root/.bashrc
    fi

    log_success "Environment file created: /root/.o3k-env"
}

# Verify installation
verify_installation() {
    log_info "Verifying installation..."

    # Source environment
    source /root/.o3k-env

    # Wait for services to be ready
    log_info "Waiting for services to be ready (30 seconds)..."
    sleep 30

    # Test token issue
    if openstack token issue > /dev/null 2>&1; then
        log_success "✓ Keystone authentication working"
    else
        log_error "✗ Keystone authentication failed"
        return 1
    fi

    # Test service list
    if openstack service list > /dev/null 2>&1; then
        log_success "✓ Service catalog accessible"
    else
        log_error "✗ Service catalog not accessible"
        return 1
    fi

    # Test endpoint list
    if openstack endpoint list > /dev/null 2>&1; then
        log_success "✓ API endpoints registered"
    else
        log_error "✗ API endpoints not registered"
        return 1
    fi

    log_success "Installation verified successfully!"
    return 0
}

# Show completion message
show_completion() {
    echo ""
    echo "╔═══════════════════════════════════════════════════════════╗"
    echo "║                                                           ║"
    echo "║   O3K Single-Node Deployment Complete! 🎉                ║"
    echo "║                                                           ║"
    echo "╚═══════════════════════════════════════════════════════════╝"
    echo ""
    log_info "Deployment Summary:"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    echo "  Host IP:              $HOST_IP"
    echo "  Hostname:             $HOSTNAME"
    echo ""
    echo "  OpenStack Services:"
    echo "    - Keystone (Identity):     http://$HOST_IP:35357/v3"
    echo "    - Nova (Compute):          http://$HOST_IP:8774/v2.1"
    echo "    - Neutron (Network):       http://$HOST_IP:9696/v2.0"
    echo "    - Cinder (Block Storage):  http://$HOST_IP:8776/v3"
    echo "    - Glance (Image):          http://$HOST_IP:9292/v2"
    echo ""

    if [[ "$DEPLOY_HORIZON" =~ ^[Yy]$ ]]; then
        echo "  Horizon Dashboard:    http://$HOST_IP"
        echo "    - Domain:   Default"
        echo "    - Username: admin"
        echo "    - Password: secret"
        echo ""
    fi

    echo "  Configuration:        /opt/o3k/config/o3k.yaml"
    echo "  Environment:          /root/.o3k-env"
    echo "  Logs:                 journalctl -u o3k.service -f"
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    log_info "Next Steps:"
    echo "  1. Source environment: source /root/.o3k-env"
    echo "  2. Test authentication: openstack token issue"
    echo "  3. Create your first VM: openstack server create --help"
    if [[ "$DEPLOY_HORIZON" =~ ^[Yy]$ ]]; then
        echo "  4. Access Horizon: http://$HOST_IP"
    fi
    echo ""
    log_info "Documentation: https://github.com/cobaltcore-dev/o3k/blob/main/docs/SINGLE_NODE_DEPLOYMENT.md"
    echo ""
}

# Main installation flow
main() {
    show_banner

    check_root
    detect_os
    check_hardware
    get_configuration

    echo ""
    log_info "Starting installation..."
    echo ""

    update_system
    configure_hostname
    install_kvm
    configure_networking
    install_postgresql
    install_docker
    clone_o3k
    build_o3k
    create_config
    create_storage_dirs
    run_migrations
    create_systemd_service
    deploy_horizon
    configure_firewall
    install_openstack_cli
    create_env_file

    echo ""
    if verify_installation; then
        show_completion
    else
        log_error "Installation completed but verification failed. Check logs: journalctl -u o3k.service -n 50"
        exit 1
    fi
}

# Run main function
main "$@"
