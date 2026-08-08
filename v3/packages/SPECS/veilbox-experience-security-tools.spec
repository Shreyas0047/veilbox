Name:           veilbox-experience-security-tools
Version:        0.1.0
Release:        1%{?dist}
Summary:        Veilbox experience: security auditing toolkit

License:        GPL-2.0-only
URL:            https://github.com/Shreyas0047/veilbox
BuildArch:      noarch

Requires:       nmap
Requires:       openscap-scanner
Requires:       lynis

%description
Veilbox experience that pulls in a small, coherent set of Fedora
security tooling: nmap for port and service discovery, OpenSCAP for
vulnerability and compliance scanning, and Lynis for host hardening
audits.

This is a meta-package: it contains no files of its own. Its value is
the dependency set it installs and removes as one Veilbox experience,
driven entirely through DNF.

%files

%changelog
* Sat Aug 08 2026 Veilbox v3 — 0.1.0-1
- Initial Veilbox v3 experience: security auditing toolkit.
