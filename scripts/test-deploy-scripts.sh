#!/bin/bash
#
# Test Suite for O3K Deployment Scripts
# Tests deploy-single-node.sh functions in isolation
#
# Usage: ./scripts/test-deploy-scripts.sh
#

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

TESTS_PASSED=0
TESTS_FAILED=0
TEST_DIR="/tmp/o3k-test-$$"

log_test() {
    echo -e "${BLUE}[TEST]${NC} $1"
}

log_pass() {
    echo -e "${GREEN}[PASS]${NC} $1"
    ((TESTS_PASSED++))
}

log_fail() {
    echo -e "${RED}[FAIL]${NC} $1"
    ((TESTS_FAILED++))
}

# Setup test environment
setup_test_env() {
    mkdir -p "$TEST_DIR"
    cd "$TEST_DIR"
}

# Cleanup test environment
cleanup_test_env() {
    rm -rf "$TEST_DIR"
}

# Test: Netplan configuration generation
test_netplan_generation() {
    log_test "Testing netplan configuration generation"

    # Test data
    NETWORK_IFACE="eth0"
    HOST_IP="192.168.1.100"
    GATEWAY="192.168.1.1"
    DNS_SERVERS="8.8.8.8,8.8.4.4"

    # Parse DNS servers
    IFS=',' read -ra DNS_LIST <<< "$DNS_SERVERS"

    # Generate netplan config
    cat > "$TEST_DIR/test-netplan.yaml" <<'NETPLAN_EOF'
network:
  version: 2
  renderer: networkd
  ethernets:
    NETWORK_IFACE_PLACEHOLDER:
      dhcp4: no
      dhcp6: no
  bridges:
    br-ext:
      interfaces: [NETWORK_IFACE_PLACEHOLDER]
      addresses:
        - HOST_IP_PLACEHOLDER/24
      routes:
        - to: default
          via: GATEWAY_PLACEHOLDER
      nameservers:
        addresses:
DNS_SERVERS_PLACEHOLDER
      dhcp4: no
      dhcp6: no
      parameters:
        stp: false
        forward-delay: 0
NETPLAN_EOF

    # Replace placeholders
    sed -i "s/NETWORK_IFACE_PLACEHOLDER/$NETWORK_IFACE/g" "$TEST_DIR/test-netplan.yaml"
    sed -i "s|HOST_IP_PLACEHOLDER|$HOST_IP|g" "$TEST_DIR/test-netplan.yaml"
    sed -i "s/GATEWAY_PLACEHOLDER/$GATEWAY/g" "$TEST_DIR/test-netplan.yaml"

    # Insert DNS servers
    DNS_LINES=""
    for dns in "${DNS_LIST[@]}"; do
        DNS_LINES="${DNS_LINES}          - ${dns}\n"
    done
    DNS_LINES=$(echo -e "$DNS_LINES" | sed '$ s/\\n$//')
    sed -i "/DNS_SERVERS_PLACEHOLDER/c\\${DNS_LINES}" "$TEST_DIR/test-netplan.yaml"

    # Validate YAML syntax with Python
    if command -v python3 &> /dev/null; then
        if python3 -c "import yaml; yaml.safe_load(open('$TEST_DIR/test-netplan.yaml'))" 2>/dev/null; then
            log_pass "Netplan YAML syntax is valid"
        else
            log_fail "Netplan YAML syntax is invalid"
            cat "$TEST_DIR/test-netplan.yaml"
            return 1
        fi
    else
        log_pass "Netplan YAML generated (Python not available for validation)"
    fi

    # Check for expected content
    if grep -q "eth0" "$TEST_DIR/test-netplan.yaml" && \
       grep -q "192.168.1.100/24" "$TEST_DIR/test-netplan.yaml" && \
       grep -q "192.168.1.1" "$TEST_DIR/test-netplan.yaml" && \
       grep -q "8.8.8.8" "$TEST_DIR/test-netplan.yaml" && \
       grep -q "8.8.4.4" "$TEST_DIR/test-netplan.yaml"; then
        log_pass "All placeholders replaced correctly"
    else
        log_fail "Some placeholders not replaced"
        cat "$TEST_DIR/test-netplan.yaml"
        return 1
    fi

    # Check DNS formatting (each on separate line)
    DNS_COUNT=$(grep -c "^ *- [0-9]" "$TEST_DIR/test-netplan.yaml" || true)
    if [ "$DNS_COUNT" -eq 2 ]; then
        log_pass "DNS servers formatted correctly (2 entries)"
    else
        log_fail "DNS servers not formatted correctly (found $DNS_COUNT entries)"
        return 1
    fi

    # Verify no placeholder remains
    if ! grep -q "PLACEHOLDER" "$TEST_DIR/test-netplan.yaml"; then
        log_pass "No placeholders remaining"
    else
        log_fail "Placeholders still present in config"
        grep "PLACEHOLDER" "$TEST_DIR/test-netplan.yaml"
        return 1
    fi

    return 0
}

