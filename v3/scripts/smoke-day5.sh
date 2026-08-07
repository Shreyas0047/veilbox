#!/usr/bin/env bash
# smoke-day5.sh — Desktop Engine live acceptance (Day 5)
#
# Verifies the desktop experience surface against the real catalog
# BEFORE any desktop activation, including the strongest acceptance
# requirement: installing/upgrading the core package must only make
# the desktop AVAILABLE — never activated. The boot target, display
# manager, and installed package set must stay untouched by a mere
# catalog update.
#
# Run from the repository root as the development user:
#   scripts/smoke-day5.sh
#
# Activation (veil desktop install) and the graphical session are
# verified manually in a separate session, as on previous days.
set -u

cd "$(dirname "$0")/.."

PASS=0
FAIL=0

ok()   { PASS=$((PASS+1)); printf 'PASS: %s\n' "$1"; }
bad()  { FAIL=$((FAIL+1)); printf 'FAIL: %s\n' "$1"; }

check() { # check <description> <condition>
    if [ "$2" = "0" ] || [ -n "$2" ] && eval "$2"; then ok "$1"; else bad "$1"; fi
}

# --- 1. desktop overview ---------------------------------------------
check "veil desktop overview runs" "veil desktop >/dev/null 2>&1"
check "overview mentions niri-desktop" "veil desktop | grep -q niri-desktop"
check "overview reports no graphical session from TTY" \
    "veil desktop | grep -q 'no graphical Veilbox desktop session detected'"

# --- 2. desktop list -------------------------------------------------
check "desktop list runs" "veil desktop list >/dev/null 2>&1"
check "list shows niri-desktop" "veil desktop list | grep -q niri-desktop"
check "list shows display name" "veil desktop list | grep -q 'Niri Experience'"
check "list shows available status" "veil desktop list | grep -q available"
check "list shows compositor" "veil desktop list | grep -q niri"

# --- 3. desktop info -------------------------------------------------
INFO=$(veil desktop info niri-desktop 2>&1)
check "info runs" "echo \"\$INFO\" | grep -q 'Desktop: niri-desktop'"
check "info shows status available" "echo \"\$INFO\" | grep -q 'Status: available'"
check "info shows package name" "echo \"\$INFO\" | grep -q veilbox-experience-niri"
check "info shows compositor niri" "echo \"\$INFO\" | grep -q 'compositor       niri'"
check "info shows shell noctalia" "echo \"\$INFO\" | grep -q 'shell            noctalia'"
check "info shows terminal kitty" "echo \"\$INFO\" | grep -q 'terminal         kitty'"
check "info shows display manager sddm" "echo \"\$INFO\" | grep -q 'display_manager  sddm'"
check "info shows shell-provided features" \
    "echo \"\$INFO\" | grep -q 'builtin (provided by shell)'"

# --- 4. unknown desktop ----------------------------------------------
check "info for unknown desktop fails" "! veil desktop info nope >/dev/null 2>&1"

# --- 5. catalog separation: core update = catalog, not activation ----
check "desktop experience NOT installed by core upgrade" \
    "! rpm -q veilbox-experience-niri >/dev/null 2>&1"
check "sddm NOT enabled" "! systemctl is-enabled sddm >/dev/null 2>&1"
check "boot target still multi-user" \
    "[ \"\$(systemctl get-default)\" = multi-user.target ]"

# --- 6. status -------------------------------------------------------
check "status reports no desktop installed" \
    "veil status | grep -q 'Desktop:        (none installed)'"

# --- 7. doctor desktop checks ----------------------------------------
DOCTOR=$(veil doctor 2>&1)
check "doctor: manifest valid check present" \
    "echo \"\$DOCTOR\" | grep -q 'Desktop manifest valid'"
check "doctor: package check present" \
    "echo \"\$DOCTOR\" | grep -q 'Desktop packages installed'"
check "doctor: session file check present" \
    "echo \"\$DOCTOR\" | grep -q 'Desktop session file present'"
check "doctor: templates check present" \
    "echo \"\$DOCTOR\" | grep -q 'Desktop templates readable'"
check "doctor: graphical check SKIP from TTY (not a failure)" \
    "echo \"\$DOCTOR\" | grep -q 'skipped (no graphical session detected'"

echo
echo "smoke-day5: $PASS passed, $FAIL failed"
if [ "$FAIL" -gt 0 ]; then exit 1; fi
echo "NOTE: desktop activation and the graphical session are verified manually next session."
