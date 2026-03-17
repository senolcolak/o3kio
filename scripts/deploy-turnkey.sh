#!/bin/bash
#
# O3K Turnkey Deployment Script
# One-command deployment of O3K + Horizon + KVM
#
# Usage:
#   sudo ./scripts/deploy-turnkey.sh
#
# This script deploys a complete, production-ready OpenStack environment:
#   - O3K (all 5 core services) with real KVM hypervisor
#   - Horizon dashboard (Flamingo 2025.2) with noVNC console
#   - PostgreSQL 18 database
#   - Network bridge (br-ext) on ens19 for VM external connectivity
#
# Network Configuration:
#   Interface: ens19
#   IP: 10.2.199.101/24
#   Gateway: 10.2.199.254
#   DNS: 8.8.8.8
#
# Requirements:
#   - Ubuntu 24.04/22.04 or Debian 12
#   - 4+ CPU cores, 16+ GB RAM, 100+ GB disk
#   - KVM virtualization support (Intel VT-x or AMD-V)
#   - Root/sudo access
#

set -e  # Exit on any error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Configuration (hardcoded for turnkey operation)
HOST_IP="10.2.199.101"
NETWORK_IFACE="ens19"
GATEWAY="10.2.199.254"
DNS_SERVERS="8.8.8.8"
NETMASK="24"
DB_PASSWORD="O3kSecure2026Password"
JWT_SECRET="$(openssl rand -hex 32 2>/dev/null || echo 'change-this-jwt-secret-in-production-12345')"
INSTALL_DIR="/opt/o3k"
CONFIG_DIR="/opt/o3k/config"
STORAGE_DIR="/var/lib/o3k"
GO_VERSION="1.26.1"

# Logging functions
log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_warning() { echo -e "${YELLOW}[WARNING]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# Banner
show_banner() {
    cat << "EOF"
╔═══════════════════════════════════════════════════════════╗
║                                                           ║
║   O3K Turnkey Deployment                                 ║
║   Complete OpenStack in One Command                      ║
║                                                           ║
╚═══════════════════════════════════════════════════════════╝
EOF
    echo ""
    echo "Network Configuration:"
    echo "  Interface: $NETWORK_IFACE"
    echo "  IP: $HOST_IP/$NETMASK"
    echo "  Gateway: $GATEWAY"
    echo "  DNS: $DNS_SERVERS"
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

    case "$OS" in
        ubuntu|debian) ;;
        *) log_error "Unsupported OS: $OS. Supported: Ubuntu, Debian"; exit 1 ;;
    esac
}

# Check hardware
check_hardware() {
    log_info "Checking hardware requirements..."

    CPU_CORES=$(nproc)
    if [ "$CPU_CORES" -lt 4 ]; then
        log_error "Insufficient CPU cores: $CPU_CORES (minimum: 4)"
        exit 1
    fi

    TOTAL_RAM=$(free -g | awk '/^Mem:/{print $2}')
    if [ "$TOTAL_RAM" -lt 16 ]; then
        log_warning "RAM is ${TOTAL_RAM}GB (recommended: 16GB+)"
    fi

    if ! egrep -c '(vmx|svm)' /proc/cpuinfo > /dev/null 2>&1; then
        log_error "KVM virtualization not supported by CPU"
        exit 1
    fi

    log_success "Hardware requirements met (${CPU_CORES} cores, ${TOTAL_RAM}GB RAM)"
}

# Install system packages
install_packages() {
    log_info "Installing system packages..."

    export DEBIAN_FRONTEND=noninteractive
    apt-get update -qq
    apt-get install -y -qq \
        apt-transport-https \
        ca-certificates \
        curl \
        gnupg \
        lsb-release \
        qemu-kvm \
        libvirt-daemon-system \
        libvirt-clients \
        bridge-utils \
        netplan.io \
        postgresql \
        postgresql-contrib \
        jq \
        python3-openstackclient \
        > /dev/null 2>&1

    log_success "System packages installed"
}

