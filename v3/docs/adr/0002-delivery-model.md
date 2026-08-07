# ADR-0002: Delivery Model — RPM Meta-packages and DNF

Status: Accepted
Date: 2026-08-07

## Context

Veilbox v3 must be installable, updatable, and removable on a stock
Fedora system using Fedora-native mechanisms (RPM, DNF, repositories).
Experiences must be independently installable and updateable without
requiring an ISO reinstall. Veilbox Core must be removable without
breaking Fedora.

## Decision

### Package topology

| RPM | Contents | Nature |
|-----|----------|--------|
| `veilbox-core` | Static `veil` binary, engine logic, default profile definitions (`/usr/share/veilbox/profiles/`), experience registry (`/usr/share/veilbox/experiences/`), systemd integration | The only mandatory Veilbox package |
| `veilbox-experience-<name>` | `Requires:` the complete package stack for a capability (e.g. niri, waybar, fuzzel, foot, mako, swaylock, swayidle, wl-clipboard, pipewire, fonts, themes); config overlays under `/usr/share/veilbox/experiences/<name>/` | One RPM per experience |
| `veilbox-repos` | DNF repository file pointing at the Veilbox RPM repository | Optional; installs the platform's own packages |

### Rules

1. **Experiences are RPMs; profiles are not.** Profiles are intent and
   state, stored as configuration by Veilbox Core (see ADR-0001). No
   profile-generated RPMs.
2. **DNF is the transaction engine.** Veilbox Core shells out to
   `dnf` (subprocess) for install/remove/update transactions in v1;
   the `libdnf5` Python/Go bindings may replace this later without
   changing the public contract.
3. **User-level activation, system-level delivery.** Experience RPMs
   deliver files under `/usr/share/veilbox/`; user configuration is
   activated per-user by `veil experience install` into
   `~/.config/veilbox/` (XDG), never by mutating system files. Removing
   an experience removes the packages; removing `veilbox-core` leaves
   Fedora packages and the desktop intact.
4. **Repository hosting.** For the prototype, the Veilbox repo is a
   `createrepo_c` repository hosted on GitHub Pages (static hosting is
   sufficient for a DNF repo). COPR is the Fedora-native path once the
   platform matures; the `veilbox-repos` RPM makes the source of the
   repository a one-line configuration concern.
5. **No privilege assumptions in product artifacts.** Passwordless sudo
   is enabled only on the disposable development VM to support
   automation. It must never appear in Veilbox Core, Veilbox RPMs,
   kickstart files, ISO defaults, or production configuration. Veilbox
   commands use the calling user's existing privileges (polkit/sudo
   prompting when root is required) and never assume NOPASSWD.
6. **ISO is a stretch goal.** The primary one-week deliverable is the
   in-VM vertical slice. The kickstart lives in `v3/kickstart/` and
   consumes the same RPMs; it is not a fork of the installation path.

### Transaction flow

```
veil experience install niri
  -> resolve experience definition (registry)
  -> dnf install veilbox-experience-niri     (one RPM transaction)
  -> activate user-level config overlay into ~/.config/veilbox/
  -> veil status reflects new state
```

## Consequences

Positive:

- Every Veilbox action maps to a standard, inspectable DNF transaction.
- `dnf remove veilbox-experience-niri` cleanly reverts a desktop
  experience; `dnf remove veilbox-core` reverts the platform.
- No custom package manager, no bespoke dependency solver, no shadow
  state: DNF/RPM *is* the package state.

Negative / risks:

- Meta-package granularity must be chosen carefully so experiences
  compose without dependency conflicts (e.g. one clipboard tool, one
  terminal per experience; conflicts resolved by defaulting, not
  fighting DNF).
- DNF transactions require root; the CLI must handle privilege
  escalation through the user's normal mechanism.

## References

- ADR-0001: Veilbox v3 architecture
- v3/packages/SPECS/: RPM specifications
