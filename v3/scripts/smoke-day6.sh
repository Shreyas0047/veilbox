#!/usr/bin/env bash
# smoke-day6.sh — Onboarding TUI live acceptance (Day 6)
#
# Verifies the interactive onboarding surface against the live
# installation:
#   - TTY runs use the Bubble Tea TUI (step headers, keyboard flow)
#   - piped runs fall back to the line UI
#   - --yes stays fully non-interactive
#   - saved selections preload in the TUI on rerun
#   - aborting at the review leaves the machine untouched
#
# TUI sessions are driven through a real PTY (python3 pty.fork) with
# marker-paced keys, mirroring a human at the keyboard.
#
# Run from the repository root as the development user:
#   scripts/smoke-day6.sh
set -u

cd "$(dirname "$0")/.."

PASS=0
FAIL=0

ok()   { PASS=$((PASS+1)); printf 'PASS: %s\n' "$1"; }
bad()  { FAIL=$((FAIL+1)); printf 'FAIL: %s\n' "$1"; }

check() { # check <description> <condition>
    if [ "$2" = "0" ] || [ -n "$2" ] && eval "$2"; then ok "$1"; else bad "$1"; fi
}

# --- snapshot helpers -------------------------------------------------
rpm_count()    { rpm -qa | wc -l; }
boot_target()  { systemctl get-default; }
snapshot()     { echo "$(rpm_count)|$(boot_target)"; }
assert_unchanged() { # <before> <description>
    local now; now="$(snapshot)"
    if [ "$now" = "$1" ]; then ok "$2"; else bad "$2 (before: $1, after: $now)"; fi
}

# --- paced PTY driver -------------------------------------------------
# tui_session <logfile> <plan>: runs 'veil onboard' on a real PTY
# (python3 pty.fork) and drives it the way a human would: each key
# chunk is sent only after the marker for the step it targets has
# appeared in the program's output (the same wait-for-marker pattern
# the Go e2e tests use, so keys can never land in the wrong program or
# be dropped by a step-transition reader). The driver also answers the
# terminal probes (OSC 11 background / DSR cursor) like a minimal
# terminal emulator so they never eat user keystrokes.
tui_session() {
    local log="$1" plan="$2" tmp rc
    tmp=$(mktemp)
    cat > "$tmp" <<'PYEOF'
import fcntl, os, pty, select, signal, struct, sys, termios, time

log = open(sys.argv[1], 'wb')

pid, fd = pty.fork()
if pid == 0:
    os.execvp('veil', ['veil', 'onboard'])

# Give the pty a real window size so the screens (and the review
# plan) render at full height instead of a 3-line slit.
fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack('HHHH', 40, 100, 0, 0))

def _timeout(_sig, _frame):
    raise SystemExit(124)
signal.signal(signal.SIGALRM, _timeout)
signal.alarm(60)

OSC_Q = b'\x1b]11;?\x1b\\'
OSC_A = b'\x1b]11;rgb:0000/0000/0000\x1b\\'
DSR_Q = b'\x1b[6n'
DSR_A = b'\x1b[1;1R'
osc_sent = dsn_sent = 0

STEPS = {
    'preload': [
        (b'Press Enter to begin', b'\r'),       # welcome
        (b'Step 1/5', b'q'),                    # role: abort
    ],
    'review': [
        (b'Press Enter to begin', b'\r'),       # welcome
        (b'Step 1/5', b'\r'),                   # role
        (b'Step 2/5', b'\r'),                   # desktop
        (b'Step 3/5', b'jjjj\r'),               # capabilities: Done
        (b'Step 4/5', b'\r\rj\r\rj\rj\rj\r'),   # workspace: Continue
        (b'Step 5/5', b'q'),                    # review: abort
    ],
}
plan = STEPS[sys.argv[2]]
step = 0
buf = bytearray()
pending = None   # (when, keys): keys to send once 'when' has passed

def settle(marker, keys):
    global pending
    pending = (time.time() + 0.6, keys)

