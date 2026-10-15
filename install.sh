#!/bin/bash

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Config
GITHUB_REPO="YevolcXhb/mini-panel"

# Installation directory (can be overridden via env)
INSTALL_DIR="${MINIPANEL_DIR:-/opt/minipanel}"
DATA_DIR="${MINIPANEL_DATA:-$INSTALL_DIR/data}"

ARCH=$(uname -m)
OS=$(uname -s | tr '[:upper:]' '[:lower:]')

# Detect architecture
case "$ARCH" in
    x86_64)  ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    armv7l|armv7) ARCH="arm" ;;
    armv8l)  ARCH="arm64" ;;
    *) echo -e "${RED}Unsupported architecture: $ARCH${NC}"; exit 1 ;;
esac

# Detect Android chroot
IS_ANDROID=false
if [ -f "/system/build.prop" ] || grep -q "Android" /proc/version 2>/dev/null; then
    IS_ANDROID=true
fi

# Detect online install (curl | bash)
IS_ONLINE=false
if [ ! -t 0 ] || [ "$(basename "$0")" = "bash" ] || [ "$(basename "$0")" = "sh" ]; then
    IS_ONLINE=true
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

# Auto-detect latest release version
#
# 探测顺序：
#   1) MINIPANEL_VERSION（面板后端触发更新时传入，最可靠，且与后端展示的版本一致）
#   2) GitHub Releases API
#   3) version.json（raw.githubusercontent.com，不占用 API 匿名速率限制）
# 三个来源都拿不到时返回空字符串，由调用方中止安装 —— 绝不回退到某个写死的旧版本。
get_latest_version() {
    if [ -n "${MINIPANEL_VERSION:-}" ]; then
        echo "${MINIPANEL_VERSION#v}"
        return 0
    fi

    local api_url="https://api.github.com/repos/${GITHUB_REPO}/releases/latest"
    local ver=""
    if command -v curl &>/dev/null; then
        ver=$(curl -fsSL "$api_url" 2>/dev/null | grep '"tag_name":' | head -n 1 | sed -E 's/.*"v?([^"]+)".*/\1/')
    elif command -v wget &>/dev/null; then
        ver=$(wget -qO- "$api_url" 2>/dev/null | grep '"tag_name":' | head -n 1 | sed -E 's/.*"v?([^"]+)".*/\1/')
    fi

    if [ -z "$ver" ]; then
        # GitHub API 匿名调用有速率限制（共享出口 IP 时极易 403），
        # version.json 走 raw.githubusercontent.com，不受该限制。
        local vj_url="https://raw.githubusercontent.com/${GITHUB_REPO}/main/version.json"
        local vj=""
        # 注意：这里是裸赋值而非管道，本脚本顶部是 set -e，
        # 必须加 || true，否则 curl 失败会直接终止安装而不是继续降级。
        if command -v curl &>/dev/null; then
            vj=$(curl -fsSL "$vj_url" 2>/dev/null) || vj=""
        elif command -v wget &>/dev/null; then
            vj=$(wget -qO- "$vj_url" 2>/dev/null) || vj=""
        fi
        if [ -n "$vj" ]; then
            ver=$(printf '%s\n' "$vj" | grep '"version"' | head -n 1 | sed -E 's/.*"v?([^"]+)".*/\1/')
        fi
    fi
    echo "$ver"
}

# 读取已安装二进制自报的版本号；读不到则输出空字符串
installed_version() {
    local bin="${INSTALL_DIR}/minipanel"
    if [ ! -x "$bin" ]; then
        echo ""
        return 0
    fi
    "$bin" -v 2>/dev/null | sed -nE 's/^Mini Panel ([0-9]+\.[0-9]+\.[0-9]+).*/\1/p' | head -n 1
}

# 版本比较：$1 大于 $2 时返回 0，否则返回 1
semver_gt() {
    if [ "$1" = "$2" ]; then
        return 1
    fi
    [ "$(printf '%s\n%s\n' "$1" "$2" | sort -V | tail -n 1)" = "$1" ]
}

