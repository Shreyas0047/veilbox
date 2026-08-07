#!/usr/bin/env bash
# Day 3 live acceptance test: profile intent engine + experience
# lifecycle + removal semantics.
#
# Prereqs: RPMs built (scripts/build-rpms.sh) and the veilbox-dev
# repository composed (scripts/compose-repo.sh).
#
# Run: scripts/smoke-day3.sh
#
# Removal semantics: DNF remains the transaction authority. Veilbox
# never adds custom package-removal logic. This script proves it with
# before/after RPM database snapshots: removing a Veilbox meta-RPM must
# never remove user-preexisting packages, and cleanup may only affect
# packages introduced solely as dependencies of the experience.
set -euo pipefail

PASS=0
FAIL=0
ok()   { PASS=$((PASS+1)); echo "PASS: $1"; }
bad()  { FAIL=$((FAIL+1)); echo "FAIL: $1"; }
# pipefail must be off inside checks: grep -q exits after the first
# match, the pipeline writer then dies with SIGPIPE (141), and
# pipefail would report the whole check as failed.
check() { if ( set +o pipefail; eval "$2" ); then ok "$1"; else bad "$1"; fi }

snapshot() { rpm -qa --queryformat '%{NAME}\n' | sort; }

echo "== controlled state: veilbox-core 0.1.0-2, no experiences =="
# Refresh root's dnf cache: sudo dnf uses a separate metadata cache that
# may predate the last compose-repo.sh run. 'upgrade' (not 'install')
# reliably moves an older installed core to the newest repo version.
sudo dnf makecache >/dev/null
sudo dnf upgrade -y veilbox-core >/dev/null
for p in $(rpm -qa 'veilbox-experience-*'); do sudo dnf remove -y "$p" >/dev/null; done

check "veil binary installed"        'command -v veil'
check "core 0.1.0-2 installed"       'rpm -q veilbox-core-0.1.0-2.fc44 >/dev/null'
check "4 profiles shipped"           '[ -f /usr/share/veilbox/profiles/sre.yaml ] && [ -f /usr/share/veilbox/profiles/platform-engineer.yaml ] && [ -f /usr/share/veilbox/profiles/cloud-engineer.yaml ]'
check "4 experiences shipped"        '[ -f /usr/share/veilbox/experiences/base-ops.yaml ] && [ -f /usr/share/veilbox/experiences/observability-cli.yaml ] && [ -f /usr/share/veilbox/experiences/terminal-ops.yaml ]'

echo "== profile list =="
check "list shows 4 profiles"        '[ "$(veil profile list | wc -l)" = "4" ]'
check "apply devops"                 'veil profile apply devops >/dev/null'
check "list marks devops active"     'veil profile list | grep -q "devops (active)"'

echo "== profile show =="
check "show role"                    'veil profile show devops | grep -q "Role: devops"'
check "show recommended"             'veil profile show devops | grep -q -- "- base-ops"'
check "show optional"                'veil profile show devops | grep -q -- "- observability-cli"'

echo "== profile apply is intent only =="
check "apply prints recommendation summary" 'veil profile apply devops | grep -q "recommends the following experiences"'
check "apply installs nothing"       '[ -z "$(rpm -qa "veilbox-experience-*")" ]'

echo "== profile diff =="
check "diff shows 3 missing"         'veil profile diff devops | grep -q "3 recommended experience(s) missing"'
check "diff lists recommended"       'veil profile diff devops | grep -q -- "- base-ops"'
check "diff deterministic"           'diff <(veil profile diff devops) <(veil profile diff devops)'

echo "== profile sync confirmation gate =="
check "sync asks confirmation"       'printf "no\n" | veil profile sync | grep -q "Proceed? \[y/N\]"'
check "sync aborts on no"            '! printf "no\n" | veil profile sync >/dev/null'
check "nothing installed after abort" '[ -z "$(rpm -qa "veilbox-experience-*")" ]'

echo "== profile sync --yes =="
check "sync installs 3 recommended"  'veil profile sync --yes | grep -q "Profile sync complete: 3 installed"'
check "meta base-ops installed"      'rpm -q veilbox-experience-base-ops >/dev/null'
check "meta terminal-ops installed"  'rpm -q veilbox-experience-terminal-ops >/dev/null'
check "meta networking-tools installed" 'rpm -q veilbox-experience-networking-tools >/dev/null'
check "status reports synced"        'veil status | grep -q "Profile sync:   synced"'
check "status shows profile"         'veil status | grep -q "Profile:        devops"'
check "second sync already synced"   'veil profile sync --yes | grep -q "already synced"'

