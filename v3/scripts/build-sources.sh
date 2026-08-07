#!/usr/bin/env bash
# Prepares RPM source tarballs into packages/SOURCES/.
# Run from anywhere; operates relative to the v3/ tree.
set -euo pipefail

cd "$(dirname "$0")/.."

VERSION="${1:-0.1.0}"
NAME="veilbox-core"
STAGE="packages/SOURCES/${NAME}-${VERSION}"
OUT="packages/SOURCES/${NAME}-${VERSION}.tar.gz"

rm -rf "${STAGE}" "${OUT}"
mkdir -p "${STAGE}/cmd" "${STAGE}/internal"

# Go module content (vendor/ included: builds are fully offline).
cp core/go.mod core/go.sum core/vendor "${STAGE}/" 2>/dev/null || cp core/go.mod "${STAGE}/"
cp -r core/vendor "${STAGE}/vendor"
cp -r core/cmd/. "${STAGE}/cmd/"
cp -r core/internal/. "${STAGE}/internal/"

# Shipped data (profiles = intent, experiences = capability catalog).
cp -r profiles "${STAGE}/profiles"
cp -r experiences "${STAGE}/experiences"

# License for %license.
cp LICENSE "${STAGE}/LICENSE"

tar -czf "${OUT}" -C packages/SOURCES "${NAME}-${VERSION}"
rm -rf "${STAGE}"
echo "built ${OUT}"