VERSION=$(get_latest_version)
if [ -z "$VERSION" ]; then
    log_err "无法获取最新版本号：GitHub API 与 version.json 均不可达。"
    log_info "为安全起见已中止，安装目录未被修改。"
    log_info "如需继续，可手动指定版本后重试，例如："
    log_info "  MINIPANEL_VERSION=7.1.1 bash install.sh"
    exit 1
fi
log_info "Latest version detected: v${VERSION}"

# 更新模式下拒绝降级或同版本覆盖，避免用旧包覆盖当前安装
if [ "${MINIPANEL_UPDATE:-}" = "1" ]; then
    _installed=$(installed_version)
    if [ -n "$_installed" ] && ! semver_gt "$VERSION" "$_installed"; then
        log_err "目标版本 v${VERSION} 不高于当前已安装版本 v${_installed}，已拒绝覆盖。"
        exit 1
    fi
fi

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
        HAS_CURL=true
    else
        HAS_CURL=false
    fi

    if command -v wget &>/dev/null; then
        HAS_WGET=true
    else
        HAS_WGET=false
    fi

    if [ "$HAS_CURL" = false ] && [ "$HAS_WGET" = false ]; then
        log_err "curl or wget is required"
        exit 1
    fi

    log_ok "Dependencies check passed"
}

download_file() {
    local url="$1"
    local output="$2"

    if [ "$HAS_CURL" = true ]; then
        local proxy_args=""
        if [ -n "${HTTPS_PROXY:-}" ]; then
            proxy_args="-x ${HTTPS_PROXY}"
        elif [ -n "${https_proxy:-}" ]; then
            proxy_args="-x ${https_proxy}"
        elif [ -n "${HTTP_PROXY:-}" ]; then
            proxy_args="-x ${HTTP_PROXY}"
        elif [ -n "${http_proxy:-}" ]; then
            proxy_args="-x ${http_proxy}"
        elif [ -n "${ALL_PROXY:-}" ]; then
            proxy_args="-x ${ALL_PROXY}"
        elif [ -n "${all_proxy:-}" ]; then
            proxy_args="-x ${all_proxy}"
        fi
        curl --connect-timeout 30 --max-time 300 -fsSL ${proxy_args} -o "$output" "$url"
    else
        local proxy_args=""
        if [ -n "${HTTPS_PROXY:-}" ]; then
            proxy_args="-e https_proxy=${HTTPS_PROXY}"
        elif [ -n "${https_proxy:-}" ]; then
            proxy_args="-e https_proxy=${https_proxy}"
        elif [ -n "${HTTP_PROXY:-}" ]; then
            proxy_args="-e http_proxy=${HTTP_PROXY}"
        elif [ -n "${http_proxy:-}" ]; then
            proxy_args="-e http_proxy=${http_proxy}"
        elif [ -n "${ALL_PROXY:-}" ]; then
            proxy_args="-e all_proxy=${ALL_PROXY}"
        elif [ -n "${all_proxy:-}" ]; then
            proxy_args="-e all_proxy=${all_proxy}"
        fi
        wget --timeout=30 --tries=2 ${proxy_args} -qO "$output" "$url"
    fi
}

download_with_fallback() {
    local url="$1"
    local output="$2"
    local mirrors=(
        "$url"
        "https://gh-proxy.com/${url#https://github.com/}"
        "https://ghproxy.cn/${url#https://github.com/}"
        "https://ghps.cc/${url#https://github.com/}"
    )

    for mirror in "${mirrors[@]}"; do
        log_info "Trying: ${mirror}"
        if download_file "$mirror" "$output"; then
            if [ -s "$output" ]; then
                return 0
            fi
            rm -f "$output"
        fi
    done
    return 1
}

