# ADR-0006: Display Manager — SDDM, and Activation Is Explicit

Status: Accepted
Date: 2026-08-07

## Context

The Day 5 desktop brings Veilbox's first real desktop stack. Any Linux
desktop needs a display manager (greeter + session launcher). Fedora
shipped options are GDM, SDDM, LightDM, and a few others. The choice
matters twice: once for the greeter experience, once for the
activation model — because a display manager is a system service and
the default boot target change is a system-level side effect.

Day 3 established the principle that experiences are capability
packages and that Veilbox never silently changes system state. A
desktop experience must be installable by DNF *without* hijacking the
machine, and activatable only through an explicit Veilbox action.

## Decision

### SDDM as the display manager

- SDDM is the display manager for the Veilbox desktop experience.
- Chosen for: Wayland-native session launching (`sddm-helper --start
  niri-session`), Qt/QML greeter, uncomplicated configuration, and a
  modest dependency footprint compared to GDM.
- SDDM session files are standard Wayland session files under
  `/usr/share/wayland-sessions/`; Veilbox ships one for the desktop.

### Installation is inert; activation is explicit

| Action | Side effects |
|---|---|
| `dnf install veilbox-experience-niri` | Installs packages and templates. **Never** changes the boot target, never enables the display manager. |
| `veil desktop install niri-desktop` | Explicit activation: creates the session file, renders user configuration, enables SDDM, sets `graphical.target`. Prints a rollback hint. |
| `veil desktop remove niri-desktop` | Removes only the experience RPM (DNF dependency cascade). Never touches the display manager or boot target; preserves user configuration; prints the deactivation hint. |
| `veil desktop provision niri-desktop` | Regenerates Veilbox-owned configuration only. |

### Fedora SDDM packaging self-enables — documented, not fought

Fedora's `sddm` package runs `systemd-update-helper
install-system-units sddm.service` in its `%post` on first install,
which creates the `/etc/systemd/system/display-manager.service` alias.
This happens at package-install time, outside Veilbox's control.
Veilbox does not try to intercept it; the invariant it guarantees is
about Veilbox's own behavior:

- Veilbox's engine never enables the display manager or changes the
  boot target except through `veil desktop install`.
- `veil desktop install` re-enables and re-asserts both idempotently.
- `veil desktop remove` deliberately leaves the display manager and
  boot target untouched (they are upstream package state, not
  Veilbox-owned state).

Acceptance observed the SDDM `%post`/`%preun` behavior directly:
installing the experience package self-enabled the display-manager
alias (manually disabled to restore the invariant), and removing the
package's `%preun` removed it again. Both are Fedora package behavior,
documented here so future acceptance runs recognize them as expected.

### Deactivation hint

`veil desktop remove` prints the explicit deactivation recipe instead
of performing it:

```
sudo systemctl disable --now sddm; sudo systemctl set-default multi-user.target
```

## Consequences

- A DNF-only install can never surprise a user with a boot-time
  greeter they did not ask for.
- The activation step is idempotent and re-runnable after removal.
- SDDM package scriptlets are upstream state; Veilbox documents them
  rather than fighting them.
- First login after activation requires a reboot (or a live
  `systemctl start sddm`); the CLI says so.

## References

- ADR-0002: delivery model (RPMs as the delivery mechanism)
- ADR-0007: desktop experience architecture
- docs/day5-desktop.md: implementation and acceptance