echo "== manual extra experience is preserved =="
check "manual install observability-cli" 'veil experience install observability-cli | grep -q "installed"'
check "status still synced"          'veil status | grep -q "Profile sync:   synced"'
check "sync keeps the extra"         'veil profile sync --yes | grep -q "already synced"'
check "extra still installed"        'rpm -q veilbox-experience-observability-cli >/dev/null'
check "diff shows optional installed" 'veil profile diff devops | grep -q "observability-cli (installed)"'

echo "== experience info =="
check "info description"             'veil experience info networking-tools | grep -q "Networking diagnostics"'
check "info packages"                'veil experience info networking-tools | grep -q -- "- bind-utils"'
check "info recommending profiles"   'veil experience info networking-tools | grep -q "Recommended by profiles:"'
check "info lists devops"            'veil experience info networking-tools | grep -q -- "- devops"'
check "info lists sre"               'veil experience info networking-tools | grep -q -- "- sre"'

echo "== removal semantics: snapshots before/after per experience =="
for EXP in base-ops terminal-ops networking-tools observability-cli; do
    META="veilbox-experience-${EXP}"
    # A package that is user-preexisting (installed independently of
    # Veilbox) for each experience. Reinstall it from scratch so its
    # RPM "reason" is user-installed, not dependency: dnf install on an
    # already-installed dependency is a no-op and leaves the dep reason
    # in place, which would make DNF's cleanup of the meta-package
    # legitimately remove it.
    PREE="git"
    case "${EXP}" in
        base-ops)           PREE="git" ;;
        terminal-ops)       PREE="htop" ;;
        networking-tools)   PREE="traceroute" ;;
        observability-cli)  PREE="jq" ;;
    esac
    sudo dnf remove -y "${META}" >/dev/null
    sudo dnf remove -y "${PREE}" >/dev/null 2>&1 || true
    sudo dnf install -y "${PREE}" >/dev/null
    PRE=$(snapshot)
    veil experience install "${EXP}" >/dev/null
    POST_INSTALL=$(snapshot)
    veil experience remove "${EXP}" >/dev/null
    FINAL=$(snapshot)

    INTRODUCED=$(comm -13 <(printf '%s\n' "${PRE}") <(printf '%s\n' "${POST_INSTALL}"))
    REMOVED=$(comm -13 <(printf '%s\n' "${POST_INSTALL}") <(printf '%s\n' "${FINAL}"))

    check "removal ${EXP}: preexisting packages survive" \
        '[ -z "$(comm -23 <(printf "%s\n" "${PRE}") <(printf "%s\n" "${FINAL}"))" ]'
    check "removal ${EXP}: cleanup only touches introduced packages" \
        '[ -z "$(comm -23 <(printf "%s\n" "${REMOVED}") <(printf "%s\n" "${INTRODUCED}"))" ]'
    check "removal ${EXP}: RPM database returns to pre-install state" \
        '[ "$(printf "%s" "${PRE}")" = "$(printf "%s" "${FINAL}")" ]'
    check "removal ${EXP}: preexisting ${PREE} still installed" \
        "rpm -q ${PREE} >/dev/null"
done

echo "== switch to sre =="
check "apply sre"                    'veil profile apply sre | grep -q "applied"'
check "status profile sre"           'veil status | grep -q "Profile:        sre"'
check "diff sre lists observability-cli missing" 'veil profile diff sre | grep -q -- "- observability-cli"'
check "diff sre terminal-ops optional not installed" 'veil profile diff sre | grep -q "terminal-ops (not installed)"'
check "sync sre installs 3"          'veil profile sync --yes | grep -q "Profile sync complete: 3 installed"'
check "status synced after sre sync" 'veil status | grep -q "Profile sync:   synced"'

echo "== extras visible but never removed =="
check "diff platform-engineer shows extra" 'veil profile diff platform-engineer | grep -q "Extra experiences (kept, not in profile):"'
check "extra is observability-cli"   'veil profile diff platform-engineer | grep -q -- "- observability-cli"'
check "sync does not remove extra"   'veil profile sync --yes >/dev/null && rpm -q veilbox-experience-observability-cli >/dev/null'

echo "== doctor =="
check "doctor passes"                'veil doctor >/dev/null'

echo "== repo contains all packages =="
check "repo has 5 packages"          '[ "$(dnf repoquery --repo veilbox-dev | wc -l)" = "5" ]'

echo
echo "smoke-day3: ${PASS} passed, ${FAIL} failed"
[[ ${FAIL} -eq 0 ]] || exit 1
echo "NOTE: reboot persistence is verified separately (next session)."
echo "NOTE: final state: profile=sre, experiences=base-ops, observability-cli, networking-tools."
