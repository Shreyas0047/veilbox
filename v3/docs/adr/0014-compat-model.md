# ADR-0014: Compositor/Shell Compatibility Model

Status: Accepted
Date: 2026-08-08

## Context

ADR-0012 makes environments assembled from components (compositor,
desktop shell, terminal, display manager, helpers) and ADR-0013
guarantees role/environment independence. Assembly implies
compatibility: not every compositor works with every shell or
display manager, and some combinations are actively hostile (two
compositors, a shell whose dependency chain conflicts with the
compositor's). Today the engine never reasons about this — the
single hardcoded niri stack makes the question moot. The moment a
second environment exists (phase C), silent incompatibility becomes
the normal bug.

## Decision

### Interface tokens

Components declare machine-readable interface tokens:

```yaml
provides: [wayland-compositor, scrollable-workspaces]
requires: [wayland, udev]
conflicts: [shell-integrated-compositor]
```

- `provides` — interfaces this component implements (one token per
  slot the component can fill).
- `requires` — interfaces/features the component needs to be
  present in the environment.
- `conflicts` — interfaces whose presence invalidates the
  component.

Tokens live on the component definitions inside environment
manifests (assembled) and on bundled environment manifests
(bundled). The registry of known tokens is documented data in the
engine (token → slot mapping for validation messages), not a schema
of arbitrary strings.

### Validation runs twice

1. **Selection time** — at the Environment detail step, every slot
   swap is validated against the current assembled set; incompatible
   swaps are refused with actionable errors ("`<shell>` requires
   `<token>` which `<compositor>` conflicts with; choose one of:
   …").
2. **Apply time** — the stored composition (ADR-0010) is
   re-validated before any engine call. Drift (a swapped package, an
   updated manifest) is reported and blocks apply, preserving the
   zero-change guarantee: nothing changes unless the composition
   validates.

### Slot completeness

Validation also enforces slot completeness per environment kind:
an assembled environment must name a compositor; a bundled
environment must declare its component set explicitly. A missing
`desktop_shell` is valid only for minimal environments (proved by
ADR-0015's second environment), and that state must be declared, not
accidental.

## Consequences

- The reference environment validates trivially (its tokens are the
  seed of the registry) — no behavior change for niri today.
- Assembly becomes a safe user action instead of a footgun; the
  engine can enumerate alternatives for a failed swap.
- Validation failures are first-class output (composition record,
  doctor), so support starts from a machine-readable reason, not a
  screenshot.
- The token registry is small and curated; adding a token is an
  ADR-adjacent data change (documented in the engine), not a
  language change.

## References

- ADR-0010: composition model (validation result recorded)
- ADR-0011: capability manifests (tokens on capabilities)
- ADR-0012: environment abstraction (assembled vs bundled)
- ADR-0013: profile/environment independence
- ADR-0015: environment provisioning contract
