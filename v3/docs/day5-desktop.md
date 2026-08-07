# Day 5: Desktop — Niri + Noctalia, install is inert, activation is explicit

Date: 2026-08-07

## Goal

Ship Veilbox's first desktop: a complete Niri + Noctalia stack on
Fedora 44, delivered as a DNF meta-package whose **installation is
inert** (no boot-target change, no services enabled) and whose
**activation is explicit** (`veil desktop install niri-desktop`).
Prove the full lifecycle on the live VirtualBox machine, including a
reboot and the black-screen episode (ADR-0009).

## Architecture (summary; see ADR-0006/0007/0008)

- Desktop = an experience manifest (`type: desktop`): compositor
  `niri`, shell `noctalia`, terminal `kitty`, display manager `sddm`,
  screenshot `grim`/`slurp`, clipboard `wl-clipboard`, XWayland.
- RPM `veilbox-experience-niri` (noarch, zero scriptlets) ships
  templates under `/usr/share/veilbox/desktop/niri/`; installation
  never activates (ADR-0006).
- `veil desktop install` is the only activation path: idempotent
  DNF step, session file registration, first-touch user config
  render, display-manager enable, `graphical.target` set, rollback
  hint printed (ADR-0007).
- Ownership (ADR-0008): `config.kdl`/`config.toml` are user-owned
  after first touch (preserved on every later run); the Veilbox-owned
  file `~/.config/veilbox/desktop/niri-desktop/noctalia-veilbox.toml`
  is regenerated and included via TOML `[include]`.

## Engine behavior

