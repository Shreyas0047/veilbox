#!/usr/bin/env bash
# Day 2 vertical slice live smoke test.
#
# Prereqs: RPMs built (scripts/build-rpms.sh) and the veilbox-dev
# repository composed (scripts/compose-repo.sh).
#
# Run: scripts/smoke-day2.sh
set -euo pipefail

PASS=0
FAIL=0
ok()   { PASS=$((PASS+1)); echo "PASS: $1"; }
bad()  { FAIL=$((FAIL+1)); echo "FAIL: $1"; }
# pipefail must be off inside checks: grep -q exits after the first
# match, the pipeline writer then dies with SIGPIPE (141), and
# pipefail would report the whole check as failed.
check() { if ( set +o pipefail; eval "$2" ); then ok "$1"; else bad "$1"; fi }

echo "== smoke-day2: installing veilbox-core from veilbox-dev repo =="
sudo dnf install -y veilbox-core

check "veil binary installed"  'command -v veil'
check "veil version"           '[ "$(veil version)" = "veil 0.1.0" ]'
check "catalog shipped"        '[ -f /usr/share/veilbox/experiences/networking-tools.yaml ]'
check "profile shipped"        '[ -f /usr/share/veilbox/profiles/devops.yaml ]'

echo "== profile lifecycle =="
check "profile reports none initially" 'veil profile | grep -q "No profile configured"'
check "profile apply devops"           'veil profile apply devops | grep -q "applied"'
check "profile shows devops"           'veil profile | grep -q "Profile: devops"'
check "state file exists"              '[ -f "$HOME/.config/veilbox/state.json" ]'
check "state holds devops"             'grep -q "active_profile" "$HOME/.config/veilbox/state.json"'

echo "== experience list =="
check "list shows networking-tools available" 'veil experience list | grep -q "networking-tools.*available"'
check "list shows terminal-ops planned"       'veil experience list | grep -q "terminal-ops.*planned"'

echo "== experience install (DNF, by package name) =="
check "install networking-tools"  'veil experience install networking-tools | grep -q "installed"'
check "meta-rpm installed"        'rpm -q veilbox-experience-networking-tools >/dev/null'
check "dep bind-utils"            'rpm -q bind-utils >/dev/null'
check "dep traceroute"            'rpm -q traceroute >/dev/null'
check "dep nmap-ncat"             'rpm -q nmap-ncat >/dev/null'
check "dep iproute"               'rpm -q iproute >/dev/null'
check "dep tcpdump"               'rpm -q tcpdump >/dev/null'
check "list now shows installed"  'veil experience list | grep -q "networking-tools.*installed"'
check "install is idempotent (errors already installed)" '! veil experience install networking-tools >/dev/null 2>&1'

echo "== status and doctor =="
check "status shows core"         'veil status | grep -q "veilbox-core-0.1.0"'
check "status shows profile"      'veil status | grep -q "Profile:.*devops"'
check "status shows experience"   'veil status | grep -q "veilbox-experience-networking-tools"'
check "status shows Fedora"       'veil status | grep -q "Fedora"'
check "status shows repo"         'veil status | grep -q "veilbox-dev"'
check "doctor passes"             'veil doctor >/dev/null'

echo "== tool actually works =="
check "dig works"                 'dig +short localhost | grep -q "127.0.0.1"'

echo
echo "smoke-day2: ${PASS} passed, ${FAIL} failed"
[[ ${FAIL} -eq 0 ]] || exit 1
echo "NOTE: persistence across reboot is verified separately (next session)."
echo "NOTE: veil experience remove is exercised after the reboot test."