install_from_release() {
    log_info "Installing from pre-built release..."

    local download_url="https://github.com/${GITHUB_REPO}/releases/download/v${VERSION}/minipanel-${VERSION}-${OS}-${ARCH}.tar.gz"
    local tmp_dir=$(mktemp -d)

    log_info "Downloading from ${download_url}"
    if ! download_with_fallback "$download_url" "${tmp_dir}/minipanel.tar.gz"; then
        log_err "Failed to download release from all mirrors."
        log_info "You can try with a proxy:"
        log_info "  export HTTPS_PROXY=http://127.0.0.1:7890"
        log_info "Or re-run the installer via a mirror:"
        log_info "  curl -fsSL \"https://gh-proxy.com/https://raw.githubusercontent.com/YevolcXhb/mini-panel/main/install.sh\" -o /tmp/install.sh && bash /tmp/install.sh"
        log_info "  curl -fsSL \"https://ghproxy.cn/https://raw.githubusercontent.com/YevolcXhb/mini-panel/main/install.sh\" -o /tmp/install.sh && bash /tmp/install.sh"
        log_info "Or manually download from: https://github.com/YevolcXhb/mini-panel/releases"
        log_info "  (the release tarball already bundles install.sh — extract and run it locally)"
        rm -rf "${tmp_dir}"
        exit 1
    fi

    log_info "Extracting to ${INSTALL_DIR}..."
    mkdir -p "${INSTALL_DIR}"
    tar -xzf "${tmp_dir}/minipanel.tar.gz" -C "${INSTALL_DIR}" --strip-components=1
    rm -rf "${tmp_dir}"

    if [ -f "${INSTALL_DIR}/DockRoot" ]; then
        chmod +x "${INSTALL_DIR}/DockRoot"
        log_ok "DockRoot binary installed"
    else
        log_warn "DockRoot not found in package, downloading..."
        DR_BASE="https://fw0.koolcenter.com/binary/DockRoot"
        DR_URL="${DR_BASE}/DockRoot.linux.${ARCH}"
        if download_file "$DR_URL" "${INSTALL_DIR}/DockRoot"; then
            chmod +x "${INSTALL_DIR}/DockRoot"
            log_ok "DockRoot downloaded"
        else
            log_err "Failed to download DockRoot. You may need to install it manually."
        fi
    fi

    chmod +x "${INSTALL_DIR}/minipanel"

    # Generate dockroot.json if not exists
    if [ ! -f "${INSTALL_DIR}/dockroot.json" ]; then
        cat > "${INSTALL_DIR}/dockroot.json" <<'DREOF'
{
  "registry-mirrors": [
    "https://docker.1ms.run",
    "https://docker.m.daocloud.io",
    "https://registry.istoreos.com",
    "https://kooldocker.openpop.cn",
    "https://kooldocker.gvpu.cn",
    "https://docker1.linkease.com:60005"
  ],
  "data-root": "DATA_ROOT_PLACEHOLDER",
  "useKspeeder": true
}
DREOF
        sed -i "s|DATA_ROOT_PLACEHOLDER|${INSTALL_DIR}/data/containers|g" "${INSTALL_DIR}/dockroot.json"
        mkdir -p "${INSTALL_DIR}/data/containers"
        log_ok "dockroot.json generated"
    fi

    # Download ruri and kspeeder
    if [ -f "${INSTALL_DIR}/DockRoot" ]; then
        log_info "Downloading DockRoot dependencies (ruri, kspeeder)..."
        cd "${INSTALL_DIR}"
        "${INSTALL_DIR}/DockRoot" ensuredeps 2>&1 || log_warn "ensuredeps failed, you may need to run it manually"
    fi

    log_ok "Binary installed to ${INSTALL_DIR}"

    # Check if frontend static files exist
    if [ ! -f "${INSTALL_DIR}/static/index.html" ]; then
        log_warn "Frontend static files not found in release package!"
        log_info "Attempting to download frontend assets separately..."
        local static_url="https://github.com/${GITHUB_REPO}/releases/download/v${VERSION}/static.tar.gz"
        if download_with_fallback "$static_url" "${INSTALL_DIR}/static.tar.gz"; then
            mkdir -p "${INSTALL_DIR}/static"
            tar -xzf "${INSTALL_DIR}/static.tar.gz" -C "${INSTALL_DIR}/static"
            rm -f "${INSTALL_DIR}/static.tar.gz"
            log_ok "Frontend assets downloaded and extracted"
        else
            log_err "Failed to download frontend assets!"
            log_info "The panel will not work properly without frontend files."
            log_info "Please build from source: git clone and run install.sh from source directory"
        fi
    fi
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

        log_info "Building DockRoot..."
        cd "${SCRIPT_DIR}/backend/cmd/dockroot"
        CGO_ENABLED=0 GOOS=linux GOARCH=${ARCH} go build -trimpath -ldflags '-s -w' \
            -tags 'containers_image_openpgp exclude_graphdriver_btrfs' \
            -o "${INSTALL_DIR}/DockRoot" .
        cd "${SCRIPT_DIR}/backend"
    else
        CGO_ENABLED=0 go build -trimpath -ldflags '-s -w' \
            -tags 'osusergo netgo static_build' \
            -o "${INSTALL_DIR}/minipanel" ./cmd/server

        log_info "Building DockRoot..."
        cd "${SCRIPT_DIR}/backend/cmd/dockroot"
        CGO_ENABLED=0 go build -trimpath -ldflags '-s -w' \
            -tags 'containers_image_openpgp exclude_graphdriver_btrfs' \
            -o "${INSTALL_DIR}/DockRoot" .
        cd "${SCRIPT_DIR}/backend"
    fi

    # Copy static files
    mkdir -p "${INSTALL_DIR}/static"
    cp -r "${SCRIPT_DIR}/backend/static/"* "${INSTALL_DIR}/static/" 2>/dev/null || true

    log_ok "Build complete"
}

