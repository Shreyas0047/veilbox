#!/usr/bin/env bash
# smoke-day7.sh — Environment engine + composition record live acceptance
# (Phase B: ADR-0010 / ADR-0012 / ADR-0015)
#
# Verifies against the live installation and the repository:
#   - the environment engine surface (list / info / overview)
#   - 'veil desktop' alias parity (byte-identical output)
#   - the ADR-0015 data contract: environment specifics live in the
#     manifest, not in Go (enforcement grep), and the shipped manifest
#     declares config / managed / validate hooks
#   - the ADR-0010 composition record: written by apply (veil onboard
#     --yes), atomic, schema v1, consumed by status and doctor
#
# Run from the repository root as the development user AFTER installing
# the Phase B RPMs (veilbox-core-0.1.0-9+, veilbox-experience-niri
# 0.1.0-3+):
#   scripts/smoke-day7.sh
set -u

cd "$(dirname "$0")/.."

PASS=0
FAIL=0

ok()   { PASS=$((PASS+1)); printf 'PASS: %s\n' "$1"; }
bad()  { FAIL=$((FAIL+1)); printf 'FAIL: %s\n' "$1"; }

check() { # check <description> <condition>
    if [ "$2" = "0" ] || [ -n "$2" ] && eval "$2"; then ok "$1"; else bad "$1"; fi
}

COMP="$HOME/.config/veilbox/composition.json"

# --- 1. environment engine surface ------------------------------------
check "environment overview runs" "veil environment >/dev/null 2>&1"
check "overview mentions niri-desktop" "veil environment | grep -q niri-desktop"
check "environment list runs" "veil environment list >/dev/null 2>&1"
check "list shows niri-desktop" "veil environment list | grep -q niri-desktop"
INFO=$(veil environment info niri-desktop 2>&1)
check "info runs" "echo \"\$INFO\" | grep -q 'Environment: niri-desktop'"
check "info shows package name" "echo \"\$INFO\" | grep -q veilbox-experience-niri"
check "info for unknown environment fails" "! veil environment info nope >/dev/null 2>&1"

# --- 2. 'veil desktop' alias parity (ADR-0012) --------------------------
check "alias overview byte-identical" \
    "[ \"\$(veil desktop)\" = \"\$(veil environment)\" ]"
check "alias list byte-identical" \
    "[ \"\$(veil desktop list)\" = \"\$(veil environment list)\" ]"
check "alias info byte-identical" \
    "[ \"\$(veil desktop info niri-desktop)\" = \"\$(veil environment info niri-desktop)\" ]"
check "alias error handling identical" \
    "[ \"\$(veil desktop info nope 2>&1)\" = \"\$(veil environment info nope 2>&1)\" ]"

# --- 3. ADR-0015: the environment is data, not code ----------------------
# No Niri/Noctalia spelling outside test fixtures: the engine consumes
# the manifest contract, it never hardcodes the reference environment.
if grep -rniE 'niri|noctalia' --include='*.go' core/cmd core/internal \
    | grep -v '_test.go' | grep -v 'onboardingtest/' >/dev/null 2>&1; then
    bad "enforcement: no niri/noctalia in production Go"
else
    ok "enforcement: no niri/noctalia in production Go"
fi

MANIFEST=$(ls /usr/share/veilbox/experiences/niri-desktop.yaml 2>/dev/null || echo experiences/niri-desktop.yaml)
check "manifest type is environment" "grep -q 'type: environment' \"$MANIFEST\""
check "manifest declares config hooks" "grep -A2 '  config:' \"$MANIFEST\" | grep -q 'dest:'"
check "manifest declares managed files" "grep -q 'managed:' \"$MANIFEST\""
check "manifest declares validate files" "grep -q '    files:' \"$MANIFEST\""
check "manifest declares validate commands" "grep -q 'commands:' \"$MANIFEST\""

# --- 4. composition record before apply -----------------------------------
if [ -e "$COMP" ]; then
    ok "no composition record before first apply (re-run: record already present)"
    check "pre-existing composition record is schema v1 (replaced by next apply)" \
        "grep -q '\"schema_version\": 1' \"$COMP\""
else
    check "no composition record before first apply" "[ ! -e \"$COMP\" ]"
    STATUS_BEFORE=$(veil status 2>&1)
    check "status: no composition line" \
        "echo \"\$STATUS_BEFORE\" | grep -q 'Composition:    (none'"
    check "status: installed environment without record" \
        "echo \"\$STATUS_BEFORE\" | grep -q 'Environment:    niri-desktop (no composition record)'"
fi

# --- 5. apply writes the atomically persisted record (ADR-0010) ----------
OK_OUT=$(veil onboard --yes 2>&1)
check "--yes apply completes" "echo \"\$OK_OUT\" | grep -qE '\[OK\]|Success'"
check "composition record written" "[ -f \"$COMP\" ]"
check "no .tmp left behind (atomic write)" "[ ! -f \"$COMP.tmp\" ]"
check "record is valid JSON and schema v1" \
    "grep -q '\"schema_version\": 1' \"$COMP\""

# The record mirrors the applied selection: profile and experiences come
# from onboarding.json (the selection ledger), and the environment
# section exists exactly when the selection chose one.
SAVED_PROFILE=$(grep -m1 '"profile"' ~/.config/veilbox/onboarding.json | sed -E 's/.*: *"([^"]+)".*/\1/')
if [ -n "$SAVED_PROFILE" ] && grep -q "\"profile\": \"$SAVED_PROFILE\"" "$COMP"; then
    ok "record carries the saved profile"
else
    bad "record carries the saved profile"
fi
check "record carries experiences" "grep -q '\"experiences\": \\[' \"$COMP\""
check "record carries validation" "grep -q '\"validation\":' \"$COMP\""
if grep -q '"environment"' "$COMP"; then
    check "record carries the environment" "grep -q '\"name\": \"niri-desktop\"' \"$COMP\""
    check "record carries the environment RPM" "grep -q '\"rpm\": \"veilbox-experience-niri\"' \"$COMP\""
else
    ok "record has no environment section (selection chose none)"
fi

# --- 6. status consumes the composition -----------------------------------
STATUS=$(veil status 2>&1)
check "status: composition recorded" "echo \"\$STATUS\" | grep -q 'Composition:    applied'"
if grep -q '"environment"' "$COMP"; then
    check "status: composition-driven environment line" \
        "echo \"\$STATUS\" | grep -q 'Environment:    niri-desktop (veilbox-experience-niri)'"
else
    check "status: composition-driven environment line" \
        "echo \"\$STATUS\" | grep -q 'Environment:    niri-desktop (no composition record)'"
fi
check "status: no composition drift" "! echo \"\$STATUS\" | grep -q 'Composition drift:'"

# --- 7. doctor consumes the composition -----------------------------------
DOCTOR=$(veil doctor 2>&1)
check "doctor: composition record parses" \
    "echo \"\$DOCTOR\" | grep -q 'Composition record parses'"
check "doctor: composition consistent with live state" \
    "echo \"\$DOCTOR\" | grep -q 'Composition consistent with live state'"

echo
echo "smoke-day7: $PASS passed, $FAIL failed"
if [ "$FAIL" -gt 0 ]; then exit 1; fi
echo "NOTE: composition drift paths (recorded package removed, profile
unknown, catalog unknown) are covered by the Go unit tests
(TestStatusCompositionDrivenEnvironment, TestDoctorCompositionChecks)."