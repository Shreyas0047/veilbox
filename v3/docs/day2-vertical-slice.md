# Day 2: Vertical Slice — Profiles, Experiences, DNF Installation

Date: 2026-08-07

## Goal

Prove the v3 architecture end-to-end: a `veil` CLI that reads profile
and experience manifests, performs DNF transactions against a real
(development) repository, and reports machine state — all on Fedora.

## What was built

### Core (`v3/core/`)

Go module `github.com/veilbox/v3/core`, vendoring `gopkg.in/yaml.v3`
(pinned in `vendor/`) so `rpmbuild` and mock work offline. The
`internal/` packages:

- `settings` — XDG paths, `VEILBOX_ROOT` override, `state.json` I/O
- `dnfops` — `Runner` interface; `rpm -q` install detection (missing
  package = "is not installed" in output); transaction runs via
  `sudo dnf` through the user's own sudo configuration
- `profile` — manifest registry, `Apply` (idempotent state write)
- `experience` — catalog with status resolution:
  `planned` (no `rpm` field) / `available` / `installed`
  (meta-package present in the RPM database)
- `workspace` — reserved for provisioning (Day 3+)

CLI commands: `veil version`, `veil profile apply <name>`,
`veil experience list|install|remove`, `veil status`, `veil doctor`.
`main.go` takes injected `deps` (stdout, stderr, runners) for tests.

### Manifests

```yaml
# v3/profiles/devops.yaml
name: devops
description: ...
capabilities:
  - domain: core
    rationale: base platform
  - domain: networking
    rationale: connectivity diagnostics
```

```yaml
# v3/experiences/networking-tools.yaml
name: networking-tools
description: Network diagnostics CLI tools
rpm: veilbox-experience-networking-tools
packages: [bind-utils, traceroute, nmap-ncat, iproute, tcpdump]
```

`terminal-ops.yaml` is `planned` (no `rpm` field) — listed, not
installable.

### Packaging

| File | Role |
|------|------|
| `packages/SPECS/veilbox-core.spec` | Go build (`-buildmode=pie`, `-mod=vendor`), ships `/usr/bin/veil` + manifests + LICENSE |
| `packages/SPECS/veilbox-experience-networking-tools.spec` | noarch meta-package, `Requires:` the 5 tools, zero files |
| `scripts/build-sources.sh` | stages `go.mod`/`go.sum`/`vendor/`, tarballs into `packages/SOURCES/` |
| `scripts/build-rpms.sh` | `rpmbuild -ba` both specs |
| `scripts/compose-repo.sh` | createrepo_c over `packages/build/RPMS` into `/srv/veilbox-repo` |
| `scripts/smoke-day2.sh` | full install → apply → install → verify procedure (27 checks) |

### Repository

`/etc/yum.repos.d/veilbox-dev.repo` → `file:///srv/veilbox-repo`,
`gpgcheck=0` (development only; signing is a hosted-repository
prerequisite). `veil experience install` resolves the meta-package by
name through DNF — the code path is identical to a future HTTP repo.

## Spec gotchas (day-2 learnings)

1. `%{debug_package} %{nil}` is required: Go builds produce no
   `debugsourcefiles.list`, which aborts `rpmbuild`.
2. Never reference rpm macro names (e.g. `%gobuild`) in spec
   **comments** — RPM macro-expands comments and breaks `%prep`.
   BuildRequires `go-rpm-macros` was removed for the same reason.
3. `%license` manages its own directory; a manual
   `%install` copy of LICENSE conflicts ("File exists").

## Test procedure

1. `scripts/build-sources.sh && scripts/build-rpms.sh && scripts/compose-repo.sh`
2. `sudo dnf install -y veilbox-core` (from `veilbox-dev`)
3. `veil profile apply devops`
4. `veil experience install networking-tools` (DNF pulls the
   meta-package + 5 tools)
5. `veil experience list` / `veil status` / `veil doctor`
6. `scripts/smoke-day2.sh` — 27/27 on a clean state

Verify the install is real: `which dig && dig +short localhost`.

## Post-reboot and removal verification (2026-08-07)

Performed after a full VM reboot with the committed tree at `6306e75`:

1. `veil status` → Core 0.1.0, Profile **devops**, Experience
   **veilbox-experience-networking-tools** — profile and experience
   state both persisted across reboot (state is user-owned JSON plus
   RPM database; nothing boots Veilbox services).
2. `veil doctor` → all 6 checks OK, exit 0.
3. `veil experience remove networking-tools` → DNF removed the
   meta-package plus 8 now-unused dependencies (bind-libs, bind-utils,
   fstrm, libmaxminddb, libpcap, nmap-ncat, tcpdump, traceroute).
   `iproute` was correctly **kept** (pre-installed Fedora package, not
   a Veilbox dependency).
4. Snapshot diff (`rpm -qa` before/after) contained **exactly** those 9
   packages — no unrelated package removed or added; `/etc/yum.repos.d/`
   unchanged; no stray systemd units created by Veilbox.
5. `veil experience list` → `networking-tools` back to **available**;
   `veil status` → Profile devops intact, `(none installed)` experiences,
   834 RPMs (843 − 9); `veil doctor` → all OK.
6. `GOFLAGS=-mod=vendor go test ./...` and `go vet ./...` → pass.

Conclusion: install/removal is symmetric through DNF, state is
RPM-consistent, and nothing outside Veilbox-managed packages was
touched.

## Current limitations

- `gpgcheck=0`; no signing yet.
- Profile manifests only drive state today; capability → config
  overlays and provisioning arrive with workspace (Day 3+).
- `packages/SOURCES/*.tar.gz` is generated (gitignored), not committed.

## References

- ADR-0001 (architecture), ADR-0002 (delivery model),
  ADR-0003 (state & config layout, dev repository)
