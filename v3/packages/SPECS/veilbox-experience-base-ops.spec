Name:           veilbox-experience-base-ops
Version:        0.1.0
Release:        1%{?dist}
Summary:        Veilbox experience: essential engineering base

License:        GPL-2.0-only
URL:            https://github.com/Shreyas0047/veilbox
BuildArch:      noarch

Requires:       git
Requires:       vim-enhanced
Requires:       curl
Requires:       strace

%description
Veilbox experience that pulls in the essential engineering base every
operations profile starts from: version control (git), an editor
(vim-enhanced), transfer/debugging tools (curl), and syscall tracing
(strace).

This is a meta-package: it contains no files of its own. Its value is
the dependency set it installs and removes as one Veilbox experience,
driven entirely through DNF. Packages already present on the system
before the experience is installed (git, curl, and friends are common
on workstations) are left untouched on removal — DNF only cleans up
packages that were introduced solely as dependencies of this
experience and are needed by nothing else.

%files

%changelog
* Fri Aug 07 2026 Veilbox v3 — 0.1.0-1
- Initial Veilbox v3 experience: essential engineering base.