while True:
    try:
        r, _, _ = select.select([fd], [], [], 0.1)
    except OSError:
        break
    if fd in r:
        try:
            d = os.read(fd, 4096)
        except OSError:
            break
        if not d:
            break
        log.write(d)
        log.flush()
        n = d.count(OSC_Q)
        if n > osc_sent:
            os.write(fd, OSC_A)
            osc_sent = n
        n = d.count(DSR_Q)
        if n > dsn_sent:
            os.write(fd, DSR_A)
            dsn_sent = n
        buf += d
        while step < len(plan) and plan[step][0] in buf:
            settle(*plan[step])
            step += 1
    if pending is not None and time.time() >= pending[0]:
        os.write(fd, pending[1])
        pending = None
    p, st = os.waitpid(pid, os.WNOHANG)
    if p:
        sys.exit(os.waitstatus_to_exitcode(st))
    time.sleep(0.05)
PYEOF
    TERM=xterm-256color python3 "$tmp" "$log" "$plan"
    rc=$?
    rm -f "$tmp"
    return $rc
}

# --- line UI helper (piped run falls back to the line UI) -------------
line_abort() {
    # Eleven empty lines keep every saved value (role, desktop, four
    # capability groups, four workspace fields), then q aborts at the
    # review prompt.
    printf '\n\n\n\n\n\n\n\n\n\n\nq\n' | veil onboard 2>&1
}

# --- 1. --yes stays non-interactive -----------------------------------
YES_OUT=$(veil onboard --yes 2>&1)
check "--yes seeds defaults on first run" \
    "echo \"\$YES_OUT\" | grep -qE 'First run|Applying your saved selection'"
check "--yes renders the apply report" \
    "echo \"\$YES_OUT\" | grep -q 'APPLY RESULT'"
check "--yes reports success" "echo \"\$YES_OUT\" | grep -qE '\[OK\]|Success'"
check "--yes persists the selection ledger" \
    "grep -q cloud-engineer \"\$HOME/.config/veilbox/onboarding.json\""

# --- 2. TTY dispatches to the Bubble Tea TUI --------------------------
LOG=/tmp/smoke-day6-tui-preload.log
rm -f "$LOG"
tui_session "$LOG" preload
check "TTY run renders the TUI chrome" \
    "grep -q 'Step 1/5 — Role' \"$LOG\""
check "TTY run renders the TUI footer" \
    "grep -q 'nothing has been changed' \"$LOG\""
check "saved role preloads (cursor on Cloud Engineer)" \
    "grep -m1 '›' \"$LOG\" | grep -q 'Cloud Engineer'"

# --- 3. full TUI walk to the review, then abort (zero change) ---------
BEFORE=$(snapshot)
LOG=/tmp/smoke-day6-tui-review.log
rm -f "$LOG"
tui_session "$LOG" review
ABORT_CODE=$?
check "TUI abort exits cleanly" "[ \"$ABORT_CODE\" = \"0\" ]"
check "TUI reached the review step" \
    "grep -q 'Step 5/5 — Review' \"$LOG\""
check "review shows the full plan" \
    "grep -q 'PROFILE' \"$LOG\" && grep -q 'EXPERIENCES' \"$LOG\""
check "abort reports no changes" \
    "grep -q 'Aborted. No changes were made.' \"$LOG\""
assert_unchanged "$BEFORE" "TUI abort changed nothing on the machine"

# --- 4. piped run falls back to the line UI (zero change) -------------
BEFORE=$(snapshot)
LINE_OUT=$(line_abort)
check "pipe run uses the line UI (no step chrome)" \
    "echo \"\$LINE_OUT\" | grep -q 'ROLE' && ! echo \"\$LINE_OUT\" | grep -q 'Step 1/5'"
check "line UI shows numbered choices" \
    "echo \"\$LINE_OUT\" | grep -qE '^\\* 0\\.|^  0\\.'"
check "line UI reaches the review prompt" \
    "echo \"\$LINE_OUT\" | grep -q 'Apply this plan'"
check "line UI abort reports no changes" \
    "echo \"\$LINE_OUT\" | grep -q 'Aborted. No changes were made.'"
assert_unchanged "$BEFORE" "line UI abort changed nothing on the machine"

echo
echo "smoke-day6: $PASS passed, $FAIL failed"
if [ "$FAIL" -gt 0 ]; then exit 1; fi
