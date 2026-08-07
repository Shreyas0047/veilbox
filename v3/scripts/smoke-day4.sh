#!/usr/bin/env bash
# smoke-day4.sh — Workspace Engine live acceptance (Day 4)
#
# Exercises the full workspace lifecycle against the real user
# environment, verifying the strongest acceptance requirement:
# a user with an existing customized shell configuration can apply
# Veilbox, switch profiles, reset Veilbox, and still retain everything
# they owned before Veilbox touched the workspace.
#
# Run from the repository root as the development user:
#   scripts/smoke-day4.sh
#
# Reboot persistence (steps 21-23) is verified manually in a separate
# session, as on previous days.
set -u

cd "$(dirname "$0")/.."

PASS=0
FAIL=0
BASHRC="$HOME/.bashrc"
WS_DIR="$HOME/.config/veilbox/workspace"
BACKUP_ROOT="$HOME/.config/veilbox/backups"

ok()   { PASS=$((PASS+1)); printf 'PASS: %s\n' "$1"; }
bad()  { FAIL=$((FAIL+1)); printf 'FAIL: %s\n' "$1"; }

check() { # check <description> <condition>
    if [ "$2" = "0" ] || [ -n "$2" ] && eval "$2"; then ok "$1"; else bad "$1"; fi
}

# --- snapshot the user's existing shell configuration ---------------
ORIG_BASHRC=$(mktemp)
cp "$BASHRC" "$ORIG_BASHRC" 2>/dev/null || ORIG_BASHRC=/dev/null
ORIG_HASH=$(sha256sum "$ORIG_BASHRC" | cut -d' ' -f1)

# --- pre-flight: drop leftovers from any interrupted run -------------
# Removes ONLY Veilbox-managed content; the user's own configuration is
# never touched. Runs before the snapshot so the snapshot is pristine.
veil workspace reset --yes >/dev/null 2>&1

# --- snapshot the user's existing shell configuration ---------------
ORIG_BASHRC=$(mktemp)
cp "$BASHRC" "$ORIG_BASHRC" 2>/dev/null || ORIG_BASHRC=/dev/null
ORIG_HASH=$(sha256sum "$ORIG_BASHRC" | cut -d' ' -f1)

echo "== controlled state: profile=$(veil profile 2>/dev/null | grep Profile || echo none), workspace fresh =="

# 1. veil workspace (overview) ---------------------------------------
check "workspace overview runs" "veil workspace >/dev/null 2>&1"
veil workspace >/dev/null 2>&1

# 2. veil workspace plan ---------------------------------------------
PLAN=$(veil workspace plan 2>&1)
check "workspace plan runs" "[ $? -eq 0 ]"
check "plan mentions CREATE" "echo \"\$PLAN\" | grep -q CREATE"

# 3. plan made ZERO filesystem changes -------------------------------
STATE_HASH_BEFORE=$(sha256sum "$WS_DIR/state.json" 2>/dev/null | cut -d' ' -f1)
HASH_BEFORE=$(sha256sum "$BASHRC" | cut -d' ' -f1)
veil workspace plan >/dev/null 2>&1
HASH_AFTER=$(sha256sum "$BASHRC" | cut -d' ' -f1)
STATE_HASH_AFTER=$(sha256sum "$WS_DIR/state.json" 2>/dev/null | cut -d' ' -f1)
check "plan made no changes to .bashrc" "[ \"$HASH_BEFORE\" = \"$HASH_AFTER\" ]"
check "plan did not modify workspace state" "[ \"$STATE_HASH_BEFORE\" = \"$STATE_HASH_AFTER\" ]"

# 4. veil workspace apply --yes --------------------------------------
check "apply --yes succeeds" "veil workspace apply --yes >/dev/null 2>&1"

# 5. verify generated files ------------------------------------------
check "generated shell.sh exists" "[ -f \"$WS_DIR/shell.sh\" ]"
check "shell.sh is bash-readable" "bash -n \"$WS_DIR/shell.sh\""
check "workspace state exists" "[ -f \"$WS_DIR/state.json\" ]"
check "state schema version 1" "grep -q '\"schema_version\": 1' \"$WS_DIR/state.json\""
check "state records applied profile" "grep -q applied_profile \"$WS_DIR/state.json\""

# 6. verify existing .bashrc content preserved -----------------------
check ".bashrc first line matches original" \
    "[ \"\$(head -1 \"$BASHRC\")\" = \"\$(head -1 \"$ORIG_BASHRC\")\" ]"
ORIG_LEN=$(wc -l < "$ORIG_BASHRC" 2>/dev/null || echo 0)
check "exactly one managed block in .bashrc" \
    "[ \"\$(grep -c '# >>> veilbox managed >>>' \"$BASHRC\")\" = 1 ] && [ \"\$(grep -c '# <<< veilbox managed <<<' \"$BASHRC\")\" = 1 ]"
