# ADR-0001: Veilbox v3 Architecture

Status: Accepted
Date: 2026-08-07
Decision makers: Lead Implementation Engineer (approved by project owner)

## Context

Veilbox v2 was a Debian Trixie-based live ISO with a fixed, hardcoded
DevOps toolchain. Veilbox v3 is a ground-up rebuild on Fedora with a
fundamentally different goal: Veilbox is an **Operations Platform** for
DevOps, SRE, Platform, Cloud, and Kubernetes engineers, not "Fedora with
DevOps tools bolted on".

Two generations of history inform this ADR:

- v1: hand-built minimal Linux (custom kernel, BusyBox, containerd) —
  demonstrates the "from scratch" path and its maintenance cost.
- v2: Debian live ISO with Calamares installer — demonstrates the
  "frozen distro image" path and its update cost.

v3 rejects both extremes: Fedora manages the operating system, Veilbox
manages the engineer.

## Decision

The system is layered. Fedora owns the operating system; Veilbox owns
everything that is specifically about the engineer.

```
Engineer
   |
Veilbox CLI / UI (veil)
   |
Veilbox Core
   |-- Profile Engine
   |-- Experience Engine
   |-- Settings Engine
   `-- Workspace Manager
   |
Fedora extension points
   |-- DNF
   |-- RPM
   |-- systemd
   |-- repositories
   `-- installer
   |
Fedora
   |
Linux
```

### Core principles

1. **Fedora manages the OS; Veilbox manages the engineer.**
2. **Never compete with Fedora. Complete Fedora.** Everything Veilbox
   installs is delivered as standard Fedora RPMs through DNF.
3. **Extend, never replace.** Veilbox never shadows or forks Fedora
   mechanisms (dnf, systemd, SELinux, RPM transactions).
4. **Ship in modules.** Veilbox Core is a small kernel of engines, not a
   monolith. Experiences and profiles are data + packages layered on top.
5. **Experiences over packages.** Users install complete experiences
   (compositor + shell + launcher + notifications + lock + idle +
   clipboard + terminal + fonts + themes + wallpaper + Veilbox config +
   defaults), never bare packages.
6. **Engineer intent over implementation mechanics.** The `veil` CLI
   speaks intent (`veil experience install hyprland`), never RPM lists.
7. **Removal must be safe.** Uninstalling Veilbox Core leaves a
   functional Fedora system. No Veilbox code becomes load-bearing for
   the OS.
8. **Reliability over breadth.** The prototype favors a small, correct,
   complete set over a large, fragile one.

### Profile vs Experience separation

This ADR records a project-owner correction to earlier planning:

- **Profile = intent.** A declared engineering role with selected
  capabilities (cloud, containers, kubernetes, infrastructure,
  observability). Profiles are **configuration and state**, owned by
  Veilbox Core, stored under `/etc/veilbox/` (system defaults) and
  `~/.config/veilbox/` (user state).
- **Experience = implementation / capability.** Installable RPM modules
  (`veilbox-experience-<name>`) that pull the concrete package stack
  and configuration for a capability.

**Profiles must never be generated as RPMs.** No
`veilbox-profile-<name>-<caps>` packages. DNF/RPM state is the *source
of truth for packages*; Veilbox Core persists profile intent separately.
`veil status` reconciles the two views.

## Consequences

Positive:

- The prototype is demonstrable on a stock Fedora system without any
  ISO: install `veilbox-core`, run `veil profile apply`, `veil
  experience install` and the machine becomes a complete operations
  workspace through standard Fedora transactions.
- Nearly the entire target toolchain (niri, hyprland, waybar, terraform,
  opentofu, pulumi, ansible, helm, k9s, kubectl, aws-cli, azure-cli,
  google-cloud-cli, podman, prometheus, grafana, loki, k3s, argocd,
  openshift-clients) is packaged in stock Fedora — verified 2026-08-07.
  No third-party repos are required for the prototype.
- Experiences are independently installable and updatable through DNF
  without reinstalling anything.

Negative / risks:

- Fedora releases are shorter-lived than Debian LTS; v3 deliberately
  embraces that (Fedora manages the OS, including its lifecycle).
- A small number of desired tools (e.g. Noctalia) are not in Fedora;
  they can be added later via COPR without changing the architecture.

## References

- ADR-0002: Delivery model (RPM meta-packages, DNF as transaction engine)
- v3/README.md: development status and layout
