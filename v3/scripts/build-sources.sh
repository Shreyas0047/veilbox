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

# Shipped data (profiles = intent, experiences = capability catalog,
# capabilities = capability manifests).
cp -r profiles "${STAGE}/profiles"
cp -r experiences "${STAGE}/experiences"
cp -r capabilities "${STAGE}/capabilities"

# License for %license.
cp LICENSE "${STAGE}/LICENSE"

# Environment experience sources (Veilbox-owned environment templates
# and the default wallpaper), staged as the veilbox-experience-niri
# tarball.
EXP="veilbox-experience-niri"
EXP_STAGE="packages/SOURCES/${EXP}-${VERSION}"
EXP_OUT="packages/SOURCES/${EXP}-${VERSION}.tar.gz"
rm -rf "${EXP_STAGE}" "${EXP_OUT}"
mkdir -p "${EXP_STAGE}/environment"
cp -r environment/niri "${EXP_STAGE}/environment/niri"
tar -czf "${EXP_OUT}" -C packages/SOURCES "${EXP}-${VERSION}"
rm -rf "${EXP_STAGE}"
echo "built ${EXP_OUT}"

tar -czf "${OUT}" -C packages/SOURCES "${NAME}-${VERSION}"
rm -rf "${STAGE}"
echo "built ${OUT}"
