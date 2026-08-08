Name:           veilbox-experience-containers-tools
Version:        0.1.0
Release:        1%{?dist}
Summary:        Veilbox experience: container workflows

License:        GPL-2.0-only
URL:            https://github.com/Shreyas0047/veilbox
BuildArch:      noarch

Requires:       podman
Requires:       buildah
Requires:       skopeo

%description
Veilbox experience that pulls in a small, coherent set of Fedora
container tooling: Podman for rootless container and pod management,
Buildah for OCI image building, and Skopeo for inspecting, copying and
verifying images and registries.

This is a meta-package: it contains no files of its own. Its value is
the dependency set it installs and removes as one Veilbox experience,
driven entirely through DNF.

%files

%changelog
* Sat Aug 08 2026 Veilbox v3 — 0.1.0-1
- Initial Veilbox v3 experience: container workflows.
