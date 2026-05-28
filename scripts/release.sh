#!/bin/bash

set -e

VERSION="${1:-1.0.0}"
DIST_DIR="dist"
BUILD_DIR="build"

echo "Creating release v${VERSION}..."

rm -rf "${DIST_DIR}"
mkdir -p "${DIST_DIR}"

# Build backend binaries
cd backend
bash build.sh
cd ..

# Build frontend
cd frontend
npm install
npm run build
cd ..

# Package each platform
for binary in ${BUILD_DIR}/minipanel-*; do
    if [ -f "${binary}" ]; then
        filename=$(basename "${binary}")
        platform=${filename#minipanel-}
        tmpdir=$(mktemp -d)
        
        mkdir -p "${tmpdir}/minipanel"
        cp "${binary}" "${tmpdir}/minipanel/minipanel"
        cp -r backend/static "${tmpdir}/minipanel/" 2>/dev/null || true
        cp install.sh "${tmpdir}/minipanel/"
        cp README.md "${tmpdir}/minipanel/"
        
        cat > "${tmpdir}/minipanel/config.yaml.example" <<EOF
port: 8080
log_level: info
db_path: ./data/minipanel.db
data_dir: ./data
jwt_secret: change-me-to-a-random-string
EOF
        
        tar -czf "${DIST_DIR}/minipanel-${VERSION}-${platform}.tar.gz" -C "${tmpdir}" minipanel
        rm -rf "${tmpdir}"
        
        echo "Packaged: ${DIST_DIR}/minipanel-${VERSION}-${platform}.tar.gz"
    fi
done

echo ""
echo "Release packages created in ${DIST_DIR}/"
ls -lh "${DIST_DIR}/"
