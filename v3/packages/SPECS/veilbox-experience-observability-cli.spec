Name:           veilbox-experience-observability-cli
Version:        0.1.0
Release:        1%{?dist}
Summary:        Veilbox experience: observability command-line toolkit

License:        GPL-2.0-only
URL:            https://github.com/Shreyas0047/veilbox
BuildArch:      noarch

Requires:       sysstat
Requires:       iotop
Requires:       jq

%description
Veilbox experience that pulls in a small, coherent set of Fedora
observability CLI tools: system performance metrics (sysstat: iostat,
sar, mpstat), interactive I/O monitoring (iotop), and JSON querying
(jq) for logs and API inspection.

This is a meta-package: it contains no files of its own. Its value is
the dependency set it installs and removes as one Veilbox experience,
driven entirely through DNF. Packages already present on the system
before the experience is installed are left untouched on removal.

%files

%changelog
* Fri Aug 07 2026 Veilbox v3 — 0.1.0-1
- Initial Veilbox v3 experience: observability command-line toolkit.
