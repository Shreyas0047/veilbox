Name:           veilbox-experience-networking-tools
Version:        0.1.0
Release:        1%{?dist}
Summary:        Veilbox experience: networking diagnostics toolkit

License:        GPL-2.0-only
URL:            https://github.com/Shreyas0047/veilbox
BuildArch:      noarch

Requires:       bind-utils
Requires:       traceroute
Requires:       nmap-ncat
Requires:       iproute
Requires:       tcpdump

%description
Veilbox experience that pulls in a small, coherent set of Fedora
networking diagnostics tools: DNS query tools (bind-utils),
traceroute, netcat (nmap-ncat), iproute2, and packet capture
(tcpdump).

This is a meta-package: it contains no files of its own. Its value is
the dependency set it installs and removes as one Veilbox experience,
driven entirely through DNF.

%files

%changelog
* Fri Aug 07 2026 Veilbox v3 — 0.1.0-1
- Initial Veilbox v3 experience: networking diagnostics toolkit.
