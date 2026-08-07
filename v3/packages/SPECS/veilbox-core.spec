%global go_version 0.1.0

# Go binaries and rpm's debugsource generation do not cooperate
# (find-debuginfo produces an empty debugsourcefiles.list for Go
# modules); split debuginfo is deferred until the packaging matures.
%global debug_package %{nil}

Name:           veilbox-core
Version:        %{go_version}
Release:        1%{?dist}
Summary:        Veilbox Core — Operations Platform for engineers

License:        GPL-2.0-only
URL:            https://github.com/Shreyas0047/veilbox
Source0:        %{name}-%{version}.tar.gz

BuildRequires:  golang >= 1.24

%description
Veilbox is an Operations Platform for DevOps, SRE, Platform, Cloud,
and Kubernetes engineers. Fedora manages the operating system;
Veilbox manages the engineer.

Veilbox Core provides the veil CLI:

  - veil profile / veil profile apply — engineer profile intent
  - veil experience list / install / remove — capability modules
  - veil status — Veilbox and system state
  - veil doctor — system and package consistency checks

Profiles are intent and state, never RPMs. Experiences are delivered
as RPM meta-packages installed through DNF.

%prep
%setup -q -n %{name}-%{version}

%build
# Module-aware build with vendored dependencies: no network access at
# build time. The GOPATH-style go-rpm-macros build macro is deliberately
# not used; this project is a Go module with committed vendor/.
GOFLAGS="-mod=vendor -trimpath" go build -buildmode=pie -o build/veil ./cmd/veil

%check
GOFLAGS="-mod=vendor" go test ./...

%install
install -Dpm755 build/veil %{buildroot}%{_bindir}/veil
for f in profiles/*.yaml; do
    install -Dpm644 "$f" "%{buildroot}%{_datadir}/veilbox/profiles/$(basename "$f")"
done
for f in experiences/*.yaml; do
    install -Dpm644 "$f" "%{buildroot}%{_datadir}/veilbox/experiences/$(basename "$f")"
done

%files
%license LICENSE
%{_bindir}/veil
%dir %{_datadir}/veilbox
%dir %{_datadir}/veilbox/profiles
%dir %{_datadir}/veilbox/experiences
%{_datadir}/veilbox/profiles/*.yaml
%{_datadir}/veilbox/experiences/*.yaml

%changelog
* Fri Aug 07 2026 Veilbox v3 — 0.1.0-1
- Initial Veilbox v3 vertical slice: veil CLI, profile engine,
  experience engine, DNF/RPM integration.