setup_environment() {
    log_info "Setting up environment..."

    # Create directories
    mkdir -p "${DATA_DIR}"
    mkdir -p "${DATA_DIR}/containers"
    mkdir -p "${DATA_DIR}/logs"

    # Generate dockroot.json if not exists
    if [ ! -f "${INSTALL_DIR}/dockroot.json" ] && [ -f "${INSTALL_DIR}/DockRoot" ]; then
        cat > "${INSTALL_DIR}/dockroot.json" <<DREOF
{
    "registry-mirrors": [
      "https://docker.1ms.run",
      "https://docker.m.daocloud.io",
      "https://registry.istoreos.com",
      "https://kooldocker.openpop.cn",
      "https://kooldocker.gvpu.cn",
      "https://docker1.linkease.com:60005"
  ],
  "data-root": "${DATA_DIR}/containers",
  "useKspeeder": true
}
DREOF
        log_ok "dockroot.json generated"
    fi

    # Download ruri and kspeeder
    if [ -f "${INSTALL_DIR}/DockRoot" ]; then
        log_info "Downloading DockRoot dependencies (ruri, kspeeder)..."
        cd "${INSTALL_DIR}"
        "${INSTALL_DIR}/DockRoot" ensuredeps 2>&1 || log_warn "ensuredeps failed, run 'DockRoot ensuredeps' manually later"
    fi

    # Generate config if not exists
    if [ ! -f "${INSTALL_DIR}/config.yaml" ]; then
        cat > "${INSTALL_DIR}/config.yaml" <<EOF
port: 8888
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
echo "Mini Panel started on http://0.0.0.0:8888"
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

    # Create restart script
    cat > "${INSTALL_DIR}/restart.sh" <<'EOF'
#!/bin/bash
cd "$(dirname "$0")"
if [ -f minipanel.pid ]; then
    kill $(cat minipanel.pid) 2>/dev/null
    rm -f minipanel.pid
    sleep 1
fi
./minipanel &
echo $! > minipanel.pid
echo "Mini Panel restarted on http://0.0.0.0:8888"
EOF
    chmod +x "${INSTALL_DIR}/restart.sh"

    # Create status script
    cat > "${INSTALL_DIR}/status.sh" <<'EOF'
#!/bin/bash
cd "$(dirname "$0")"
if [ -f minipanel.pid ] && kill -0 $(cat minipanel.pid) 2>/dev/null; then
    echo "Mini Panel is running (PID: $(cat minipanel.pid))"
    echo "Access: http://$(hostname -I 2>/dev/null | awk '{print $1}' || echo 'localhost'):8888"
else
    echo "Mini Panel is not running"
fi
EOF
    chmod +x "${INSTALL_DIR}/status.sh"

    # Create symlink
    if [ -d "/usr/local/bin" ]; then
        ln -sf "${INSTALL_DIR}/minipanel" /usr/local/bin/minipanel 2>/dev/null || true
        if [ -f "${INSTALL_DIR}/DockRoot" ]; then
            ln -sf "${INSTALL_DIR}/DockRoot" /usr/local/bin/dockroot 2>/dev/null || true
        fi
    fi

    # Add to PATH if not already there
    local profile_file=""
    if [ -f "$HOME/.bashrc" ]; then
        profile_file="$HOME/.bashrc"
    elif [ -f "$HOME/.profile" ]; then
        profile_file="$HOME/.profile"
    elif [ -f "$HOME/.bash_profile" ]; then
        profile_file="$HOME/.bash_profile"
    fi

    if [ -n "$profile_file" ]; then
        if ! grep -q "minipanel" "$profile_file" 2>/dev/null; then
            cat >> "$profile_file" <<EOF

# Mini Panel
export MINIPANEL_DIR="${INSTALL_DIR}"
export MINIPANEL_DATA="${DATA_DIR}"
export PATH="${INSTALL_DIR}:\$PATH"
EOF
            log_ok "Environment variables added to ${profile_file}"
        fi
    fi

    # Export for current session
    export PATH="${INSTALL_DIR}:${PATH}"
    export MINIPANEL_DIR="${INSTALL_DIR}"
    export MINIPANEL_DATA="${DATA_DIR}"

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
    echo "Access URL: http://${ip}:8888"
    echo ""
    echo "Default Login:"
    echo "  Username: admin"
    echo "  Password: admin123"
    echo ""

    if [ "$IS_ANDROID" = true ]; then
        echo -e "${YELLOW}Android Chroot Tips:${NC}"
        echo "  - Run as root for full functionality"
        echo "  - DockRoot is now bundled and ready to use"
        echo "  - Use tmux/screen to keep running in background"
        echo ""
    fi

    echo -e "${YELLOW}Environment Variables:${NC}"
    echo "  MINIPANEL_DIR=${INSTALL_DIR}"
    echo "  MINIPANEL_DATA=${DATA_DIR}"
    echo "  PATH includes ${INSTALL_DIR}"
    echo ""
    local shell_profile="~/.bashrc"
    if [ -f "$HOME/.profile" ]; then
        shell_profile="~/.profile"
    fi
    echo "  Run 'source ${shell_profile}' to apply now"
    echo ""
}

main() {
    SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

    print_banner
    check_deps

    # Check if running from source directory (not online install)
    if [ "$IS_ONLINE" = false ] && [ -d "${SCRIPT_DIR}/backend" ] && [ -d "${SCRIPT_DIR}/frontend" ]; then
        log_info "Source directory detected"
        install_from_source
    else
        log_info "Downloading release package..."
        install_from_release
    fi

    setup_environment

    # Auto-restart if minipanel is already running (update scenario)
    if [ -f "${INSTALL_DIR}/minipanel.pid" ]; then
        local old_pid=$(cat "${INSTALL_DIR}/minipanel.pid" 2>/dev/null)
        if [ -n "$old_pid" ] && kill -0 "$old_pid" 2>/dev/null; then
            log_info "Existing Mini Panel process detected (PID: $old_pid), restarting to apply updates..."
            "${INSTALL_DIR}/stop.sh" >/dev/null 2>&1 || true
            sleep 1
            "${INSTALL_DIR}/start.sh" >/dev/null 2>&1
            log_ok "Mini Panel restarted successfully"
        fi
    fi

    print_finish
}

main "$@"
