Name:           veilbox-experience-terminal-ops
Version:        0.1.0
Release:        1%{?dist}
Summary:        Veilbox experience: terminal operations toolkit

License:        GPL-2.0-only
URL:            https://github.com/Shreyas0047/veilbox
BuildArch:      noarch

Requires:       tmux
Requires:       ripgrep
Requires:       htop

%description
Veilbox experience that pulls in a small, coherent set of Fedora
terminal operations tools: terminal multiplexing (tmux), fast search
(ripgrep), and interactive process inspection (htop).

This is a meta-package: it contains no files of its own. Its value is
the dependency set it installs and removes as one Veilbox experience,
driven entirely through DNF. Packages already present on the system
before the experience is installed are left untouched on removal.

%files

%changelog
* Fri Aug 07 2026 Veilbox v3 — 0.1.0-1
- Initial Veilbox v3 experience: terminal operations toolkit.
