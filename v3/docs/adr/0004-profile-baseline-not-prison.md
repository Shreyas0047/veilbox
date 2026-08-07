# ADR-0004: Profiles are a desired baseline, not an enforced prison

Status: Accepted
Date: 2026-08-07

## Context

Day 2 established that a profile is intent, not a package list. Day 3
makes profiles real intent objects: they recommend experiences, and
`veil profile sync` can install what the active profile wants. The
risk is that "sync" reads as "make the machine exactly match this
profile" — a prescription that removes what the engineer installed
themselves. An operations platform that fights its engineer is worse
than no platform at all.

## Decision

A profile is the **desired baseline** for a role: the minimum
capability set that role should have. It is never the full,
authoritative description of what a machine may contain.

Consequences encoded in behavior:

1. `veil profile sync` installs **missing recommended experiences
   only**. It never removes installed experiences — neither the
   profile's own optional experiences nor experiences unrelated to the
   profile (extras). Removing anything is always an explicit user
   action (`veil experience remove`), and even that is a plain DNF
   transaction.
2. Sync state (`veil status`) reports whether the baseline is met.
   Extra experiences do not make a profile "not synced".
3. `veil profile diff` surfaces extras with the label
   "Extra experiences (kept, not in profile)" — information, not
   instruction.
4. Profiles only recommend. Optional experiences are never installed
   by sync; the engineer decides.
5. Future commands that do offer removal (e.g. planned
   "uninstall" flows) must ask explicitly and never run as part of
   sync.

Rationale:

- Engineers accumulate tools that fit their actual work, not their
  role label. The machine's true state is the union of intent and
  individual choices.
- "The user remains in control" is a product principle: Veilbox helps
  the engineer, it does not fight them.
- Removing packages is destructive; the default must be to not do it.

## Non-goals

- No "profile parity" or drift-toward-template behavior.
- No automatic downgrades or removals driven by profile changes.

## Consequences

- Sync is monotonic with respect to installed experiences: it only
  ever adds.
- Extras are tracked and shown but never acted on.
- The behavior is testable: the smoke test verifies a manually
  installed experience survives `profile sync`, and that diff reports
  it as an extra while status stays "synced".

## References

- ADR-0001: architecture (profiles = intent, experiences = capability)
- docs/day3-intent-engine.md: implementation and test procedure
