%global go_version 0.1.0

# Go binaries and rpm's debugsource generation do not cooperate
# (find-debuginfo produces an empty debugsourcefiles.list for Go
# modules); split debuginfo is deferred until the packaging matures.
%global debug_package %{nil}

Name:           veilbox-core
Version:        %{go_version}
Release:        7%{?dist}
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

  - veil profile / list / show / apply / diff / sync — engineer intent
  - veil experience list / info / install / remove — capability modules
  - veil desktop list / info / install / remove / provision — complete
    desktop experiences (compositor + shell + defaults, not bare
    compositors); package install and desktop activation are separate
    responsibilities — only 'veil desktop install' enables the display
    manager and switches the boot target
  - veil workspace / plan / apply / status / reset — user workspace
  - veil status — Veilbox and system state (profile sync included)
  - veil doctor — system, profile, and package consistency checks

Profiles are intent and state, never RPMs. Experiences are delivered
as RPM meta-packages installed through DNF. Workspace configuration is
generated under ~/.config/veilbox/workspace and integrated into user
shell files through a single marked include block; Veilbox never
rewrites user-owned files wholesale (see ADR-0005). Desktop
configuration follows the same rule: RPM-owned templates under
/usr/share/veilbox/desktop/, first-touch user config that is never
overwritten, and Veilbox-owned include files that are regenerated
(see ADR-0007).

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
* Fri Aug 07 2026 Veilbox v3 — 0.1.0-7
- doctor templates check resolves the RPM-owned desktop template
  directory via the engine (matches provision/install discovery).

* Fri Aug 07 2026 Veilbox v3 — 0.1.0-6
- Desktop template discovery follows the RPM-owned directory
  (veilbox-experience-niri templates live in
  /usr/share/veilbox/desktop/niri/); wallpaper path rendered from
  the system template dir.

* Fri Aug 07 2026 Veilbox v3 — 0.1.0-5
- Idempotent desktop install: 'veil desktop install' treats an
  already-installed experience package as a no-op step and proceeds
  with provisioning and activation (supports the approved flow of
  installing the experience with DNF first, then activating with
  veil desktop install).

* Fri Aug 07 2026 Veilbox v3 — 0.1.0-4
- Day 5 desktop engine: experience manifest type/display_name/
  components with strict declarative validation (safe tokens only,
  structural components must name real packages), veil desktop
  list/info/install/remove/provision commands, display-manager
  enablement and graphical-target activation as explicit Desktop
  Engine steps (never in the RPM), conservative removal that never
  touches the display manager, the boot target, or user config,
  first-touch desktop config provisioning consuming workspace
  preferences (terminal, editor), session detection that never
  guesses from a TTY, desktop checks in doctor. Ships the
  niri-desktop experience catalog entry.

* Fri Aug 07 2026 Veilbox v3 — 0.1.0-3
- Day 4 workspace engine: structured workspace_preferences
  (shell, editor, terminal, prompt, tmux, aliases, environment) with
  strict declarative validation (no shell metacharacters), workspace
  plan/apply/status/reset commands, Veilbox-owned generated files
  under ~/.config/veilbox/workspace/, single marked include block in
  ~/.bashrc and ~/.tmux.conf, first-touch backups under
  ~/.config/veilbox/backups/, hash-based drift detection with
  apply --force recovery (never overwrites whole user files),
  capability reporting without DNF, profile-switch cleanup, workspace
  checks in doctor.

* Fri Aug 07 2026 Veilbox v3 — 0.1.0-2
- Day 3 intent engine: profile schema (recommended/optional
  experiences, role, tags, workspace preferences), profile
  list/show/diff/sync, experience info, profile sync state in status,
  profile consistency checks in doctor. Ships four profiles
  (devops, sre, platform-engineer, cloud-engineer) and four
  experiences (base-ops, networking-tools, terminal-ops,
  observability-cli).

* Fri Aug 07 2026 Veilbox v3 — 0.1.0-1
- Initial Veilbox v3 vertical slice: veil CLI, profile engine,
  experience engine, DNF/RPM integration.
