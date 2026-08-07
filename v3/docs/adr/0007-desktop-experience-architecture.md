# ADR-0007: Desktop Experience Architecture

Status: Accepted
Date: 2026-08-07

## Context

Day 5 introduces the first Veilbox desktop. The question is how a
desktop fits the Day-3 architecture: experiences are DNF
meta-packages, profiles recommend them, and Veilbox owns generated
user configuration. A desktop is bigger than a CLI experience: it
pulls a compositor, a shell, a terminal, a display manager, and it
carries an activation story (boot target, services). It must follow
the same discipline — install is capability, activation is explicit —
without inventing a second architecture.

## Decision

### A desktop is an experience

A desktop is a normal experience manifest with `type: desktop`:

```yaml
name: niri-desktop
type: desktop
rpm: veilbox-experience-niri
components:
  compositor: niri
  shell: noctalia
  terminal: kitty
  display_manager: sddm
packages: [niri, noctalia, kitty, xorg-x11-server-Xwayland, sddm, wl-clipboard, grim, slurp]
```

`components` names the desktop stack (each entry is the Fedora
package name, or `builtin` when the shell integrates the capability).
The catalog, `veil desktop list`, `veil desktop info` and
`veil doctor` all consume the same manifest.

### The desktop stack

| Component | Package | Role |
|---|---|---|
| Compositor | `niri` | Scrollable-tiling Wayland compositor |
| Shell | `noctalia` | Bar, dock, launcher, notifications, lock, idle, wallpaper, clipboard, screenshots, OSDs |
| Terminal | `kitty` | Spawned by the default binds (Mod+T) |
| XWayland | `xorg-x11-server-Xwayland` | X11 compatibility layer |
| Display manager | `sddm` | Greeter and session launcher (ADR-0006) |
| Screenshot | `grim` + `slurp` | Region screenshots via the Print bind |
| Clipboard | `wl-clipboard` | Wayland clipboard utilities |

### One activation path

`veil desktop install <name>` is the only activation path. It:

1. Installs the experience RPM through DNF (or skips the package
   step when the experience is already installed — idempotent).
2. Registers the session file (`/usr/share/wayland-sessions/`).
3. Renders user configuration on first touch (first-touch wins;
   existing user files are preserved).
4. Enables the display manager (systemd unit).
5. Sets the default boot target to `graphical.target` and prints the
   rollback command.

The install step is therefore not a package *install* but an
*activate*: it converges the machine to the desktop's desired state
and is safe to re-run.

### Provision is separate from activation

`veil desktop provision <name>` renders/regenerates only Veilbox-owned
configuration (`~/.config/veilbox/desktop/<name>/`), preserving
user-owned files. This gives a repair tool that never steps on the
user (ADR-0008).

### Removal is conservative

`veil desktop remove <name>` removes the experience RPM (DNF
dependency cascade) and reports:

- preserved user configuration,
- Veilbox-managed configuration that remains,
- the explicit deactivation hint.

It never disables services and never changes the boot target
(ADR-0006).

### Template discovery follows the RPM

Desktop templates ship in the experience package under
`/usr/share/veilbox/desktop/<rpm-short-name>/` (e.g. `niri` for
`veilbox-experience-niri`). The engine derives the template directory
from the manifest RPM name so the catalog stays the single source of
truth.

## Consequences

- Desktop, CLI experiences, workspace and profiles share one mental
  model: intent, capability, explicit application.
- Activation, provision and removal are each idempotent and
  independently testable.
- The manifest is the contract: doctor checks manifest validity,
  package presence, session file, template readability and shell
  config validity against it.

## References

- ADR-0006: display manager and explicit activation
- ADR-0008: desktop configuration ownership
- ADR-0001: architecture
- docs/day5-desktop.md: implementation and acceptance
