#!/bin/sh
set -e

# O3K Install Script
# Usage: curl -sfL https://get.o3k.io | sh -
# Or: curl -sfL https://get.o3k.io | O3K_MODE=agent O3K_SERVER=10.0.0.1:6443 O3K_TOKEN=xxx sh -

GITHUB_REPO="cobaltcore-dev/o3k"
INSTALL_DIR="${O3K_INSTALL_DIR:-/usr/local/bin}"
DATA_DIR="${O3K_DATA_DIR:-/var/lib/o3k}"

# Detect OS
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
    linux) ;;
    darwin) ;;
    *) echo "ERROR: Unsupported OS: $OS"; exit 1 ;;
esac

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
    x86_64|amd64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) echo "ERROR: Unsupported architecture: $ARCH"; exit 1 ;;
esac

# darwin/amd64 has no release binary
if [ "$OS" = "darwin" ] && [ "$ARCH" = "amd64" ]; then
    echo "ERROR: darwin/amd64 is not supported. Only darwin/arm64 (Apple Silicon) is released."
    exit 1
fi

# Determine version
VERSION="${O3K_VERSION:-latest}"
if [ "$VERSION" = "latest" ]; then
    VERSION=$(curl -sfL "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/' || echo "")
    if [ -z "$VERSION" ]; then
        echo "ERROR: Could not determine latest version. Set O3K_VERSION explicitly."
        exit 1
    fi
fi

echo "Installing O3K ${VERSION} (${OS}/${ARCH})..."

# Download binary
BINARY_URL="https://github.com/${GITHUB_REPO}/releases/download/${VERSION}/o3k-${OS}-${ARCH}"
TMP_FILE=$(mktemp)
if ! curl -sfL "$BINARY_URL" -o "$TMP_FILE"; then
    echo "ERROR: Failed to download $BINARY_URL"
    rm -f "$TMP_FILE"
    exit 1
fi

# Install binary
chmod +x "$TMP_FILE"
mkdir -p "$INSTALL_DIR"
mv "$TMP_FILE" "${INSTALL_DIR}/o3k"
echo "Installed: ${INSTALL_DIR}/o3k"

# Create data directory
mkdir -p "$DATA_DIR"

# Skip service setup if requested
if [ "${O3K_SKIP_SERVICE:-}" = "true" ]; then
    echo "Skipping service setup (O3K_SKIP_SERVICE=true)"
    echo "Run manually: o3k server"
    exit 0
fi

# Determine mode (server or agent)
MODE="${O3K_MODE:-server}"

# Install systemd service (Linux only)
if command -v systemctl >/dev/null 2>&1; then
    if [ "$MODE" = "agent" ]; then
        if [ -z "${O3K_SERVER:-}" ] || [ -z "${O3K_TOKEN:-}" ]; then
            echo "ERROR: Agent mode requires O3K_SERVER and O3K_TOKEN"
            echo "Usage: curl -sfL https://get.o3k.io | O3K_MODE=agent O3K_SERVER=host:port O3K_TOKEN=xxx sh -"
            exit 1
        fi
        cat > /etc/systemd/system/o3k-agent.service <<EOF
[Unit]
Description=O3K Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=${INSTALL_DIR}/o3k agent --server ${O3K_SERVER} --token ${O3K_TOKEN}
Restart=always
RestartSec=5
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF
        systemctl daemon-reload
        systemctl enable --now o3k-agent
        echo ""
        echo "O3K Agent installed and running!"
        echo "  Connected to: ${O3K_SERVER}"
    else
        cat > /etc/systemd/system/o3k.service <<EOF
[Unit]
Description=O3K OpenStack Server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=${INSTALL_DIR}/o3k server
Environment=O3K_DATA_DIR=${DATA_DIR}
Restart=always
RestartSec=5
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF
        systemctl daemon-reload
        systemctl enable --now o3k

        # Wait for startup
        echo "Waiting for O3K to start..."
        i=0
        while [ "$i" -lt 30 ]; do
            if curl -sf http://localhost:35357/v3 >/dev/null 2>&1; then
                echo ""
                echo "═══════════════════════════════════════════"
                echo "  O3K installed and running!"
                echo "═══════════════════════════════════════════"
                echo "  API:      http://localhost:35357/v3"
                if [ -f "${DATA_DIR}/initial-password" ]; then
                    echo "  User:     admin"
                    echo "  Password: $(cat "${DATA_DIR}/initial-password")"
                fi
                if [ -f "${DATA_DIR}/agent-token" ]; then
                    MYIP=$(hostname -I 2>/dev/null | awk '{print $1}' || echo "localhost")
                    echo "───────────────────────────────────────────"
                    echo "  Add nodes:"
                    printf "  curl -sfL https://get.o3k.io | O3K_MODE=agent O3K_SERVER=%s:6443 O3K_TOKEN=%s sh -\n" \
                        "$MYIP" "$(cat "${DATA_DIR}/agent-token")"
                fi
                echo "═══════════════════════════════════════════"
                exit 0
            fi
            sleep 1
            i=$((i + 1))
        done
        echo "WARNING: O3K service started but API not responding yet."
        echo "Check: journalctl -u o3k -f"
    fi
else
    echo ""
    echo "No systemd detected. Start manually:"
    if [ "$MODE" = "agent" ]; then
        echo "  o3k agent --server ${O3K_SERVER:-<server>} --token ${O3K_TOKEN:-<token>}"
    else
        echo "  o3k server"
    fi
fi