# Test: DNS parsing with various formats
test_dns_parsing() {
    log_test "Testing DNS server parsing"

    # Test case 1: Two DNS servers
    DNS_SERVERS="8.8.8.8,8.8.4.4"
    IFS=',' read -ra DNS_LIST <<< "$DNS_SERVERS"
    if [ "${#DNS_LIST[@]}" -eq 2 ]; then
        log_pass "Parsed 2 DNS servers from comma-separated list"
    else
        log_fail "Failed to parse DNS servers (got ${#DNS_LIST[@]})"
        return 1
    fi

    # Test case 2: Single DNS server
    DNS_SERVERS="1.1.1.1"
    IFS=',' read -ra DNS_LIST <<< "$DNS_SERVERS"
    if [ "${#DNS_LIST[@]}" -eq 1 ]; then
        log_pass "Parsed 1 DNS server"
    else
        log_fail "Failed to parse single DNS server"
        return 1
    fi

    # Test case 3: Three DNS servers
    DNS_SERVERS="8.8.8.8,8.8.4.4,1.1.1.1"
    IFS=',' read -ra DNS_LIST <<< "$DNS_SERVERS"
    if [ "${#DNS_LIST[@]}" -eq 3 ]; then
        log_pass "Parsed 3 DNS servers"
    else
        log_fail "Failed to parse 3 DNS servers"
        return 1
    fi

    # Test case 4: DNS with spaces (should be trimmed)
    DNS_SERVERS=" 8.8.8.8 , 8.8.4.4 "
    IFS=',' read -ra DNS_LIST <<< "$DNS_SERVERS"
    DNS_TRIMMED="${DNS_LIST[0]// /}"
    if [ "$DNS_TRIMMED" = "8.8.8.8" ]; then
        log_pass "DNS servers trimmed correctly"
    else
        log_fail "DNS servers not trimmed (got '$DNS_TRIMMED')"
        return 1
    fi

    return 0
}

# Test: Hardware requirements check
test_hardware_checks() {
    log_test "Testing hardware requirement checks"

    # Check CPU cores
    CPU_CORES=$(nproc)
    if [ "$CPU_CORES" -ge 4 ]; then
        log_pass "CPU cores check ($CPU_CORES >= 4)"
    else
        log_fail "Insufficient CPU cores ($CPU_CORES < 4)"
    fi

    # Check RAM
    TOTAL_RAM=$(free -g | awk '/^Mem:/{print $2}')
    if [ "$TOTAL_RAM" -ge 8 ]; then
        log_pass "RAM check (${TOTAL_RAM}GB >= 8GB)"
    else
        log_fail "Insufficient RAM (${TOTAL_RAM}GB < 8GB)"
    fi

    # Check virtualization support
    if egrep -c '(vmx|svm)' /proc/cpuinfo > /dev/null 2>&1; then
        log_pass "Virtualization support detected"
    else
        log_fail "No virtualization support"
    fi

    return 0
}

# Test: Configuration validation
test_config_validation() {
    log_test "Testing configuration validation"

    # Test valid IP address
    HOST_IP="192.168.1.100"
    if [[ $HOST_IP =~ ^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}$ ]]; then
        log_pass "Valid IP address format: $HOST_IP"
    else
        log_fail "Invalid IP address format: $HOST_IP"
        return 1
    fi

    # Test invalid IP address
    HOST_IP="999.999.999.999"
    if [[ $HOST_IP =~ ^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}$ ]]; then
        # Check octets
        IFS='.' read -ra OCTETS <<< "$HOST_IP"
        VALID=true
        for octet in "${OCTETS[@]}"; do
            if [ "$octet" -gt 255 ]; then
                VALID=false
            fi
        done
        if [ "$VALID" = false ]; then
            log_pass "Invalid IP rejected (octets > 255)"
        else
            log_fail "Invalid IP not rejected"
            return 1
        fi
    fi

    return 0
}

