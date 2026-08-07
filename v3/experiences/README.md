# Experiences

An **experience** is an installable capability: a complete, coherent
environment delivered as an RPM module (`veilbox-experience-<name>`),
never as bare packages.

A desktop experience, for example, is not "install the compositor".
It is the compositor plus shell, launcher, notifications, lock screen,
idle management, clipboard, terminal, fonts, themes, wallpaper,
Veilbox configuration, and sensible defaults — so the desktop feels
complete immediately after installation.

Compositors that do not provide a complete desktop shell get one
provided by Veilbox (waybar-based, quickshell-based, or similar
integrated shell), keeping each desktop experience whole.

Experience definitions and config overlays:

- `experiences/<name>.yaml` — definition: RPM meta-package name,
  package requirements, config overlay reference, activation rules
- `configs/<name>/` — user-level configuration overlays delivered by
  the experience RPM under `/usr/share/veilbox/experiences/<name>/`
  and activated per-user into `~/.config/veilbox/` by
  `veil experience install`

See `docs/adr/0002-delivery-model.md` for the RPM delivery model.
