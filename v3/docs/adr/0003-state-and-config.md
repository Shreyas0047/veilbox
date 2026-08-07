# ADR-0003: Configuration, State, and Development Repository

Status: Accepted
Date: 2026-08-07

## Context

Day 2 of the v3 prototype introduces the first vertical slice: profile
state, an experience catalog, and a DNF-driven installation path. The
layout of configuration and state, the data formats, and the source of
Veilbox packages must be pinned so later days (workspace provisioning,
config overlays, hosted repository) build on stable ground.

## Decision

### Paths

| Path | Owner | Contents |
|------|-------|----------|
| `/usr/share/veilbox/profiles/*.yaml` | RPM (`veilbox-core`) | Profile manifests (intent definitions) |
| `/usr/share/veilbox/experiences/*.yaml` | RPM (`veilbox-core`) | Experience catalog (capability definitions) |
| `~/.config/veilbox/state.json` | user / `veil` | Machine-written state: active profile, applied timestamp |
| `/srv/veilbox-repo` | system (dev VM) | Local `file://` development DNF repository |
| `/etc/yum.repos.d/veilbox-dev.repo` | system (dev VM) | Repo configuration pointing at the dev repository |

`VEILBOX_ROOT` overrides `/usr/share/veilbox` for tests and development.

### Formats

- **YAML** for everything humans author and edit: profile manifests and
  experience catalog entries. "The engineer's environment must remain
  editable after installation" — manifests are plain files, not
  generated code.
- **JSON** for machine-written state (`state.json`): unambiguous,
  versioned via `version` field, trivially diffable.
- The experience manifest declares `name`, `description`, `rpm` (the
  meta-package implementing it; empty = planned), and informational
  `packages`. The RPM `Requires:` are authoritative for the package set.

### Privileges

- `veil status`, `veil profile`, `veil experience list` run
  unprivileged; installed-state queries go through `rpm` (no root).
- Transactions go through `sudo dnf` using the calling user's normal
  sudo configuration. Veilbox never assumes passwordless sudo exists
  (see ADR-0002).
- `veil doctor` checks system prerequisites and reports; it returns
  non-zero only for failed critical checks.

### Development repository

The local `file://` repository at `/srv/veilbox-repo` is the
development equivalent of the future hosted Veilbox repository. It is
composed by `scripts/compose-repo.sh` (createrepo_c over built RPMs).
`veil experience install` resolves experiences **by package name**
through DNF against configured repositories; Veilbox code never
special-cases local RPM file paths. The hosted repository will be the
same artifact published over HTTP(S) (see ADR-0002).

## Consequences

- Profile state survives reboots (user-owned JSON, RPM-independent).
- Experience state is derived from the RPM database — `veil status`
  cannot disagree with DNF because it *is* DNF's view.
- The dev repository keeps the install path honest: it exercises the
  same "install by name from a configured repo" flow the hosted
  repository will use.
- gpgcheck is disabled for the dev repository only; signing is a
  prerequisite for the hosted repository (future ADR).

## References

- ADR-0001: architecture (profiles = intent, experiences = capability)
- ADR-0002: delivery model (RPM meta-packages, DNF transactions)
- docs/day2-vertical-slice.md: implementation notes and test procedure
