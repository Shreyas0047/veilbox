# Day 7 — Environment Engine, Composition Record, Provisioning Contract

Phase B acceptance of the environment abstraction (ADR-0012), the
applied-product composition record (ADR-0010), and the environment
provisioning contract (ADR-0015).

## What changed

- **Environment engine** (ADR-0012): `internal/desktop/*` →
  `internal/environment/*`; `veil desktop` → `veil environment`
  (the legacy spelling is accepted as an alias for one release and
  produces byte-identical output, `TestDesktopAliasMatchesEnvironment`).
  The engine consumes the manifest contract only; there is no
  Niri/Noctalia-specific code path left in production Go (enforced by
  `scripts/smoke-day7.sh`).
- **Composition record** (ADR-0010): `~/.config/veilbox/composition.json`
  — the applied product record. Written only by the apply path
  (`veil onboard --yes` / wizard `Apply`), atomically
  (write-temp-rename, `0600`, no `.tmp` residue), recreated — never
  edited — on every apply, and consumed by `veil status` and
  `veil doctor`. `LoadComposition` treats a missing file as an empty
  record and a corrupt file as an error. `onboarding.json` remains the
  volatile selection draft.
- **Status/doctor**: status reports `Composition: applied <ts>`,
  a composition-driven `Environment:` line with explicit drift variants
  (package not installed / unknown to catalog), and
  `Composition drift:` for recorded experiences no longer installed.
  Doctor verifies "Composition record parses" and "Composition
  consistent with live state" (profile, experience, and environment
  drift, joined).
- **Reference environment re-expression** (ADR-0015): the Niri +
  Noctalia stack is a catalog data contract
  (`experiences/niri-desktop.yaml`: `environment: config (2) /
  managed (1) / validate (1 file + 1 command)`); templates moved to
  `environment/niri/`; the experience spec now installs under
  `/usr/share/veilbox/environment/niri/` (core Release 9, niri
  Release 3).

## Findings

- Experience drift compares **experience names**, never RPM names:
  the recorded experience is checked against the catalog's installed
  experience entries, and only `StatusInstalled` entries count. The
  first draft compared against RPM names (`veilbox-experience-X`),
  which produced false drift in both directions.
- The composition's validation block is informational for contract
  declarations and authoritative for selection problems
  (`problem:`-prefixed notes invalidate the record).
- `scripts/smoke-day5.sh` is now lifecycle-state-aware: on a fresh
  machine it asserts the pre-activation guarantees (catalog only, no
  display-manager or boot-target changes); on an activated machine it
  asserts the corresponding installed/enabled/graphical state. Both
  lifecycle states produce a green 29/29 run; smoke-day7 (33/33) and
  smoke-capabilities (16/16) are unchanged in scope.

## Infrastructure note: mid-session package disappearance

During Phase B verification on 2026-08-08, the reference environment
stack — `veilbox-experience-niri` and its runtime dependencies (niri,
noctalia, sddm, kitty, and the rest of the desktop stack,
269 packages) — disappeared from the VM between verification runs,
alongside the other experience meta-packages.

Initial diagnosis suggested an anomaly with no corresponding DNF
transaction. **That was wrong**: the disappearance was caused by real
DNF transactions — `dnf history` records 102-107
(2026-08-08T16:02:56Z-16:03:19Z, `dnf remove -y veilbox-experience-*`,
culminating in `dnf remove -y veilbox-experience-niri` altering
269 packages). The 269-package cascade is dnf's
`clean_requirements_on_remove` removing the desktop stack that was
installed as dependencies of the experience meta-package. The earlier
"no transaction" conclusion was an artifact of reading only the most
recent history records, where the truncated command display
(`...veilbox-experience-nir`) does not match a "niri" search.

The transactions were executed outside the Phase B verification
command history (an external or concurrent session on the dev VM); no
Phase B verification step issues these removals, and the environment
engine's remove path was not invoked by any Phase B check. **It is not
attributed to Phase B code.** The machine was restored to the
activated baseline (core 0.1.0-9, niri experience 0.1.0-3, all eight
experiences, sddm enabled, graphical target) and re-verified green.

## Live state after acceptance

`veilbox-core-0.1.0-9.fc44.x86_64` (reinstalled from the clean mock
build; the installed binary is byte-identical to the mock artifact),
`veilbox-experience-niri-0.1.0-3`; experience meta-packages installed:
base-ops, kubernetes-tools, networking-tools, niri, observability-cli,
security-tools (containers-tools and terminal-ops not installed — the
end state of the capability smoke's demo phases; `veil profile sync`
reports 1 missing recommended experience, which is the designed
"sync only adds" behavior). Profile `sre` active; sddm enabled;
`graphical.target`; a composition record is present (last written by
`veil onboard --yes` during the capability smoke, profile `sre`, no
environment section — so `veil status` shows
`Environment: niri-desktop (no composition record)`); `veil doctor`
fully green.

A composition record that carries the environment section is written
by the interactive wizard with an environment selected (the line UI on
a pipe: role, environment 1=niri-desktop, capability toggles kept,
workspace kept, review yes, confirm activation). With such a record,
status shows `Environment: niri-desktop (veilbox-experience-niri)` and
smoke-day7 counts 33 checks; with a no-environment record it counts
32 — both forms are fully green.

Smoke results: day5 29/29 (state-aware, both lifecycle states),
day7 green in both record forms (33/33 with the environment section,
32/32 without), smoke-capabilities 16/16.

## References

- v3/core/internal/onboarding/composition.go, composition_test.go
- v3/core/cmd/veil/main.go (status/doctor composition consumption),
  main_test.go (TestStatusCompositionDrivenEnvironment,
  TestDoctorCompositionChecks)
- v3/core/internal/environment/environment.go (SessionDir)
- v3/experiences/niri-desktop.yaml, v3/environment/niri/*
- v3/packages/SPECS/veilbox-core.spec, veilbox-experience-niri.spec
- v3/scripts/smoke-day5.sh, smoke-day7.sh, build-sources.sh
- v3/docs/adr/0010-composition-model.md, 0012-environment-abstraction.md,
  0015-environment-provisioning-contract.md
