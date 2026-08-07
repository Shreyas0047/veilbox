# Veilbox v3

<div align="center">

> ⚠️ **STILL UNDER DEVELOPMENT** ⚠️
>
> This is an in-progress rebuild. **No v3 ISO is available yet.**
> The `veil` CLI and its RPM packaging are functional prototypes.

</div>

Veilbox v3 is a **ground-up, Fedora-based rebuild** of Veilbox Linux.

## What this means

- **Base system:** Fedora (replacing the Debian base of v2)
- **Approach:** rebuilt from scratch — the existing v2 implementation is not carried forward
- **Current state:** prototype; `veil` (Core) ships as an RPM with profile intent
  manifests, experience catalogs, and a Workspace Engine; profiles recommend,
  `veil profile sync` installs experiences as DNF meta-packages, and
  `veil workspace apply` translates profile preferences into Veilbox-owned
  user configuration (see `docs/day4-workspace.md`)

## v2 preservation

The complete Veilbox v2 implementation remains preserved at the **repository root**
for now. Do not remove, move, or modify v2 files from the root while v3 development
is ongoing. When v3 is stable it may replace the root contents, with v2 preserved
through tags, releases, and Git history.

## Directory layout

```
v3/
├── README.md      ← you are here
├── docs/
│   └── adr/       ← architecture decision records
├── core/          ← Veilbox Core (Go)
│   ├── cmd/veil/  ← the veil CLI
│   └── internal/  ← profile, experience, settings, workspace, dnfops engines
├── profiles/      ← profile definitions (intent — configuration, never RPMs)
├── experiences/   ← experience definitions (capability — shipped as RPMs)
├── packages/      ← RPM specs, sources, built artifacts
├── configs/       ← user-level config overlays delivered by experiences
├── kickstart/     ← Fedora kickstart files (stretch goal)
├── scripts/       ← build & tooling scripts
├── branding/      ← logos, themes, assets
└── tests/         ← test suites and smoke checks
```

## Development

- All v3 work happens on the `v3` branch.
- The `main` branch continues to host v2.
- See `docs/adr/` for the architecture decision records (start with
  ADR-0001 and ADR-0002).
- See `docs/day4-workspace.md` for the current prototype: what is
  implemented, spec gotchas, and the test procedure.
- The disposable development VM has passwordless sudo enabled **for
  automation only** (see ADR-0002). It is deliberately not part of any
  Veilbox artifact, kickstart, or default configuration.

## Quick start (development VM)

```sh
scripts/build-sources.sh   # stage tarball (go.mod/go.sum/vendor)
scripts/build-rpms.sh      # rpmbuild -ba all specs in packages/SPECS
scripts/compose-repo.sh    # createrepo_c -> /srv/veilbox-repo (file:// repo)
sudo dnf install -y veilbox-core
veil profile apply devops
veil profile sync --yes    # installs missing recommended experiences
veil workspace apply --yes # translates profile preferences into user config
veil status                # core, profile+sync state, experiences, repos
scripts/smoke-day4.sh      # 43 checks, run on clean state
```

## Command reference

```
veil profile list                    profiles + active marker
veil profile show <name>             role, description, recommended/optional with status
veil profile apply <name>            intent only: validate + persist + summary
veil profile diff [<name>]           missing / planned / unknown / optional / extras
veil profile sync [--yes]            install missing recommended; never removes
veil experience list                 catalog with statuses
veil experience info <name>          packages + recommending profiles
veil experience install <name>       DNF install meta-package by name
veil experience remove <name>        DNF remove meta-package by name
veil workspace                       workspace overview
veil workspace plan                  what apply would do (no changes)
veil workspace apply [--yes] [--force]  apply profile workspace prefs
veil workspace status                applied state, drift, conflicts
veil workspace reset [--yes]         remove only Veilbox-managed config
veil status                          core, profile, experiences, repos
veil doctor                          full health check
```

Profiles are intent, not enforcement: sync only ever adds
(see `docs/adr/0004-profile-baseline-not-prison.md`). The Workspace
Engine applies the same philosophy to files: Veilbox owns what it
generates under `~/.config/veilbox/workspace/`, integrates through a
single marked include block in `~/.bashrc`/`~/.tmux.conf`, backs up
before first touch, detects drift, and never overwrites whole
user-owned files (see `docs/adr/0005-workspace-ownership.md`).
