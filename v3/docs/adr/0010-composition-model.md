# ADR-0010: Composition Model

Status: Accepted
Date: 2026-08-08

## Context

Day 6 shipped the onboarding wizard, but it left state ambiguous.
`~/.config/veilbox/onboarding.json` holds the last wizard
`Selection{SchemaVersion, Profile, Desktop, Experiences, Workspace,
LastApply}` — a mix of *session* state (what the wizard last
proposed) and *applied* state (what the machine converged to). There
is no single record of the complete applied product, and no place
for the capabilities and environment layers ADR-0011 and ADR-0012
introduce.

One file cannot serve both roles honestly: the wizard mutates
selection freely on every run, while applied state must be stable,
diffable, and attributable to a point in time. Mixing them makes
`veil status` guess what "the product" is.

## Decision

### Composition is the product record

**Composition** is the engineer's complete, applied Veilbox product:

- Profile (the role; ADR-0001)
- Capabilities (the selected capability set; ADR-0011)
- Environment (the assembled or bundled desktop environment;
  ADR-0012) and its concrete components
- Experiences (the tooling capability implementations)
- Workspace (login shell, editor, terminal, prompt, tmux, aliases,
  environment)
- Defaults and compatibility validation (ADR-0014)

### Two files, two owners

| File | Role | Written by | Ownership |
|------|------|-----------|-----------|
| `~/.config/veilbox/onboarding.json` | Wizard session state | wizard steps | Volatile; safe to delete |
| `~/.config/veilbox/composition.json` | Applied product record | `veil onboard --yes` / apply path only | Stable; `veil status` source of truth |

- `composition.json` is **recreated, not edited**: every apply writes
  a fresh record from the approved selection. It is versioned
  (`SchemaVersion`), diffable, and records `AppliedAt` plus the
  compatibility validation result.
- `onboarding.json` continues to preload later wizard runs
  (Day 6 behavior) but is never consulted by `veil status`.
- `veil status`, `veil list` and `veil doctor` consume
  `composition.json` plus the RPM database; they never guess.

### Composition is produced, not pre-authored

Composition is the *output* of applying a selection through the
engines — never a file a user hand-writes and never an input to the
wizard. It exists so the machine can answer "what did we agree to
and is it still true?" against live DNF state (ADR-0003: DNF/RPM is
the source of truth for packages; composition records intent, DNF
records implementation).

## Consequences

- Applying a new selection replaces `composition.json` atomically
  (write-temp-rename); an aborted apply leaves the previous record
  untouched, preserving the zero-change guarantee.
- `onboarding.json` can grow wizard-only fields freely without
  polluting the applied record.
- Migration: a pre-composition install (Day 6) synthesizes its
  first `composition.json` from the existing `Selection` on the next
  apply — no hand-migration.
- The compatibility validator (ADR-0014) re-validates the recorded
  composition on every apply, so drift is detected, never papered
  over.

## References

- ADR-0003: state and config (JSON state, DNF as package truth)
- ADR-0001: architecture (profiles = intent, experiences =
  implementation)
- ADR-0011: capabilities
- ADR-0012: environment abstraction
- ADR-0014: compositor/shell compatibility model
- docs/day6-onboarding.md: wizard session state today
