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

for spec in "${TOPDIR}"/SPECS/*.spec; do
    rpmbuild --define "_topdir ${TOPDIR}" -ba "${spec}"
done

echo "--- built RPMs ---"
find "${TOPDIR}/RPMS" -name '*.rpm' -printf '%f\n' | sort
