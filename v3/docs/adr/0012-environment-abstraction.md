# ADR-0012: Environment Abstraction

Status: Accepted
Date: 2026-08-08

## Context

ADR-0007 modeled the desktop as an experience with `type: desktop`
and ADR-0006 pinned SDDM as the display manager. The Day 5/6
implementation goes further than the manifest: the desktop engine
hardcodes the Niri + Noctalia stack in Go code —

- `internal/desktop/install.go` (first-touch path and provisioning:
  niri-specific config files and templates),
- `internal/desktop/remove.go` (niri-specific cleanup),
- `cmd/veil/main.go` (doctor special-cases noctalia),
- `cmd/veil/onboard_cmd.go` (line UI hardcodes the "enable SDDM"
  step).

Niri + Noctalia is an excellent **reference environment** but it is
not "the Veilbox desktop". The product is a composition (ADR-0010)
whose environment axis must be able to carry other environments —
assembled stacks of compatible components and, later, bundled
minimal environments — without forking the engine per environment.
The day-7+ review concluded: `desktop` conflates a component slot
(the compositor desktop) with a whole environment, and the CLI name
`veil desktop` will collide with the workspace concept of a
`desktop_shell`.

## Decision

### Environment is a first-class axis

**Environment** = the complete graphical stack of a composition:
compositor, desktop shell, terminal, display manager, and helpers,
with an activation story (ADR-0007 mechanics: DNF meta-package,
session file, DM enable, boot target). Two kinds:

- **Assembled** — components chosen from compatible manifests
  (ADR-0014), e.g. `niri + noctalia + kitty + sddm`.
- **Bundled** — a fixed component set shipped as one experience
  package, e.g. the minimal environment (ADR-0015's second
  environment phase C proof).

Niri + Noctalia is re-expressed as the **reference bundled
environment**, using the same contract as any other environment.

### Renames with compatibility aliases

| Today | Becomes | Alias for reads |
|---|---|---|
| `type: desktop` (manifest) | `type: environment` | accept `desktop`, write `environment` |
| `veil desktop` (CLI) | `veil environment` | `veil desktop` accepted for one release |
| `Selection.Desktop` (state) | `Selection.Environment` | JSON tag `environment`, accept `desktop` on read |

The loader is the single migration point; manifests and state never
sit on a forked path.

### The Desktop Engine becomes the Environment Engine

The engine keeps the shared mechanics it already has (DNF install,
session file registration, DM enable, boot target, idempotence,
provision/remove conservatism — ADR-0007) and delegates
environment-specific work to a per-environment **integration
contract** (ADR-0015): where rendered config lands, which files are
managed, which validation hooks doctor runs. The kill list from the
Context section above is removed — each hardcoded site becomes a
contract call or manifest data.

### Naming: `desktop_shell` (environment) vs `login_shell` (workspace)

The word "shell" must not be ambiguous: environment components use
`desktop_shell` (the bar/launcher shell, e.g. noctalia); workspace
state keeps `login_shell` (the user's interactive shell). See
ADR-0013 for why the two axes stay independent.

## Consequences

- Adding an environment = adding a manifest (+ templates + RPM), not
  adding Go code to the engine.
- The doctor's desktop checks generalize to environment checks via
  the validation hooks; the noctalia special-case disappears.
- The CLI surface stays stable through the alias window; docs,
  smokes and ADR-0007 wording are updated to "environment".
- The reference environment continues to be the acceptance yardstick
  — everything the engine does is proven against niri first.

## References

- ADR-0007: desktop experience architecture (mechanic to generalize)
- ADR-0006: display manager and explicit activation
- ADR-0010: composition model (environment in the product record)
- ADR-0013: profile/environment independence
- ADR-0014: compatibility model
- ADR-0015: environment provisioning contract
- docs/day5-desktop.md, docs/day6-onboarding.md: current niri-first
  implementation
