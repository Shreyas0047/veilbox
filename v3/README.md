# Veilbox v3

<div align="center">

> ⚠️ **STILL UNDER DEVELOPMENT** ⚠️
>
> This is an in-progress rebuild. **No v3 ISO is available yet.**
> Nothing here is installable or bootable at this stage.

</div>

Veilbox v3 is a **ground-up, Fedora-based rebuild** of Veilbox Linux.

## What this means

- **Base system:** Fedora (replacing the Debian base of v2)
- **Approach:** rebuilt from scratch — the existing v2 implementation is not carried forward
- **Current state:** early development; nothing is installable or bootable yet

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
- The disposable development VM has passwordless sudo enabled **for
  automation only** (see ADR-0002). It is deliberately not part of any
  Veilbox artifact, kickstart, or default configuration.
