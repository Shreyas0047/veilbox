# ADR-0009: VirtualBox Limitations for the Desktop

Status: Accepted
Date: 2026-08-07

## Context

The Day 5 desktop acceptance runs in the disposable development VM,
which is a VirtualBox guest (VMware SVGA II adapter, `vmwgfx` kernel
driver, no Guest Additions, no RPM Fusion). After a successful login
the screen was black: the compositor ran, but nothing rendered. The
journal told the story:

```
niri: using as the render node: "/dev/dri/renderD128"
niri::backend::tty: initializing the primary renderer
WARN: VMware: No 3D enabled (0, Success)
WARN: failed to initialize renderer, falling back to primary gpu:
      software EGL renderers are skipped
WARN: error adding primary node device, display-only devices may not
      work: no allocator available for device
```

`vmwgfx` exposes a render node but no 3D capability (the VirtualBox
setting "Enable 3D Acceleration" was off), so no GBM/EGL allocator
exists, and niri deliberately skips software (llvmpipe) EGL renderers.
niri's own documentation states the requirement: *"To run niri in a
VM, make sure to enable 3D acceleration."*

## Decision

### The VirtualBox 3D acceleration checkbox is a hard requirement

Running a Veilbox desktop in VirtualBox requires the VM to have
**Display → Enable 3D Acceleration** checked. This is a host-side VM
setting, not a guest package; the guest cannot turn it on from the
inside. With it enabled, `vmwgfx` reports 3D support and the
compositor initializes the primary renderer normally (verified in
acceptance: clean renderer init, shell spawns, desktop fully
functional).

### Guest-side software rendering is not an option today

niri's EGL backend explicitly skips software EGL renderers
(`EGL_MESA_device_software`); there is no `software-rendering` config
option in niri 26.04, and upstream software-renderer support is an
open enhancement with a still-open design PR (niri issue #218 /
PR #3959), not part of the shipped version. Veilbox therefore does
not attempt a guest-side workaround and documents the VM requirement
instead.

### Triage table for "black screen after login"

| Observation | Classification | Action |
|---|---|---|
| Config parse error in journal; shell never spawned | **Veilbox bug** | fix template, re-render, release |
| Compositor crash / shell crash | Package bug | report to Fedora/upstream |
| Compositor running, no renderer ("No 3D enabled", "no allocator") | **VirtualBox/Vulkan limitation** | enable 3D Acceleration in the VM settings, reboot |
| Hardware GPU absent on bare metal | Hardware limitation | use a GPU with working DRM driver |

## Consequences

- Acceptance documentation must record "Enable 3D Acceleration" as a
  prerequisite for graphical desktop testing in VirtualBox.
- No guest-side software-rendering escape hatch is promised or
  investigated further until upstream lands software renderer support.
- The development VM remains Guest Additions-free and RPM
  Fusion-free; 3D acceleration requires neither.

## References

- niri wiki, Getting Started: "To run niri in a VM, make sure to
  enable 3D acceleration."
- niri issue #218 / PR #3959: software renderer (open, not shipped)
- docs/day5-desktop.md: the black-screen episode and resolution
