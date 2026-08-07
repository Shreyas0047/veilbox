# Day 4: Workspace Engine — preferences become safe user configuration

Date: 2026-08-07

## Goal

Translate `workspace_preferences` from the active profile into
user-level workspace configuration, under an explicit ownership model:
**Veilbox owns what Veilbox generates; it never destroys what the
user owns** (ADR-0005). Not a dotfiles manager.

## Schema (structured, declarative)

Replaces the day-3 flat `map[string]string`:

```yaml
workspace_preferences:
  shell: bash          # enum; only bash today (others rejected)
  editor: vim          # simple command name; sets EDITOR; binary must exist
  terminal: system     # enum: system|kitty|wezterm|ghostty|alacritty; validated, informational
  prompt: plain        # enum: plain|veilbox; deterministic PS1 or none
  tmux: false          # bool; requires the tmux binary (terminal-ops)
  aliases:             # name -> simple command+args, safe tokens only
    k: kubectl
  environment:         # name -> value, safe tokens only
    KUBECONFIG: ~/.kube/config
```

Validation rejects every shell metacharacter (`; | & $ \` ( ) < > ' "
* ? [ ] { }`): aliases/env values must match
`^[A-Za-z0-9_./:+\-=~]+( [A-Za-z0-9_./:+\-=~]+)*$`. No manifest field
can become an arbitrary shell command.

### Shipped preferences

| Profile | shell | editor | prompt | tmux | aliases | environment |
|---|---|---|---|---|---|---|
| devops | bash | vim | veilbox | true | k: kubectl, tf: terraform | KUBECONFIG |
| sre | bash | vim | plain | false | ll: ls -la | — |
| platform-engineer | bash | vim | veilbox | true | kg: kubectl get | — |
| cloud-engineer | bash | vim | veilbox | false | k: kubectl | AWS_REGION |

## Ownership (summary; full detail in ADR-0005)

- Generated files: `~/.config/veilbox/workspace/{shell.sh,tmux.conf}`
- State: `~/.config/veilbox/workspace/state.json` (schema v1) — tracks
  hashes of Veilbox-owned files and managed-block payloads only
- Backups: `~/.config/veilbox/backups/<ts>/` + `backup.json`
  (original_path, created_at, reason, sha256); first-touch only,
  never overwritten
- `~/.bashrc` / `~/.tmux.conf`: user-owned; at most one managed block
  (`# >>> veilbox managed >>>` … `# <<< veilbox managed <<<`),
  everything else preserved byte-for-byte. Symlinks and ambiguous
  blocks → CONFLICT, never repaired destructively.

## Engine architecture

```
Profile Engine (intent) → Workspace Engine → user files
                               |
                 plan (pure) → apply / reset / status
                               |
                 capability checks (exec.LookPath only)
```

- `workspace.BuildPlan(prefs, state)` — pure read-only; actions
  CREATE / UPDATE / UNCHANGED / REMOVE / CONFLICT / SKIP,
  deterministic; zero filesystem writes (verified by test)
- `workspace.Apply(prefs, profileName, force)` — executes the plan:
  backups first, then writes (atomic tmp+rename), block ops, state
  save; refuses conflicts without `--force`; no-op applies change
  nothing; `--force` restores drifted Veilbox-managed content only
- `workspace.Reset()` — removes only managed content; refuses
  ambiguity; keeps backups; clears state
- `workspace.Status(prefs, activeProfile)` — per-item verdicts:
  clean / drifted / missing / conflict / outdated; reports unmet
  capabilities
- **No DNF in the Workspace Engine.** Missing editor/tmux → SKIP +
  report ("recommend the terminal-ops experience"), never install.

## Commands

- `veil workspace` — overview (profile, prefs, state health)
- `veil workspace plan` — dry run; conflicts listed; no changes
- `veil workspace apply [--yes] [--force]`
- `veil workspace status` — per-file health + applied profile/generation
- `veil workspace reset [--yes]` — managed config only

## Drift workflow (verified live)

apply → record hash → user edits generated file → `status` = DRIFTED →
`apply` refuses (conflict) → `apply --force` restores only
Veilbox-managed content → `status` = clean.

## Verification

- Unit tests (temp HOME/XDG_CONFIG_HOME only): prefs parsing and
  injection rejection; plan-no-writes; block insert/update/remove and
  byte-preservation; duplicate-block and symlink conflicts; backup
  once-only; drift detect/refuse/force; profile-switch cleanup;
  capability SKIP; reset purity (original `.bashrc` byte-identical);
  CLI tests for plan/apply/status/reset/profile-switch.
- `go test ./...` (6 packages), `go vet`, `go build` — clean.
- rpmlint: 0 errors / 0 warnings (binary).
- mock fedora-44-x86_64: SRPM build + chroot rebuild including the
  full Go test suite — clean.
- `scripts/smoke-day4.sh`: **43/43** on the live machine — plan
  zero-writes, apply, idempotency, drift refuse + `--force` recovery,
  devops→sre switch with stale tmux cleanup, reset purity
  (byte-identical original `.bashrc`), backups preserved, reapply.
- `veil doctor`: 15 checks, all OK (3 new workspace checks).

## Live acceptance (23 steps)

Snapshot original config → plan (zero writes) → apply --yes → verify
generated files → `.bashrc` preserved → apply again (idempotent) →
status clean → manual drift → status drifted → no silent overwrite →
`--force` reconcile → switch profile → plan reflects new profile →
apply → stale tmux config removed → reset → only managed config gone,
original intact → reapply → **reboot (manual, next session)** →
state persists, status clean → doctor.

## Limitations

- bash-only shell integration; `terminal` informational
- `--force` is all-or-nothing (no per-file override)
- No restore command for backups (manual recovery)
- No zsh/fish, no editor config generation, no dotfile import

## References

- ADR-0005 (ownership), ADR-0001/0003 (architecture, paths),
  ADR-0004 (baseline philosophy)
- v3/core/internal/workspace/{prefs,generate,block,state,backup,plan,apply,reset,status}.go
- v3/core/cmd/veil/workspace_cmd.go
