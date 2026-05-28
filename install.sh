#!/bin/bash

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Config
INSTALL_DIR="/opt/minipanel"
DATA_DIR="/opt/minipanel/data"
VERSION="1.0.0"
GITHUB_REPO="minipanel/minipanel"

ARCH=$(uname -m)
OS=$(uname -s | tr '[:upper:]' '[:lower:]')

# Detect architecture
 case "$ARCH" in
    x86_64)  ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    armv7l)  ARCH="arm" ;;
    *) echo -e "${RED}Unsupported architecture: $ARCH${NC}"; exit 1 ;;
esac

# Detect Android chroot
IS_ANDROID=false
if [ -f "/system/build.prop" ] || grep -q "Android" /proc/version 2>/dev/null; then
    IS_ANDROID=true
fi

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_ok() {
    echo -e "${GREEN}[OK]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_err() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_banner() {
    echo ""
    echo "========================================"
    echo "  Mini Panel Installer v${VERSION}"
    echo "========================================"
    echo ""
}

check_deps() {
    log_info "Checking dependencies..."
    
    # Check for curl or wget
    if command -v curl &>/dev/null; then
        DOWNLOADER="curl -fsSL"
    elif command -v wget &>/dev/null; then
        DOWNLOADER="wget -qO-"
    else
        log_err "curl or wget is required"
        exit 1
    fi
    
    # Check for dockroot (optional but recommended)
    if command -v dockroot &>/dev/null; then
        log_ok "DockRoot found: $(dockroot --version 2>/dev/null || echo 'unknown')"
    else
        log_warn "DockRoot not found. Container management will be disabled."
        log_warn "Install DockRoot from: https://github.com/your-repo/dockroot"
    fi
    
    log_ok "Dependencies check passed"
}

install_from_release() {
    log_info "Installing from pre-built release..."
    
    local download_url="https://github.com/${GITHUB_REPO}/releases/download/v${VERSION}/minipanel-${OS}-${ARCH}.tar.gz"
    local tmp_dir=$(mktemp -d)
    
    log_info "Downloading from ${download_url}"
    if ! $DOWNLOADER "$download_url" > "${tmp_dir}/minipanel.tar.gz" 2>/dev/null; then
        log_err "Failed to download release. Trying alternative method..."
        # Fallback: build from source if release not available
        install_from_source
        return
    fi
    
    mkdir -p "${INSTALL_DIR}"
    tar -xzf "${tmp_dir}/minipanel.tar.gz" -C "${INSTALL_DIR}"
    rm -rf "${tmp_dir}"
    
    log_ok "Binary installed to ${INSTALL_DIR}"
}

install_from_source() {
    log_info "Installing from source..."
    
    # Check Go
    if ! command -v go &>/dev/null; then
        log_err "Go is not installed. Please install Go 1.23+ first."
        exit 1
    fi
    
    local GO_VERSION=$(go version | grep -o 'go[0-9.]*' | head -1)
    log_info "Found Go: ${GO_VERSION}"
    
    # Check Node.js
    if ! command -v node &>/dev/null; then
        log_err "Node.js is not installed. Please install Node.js 18+ first."
        exit 1
    fi
    
    local NODE_VERSION=$(node --version)
    log_info "Found Node.js: ${NODE_VERSION}"
    
    # Build frontend
    log_info "Building frontend..."
    cd "${SCRIPT_DIR}/frontend"
    npm install
    npm run build
    
    # Build backend
    log_info "Building backend..."
    cd "${SCRIPT_DIR}/backend"
    
    if [ "$IS_ANDROID" = true ]; then
        log_info "Android chroot detected, building static binary..."
        CGO_ENABLED=0 GOOS=linux GOARCH=${ARCH} go build -trimpath -ldflags '-s -w' \
            -tags 'osusergo netgo static_build' \
            -o "${INSTALL_DIR}/minipanel" ./cmd/server
    else
        CGO_ENABLED=0 go build -trimpath -ldflags '-s -w' \
            -tags 'osusergo netgo static_build' \
            -o "${INSTALL_DIR}/minipanel" ./cmd/server
    fi
    
    # Copy static files
    cp -r "${SCRIPT_DIR}/backend/static" "${INSTALL_DIR}/" 2>/dev/null || true
    
    log_ok "Build complete"
}

