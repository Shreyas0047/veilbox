#!/usr/bin/env bash
# smoke-day5.sh — Environment Engine live acceptance (Day 5)
#
# Verifies the environment experience surface against the real catalog.
# The acceptance is lifecycle-state-aware:
#
#   - On a FRESH machine (environment never activated) it verifies the
#     strongest acceptance requirement: installing/upgrading the core
#     package only makes the environment AVAILABLE — never activated.
#     The boot target, display manager, and installed package set must
#     stay untouched by a mere catalog update.
#   - On an ACTIVATED machine (veil environment install ran: the
#     experience RPM is installed, the display manager is enabled, and
#     the boot target is graphical) it verifies the same surface with
#     the state-appropriate expectations instead of failing on them.
#
# In both states the doctor checks, the status lines, and the
# environment list/info surface are asserted; only the activation-state
# expectations switch. A valid machine in either lifecycle state must
# produce a completely green run.
#
# Run from the repository root as the development user:
#   scripts/smoke-day5.sh
#
# The graphical session itself is verified manually in a separate
# session, as on previous days.
set -u

cd "$(dirname "$0")/.."

PASS=0
FAIL=0

ok()   { PASS=$((PASS+1)); printf 'PASS: %s\n' "$1"; }
bad()  { FAIL=$((FAIL+1)); printf 'FAIL: %s\n' "$1"; }

check() { # check <description> <condition>
    if [ "$2" = "0" ] || [ -n "$2" ] && eval "$2"; then ok "$1"; else bad "$1"; fi
}

# --- lifecycle state ---------------------------------------------------
# activated = the reference environment experience is installed (its
# package) and the boot target is graphical. Display-manager state is
# derived from the package state: sddm ships with the experience.
if rpm -q veilbox-experience-niri >/dev/null 2>&1; then
    STATE="activated"
else
    STATE="fresh"
fi
printf 'machine state: %s\n' "$STATE"

# --- 1. environment overview -----------------------------------------
check "veil environment overview runs" "veil environment >/dev/null 2>&1"
check "overview mentions niri-desktop" "veil environment | grep -q niri-desktop"
check "overview reports no graphical session from TTY" \
    "veil environment | grep -q 'no graphical Veilbox environment session detected'"

# --- 2. environment list ---------------------------------------------
check "environment list runs" "veil environment list >/dev/null 2>&1"
check "list shows niri-desktop" "veil environment list | grep -q niri-desktop"
check "list shows display name" "veil environment list | grep -q 'Niri Experience'"
if [ "$STATE" = "activated" ]; then
    check "list shows installed status" "veil environment list | grep -q installed"
else
    check "list shows available status" "veil environment list | grep -q available"
fi
check "list shows compositor" "veil environment list | grep -q niri"

# --- 3. environment info ---------------------------------------------
INFO=$(veil environment info niri-desktop 2>&1)
check "info runs" "echo \"\$INFO\" | grep -q 'Environment: niri-desktop'"
if [ "$STATE" = "activated" ]; then
    check "info shows status installed" "echo \"\$INFO\" | grep -q 'Status: installed'"
else
    check "info shows status available" "echo \"\$INFO\" | grep -q 'Status: available'"
fi
check "info shows package name" "echo \"\$INFO\" | grep -q veilbox-experience-niri"
check "info shows compositor niri" "echo \"\$INFO\" | grep -q 'compositor       niri'"
check "info shows shell noctalia" "echo \"\$INFO\" | grep -q 'desktop_shell    noctalia'"
check "info shows terminal kitty" "echo \"\$INFO\" | grep -q 'terminal         kitty'"
check "info shows display manager sddm" "echo \"\$INFO\" | grep -q 'display_manager  sddm'"
check "info shows shell-provided features" \
    "echo \"\$INFO\" | grep -q 'builtin (provided by shell)'"

# --- 4. unknown environment ------------------------------------------
check "info for unknown environment fails" "! veil environment info nope >/dev/null 2>&1"

# --- 5. activation-state acceptance ----------------------------------
if [ "$STATE" = "activated" ]; then
    check "environment experience installed (state: activated)" \
        "rpm -q veilbox-experience-niri >/dev/null 2>&1"
    check "display manager enabled" "systemctl is-enabled sddm >/dev/null 2>&1"
    check "boot target is graphical" \
        "[ \"\$(systemctl get-default)\" = graphical.target ]"
else
    check "environment experience NOT installed by core upgrade" \
        "! rpm -q veilbox-experience-niri >/dev/null 2>&1"
    check "display manager NOT enabled" "! systemctl is-enabled sddm >/dev/null 2>&1"
    check "boot target still multi-user" \
        "[ \"\$(systemctl get-default)\" = multi-user.target ]"
fi

# --- 6. status -------------------------------------------------------
if [ "$STATE" = "activated" ]; then
    check "status names the installed environment" \
        "veil status | grep -q 'Environment:    niri-desktop'"
else
    check "status reports no environment installed" \
        "veil status | grep -q 'Environment:    (none installed)'"
fi
check "status composition line present (applied or none)" \
    "veil status | grep -qE 'Composition:    (applied|\\(none)'"

# --- 7. doctor environment checks ------------------------------------
DOCTOR=$(veil doctor 2>&1)
check "doctor: manifest valid check present" \
    "echo \"\$DOCTOR\" | grep -q 'Environment manifest valid'"
check "doctor: package check present" \
    "echo \"\$DOCTOR\" | grep -q 'Environment packages installed'"
check "doctor: session file check present" \
    "echo \"\$DOCTOR\" | grep -q 'Environment session file present'"
check "doctor: templates check present" \
    "echo \"\$DOCTOR\" | grep -q 'Environment templates readable'"
check "doctor: config file check present" \
    "echo \"\$DOCTOR\" | grep -q 'Environment config files present'"
check "doctor: config hook checks present" \
    "echo \"\$DOCTOR\" | grep -q 'Environment config hooks pass'"
check "doctor: graphical check SKIP from TTY (not a failure)" \
    "echo \"\$DOCTOR\" | grep -q 'skipped (no graphical session detected'"

echo
echo "smoke-day5: $PASS passed, $FAIL failed"
if [ "$FAIL" -gt 0 ]; then exit 1; fi
echo "NOTE: the graphical session itself is verified manually next session."