check "managed block appears after original content" \
    "grep -n '# >>> veilbox managed >>>' \"$BASHRC\" | cut -d: -f1 | xargs -I{} test {} -gt $ORIG_LEN"

# 7. apply again -> idempotent ---------------------------------------
BASH_HASH_1=$(sha256sum "$BASHRC" | cut -d' ' -f1)
APPLY2=$(veil workspace apply --yes 2>&1)
check "second apply reports up to date" "echo \"\$APPLY2\" | grep -q 'already up to date'"
check "second apply changed nothing" \
    "[ \"\$(sha256sum \"$BASHRC\" | cut -d' ' -f1)\" = \"$BASH_HASH_1\" ]"

# 8. status -> clean --------------------------------------------------
STATUS=$(veil workspace status 2>&1)
check "status reports clean" "echo \"\$STATUS\" | grep -q clean"
check "status lists managed files" "echo \"\$STATUS\" | grep -q shell.sh"

# 9-10. manual drift detection ---------------------------------------
echo "export DRIFT_MARKER=1" >> "$WS_DIR/shell.sh"
STATUS=$(veil workspace status 2>&1)
check "status detects drift" "echo \"\$STATUS\" | grep -q drifted"

# 11. Veilbox must NOT silently overwrite ----------------------------
APPLY_DRIFT=$(veil workspace apply --yes 2>&1)
check "apply refuses drifted content" "echo \"\$APPLY_DRIFT\" | grep -qi conflict"
check "drifted content untouched" "grep -q DRIFT_MARKER \"$WS_DIR/shell.sh\""

# 12. restore/reconcile safely with --force --------------------------
APPLY_FORCE=$(veil workspace apply --yes --force 2>&1)
check "apply --force restores" "[ $? -eq 0 ]"
check "drift marker gone after --force" "! grep -q DRIFT_MARKER \"$WS_DIR/shell.sh\""
STATUS=$(veil workspace status 2>&1)
check "status clean after --force" "echo \"\$STATUS\" | grep -q clean"

# 13. switch engineer profile (to devops) ----------------------------
check "profile apply devops" "veil profile apply devops >/dev/null 2>&1"

# 14. workspace plan reflects new profile ----------------------------
PLAN=$(veil workspace plan 2>&1)
check "plan after switch mentions changes" "echo \"\$PLAN\" | grep -qiE 'create|update'"

# 15. apply new workspace --------------------------------------------
check "apply after switch" "veil workspace apply --yes >/dev/null 2>&1"
STATUS=$(veil workspace status 2>&1)
check "status shows applied for devops" "echo \"\$STATUS\" | grep -q 'Applied for:     devops'"

# 16. stale generated configuration handled --------------------------
check "tmux.conf generated for devops" "[ -f \"$WS_DIR/tmux.conf\" ]"
# switch back to sre: tmux no longer wanted
check "profile apply sre" "veil profile apply sre >/dev/null 2>&1"
check "apply sre workspace" "veil workspace apply --yes >/dev/null 2>&1"
check "stale tmux.conf removed" "[ ! -f \"$WS_DIR/tmux.conf\" ]"
check "stale .tmux.conf (Veilbox-created) removed" "[ ! -f \"$HOME/.tmux.conf\" ]"
check "no veilbox marker left in .tmux.conf" "! grep -q 'veilbox' \"$HOME/.tmux.conf\" 2>/dev/null"

# 17-19. reset --------------------------------------------------------
RESET=$(veil workspace reset --yes 2>&1)
check "reset succeeds" "echo \"\$RESET\" | grep -q 'Workspace reset'"
check "shell.sh removed" "[ ! -f \"$WS_DIR/shell.sh\" ]"
check "tmux.conf removed" "[ ! -f \"$WS_DIR/tmux.conf\" ]"
check "no managed block remains in .bashrc" "! grep -q 'veilbox managed' \"$BASHRC\""
check "original user config byte-identical" \
    "[ \"\$(sha256sum \"$BASHRC\" | cut -d' ' -f1)\" = \"$ORIG_HASH\" ]"
check "backups preserved" "[ -d \"$BACKUP_ROOT\" ] && [ -n \"\$(find \"$BACKUP_ROOT\" -name '*.bak' | head -1)\" ]"
check "workspace state cleared" \
    "! grep -q '\"applied_profile\"' \"$WS_DIR/state.json\" && grep -q '\"generation\"' \"$WS_DIR/state.json\""

# 20. reapply workspace ----------------------------------------------
check "reapply workspace" "veil workspace apply --yes >/dev/null 2>&1"
STATUS=$(veil workspace status 2>&1)
check "status clean after reapply" "echo \"\$STATUS\" | grep -q clean"

# 21-23 (reboot persistence + doctor) are verified manually next session.

echo
echo "smoke-day4: $PASS passed, $FAIL failed"
if [ "$FAIL" -gt 0 ]; then exit 1; fi
echo "NOTE: reboot persistence and post-reboot doctor are verified separately (next session)."
echo "NOTE: final state: profile=sre, workspace applied and clean."
