Name:           veilbox-experience-niri
Version:        0.1.0
Release:        3%{?dist}
Summary:        Veilbox environment experience: Niri + Noctalia complete desktop

License:        GPL-2.0-only
URL:            https://github.com/Shreyas0047/veilbox
Source0:        %{name}-%{version}.tar.gz
BuildArch:      noarch

Requires:       niri
Requires:       noctalia
Requires:       kitty
Requires:       xorg-x11-server-Xwayland
Requires:       sddm
Requires:       wl-clipboard
Requires:       grim
Requires:       slurp

%description
Veilbox environment experience that installs a complete, usable desktop:
the Niri scrollable-tiling Wayland compositor with the Noctalia shell
(bar, dock, launcher, notifications, lock screen, idle handling,
wallpaper, clipboard, screenshots, OSDs), the kitty terminal,
XWayland, and Veilbox-owned environment configuration templates under
/usr/share/veilbox/environment/niri/.

A compositor is not an environment experience: this package installs
the whole stack plus Veilbox defaults, so the desktop is intentionally
configured at first login.

Package installation and environment activation are separate
responsibilities. This package installs an environment; it does NOT
activate it. Installing this RPM never changes the systemd default
target and never enables services. Environment activation — display
manager enablement and the switch to graphical.target — happens only
through 'veil environment install niri'.

%prep
%setup -q

%install
mkdir -p %{buildroot}%{_datadir}/veilbox/environment/niri
install -pm644 environment/niri/niri.config.kdl       %{buildroot}%{_datadir}/veilbox/environment/niri/
install -pm644 environment/niri/noctalia.config.toml  %{buildroot}%{_datadir}/veilbox/environment/niri/
install -pm644 environment/niri/noctalia-veilbox.toml %{buildroot}%{_datadir}/veilbox/environment/niri/
install -pm644 environment/niri/wallpaper.png         %{buildroot}%{_datadir}/veilbox/environment/niri/
install -pm644 environment/niri/README               %{buildroot}%{_datadir}/veilbox/environment/niri/

%files
%dir %{_datadir}/veilbox/environment/niri
%{_datadir}/veilbox/environment/niri/*

%changelog
* Sat Aug 08 2026 Veilbox v3 — 0.1.0-3
- Re-express the Niri experience as the reference environment
  (ADR-0012/ADR-0015): installed data moves from
  /usr/share/veilbox/desktop/niri/ to /usr/share/veilbox/environment/niri/.
  Package name and activation wording unchanged.

* Fri Aug 07 2026 Veilbox v3 — 0.1.0-2
- Fix niri.config.kdl template: valid KDL syntax (spacing around '=',
  semicolons after bind actions, modern niri 26 touchpad/layout keys).
  Previous template was rejected by niri, leaving the desktop shell
  unstarted at first login.

* Fri Aug 07 2026 Veilbox v3 — 0.1.0-1
- Initial Veilbox v3 desktop experience: Niri + Noctalia complete desktop.
