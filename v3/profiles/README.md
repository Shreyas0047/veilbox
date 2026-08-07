# Profiles

A **profile** declares *intent*: who the engineer is, the capabilities
they likely need (recommended experiences), the ones they might want
(optional experiences), and workspace preferences. Profiles are
configuration and state owned by Veilbox Core — **never RPMs** (see
`docs/adr/0001-architecture.md`).

Profile definitions live in this directory (shipped by `veilbox-core`
as packaged data). The active profile is persisted in
`~/.config/veilbox/state.json` by `veil profile apply`.

## Schema

```yaml
name: sre                       # required, matches the filename
display_name: Site Reliability Engineer
description: >-                 # required
  ...
role: sre                       # defaults to name
recommended_experiences:        # baseline: sync installs these when missing
  - base-ops
  - observability-cli
optional_experiences:           # shown, never installed by sync
  - terminal-ops
tags: [reliability, monitoring]
workspace_preferences:          # informational (workspace engine later)
  shell: bash
  editor: vim
```

## Current profiles

| Profile | Role | Recommended | Optional |
|---|---|---|---|
| devops | DevOps Engineer | base-ops, networking-tools, terminal-ops | observability-cli |
| sre | Site Reliability Engineer | base-ops, observability-cli, networking-tools | terminal-ops |
| platform-engineer | Platform Engineer | base-ops, terminal-ops | networking-tools |
| cloud-engineer | Cloud Engineer | base-ops, networking-tools | terminal-ops |

## Semantics

- `veil profile apply <name>` validates and persists intent. It never
  installs anything.
- `veil profile sync` installs missing **recommended** experiences
  only — never optional, never anything the engineer installed
  manually. A profile is a desired baseline, not an enforced prison
  (`docs/adr/0004-profile-baseline-not-prison.md`).
- `veil profile diff <name>` compares the machine against the profile;
  installed-but-unreferenced experiences are reported as extras and
  kept.

The environment stays editable after installation: applying a profile
is a mutation of intent, not a one-shot script.
