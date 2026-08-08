#!/usr/bin/env bash
# smoke-capabilities.sh — Capability-layer acceptance demo (Phase A)
#
# Drives the exact accepted demo through the live installation:
#   Profile SRE → recommended capabilities Networking, Observability,
#   Containers, Kubernetes → the engineer REMOVES Containers and ADDS
#   Security → the Experience Engine derives the required experiences →
#   DNF resolves and installs the RPMs.
#
# Asserts the capability semantics end to end:
#   - the review shows the capability selection (not raw packages)
#   - derived experiences install: base-ops, networking-tools,
#     observability-cli, kubernetes-tools, security-tools
#   - removed capability's experience (containers-tools) is NOT
#     installed
#   - the onboarding selection persists the capabilities axis
#
# Reset + demo run are explicit: this smoke changes the machine
# (installs/removes experience RPMs) by design. Run from the
# repository root as the development user:
#   scripts/smoke-capabilities.sh
set -euo pipefail

cd "$(dirname "$0")/.."

PASS=0
FAIL=0

ok()   { PASS=$((PASS+1)); printf 'PASS: %s\n' "$1"; }
bad()  { FAIL=$((FAIL+1)); printf 'FAIL: %s\n' "$1"; }

check() { # check <description> <condition>
    if eval "$2"; then ok "$1"; else bad "$1"; fi
}

# --- reset the machine to a fresh, capability-free state --------------
sudo dnf remove -y \
    veilbox-experience-base-ops \
    veilbox-experience-containers-tools \
    veilbox-experience-kubernetes-tools \
    veilbox-experience-networking-tools \
    veilbox-experience-observability-cli \
    veilbox-experience-security-tools \
    veilbox-experience-terminal-ops >/dev/null 2>&1 || true
rm -f "$HOME/.config/veilbox/onboarding.json" "$HOME/.config/veilbox/state.json"
rm -rf "$HOME/.config/veilbox/workspace"
# Strip the Veilbox managed include block from user shell files so the
# reset is truly pristine (the demo then re-applies it cleanly).
for f in "$HOME/.bashrc" "$HOME/.tmux.conf"; do
    if [ -f "$f" ]; then
        python3 - "$f" <<'PYEOF'
import sys
path = sys.argv[1]
lines = open(path).readlines()
out, inside = [], False
for line in lines:
    if line.strip() == "# >>> veilbox managed >>>":
        inside = True
        continue
    if inside and line.strip() == "# <<< veilbox managed <<<":
        inside = False
        continue
    if not inside:
        out.append(line)
open(path, "w").writelines(out)
PYEOF
    fi
done

check "reset: no onboarding selection remains" \
    "[ ! -f \"\$HOME/.config/veilbox/onboarding.json\" ]"
check "reset: no experience RPM remains" \
    "[ -z \"\$(rpm -qa | grep veilbox-experience | grep -v veilbox-experience-niri || true)\" ]"

# --- the demo ----------------------------------------------------------
# Line UI (piped) script:
#   role: 3 (sre — recommended: networking, observability, containers,
#        kubernetes)
#   desktop: 0 (none)
#   Containers: 0 (toggle OFF — the engineer removes it)
#   Kubernetes / Networking / Observability: keep
#   Security: 0 (toggle ON — the engineer adds it)
#   Terminal Operations: keep (empty)
#   workspace: all keep (empty)
#   review: y (apply)
DEMO_OUT=$(printf '3\n0\n0\n\n\n\n0\n\n\n\n\n\ny\n' | veil onboard 2>&1)

check "demo: review shows the capability selection" \
    "echo \"\$DEMO_OUT\" | grep -q 'CAPABILITIES'"
check "demo: review shows security added" \
    "echo \"\$DEMO_OUT\" | grep -q 'security'"
check "demo: apply ran" "echo \"\$DEMO_OUT\" | grep -q 'APPLY RESULT'"
check "demo: apply succeeded" \
    "echo \"\$DEMO_OUT\" | grep -qE '\[OK\]|Success: selection applied'"

# --- asserted end state -----------------------------------------------
INSTALLED=$(rpm -qa | grep veilbox-experience | grep -v niri | sort)

for exp in veilbox-experience-base-ops \
    veilbox-experience-networking-tools \
    veilbox-experience-observability-cli \
    veilbox-experience-kubernetes-tools \
    veilbox-experience-security-tools; do
    check "demo: $exp installed" "echo \"\$INSTALLED\" | grep -q \"$exp\""
done

check "demo: removed capability's experience NOT installed" \
    "! echo \"\$INSTALLED\" | grep -q 'veilbox-experience-containers-tools'"
check "demo: optional terminal-operations NOT installed" \
    "! echo \"\$INSTALLED\" | grep -q 'veilbox-experience-terminal-ops'"

# The onboarding selection persists the capability axis: the five
# selected capabilities, never the removed containers.
CAPS=$(python3 -c 'import json,sys
d=json.load(open(sys.argv[1])); print(" ".join(sorted(d.get("capabilities", []))))' \
    "$HOME/.config/veilbox/onboarding.json")
check "demo: selection saves the capabilities axis" \
    "[ \"\$CAPS\" = 'base-operations kubernetes networking observability security' ]"

check "demo: SRE profile persisted as active" \
    "grep -q sre \"\$HOME/.config/veilbox/state.json\""

check "demo: derived experiences saved in the selection" \
    "python3 -c 'import json,sys
d=json.load(open(sys.argv[1]))
assert set(d.get(\"experiences\", [])) == {\"base-ops\", \"kubernetes-tools\", \"networking-tools\", \"observability-cli\", \"security-tools\"}' \
    \"\$HOME/.config/veilbox/onboarding.json\""

echo
echo "smoke-capabilities: $PASS passed, $FAIL failed"
if [ "$FAIL" -gt 0 ]; then exit 1; fi