# Install Docker
install_docker() {
    if command -v docker &> /dev/null; then
        log_success "Docker already installed ($(docker --version))"
        return
    fi

    log_info "Installing Docker..."

    curl -fsSL https://download.docker.com/linux/$OS/gpg | gpg --dearmor -o /usr/share/keyrings/docker-archive-keyring.gpg
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] https://download.docker.com/linux/$OS $(lsb_release -cs) stable" | \
        tee /etc/apt/sources.list.d/docker.list > /dev/null

    apt-get update -qq
    apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-compose-plugin > /dev/null 2>&1

    systemctl enable --now docker > /dev/null 2>&1
    usermod -aG docker ${SUDO_USER:-$USER} 2>/dev/null || true

    log_success "Docker installed ($(docker --version))"
}

# Configure networking
configure_networking() {
    log_info "Configuring network bridge on $NETWORK_IFACE..."

    # Verify interface exists
    if ! ip link show "$NETWORK_IFACE" > /dev/null 2>&1; then
        log_error "Network interface $NETWORK_IFACE not found"
        exit 1
    fi

    # Create netplan configuration
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
        - $HOST_IP/$NETMASK
      routes:
        - to: default
          via: $GATEWAY
      nameservers:
        addresses:
          - $DNS_SERVERS
      dhcp4: no
      dhcp6: no
      parameters:
        stp: false
        forward-delay: 0
EOF

    # Apply netplan
    netplan apply
    sleep 3

    # Verify bridge
    if ! ip link show br-ext > /dev/null 2>&1; then
        log_error "Bridge br-ext creation failed"
        exit 1
    fi

    log_success "Network bridge configured (br-ext on $NETWORK_IFACE)"
}

# Configure bridge netfilter
configure_bridge_netfilter() {
    log_info "Configuring bridge netfilter..."

    if modprobe br_netfilter 2>/dev/null; then
        echo "br_netfilter" > /etc/modules-load.d/o3k.conf

        cat > /etc/sysctl.d/99-o3k-bridge.conf <<EOF
net.bridge.bridge-nf-call-iptables = 1
net.bridge.bridge-nf-call-ip6tables = 1
net.ipv4.ip_forward = 1
EOF
        sysctl -p /etc/sysctl.d/99-o3k-bridge.conf > /dev/null 2>&1
        log_success "Bridge netfilter configured"
    else
        log_warning "Could not load br_netfilter module (may not be needed)"
    fi
}

# Setup PostgreSQL
setup_database() {
    log_info "Configuring PostgreSQL database..."

    systemctl enable --now postgresql > /dev/null 2>&1

    # Create database and user
    sudo -u postgres psql > /dev/null 2>&1 <<EOF || true
CREATE DATABASE o3k;
CREATE USER o3k WITH ENCRYPTED PASSWORD '$DB_PASSWORD';
GRANT ALL PRIVILEGES ON DATABASE o3k TO o3k;
ALTER DATABASE o3k OWNER TO o3k;
EOF

    # Allow local connections
    PG_VERSION=$(psql --version | awk '{print $3}' | cut -d. -f1)
    PG_HBA="/etc/postgresql/$PG_VERSION/main/pg_hba.conf"

    if [ -f "$PG_HBA" ]; then
        if ! grep -q "host.*o3k.*o3k.*127.0.0.1" "$PG_HBA"; then
            echo "host    o3k    o3k    127.0.0.1/32    md5" >> "$PG_HBA"
            systemctl restart postgresql
        fi
    fi

    log_success "PostgreSQL configured"
}

# Create storage directories
create_storage() {
    log_info "Creating storage directories..."

    mkdir -p "$STORAGE_DIR"/{volumes,images,instances}
    chmod 755 "$STORAGE_DIR"/{volumes,images,instances}

    log_success "Storage directories created at $STORAGE_DIR"
}

