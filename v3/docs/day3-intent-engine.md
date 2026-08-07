# Day 3: Intent Engine — Profiles that recommend, sync that installs

Date: 2026-08-07

## Goal

Turn profiles from passive state into an intent engine: profiles
describe who the engineer is and what they likely need; `veil` can
show, diff, and sync a machine toward that baseline — while never
taking away the engineer's own choices (see ADR-0004).

## Profile schema

`v3/profiles/<name>.yaml`:

```yaml
name: sre                      # required, ^[a-z0-9-]+$, matches filename
display_name: Site Reliability Engineer   # optional; defaults to name
description: >-                # required
  Keeps systems reliable: monitoring, incident response, ...
role: sre                      # optional; defaults to name
recommended_experiences:       # the baseline sync installs
  - base-ops
  - observability-cli
  - networking-tools
optional_experiences:          # never installed by sync
  - terminal-ops
tags: [reliability, monitoring, incidents]
workspace_preferences:         # informational; workspace engine consumes later
  shell: bash
  editor: vim
```

The Day 2 `capabilities` map is gone — recommended/optional experiences
are the single concept of intent. `Load` validates structure; cross-
catalog reference checks live in `CheckReferences` (used by doctor and
available to apply).

## Profiles and experiences (shipped in veilbox-core 0.1.0-2)

| Profile | Recommended | Optional |
|---|---|---|
| devops | base-ops, networking-tools, terminal-ops | observability-cli |
| sre | base-ops, observability-cli, networking-tools | terminal-ops |
| platform-engineer | base-ops, terminal-ops | networking-tools |
| cloud-engineer | base-ops, networking-tools | terminal-ops |

| Experience | Packages (meta-RPM) | Recommended by |
|---|---|---|
| base-ops | git, vim-enhanced, curl, strace | all 4 profiles |
| networking-tools | bind-utils, traceroute, nmap-ncat, iproute, tcpdump | devops, sre, cloud-engineer |
| terminal-ops | tmux, ripgrep, htop | devops, platform-engineer, cloud-engineer |
| observability-cli | sysstat, iotop, jq | sre |

`observability-cli` is intentionally unreferenced by
platform-engineer/cloud-engineer so "extra experiences" are
demonstrable.

## Engine ownership

```
Profile Engine (profile)         Experience Engine (experience)     DNF layer (dnfops)
  manifests, state                 catalog, status, install/remove     rpm -q, sudo dnf
  Diff / SyncPlan / CheckRefs  -->  List/Install  ------------------>  Transaction
  (no dnfops import)
```

- `profile.Diff(reg, cat, name)` → `Plan`: missing/not-installable/
  unknown/satisfied recommended, optional installed/missing, extras.
  Pure computation over catalog statuses; deterministic (sorted).
- `profile.SyncPlan(p)` → exactly the missing installable recommended
  experiences. Planned (no rpm) and unknown references are reported,
  never installed.
- `profile.CheckReferences(reg, cat, name)` → unknown references for
  doctor.
- Sync execution lives in the CLI: plan → confirmation (unless
  `--yes`) → `catalog.Install` per experience → DNF. The Profile
  Engine never touches DNF.

## Commands

- `veil profile list` — profiles, `(active)` marker
- `veil profile show <name>` — role, description, recommended/optional
  with live statuses, workspace preferences
- `veil profile apply <name>` — validate + persist + recommendation
  summary; installs nothing
- `veil profile diff <name>` — Missing / Not yet installable / Unknown
  / Already satisfied / Optional / Extra (kept)
- `veil profile sync [--yes]` — active profile only; installs missing
  recommended; never removes
- `veil experience info <name>` — status, packages, recommending
  profiles (reverse lookup via profile registry)
- `veil status` — adds `Profile sync: synced | missing N ...`
- `veil doctor` — adds profile state/manifest/reference checks and
  repository reachability (critical) + Veilbox repo present (warn)

## Removal semantics (verified live)

Veilbox adds no custom removal logic; DNF is the transaction
authority. `smoke-day3.sh` proves it with RPM database snapshots per
experience:

- `PRE` = snapshot with the preexisting package user-installed
- install experience → `POST_INSTALL`; remove → `FINAL`
- assert: nothing in `PRE` is missing from `FINAL` (preexisting
  survives), packages removed by cleanup ⊆ packages introduced by the
  experience, and `FINAL == PRE` exactly.

For `base-ops` this is the critical case: `git`/`curl` are commonly
already installed; removing the meta-RPM must keep them. DNF only
removes packages whose RPM reason is "dependency" and that nothing
else needs.

Note: `dnf install <pkg>` on an already-installed dependency does not
change its reason; the smoke script reinstalls the preexisting package
from scratch so it is genuinely user-installed.

## Packaging

- `veilbox-core` Release 1 → **2** (new manifests, binary, commands).
  Version stays 0.1.0 per Day 3 decision.
- New noarch meta-packages at 0.1.0-1:
  `veilbox-experience-{base-ops,terminal-ops,observability-cli}`.
- `build-rpms.sh` now loops over all `packages/SPECS/*.spec`.
- `compose-repo.sh` cleans stale RPMs before composing.
- rpmlint split per package type: `rpmlintrc-core`,
  `rpmlintrc-experience` (shared-file unused-filter errors).

## Verification

- `go test ./...` — 5 packages, incl. new plan tests (diff sections,
  planned/unknown handling, extras, determinism, references) and CLI
  tests (list/show/diff/sync/status/doctor, transaction recording).
- rpmlint: 0 errors / 0 warnings on all 5 RPMs.
- mock `--buildsrpm` + `--rebuild` (fedora-44-x86_64): core 0.1.0-2
  and the 3 new experiences; core test suite runs inside the chroot.
- `scripts/smoke-day3.sh`: **62/62** on clean state — acceptance
  steps 1–17, removal semantics, extras preservation, sre switch,
  doctor.

## Current limitations

- `workspace_preferences` is carried but not consumed yet (workspace
  engine, Day 4+).
- Sync with a "y" prompt reads stdin; no `--assume-no` (Abort on
  non-y already).
- Reboot persistence check pending (next session, like Day 2).

## References

- ADR-0001, ADR-0002, ADR-0003, ADR-0004
- v3/core/internal/profile/{profile,plan}.go
- v3/core/cmd/veil/main.go