setup_environment() {
    log_info "Setting up environment..."
    
    # Create directories
    mkdir -p "${DATA_DIR}"
    mkdir -p "${INSTALL_DIR}/logs"
    
    # Generate config if not exists
    if [ ! -f "${INSTALL_DIR}/config.yaml" ]; then
        cat > "${INSTALL_DIR}/config.yaml" <<EOF
port: 8080
log_level: info
db_path: ${DATA_DIR}/minipanel.db
data_dir: ${DATA_DIR}
jwt_secret: $(openssl rand -hex 16 2>/dev/null || cat /dev/urandom | tr -dc 'a-zA-Z0-9' | head -c 32)
EOF
        log_ok "Config generated at ${INSTALL_DIR}/config.yaml"
    fi
    
    # Create start script
    cat > "${INSTALL_DIR}/start.sh" <<'EOF'
#!/bin/bash
cd "$(dirname "$0")"
./minipanel &
echo $! > minipanel.pid
echo "Mini Panel started on http://0.0.0.0:8080"
EOF
    chmod +x "${INSTALL_DIR}/start.sh"
    
    # Create stop script
    cat > "${INSTALL_DIR}/stop.sh" <<'EOF'
#!/bin/bash
cd "$(dirname "$0")"
if [ -f minipanel.pid ]; then
    kill $(cat minipanel.pid) 2>/dev/null
    rm -f minipanel.pid
    echo "Mini Panel stopped"
else
    echo "Mini Panel is not running"
fi
EOF
    chmod +x "${INSTALL_DIR}/stop.sh"
    
    # Create status script
    cat > "${INSTALL_DIR}/status.sh" <<'EOF'
#!/bin/bash
cd "$(dirname "$0")"
if [ -f minipanel.pid ] && kill -0 $(cat minipanel.pid) 2>/dev/null; then
    echo "Mini Panel is running (PID: $(cat minipanel.pid))"
    echo "Access: http://$(hostname -I 2>/dev/null | awk '{print $1}' || echo 'localhost'):8080"
else
    echo "Mini Panel is not running"
fi
EOF
    chmod +x "${INSTALL_DIR}/status.sh"
    
    # Create symlink
    if [ -d "/usr/local/bin" ]; then
        ln -sf "${INSTALL_DIR}/minipanel" /usr/local/bin/minipanel 2>/dev/null || true
    fi
    
    log_ok "Environment setup complete"
}

print_finish() {
    echo ""
    echo "========================================"
    echo -e "${GREEN}Installation Complete!${NC}"
    echo "========================================"
    echo ""
    echo "Installation Directory: ${INSTALL_DIR}"
    echo "Data Directory: ${DATA_DIR}"
    echo ""
    echo "Commands:"
    echo "  Start:   ${INSTALL_DIR}/start.sh"
    echo "  Stop:    ${INSTALL_DIR}/stop.sh"
    echo "  Status:  ${INSTALL_DIR}/status.sh"
    echo ""
    
    local ip=$(hostname -I 2>/dev/null | awk '{print $1}' || echo 'localhost')
    echo "Access URL: http://${ip}:8080"
    echo ""
    echo "Default Login:"
    echo "  Username: admin"
    echo "  Password: admin123"
    echo ""
    
    if [ "$IS_ANDROID" = true ]; then
        echo -e "${YELLOW}Android Chroot Tips:${NC}"
        echo "  - Run as root for full functionality"
        echo "  - Install DockRoot for container support"
        echo "  - Use tmux/screen to keep running in background"
        echo ""
    fi
}

main() {
    SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
    
    print_banner
    check_deps
    
    # Check if running from source directory
    if [ -d "${SCRIPT_DIR}/backend" ] && [ -d "${SCRIPT_DIR}/frontend" ]; then
        log_info "Source directory detected"
        install_from_source
    else
        log_info "No source found, downloading release..."
        install_from_release
    fi
    
    setup_environment
    print_finish
}

main "$@"
