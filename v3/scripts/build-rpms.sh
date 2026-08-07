#!/usr/bin/env bash
# Builds all Veilbox RPMs with rpmbuild (user-level, no root).
# Output: packages/build/RPMS and packages/build/SRPMS.
set -euo pipefail

cd "$(dirname "$0")/.."

TOPDIR="${PWD}/packages/build"
SPECS="${PWD}/packages/SPECS"

rm -rf "${TOPDIR}"
mkdir -p "${TOPDIR}"/{BUILD,BUILDROOT,RPMS,SOURCES,SPECS,SRPMS}

cp "${SPECS}"/*.spec "${TOPDIR}/SPECS/"
cp packages/SOURCES/*.tar.gz "${TOPDIR}/SOURCES/" 2>/dev/null || true

rpmbuild --define "_topdir ${TOPDIR}" -ba "${TOPDIR}/SPECS/veilbox-core.spec"
rpmbuild --define "_topdir ${TOPDIR}" -ba "${TOPDIR}/SPECS/veilbox-experience-networking-tools.spec"

echo "--- built RPMs ---"
find "${TOPDIR}/RPMS" -name '*.rpm' -printf '%f\n' | sort
