#!/bin/bash

set -euo pipefail

VERSION="${VERSION:-1.0.0}"
BUILD_TIME=$(date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")

LDFLAGS="-X main.version=${VERSION} -X main.buildTime=${BUILD_TIME} -X main.gitCommit=${GIT_COMMIT} -s -w"
BUILDS="linux/amd64 linux/arm64 linux/arm"
OUTPUT_DIR="../build"

echo "========================================"
echo "  Mini Panel Builder v${VERSION}"
echo "========================================"
echo ""

mkdir -p "${OUTPUT_DIR}"

for platform in ${BUILDS}; do
    GOOS=${platform%/*}
    GOARCH=${platform#*/}
    output="${OUTPUT_DIR}/minipanel-${GOOS}-${GOARCH}"
    
    echo "Building for ${GOOS}/${GOARCH}..."
    
    CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} \
        go build -trimpath -ldflags "${LDFLAGS}" \
        -tags 'osusergo netgo static_build' \
        -o "${output}" ./cmd/server
    
    if command -v upx &>/dev/null; then
        upx -q "${output}" 2>/dev/null || true
    fi
    
    echo "  -> ${output}"
done

echo ""
echo "All builds complete in ${OUTPUT_DIR}/"
