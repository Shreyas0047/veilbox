# Capability Layer (Phase A)

Profiles recommend **capabilities** — intent concepts the engineer
understands ("Networking", "Observability", "Containers") — and the
**Experience Engine** derives the concrete experiences (and from them
the RPMs) that implement the chosen concepts. This is ADR-0011 in
code.

## The model

- `capabilities/` ships capability manifests (one file per capability):
  `name`, `domain`, `tier`, `description`.
- `experiences/` manifests declare which capabilities they implement
  with a `capabilities:` list. A capability may map to several
  experiences; an experience may implement several capabilities.
- `profiles/` recommend (`recommended_capabilities`) and offer
  (`optional_capabilities`) capabilities. The base capability
  (`base-operations`) is implicit: always included, never removable,
  and therefore never listed in a profile manifest.
- The onboarding selection (schema version 2) stores the chosen
  capability axis; the experience list is **derived state**, computed
  through the mapping on every step/apply/verify. v1 selections are
  backfilled from their experience list (inverse mapping) and upgraded
  on save.

## Fresh-run semantics

- A fresh wizard seeds the profile's **recommended** capabilities plus
  the base. Optional capabilities are a menu, never a default — the
  engineer adds them explicitly.
- Changing roles on an existing customization never reseeds the
  recommendations.
- The base capability row is locked (`required`) in both the TUI and
  the line UI; the line UI skips a group whose only capability is
  required.

## Derivation

`capability.Resolver` owns the mapping:

- `ExperiencesFor(caps)` → sorted, deduplicated experience names,
  including planned experiences (their status is reported downstream;
  apply refuses them via validation).
- `CapabilitiesOf(experiences)` → the inverse mapping used for v1
  backfill.
- `CheckMapping()` → doctor's consistency report: capabilities without
  any installable experience (planned) and experience capability
  references unknown to the registry.

## CLI surface

- `veil profile show <name>` / `apply` / `diff` / `sync` — capability
  recommendations resolve to experiences with install status.
- `veil capability list` / `veil capability info <name>` — the concept
  catalog and its implementing experiences.
- `veil experience info <name>` — shows the capabilities an experience
  implements and the profiles that recommend them.
- `veil doctor` — capability catalog checks and profile→capability
  consistency.
- `veil onboard` — role → desktop → **capabilities** → workspace →
  review; the review shows the capability selection and the derived
  experience actions.

## Acceptance demo

```
scripts/smoke-capabilities.sh
```

Resets the machine (removes the experience RPMs, onboarding selection
and workspace include blocks), then drives the exact accepted flow:

Profile SRE → recommended capabilities Networking, Observability,
Containers, Kubernetes → the engineer removes Containers and adds
Security → the Experience Engine derives the required experiences
(`base-ops`, `networking-tools`, `observability-cli`,
`kubernetes-tools`, `security-tools`) → DNF resolves and installs the
RPMs. `containers-tools` and the optional `terminal-ops` are never
installed.
