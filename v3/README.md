# Veilbox v3

> **Status: Under active development — no ISO available yet.**

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
├── docs/          ← design docs, architecture decisions
├── kickstart/     ← Fedora kickstart files
├── configs/       ← system & application configs
├── packages/      ← custom package specs / overlays
├── scripts/       ← build & tooling scripts
├── branding/      ← logos, themes, assets
└── tests/         ← test suites and smoke checks
```

## Development

- All v3 work happens on the `v3` branch.
- The `main` branch continues to host v2.
- See `docs/` for design notes as they are written.