- `catalogIsInstalled` makes install idempotent: an already-installed
  experience skips DNF ("experience niri-desktop already installed
  (veilbox-experience-niri) — package step skipped") and proceeds
  with activation.
- Template directory is derived from the manifest RPM
  (`veilbox-experience-niri` → `niri`); rendering context is
  `{DisplayName, Name, SystemDir}` (wallpaper path resolves to the
  RPM-installed file).
- `Engine.TemplateDir(m)` drives the doctor templates check.
- Remove = DNF removal of the meta-package + full preservation
  report + deactivation hint; never touches DM/target (ADR-0006).

## Test procedure

### Unit + packaging

- 8 Go packages: full test suite green (`go test ./...`), `go vet`,
  `go build`, gofmt clean. Coverage includes: install with
  already-installed experience (DNF skipped), first-touch render,
  preservation, provision regeneration, template dir from RPM,
  full install sequence (stateful fake), remove plan preservation.
- rpmbuild `-ba` all specs; rpmlint 0 errors/0 warnings (core +
  experience); mock `--buildsrpm` + `--rebuild` green
  (fedora-44-x86_64) for core 0.1.0-7 and experience 0.1.0-2.
- `scripts/smoke-day5.sh`: **26/26** on the live machine before
  install — catalog separation (experience not installed by core
  upgrade; sddm not enabled; boot target multi-user), overview/info,
  unknown command fails, doctor desktop checks.

### Live install + activation

1. `sudo dnf install -y veilbox-experience-niri` — 269 packages
   (niri 26.04, noctalia 5.0.0~beta.7, kitty, sddm, grim, slurp,
   wl-clipboard, Xwayland). Boot target stayed `multi-user.target`.
   **Finding:** Fedora's `sddm %post` self-enables
   (`display-manager.service` alias) on first install — upstream
   behavior, documented in ADR-0006; disabled with `systemctl
   disable sddm` to restore the invariant.
2. `veil desktop install niri-desktop` — activation succeeded
   (core 0.1.0-4 → -5 → -6 → -7 fix chain: idempotent install, RPM-
   derived template dir + SystemDir, doctor templates check).
   Configs created, session file registered, sddm enabled,
   `graphical.target` set with rollback printed.
3. `veil doctor` — all desktop checks OK (manifest, packages, session
   file, templates, `noctalia config validate`).

### Reboot 1 — black screen episode (ADR-0009)

After reboot, SDDM greeter worked; login left a black screen with
niri running. Journal: config parse error (`error parsing KDL` at the
`layer-rule`) **and** renderer failure.

- **Config bug (Veilbox):** the shipped `niri.config.kdl` template
  was invalid KDL — `match namespace = "..."` (spaces around `=`),
  missing `;` after bind actions, removed `tap-to-click`/
  `column-gap`/`row-gap` keys and `focus-ring enable "on"`. niri
  rejected the whole config, so `spawn-at-startup "noctalia"` never
  ran — no shell. Template rewritten, iterated against
  `niri validate -c`, final render valid. Shipped as
  `veilbox-experience-niri-0.1.0-2` (rpmlint 0/0, mock green,
  `dnf upgrade` since NVR changed); live user config re-rendered
  (recorded as bug-affected artifact, ADR-0008).
- **Renderer (VirtualBox limitation):** `vmwgfx` reported "No 3D
  enabled"; niri skips software EGL renderers; no allocator → black
  screen even with a valid config. Software renderer is an open
  upstream item (niri #218 / PR #3959), not in 26.04.

**Fix (host-side):** user enabled **3D Acceleration** in the VM's
Display settings and rebooted.

### Reboot 2 — verified desktop

- niri initializes the primary renderer on `/dev/dri/renderD128`;
  config loads cleanly; **noctalia spawns** (bar running, dmenu/
  launcher sockets up).
- Functional checks via niri IPC + noctalia msg: output live
  (Virtual-1 1280x800@60), kitty spawns and tiles (Mod+T), editor
  spawn (Mod+E), launcher panel toggles (Mod+D), volume OSD
  (XF86Audio), grim captures a real screenshot.
- `veil desktop list` = installed; `veil status` = "Session: niri
  active"; `veil doctor` all OK including *"graphical Veilbox desktop
  session detected — niri (niri-desktop)"*.
- User confirmed visually: desktop works.

### Remove/reinstall lifecycle (non-destructive)

Snapshot (boot target, sddm state, sha256 of the three configs) →
`veil desktop remove niri-desktop`:

- 267 packages removed via DNF dependency cascade (sddm's own
  `%preun` removed the display-manager alias — upstream, expected).
- Configs byte-identical; `graphical.target` untouched; catalog back
  to `available`; deactivation hint printed.

`veil desktop install niri-desktop` restored everything: DNF
reinstall, user configs **preserved** (not overwritten), Veilbox file
regenerated, sddm re-enabled, "default target already
graphical.target" (idempotent). `veil doctor` all OK. SDDM restarted;
greeter back.

## Findings

- Fedora `sddm` `%post`/`%preun` manage the display-manager alias on
  install/remove — upstream package state, documented (ADR-0006).
- niri's KDL is strict: no spaces around `=`, `;` after actions,
  modern key names — caught only at compositor startup; the template
  fix added `niri validate` to the acceptance loop.
- VirtualBox without 3D acceleration = no niri renderer, black
  screen (ADR-0009); requirement documented for test VMs.
- Benign warnings: `import environment shell exited with status 1`
  (SDDM exports nothing to import), EDID/vblank notices — cosmetic.

## Live state after acceptance

`veilbox-core-0.1.0-7.fc44.x86_64`, `veilbox-experience-niri-0.1.0-2.fc44.noarch`;
boot target `graphical.target`, sddm enabled; desktop activated and
verified across two reboots and a remove/reinstall cycle;
pre-reboot snapshot in `/home/veilbox/acceptance-snapshots/day5-pre-reboot.txt`.

## References

- ADR-0006 (SDDM + explicit activation), ADR-0007 (desktop
  architecture), ADR-0008 (config ownership), ADR-0009 (VirtualBox
  limitations)
- v3/core/cmd/veil/desktop_cmd.go, v3/core/internal/desktop/*
  (install.go, desktop.go, render.go, remove.go), desktop_cmd_test.go
- v3/experiences/niri-desktop.yaml, v3/desktop/niri/*
- v3/packages/SPECS/veilbox-experience-niri.spec, veilbox-core.spec
- v3/scripts/smoke-day5.sh
