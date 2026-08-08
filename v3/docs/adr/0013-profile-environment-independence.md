# ADR-0013: Profile/Environment Independence

Status: Accepted
Date: 2026-08-08

## Context

ADR-0001 defines a profile as an engineering role with selected
capabilities, and the review verified that profiles carry **no**
environment fields today. The product vision is a matrix: any role
may run any environment, and role selection must never coerce an
environment choice. The temptation to couple them is real — a
"cloud engineer" profile "obviously" wants the full desktop — but
the wizard's job is to compose intent, not to lock roles to stacks.

Without independence, two failure modes appear: profiles silently
grow environment opinions (breaking the role→capability contract),
and environment slot decisions (compositor, shell, DM) get made by
capability mapping instead of by the user at the environment step.

## Decision

### Profile and environment are orthogonal axes

- A profile manifest declares **only** intent: name, description,
  role domain, and recommended capabilities (ADR-0011). It never
  declares an environment, a compositor, a shell, or a display
  manager — by schema, not by convention. The schema validation
  rejects environment fields in profile manifests.
- Environment choice belongs to the composition step (ADR-0010),
  made by the user at the wizard's Environment step, pre-seeded with
  the reference environment but never forced by the role.
- Capability manifests carry interface tokens (ADR-0011, ADR-0014)
  but no environment affinity. If a capability is environment-aware,
  it expresses that as `requires` tokens, which the validator checks
  against the chosen environment — it never changes the environment
  choice.

### Naming to keep the axes honest

- `desktop_shell` = environment component slot (ADR-0012).
- `login_shell` = workspace preference (user's interactive shell).

The two words never mix in manifests or state, so a reader cannot
conflate "the shell in the desktop" with "the shell I log in to".

## Consequences

- The role→environment matrix is fully general: any role × any
  compatible environment is valid and expressible.
- Wizard flow stays linear and honest: Role → Capabilities →
  Environment → Environment detail → Workspace → Review → Apply.
- Profile schema validation gains a new rejection rule (environment
  fields) — cheap, and it keeps future profiles on the contract.
- Compatibility failures surface at the environment step with
  actionable errors (ADR-0014), not as hidden role restrictions.

## References

- ADR-0001: profiles = intent
- ADR-0010: composition model
- ADR-0011: capability manifests
- ADR-0012: environment abstraction (slot naming)
- ADR-0014: compatibility model