# Generate O3K configuration
generate_config() {
    log_info "Generating O3K configuration..."

    REPO_DIR="/root/o3k"
    mkdir -p "$CONFIG_DIR"

    # O3K main configuration
    cat > "$CONFIG_DIR/o3k.yaml" <<EOF
database:
  url: "postgres://o3k:$DB_PASSWORD@localhost:5432/o3k?sslmode=disable"
  max_connections: 20

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
  instance_storage_path: "$STORAGE_DIR/instances"
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
  local_storage_path: "$STORAGE_DIR/volumes"

glance:
  host: "0.0.0.0"
  port: 9292
  storage_mode: local
  local_storage_path: "$STORAGE_DIR/images"

metadata:
  host: "0.0.0.0"
  port: 8775

compute:
  node_id: "compute-node-1"
  node_name: "$(hostname)"
  tunnel_ip: "$HOST_IP"
EOF

    # Copy Horizon configuration from repo and modify for host networking
    if [ ! -d "$REPO_DIR/deployments/horizon-config" ]; then
        log_error "Horizon config not found in repository at $REPO_DIR/deployments/horizon-config"
        exit 1
    fi

    # Create modified local_settings for host networking (memcached on HOST_IP)
    log_info "Creating Horizon configuration for host networking..."
    mkdir -p "$CONFIG_DIR/horizon-config"

    # Copy local_settings and change memcached location for host networking
    # Use HOST_IP instead of localhost since containers have separate network namespaces
    sed "s/memcached:11211/$HOST_IP:11211/g" \
        "$REPO_DIR/deployments/horizon-config/local_settings" > "$CONFIG_DIR/horizon-config/local_settings"

    # Also fix OPENSTACK_HOST to use HOST_IP for host networking
    sed -i "s/OPENSTACK_HOST = \"o3k\"/OPENSTACK_HOST = \"$HOST_IP\"/g" "$CONFIG_DIR/horizon-config/local_settings"

    # Fix noVNC URL to use HOST_IP
    sed -i "s/novnc:6080/$HOST_IP:6080/g" "$CONFIG_DIR/horizon-config/local_settings"

    log_success "Configuration generated at $CONFIG_DIR"
}

# Create Docker Compose file
create_docker_compose() {
    log_info "Creating Docker Compose configuration..."

    REPO_DIR="/root/o3k"
    cd "$REPO_DIR"

    cat > "$REPO_DIR/deployments/docker-compose-turnkey.yml" <<EOF
services:
  # Memcached for Horizon sessions
  memcached:
    image: memcached:1.6-alpine
    container_name: o3k-memcached
    hostname: memcached
    network_mode: host
    command: memcached -m 64 -l 0.0.0.0
    restart: unless-stopped

  # O3K OpenStack Services with KVM
  o3k:
    build:
      context: ..
      dockerfile: build/package/Dockerfile
    image: o3k:latest
    container_name: o3k
    privileged: true
    network_mode: host
    volumes:
      - /var/run/libvirt/libvirt-sock:/var/run/libvirt/libvirt-sock
      - $STORAGE_DIR/volumes:$STORAGE_DIR/volumes
      - $STORAGE_DIR/images:$STORAGE_DIR/images
      - $STORAGE_DIR/instances:$STORAGE_DIR/instances
      - $CONFIG_DIR:/opt/o3k/config:ro
    command: [/app/o3k, -config, /opt/o3k/config/o3k.yaml]
    restart: unless-stopped
    healthcheck:
      test: [CMD-SHELL, "curl -f http://localhost:35357/v3 || exit 1"]
      interval: 10s
      timeout: 5s
      retries: 3

  # noVNC Console Proxy
  novnc:
    image: dougw/novnc:latest
    container_name: o3k-novnc
    hostname: novnc
    network_mode: host
    environment:
      - DISPLAY_WIDTH=1280
      - DISPLAY_HEIGHT=720
    restart: unless-stopped
    healthcheck:
      test: [CMD, nc, -z, localhost, "6080"]
      interval: 30s
      timeout: 5s
      retries: 3

  # OpenStack Horizon Dashboard (using working config from repo)
  horizon:
    image: quay.io/openstack.kolla/horizon:2025.2-ubuntu-noble
    container_name: o3k-horizon
    hostname: horizon
    network_mode: host
    depends_on:
      o3k:
        condition: service_healthy
      novnc:
        condition: service_started
    environment:
      - KOLLA_INSTALL_TYPE=source
      - KOLLA_CONFIG_STRATEGY=COPY_ALWAYS
    volumes:
      - ./horizon-config/config.json:/var/lib/kolla/config_files/config.json:ro
      - $CONFIG_DIR/horizon-config/local_settings:/var/lib/kolla/config_files/local_settings:ro
      - ./horizon-config/apache/ports.conf:/var/lib/kolla/config_files/ports.conf:ro
      - ./horizon-config/apache/horizon-nolist.conf:/var/lib/kolla/config_files/horizon-nolist.conf:ro
      - horizon-static:/var/lib/kolla/venv/lib/python3.12/site-packages/static:rw
      - horizon-logs:/var/log/kolla/horizon:rw
    restart: unless-stopped
    healthcheck:
      test: [CMD, curl, -f, "http://localhost:80/dashboard/auth/login/"]
      interval: 30s
      timeout: 10s
      retries: 5
      start_period: 60s

volumes:
  horizon-static:
  horizon-logs:
EOF

    log_success "Docker Compose configuration created"
}

