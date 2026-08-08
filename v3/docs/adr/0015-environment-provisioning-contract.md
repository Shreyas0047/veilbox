# ADR-0015: Environment Provisioning Contract

Status: Accepted
Date: 2026-08-08

## Context

ADR-0012 generalizes the desktop engine but does not say how an
environment's *specifics* are expressed. The current niri path
proves the shared mechanics work (DNF meta-package, session file,
DM enable, boot target — ADR-0007) and also proves the failure mode:
every environment-specific fact is a hardcoded Go site
(`internal/desktop/install.go` first-touch and provisioning paths,
`internal/desktop/remove.go`, the noctalia special-case in
`cmd/veil/main.go`, the "enable SDDM" step in
`cmd/veil/onboard_cmd.go`). Phase C's second minimal environment
must not require touching the engine; therefore the boundary between
"engine mechanics" and "environment specifics" must be a written
contract before any second environment exists.

## Decision

### The contract: mechanics in the engine, specifics as data + hooks

Each environment implements the integration contract. The engine
provides the shared mechanics; the environment provides:

| Concern | Engine (shared) | Environment (contract) |
|---|---|---|
| Package install | DNF meta-package install, idempotent skip (ADR-0007) | Manifest `rpm` name |
| Activation | Session file registration, DM enable, boot target, rollback print | Session file name/path, DM slot component |
| Provisioning | Template rendering, first-touch-wins, user-file preservation (ADR-0008) | Template directory, managed-file list, config destination |
| Validation | Doctor orchestration, compat validation (ADR-0014) | Validation hooks: file expectations, service/session checks |
| Removal | RPM dependency cascade removal, conservative report (ADR-0007) | Managed-file list, preservation note |

Environment specifics are expressed as **manifest data plus
rendered templates** — never as engine forks. The template
directory remains derived from the manifest RPM short name
(ADR-0007), so the catalog stays the single source of truth.

### Hook points are explicit and enumerated

The engine defines a fixed, small hook set (validation, provision,
remove-report). An environment cannot invent hooks; anything an
environment needs outside the set is a contract gap to be closed by
an ADR-adjacent documented extension — not by a `switch` on the
environment name in engine code. The hardcoded sites in the Context
section are converted one by one, each conversion tracked as a
removal of a `switch`-like site, and each is covered by the smokes.

### Bundled reference environment first

The niri + noctalia stack is converted to the reference bundled
environment under the new contract **before** any second
environment is built. Until the reference environment passes the
full Day-5 acceptance through the contract, the contract is
unproven and no second environment is added.

## Consequences

- Adding an environment is a pure additive act: manifest + templates
  + RPM package; engine diff is zero or a contract extension.
- The kill list shrinks monotonically; a `grep` for niri/noctalia
  in `internal/` and `cmd/` is the enforcement test.
- Doctor output and error messages become environment-driven
  (validation hooks), removing role-specific special cases.
- The contract doubles as documentation: a new environment author
  reads the contract, not the engine.
- The second minimal environment (phase C) is the acceptance proof:
  if it cannot be added without an engine change, the contract has a
  gap and that gap is fixed deliberately.

## References

- ADR-0007: desktop experience architecture (mechanics)
- ADR-0008: desktop configuration ownership (first-touch, user files)
- ADR-0012: environment abstraction
- ADR-0014: compatibility model (tokens for validation)
- docs/day5-desktop.md, docs/day6-onboarding.md: current hardcoded
  sites and acceptance procedures
