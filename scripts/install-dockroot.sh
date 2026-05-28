#!/bin/bash

# Helper script to install DockRoot for container support

set -e

ARCH=$(uname -m)
case "$ARCH" in
    x86_64)  ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    armv7l)  ARCH="arm" ;;
    *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

INSTALL_DIR="/usr/local/bin"

echo "Installing DockRoot for ${ARCH}..."

# Check if dockroot already installed
if command -v dockroot &>/dev/null; then
    echo "DockRoot already installed: $(dockroot --version 2>/dev/null || echo 'unknown')"
    exit 0
fi

# Try to download prebuilt binary
DOWNLOAD_URL="https://github.com/your-org/dockroot/releases/latest/download/dockroot-linux-${ARCH}"

echo "Downloading from ${DOWNLOAD_URL}..."
if command -v curl &>/dev/null; then
    sudo curl -fsSL "${DOWNLOAD_URL}" -o "${INSTALL_DIR}/dockroot"
elif command -v wget &>/dev/null; then
    sudo wget -q "${DOWNLOAD_URL}" -O "${INSTALL_DIR}/dockroot"
else
    echo "curl or wget required"
    exit 1
fi

chmod +x "${INSTALL_DIR}/dockroot"

# Initialize dockroot config
sudo mkdir -p /opt/dockroot
cat | sudo tee "${INSTALL_DIR}/dockroot.json" >/dev/null <<'EOF'
{
  "registry-mirrors": ["https://docker.m.daocloud.io", "https://docker.1panel.live"],
  "data-root": "/opt/dockroot",
  "useKspeeder": false
}
EOF

echo "DockRoot installed successfully!"
echo "Run 'dockroot --help' to get started"