# Build and start services
deploy_services() {
    log_info "Building O3K image (this may take several minutes)..."

    REPO_DIR="/root/o3k"
    cd "$REPO_DIR/deployments"

    docker compose -f docker-compose-turnkey.yml build --quiet
    log_success "O3K image built"

    log_info "Starting services..."
    docker compose -f docker-compose-turnkey.yml up -d

    log_info "Waiting for services to be ready..."
    sleep 10

    # Wait for O3K to be healthy
    for i in {1..30}; do
        if curl -sf http://localhost:35357/v3 > /dev/null 2>&1; then
            log_success "O3K services are ready"
            break
        fi
        if [ $i -eq 30 ]; then
            log_error "O3K failed to start within 5 minutes"
            docker compose -f docker-compose-turnkey.yml logs o3k
            exit 1
        fi
        sleep 10
    done

    # Wait for Horizon to be ready
    log_info "Waiting for Horizon dashboard..."
    for i in {1..30}; do
        if curl -sf http://localhost:8080/dashboard/auth/login/ > /dev/null 2>&1; then
            log_success "Horizon dashboard is ready"
            break
        fi
        if [ $i -eq 30 ]; then
            log_warning "Horizon may still be starting (takes 2-3 minutes)"
        fi
        sleep 10
    done
}

# Create CLI environment file
create_cli_env() {
    log_info "Creating OpenStack CLI environment file..."

    cat > /root/.o3k-env <<EOF
# O3K OpenStack Environment
export OS_AUTH_URL=http://$HOST_IP:35357/v3
export OS_PROJECT_NAME=default
export OS_USERNAME=admin
export OS_PASSWORD=secret
export OS_USER_DOMAIN_NAME=Default
export OS_PROJECT_DOMAIN_NAME=Default
export OS_IDENTITY_API_VERSION=3
EOF

    # Add to bashrc if not already there
    if ! grep -q "source /root/.o3k-env" /root/.bashrc; then
        echo "source /root/.o3k-env" >> /root/.bashrc
    fi

    log_success "CLI environment configured"
}

# Show completion message
show_completion() {
    log_success "O3K deployment complete!"
    echo ""
    echo "╔═══════════════════════════════════════════════════════════╗"
    echo "║                    Access Information                      ║"
    echo "╚═══════════════════════════════════════════════════════════╝"
    echo ""
    echo "🌐 Horizon Dashboard:"
    echo "   URL: http://$HOST_IP/dashboard"
    echo "   Username: admin"
    echo "   Password: secret"
    echo "   Domain: Default"
    echo ""
    echo "🖥️  noVNC Console:"
    echo "   URL: http://$HOST_IP:6080"
    echo ""
    echo "🔧 OpenStack CLI:"
    echo "   source /root/.o3k-env"
    echo "   openstack token issue"
    echo "   openstack server list"
    echo ""
    echo "📊 Service Status:"
    echo "   docker compose -f /root/o3k/deployments/docker-compose-turnkey.yml ps"
    echo ""
    echo "📋 Logs:"
    echo "   docker compose -f /root/o3k/deployments/docker-compose-turnkey.yml logs -f"
    echo ""
    echo "Network Configuration:"
    echo "   Bridge: br-ext"
    echo "   Interface: $NETWORK_IFACE"
    echo "   Host IP: $HOST_IP/$NETMASK"
    echo "   Gateway: $GATEWAY"
    echo ""
    echo "⚡ Quick Test:"
    echo "   source /root/.o3k-env"
    echo "   openstack network create test-net"
    echo "   openstack server create --flavor m1.small --image cirros --network test-net test-vm"
    echo "   openstack server list"
    echo ""
}

# Main deployment flow
main() {
    show_banner
    check_root
    detect_os
    check_hardware
    install_packages
    install_docker
    configure_networking
    configure_bridge_netfilter
    setup_database
    create_storage
    generate_config
    create_docker_compose
    deploy_services
    create_cli_env
    show_completion
}

# Run main function
main "$@"
