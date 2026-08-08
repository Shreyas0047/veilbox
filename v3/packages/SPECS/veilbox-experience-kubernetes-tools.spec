Name:           veilbox-experience-kubernetes-tools
Version:        0.1.0
Release:        1%{?dist}
Summary:        Veilbox experience: Kubernetes operations toolkit

License:        GPL-2.0-only
URL:            https://github.com/Shreyas0047/veilbox
BuildArch:      noarch

Requires:       kubernetes1.36-client
Requires:       helm
Requires:       k9s

%description
Veilbox experience that pulls in a small, coherent set of Fedora
Kubernetes tooling: kubectl (kubernetes1.36-client) for cluster
administration, Helm for release management, and k9s for fast
interactive cluster navigation.

This is a meta-package: it contains no files of its own. Its value is
the dependency set it installs and removes as one Veilbox experience,
driven entirely through DNF.

%files

%changelog
* Sat Aug 08 2026 Veilbox v3 — 0.1.0-1
- Initial Veilbox v3 experience: Kubernetes operations toolkit.
