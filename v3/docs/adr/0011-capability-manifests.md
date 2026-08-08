# ADR-0011: Capability Manifests

Status: Accepted
Date: 2026-08-08

## Context

ADR-0001 distinguishes profile (= intent, with selected capabilities)
from experience (= implementation, installable RPM module), but the
day-6 implementation skips a capability layer: profiles recommend
*experiences* directly, the wizard exposes the experience catalog,
and "capability" exists only as a string list inside profile YAML.
The product vision is that **users pick capabilities, never RPMs** —
and that capability selection is richer than a flat experience list
(grouped, tiered, and independent of how the capability is
implemented).

A capability must be a first-class, owned manifest so that:

- the catalog of capabilities is authored data, not code;
- one capability maps to potentially many experiences, and one
  experience implements potentially many capabilities (N→M);
- experiences may be reimplemented (a new package stack) without
  changing what the user selected.

## Decision

### Capability manifests are first-class

New `capabilities/*.yaml` manifests under `/usr/share/veilbox/`
(shipped by `veilbox-core`, overridable via `VEILBOX_ROOT` per
ADR-0003):

```yaml
name: kubernetes-operations
description: Cluster administration: manifests, Helm, k9s, cluster state
domain: kubernetes
tier: core
provides: [kubernetes-operations]
```

Fields: `name`, `description`, `domain`, `tier` (core / tooling /
expert), and interface tokens for compatibility validation
(ADR-0014). YAML, because humans author and edit it (ADR-0003).

### Experiences reference capabilities, not vice versa

Experience manifests gain a `capabilities:` reference list:

```yaml
name: kubectl
type: tooling
capabilities: [kubernetes-operations]
```

The mapping is N→M, resolved by the capability engine:
`capability selection → derived experience set → tooling engine
(unchanged)`. A capability with no matching experience is
resolvable-but-uninstalled; an experience with no capability is
catalog-only (ADR-0007 discipline: manifest is the contract).

### Selection stores the capability set

The wizard and `Selection` record the selected *capabilities*; the
experience list becomes derived state. Existing saved selections
(pre-ADR-0011) keep their stored experience list, which still maps
cleanly for status/doctor — no data migration of user state is
required, only the ability to express both.

### Onboarding surface changes

The capabilities screen shows capability-level toggles grouped by
domain, with a "show tooling" expansion for tier=tooling entries.
Role recommendation seeding (Day 6) now seeds capabilities; the
derived experience list is presented at review.

## Consequences

- The user's contract with the machine is capability-level and
  stable across implementation churn.
- The experience catalog stays the single source of truth for
  packages (ADR-0003), so DNF reconciliation is unchanged.
- Profiles remain capability-intent documents; no profile grows
  experience lists or environment fields (ADR-0013).
- New surface for validation: the capability engine must reject
  unknown capability references and detect mapping cycles.

## References

- ADR-0001: profile = intent, experience = implementation
- ADR-0003: state and config (YAML authoring, VEILBOX_ROOT)
- ADR-0007: experience manifest discipline
- ADR-0010: composition model (capabilities in the product record)
- ADR-0014: compatibility model (interface tokens)
- docs/day6-onboarding.md: current experience-level selection
