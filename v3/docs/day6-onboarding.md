# Day 6: Onboarding — the wizard, `veil onboard`, TUI + line UI, zero-change guarantee

Date: 2026-08-08

## Goal

Ship Veilbox's first-run experience: `veil onboard` guides a new
engineer through role, desktop, capabilities and workspace
preferences on **interactive** TTYs (Bubble Tea TUI) and on
**piped/non-interactive** terminals (line UI), and `veil onboard
--yes` stays fully non-interactive. The machine changes only at the
explicit review confirmation; every abort path leaves the system
byte-for-byte untouched.

## Architecture (summary)

- `cmd/veil/onboard_cmd.go`: entry point; `--yes` applies the
  selection directly, otherwise `isTerminal` dispatches to the Bubble
  Tea UI (TTY) or the line UI (pipe).
- `internal/onboarding/wizard.go`: the wizard. Steps: welcome → role →
  desktop → capabilities → workspace → review. The review screen
  renders the full plan (profile, desktop, experiences, workspace
  prefs) and offers Apply / Restart / Abort; **nothing is applied
  until review confirms** (the engines run only then). Every step and
  the wizard itself can abort with `ErrAborted`.
- `cmd/veil/tui/`: the TUI (Bubble Tea) — `welcomeModel`,
  `pickModel` (role/desktop), `multiModel` (capabilities with header
  skipping, `[x]` toggles, `[r]` recommendations), `wsModel`
  (workspace fields with `(inherit)` empty values), `reviewModel`
  (plan, activation toggle, actions), `reportModel` (apply result).
  Navigation: `↑/↓` or `j/k`, `enter` edit/cycle, `q` quit, `g`/`G`
  top/bottom.
- `internal/onboarding/onboardingtest`: shared test double (`Runner`)
  plus `SetupEnv`, used by the package tests and the e2e tests without
  importing the tui package (avoids the tui→onboarding cycle).
- Selection state lives in `~/.config/veilbox/onboarding.json`; a
  saved selection preloads on later runs ("Applying your saved
  selection"), role preloads its recommended experiences, and the
  wizard shows "(current)"/"already applied" markers from live state.

## Key behaviors

- **TTY → TUI, pipe → line UI.** `newOnboardUI` decides on the input;
  the line UI is the same wizard driven over text prompts (numbered
  choices, defaults on empty lines, `q` aborts anywhere).
- **Zero-change guarantee:** abort at any step or at the review aborts
  before any engine call — verified by snapshot (rpm count + boot
  target) before/after in the smoke.
- **Recommendation seeding:** a fresh role selection seeds the
  capabilities screen with the profile's recommended experiences
  (`[r]` + pre-selected `[x]`); space toggles.
- **Coalesced runes:** bubble tea merges runes that arrive in one
  read, so rapid typing or a held key arrives as a single `KeyRunes`
  message with several runes. The models now apply **every** rune
  (`keyRunes` helper) instead of dropping all but the first — caught
  by the PTY smoke, regression-tested in `tui_test.go`.
- **Report height limit:** the report screen clamps to its viewport,
  so e2e assertions only rely on lines that render without a window
  size.

## Test procedure

### Unit + e2e

- 8+ Go packages green: `go test ./...`, `go vet`, `go build`, gofmt
  clean (vendor excluded).
- TUI model tests: pick/multi/ws/review/report/welcome, header
  skipping, coalesced-rune handling, role selection through a reader.
- Wizard tests on the shared `onboardingtest.Runner`: full run, abort
  everywhere, restart revisits steps (Step 1/5 and 5/5 count == 2),
  apply errors return to the UI.
- e2e (`internal/onboarding/tui_e2e_test.go`): drives the real
  engines over the shared fake with an `os.Pipe` input and a
  wait-for-marker driver: full run, abort at review, restart, apply
  error. Uses a real `*os.File` so the input loop cancels cleanly
  (non-file readers leave stale goroutines that steal released keys).
- Packaging: `scripts/build-sources.sh` + `scripts/build-rpms.sh` →
  `veilbox-core-0.1.0-7.fc44.x86_64`; mock `--buildsrpm` +
  `--rebuild` green; installed on the host with
  `sudo dnf reinstall`; `/usr/bin/veil onboard` verified live.

### Live smoke (`scripts/smoke-day6.sh`, 17/17 on the live machine)

Drives `/usr/bin/veil onboard` exactly as a human would, through a
real PTY (python3 `pty.fork`, no `script(1)` needed):

1. `--yes` on a clean machine: seeds role defaults, renders the apply
   report, persists `~/.config/veilbox/onboarding.json`, reports
   success.
2. TTY run: dispatches to the Bubble Tea TUI (step chrome, footer),
   saved role preloads with the cursor on Cloud Engineer.
3. Full TUI walk to the review (marker-paced keys: each key chunk is
   sent only after its step's marker appears in the output), then
   abort: exit 0, "Aborted. No changes were made.", machine snapshot
   unchanged.
4. Piped run: falls back to the line UI (numbered choices, reaches
   "Apply this plan"), abort leaves the machine unchanged.

The smoke is idempotent (second run exercises the "Applying your
saved selection" branch).

### PTY driver notes (why the smoke works)

- The terminal probe (termenv OSC 11 background + DSR cursor queries,
  sent once per process and per Bubble Tea init) reads responses from
  the input and **eats early keystrokes** on a bare pty. The driver
  answers the queries like a minimal emulator (`\x1b]11;rgb:...\x1b\\`,
  `\x1b[1;1R`).
- Keys must not be sent in the same read that detected a step marker:
  the probe can still be pending and consume them as "responses". A
  short settle after each marker avoids the race.
- Keys destined for the next step must not sit in the tty buffer while
  the current step's reader is torn down (a cancelling reader can
  drain and drop them). Marker-paced sending means a step's keys only
  exist once that step is running.
- The pty needs a real window size (TIOCSWINSZ) so screens and the
  review plan render at full height instead of a 3-line slit.

## Findings

- Bubble Tea coalesces identical runes from one read into a single
  `KeyRunes` message; handlers that only look at `Runes[0]` silently
  lose input. Fixed across all models and covered by a regression
  test (held keys / rapid typing / paste).
- One Bubble Tea program per step, each with its own cancelreader, is
  fine when the driver paces keys by output markers; blind sleep-based
  pacing races the probe and the reader teardown windows.
- `os.Pipe` input gives Linux epoll-based cancel readers whose dead
  read loops die cleanly — the reliable basis for the Go e2e driver.
- termenv probes and reader lifecycle only matter on real PTYs, which
  is exactly why the smoke drives the **installed RPM** rather than
  the Go tests alone.

## Live state after acceptance

`veilbox-core-0.1.0-7.fc44.x86_64` installed; profile `sre` active;
onboarding selection for `cloud-engineer` with base-ops +
networking-tools persisted; `scripts/smoke-day6.sh` 17/17 twice
(idempotent).

## References

- v3/core/cmd/veil/onboard_cmd.go, onboard_cmd_test.go,
  v3/core/cmd/veil/tui/* (models.go, tui.go, tui_test.go)
- v3/core/internal/onboarding/* (wizard.go, apply.go,
  tui_e2e_test.go, onboardingtest/fake.go)
- v3/packages/SPECS/veilbox-core.spec, v3/scripts/build-*.sh
- v3/scripts/smoke-day6.sh