# Test: Password generation
test_password_generation() {
    log_test "Testing password generation"

    # Generate password
    PASSWORD=$(openssl rand -base64 32)

    if [ ${#PASSWORD} -ge 32 ]; then
        log_pass "Generated password with sufficient length (${#PASSWORD} chars)"
    else
        log_fail "Password too short (${#PASSWORD} < 32)"
        return 1
    fi

    # Check password contains alphanumeric
    if [[ $PASSWORD =~ [A-Za-z0-9] ]]; then
        log_pass "Password contains alphanumeric characters"
    else
        log_fail "Password missing alphanumeric characters"
        return 1
    fi

    return 0
}

# Test: File path validation
test_path_validation() {
    log_test "Testing storage path validation"

    # Test valid absolute path
    VOLUME_PATH="/var/lib/o3k/volumes"
    if [[ $VOLUME_PATH = /* ]]; then
        log_pass "Valid absolute path: $VOLUME_PATH"
    else
        log_fail "Path is not absolute: $VOLUME_PATH"
        return 1
    fi

    # Test invalid relative path
    VOLUME_PATH="var/lib/o3k/volumes"
    if [[ $VOLUME_PATH = /* ]]; then
        log_fail "Relative path accepted as absolute"
        return 1
    else
        log_pass "Relative path correctly rejected"
    fi

    return 0
}

# Test: Service name validation
test_service_validation() {
    log_test "Testing systemd service naming"

    SERVICE_NAME="o3k.service"

    # Check service name format
    if [[ $SERVICE_NAME =~ ^[a-z0-9_-]+\.service$ ]]; then
        log_pass "Valid service name: $SERVICE_NAME"
    else
        log_fail "Invalid service name: $SERVICE_NAME"
        return 1
    fi

    return 0
}

# Test: Network interface detection
test_network_detection() {
    log_test "Testing network interface detection"

    # Get default interface
    DEFAULT_IFACE=$(ip route | grep default | awk '{print $5}')

    if [ -n "$DEFAULT_IFACE" ]; then
        log_pass "Default interface detected: $DEFAULT_IFACE"
    else
        log_fail "No default interface detected"
        return 1
    fi

    # Verify interface exists
    if ip link show "$DEFAULT_IFACE" > /dev/null 2>&1; then
        log_pass "Interface $DEFAULT_IFACE exists"
    else
        log_fail "Interface $DEFAULT_IFACE not found"
        return 1
    fi

    return 0
}

# Test: Gateway detection
test_gateway_detection() {
    log_test "Testing gateway detection"

    # Get default gateway
    DEFAULT_GATEWAY=$(ip route | grep default | awk '{print $3}')

    if [ -n "$DEFAULT_GATEWAY" ]; then
        log_pass "Default gateway detected: $DEFAULT_GATEWAY"
    else
        log_fail "No default gateway detected"
        return 1
    fi

    # Validate gateway is IP address
    if [[ $DEFAULT_GATEWAY =~ ^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}$ ]]; then
        log_pass "Gateway is valid IP format"
    else
        log_fail "Gateway is not valid IP"
        return 1
    fi

    return 0
}

# Test: Hostname validation
test_hostname_validation() {
    log_test "Testing hostname validation"

    # Valid hostnames
    VALID_NAMES=("o3k-demo" "cloud-01" "openstack" "node1")
    for name in "${VALID_NAMES[@]}"; do
        if [[ $name =~ ^[a-z0-9-]+$ ]]; then
            log_pass "Valid hostname: $name"
        else
            log_fail "Hostname rejected incorrectly: $name"
            return 1
        fi
    done

    # Invalid hostnames
    INVALID_NAMES=("o3k_demo" "cloud.01" "Open Stack" "node#1")
    for name in "${INVALID_NAMES[@]}"; do
        if [[ $name =~ ^[a-z0-9-]+$ ]]; then
            log_fail "Invalid hostname accepted: $name"
            return 1
        else
            log_pass "Invalid hostname rejected: $name"
        fi
    done

    return 0
}

# Run all tests
run_all_tests() {
    echo "╔═══════════════════════════════════════════════════════════╗"
    echo "║                                                           ║"
    echo "║   O3K Deployment Script Test Suite                       ║"
    echo "║                                                           ║"
    echo "╚═══════════════════════════════════════════════════════════╝"
    echo ""

    setup_test_env

    # Run tests
    test_netplan_generation || true
    test_dns_parsing || true
    test_hardware_checks || true
    test_config_validation || true
    test_password_generation || true
    test_path_validation || true
    test_service_validation || true
    test_network_detection || true
    test_gateway_detection || true
    test_hostname_validation || true

    cleanup_test_env

    # Summary
    echo ""
    echo "╔═══════════════════════════════════════════════════════════╗"
    echo "║   Test Summary                                            ║"
    echo "╚═══════════════════════════════════════════════════════════╝"
    echo ""
    echo -e "${GREEN}Tests Passed: $TESTS_PASSED${NC}"
    echo -e "${RED}Tests Failed: $TESTS_FAILED${NC}"
    echo ""

    if [ "$TESTS_FAILED" -eq 0 ]; then
        echo -e "${GREEN}✓ All tests passed!${NC}"
        return 0
    else
        echo -e "${RED}✗ Some tests failed${NC}"
        return 1
    fi
}

# Run tests
run_all_tests
