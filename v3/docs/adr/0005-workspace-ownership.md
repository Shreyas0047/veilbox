# ADR-0005: Workspace Ownership

Status: Accepted
Date: 2026-08-07

## Context

Day 3 made profiles intent and sync installable. Day 4 introduces the
first real Workspace Engine: it translates `workspace_preferences`
from the active profile into user-level workspace configuration. The
moment Veilbox writes into the user's home directory, the ownership
question becomes existential: who owns what, and what may Veilbox do
with it?

This ADR establishes the boundary:

> **Veilbox owns generated workspace configuration, not the user's
> home directory.**

## Decision

### Ownership boundaries

| Content | Owner | Veilbox behavior |
|---|---|---|
| `~/.config/veilbox/` | Veilbox (user-owned state) | creates, writes, resets |
| `~/.config/veilbox/workspace/*` | Veilbox (generated) | creates, updates, removes, drift-checks |
| `~/.config/veilbox/backups/` | Veilbox (recovery ledger) | append-only |
| `~/.bashrc`, `~/.tmux.conf` | **user** | one marked include block only; never wholesale rewrite; symlinks refused |
| everything else in `$HOME` | user | never touched |

"User-owned" files may carry exactly one Veilbox-managed include
block. Veilbox may create a user file that did not exist (the whole
file is then Veilbox content) — and may delete it again on reset only
while it still contains exclusively Veilbox content.

### Paths

- Generated: `~/.config/veilbox/workspace/shell.sh`,
  `~/.config/veilbox/workspace/tmux.conf`
- State: `~/.config/veilbox/workspace/state.json`
  (schema_version 1; tracks only Veilbox-owned files and managed
  blocks — deliberately **not** a hash database of user files)
- Backups: `~/.config/veilbox/backups/<UTC timestamp>/` with a
  `backup.json` sidecar (original path, created time, reason, sha256)

### Managed block strategy

User files integrate through one clearly marked block:

```
# >>> veilbox managed >>>
[ -f "$HOME/.config/veilbox/workspace/shell.sh" ] && . "$HOME/.config/veilbox/workspace/shell.sh"
# <<< veilbox managed <<<
```

- exactly one block per file; everything outside preserved
  byte-for-byte; idempotent
- insertion appends at end-of-file (newline-normalized)
- update replaces only the block interior
- reset removes the block; Veilbox-created whole files are deleted
  only when they still contain exactly the Veilbox content
- ambiguity is a conflict, never a repair: multiple blocks,
  unterminated markers, or symlinked user files abort plan/apply/
  reset with a clear message

### Backup strategy

- First modification of an existing user-owned file creates a backup
  under the Veilbox-owned backup root with metadata
- The original backup is **never overwritten** on later applies
- No `.bak` files are ever scattered through `$HOME`
- Reset keeps the backup ledger as the recovery trail

### Drift detection

- Every apply records sha256 hashes of generated files and of the
  exact managed-block payload (Veilbox-owned content only)
- `veil workspace status` recomputes and reports
  clean / drifted / missing / conflict / outdated per item
- Veilbox **never silently overwrites** drifted content: `apply`
  refuses; `apply --force` restores drifted Veilbox-managed content
  only. `--force` never permits destructive replacement of a whole
  user-owned file (that remains a structural conflict).

### Plan/apply/reset semantics

- `plan` is a pure read-only computation producing
  CREATE / UPDATE / UNCHANGED / REMOVE / CONFLICT / SKIP per path
- `apply` executes the plan; without `--yes` it asks; conflicts abort
  unless `--force`; no-op applies change nothing
- `reset` removes only Veilbox-managed configuration and refuses
  ambiguous states

### Profile switching

- The workspace state records the profile that generated the current
  configuration
- Applying a new profile updates changed content and REMOVEs stale
  Veilbox-managed files/blocks (e.g. tmux config when tmux is no
  longer preferred); no stale profile-generated configuration remains

### Security model

- Preferences are declarative primitives; validation rejects shell
  metacharacters (aliases/environment values are safe tokens only)
- `prompt` and `terminal` are enums; no manifest field can carry
  arbitrary shell
- The Workspace Engine never executes DNF: missing capabilities are
  reported (SKIP), never installed
- Day 4 scope is user workspace only: no `/etc`, systemd, SELinux,
  firewall, NetworkManager, desktop environments, display managers,
  or kernel parameters

## Limitations

- Only bash integration (shell.sh include) and tmux are generated
  today; `terminal` is validated but informational
- `--force` requires the whole drift to be accepted at once (no
  per-file override yet)
- Backups are not exposed by a restore command yet (manual recovery
  from the ledger)

## References

- ADR-0001: architecture (Workspace Manager layer)
- ADR-0003: configuration/state paths (this ADR extends the layout)
- ADR-0004: profiles are a baseline, not a prison (same philosophy,
  applied to files)
- docs/day4-workspace.md: implementation and test procedure
