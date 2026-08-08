package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Shreyas0047/veilbox/v3/core/cmd/veil/tui"
)

const cliProfileYAML = `name: cloud-engineer
display_name: Cloud Engineer
description: Works with cloud infrastructure.
role: cloud-engineer
recommended_experiences:
  - networking-tools
optional_experiences: []
workspace_preferences:
  shell: bash
  editor: vim
  terminal: system
  prompt: veilbox
  tmux: false
`

// setupOnboardCLI wires the full onboarding environment: catalog,
// profiles, HOME, session dir, PATH and a fake runner.
func setupOnboardCLI(t *testing.T) (deps, *fakeCLIRunner, string) {
	t.Helper()
	d, f := setupEnvironmentCLI(t)
	profDir := filepath.Join(os.Getenv("VEILBOX_ROOT"), "profiles")
	if err := os.MkdirAll(profDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profDir, "cloud-engineer.yaml"), []byte(cliProfileYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	stateDir := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "veilbox")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return d, f, stateDir
}

// TestOnboardYesFreshDefaults tests 'veil onboard --yes' on a fresh
// machine: it defaults to the first profile, applies its recommended
// experiences and persists the selection ledger.
func TestOnboardYesFreshDefaults(t *testing.T) {
	d, f, stateDir := setupOnboardCLI(t)
	// Everything already installed: the fake runner is not stateful,
	// so pre-marking keeps the apply honest without DNF semantics.
	f.installed("veilbox-experience-networking-tools")
	f.responses[f.key("rpm", "-qa", "--queryformat", "%{NAME}\n")] = "veilbox-experience-networking-tools\n"

	r := runCLI(t, d, "onboard", "--yes")
	if r.code != 0 {
		t.Fatalf("code=%d stderr=%q", r.code, r.stderr)
	}
	for _, want := range []string{
		"First run: defaulting to profile",
		"cloud-engineer",
		"APPLY RESULT",
		"Success: selection applied and verified.",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Fatalf("missing %q in:\n%s", want, r.stdout)
		}
	}
	// The selection and its ledger must be persisted.
	data, err := os.ReadFile(filepath.Join(stateDir, "onboarding.json"))
	if err != nil {
		t.Fatalf("selection not saved: %v", err)
	}
	if !strings.Contains(string(data), "last_apply") || !strings.Contains(string(data), `"success": true`) {
		t.Fatalf("ledger missing from selection:\n%s", data)
	}
}

// TestOnboardYesAppliesSavedSelection tests that --yes applies exactly
// the saved selection without re-seeding.
func TestOnboardYesAppliesSavedSelection(t *testing.T) {
	d, f, stateDir := setupOnboardCLI(t)
	f.installed("veilbox-experience-networking-tools")
	f.responses[f.key("rpm", "-qa", "--queryformat", "%{NAME}\n")] = "veilbox-experience-networking-tools\n"

	sel := `{"schema_version":1,"profile":"cloud-engineer","experiences":["networking-tools"],"workspace":{"editor":"vim","prompt":"veilbox","terminal":"system"}}`
	if err := os.WriteFile(filepath.Join(stateDir, "onboarding.json"), []byte(sel), 0o644); err != nil {
		t.Fatal(err)
	}

	r := runCLI(t, d, "onboard", "--yes")
	if r.code != 0 {
		t.Fatalf("code=%d stderr=%q", r.code, r.stderr)
	}
	if !strings.Contains(r.stdout, "Applying your saved selection.") {
		t.Fatalf("expected saved-selection path:\n%s", r.stdout)
	}
}

// TestOnboardHelp tests the usage entry.
func TestOnboardHelp(t *testing.T) {
	d, _, _ := setupOnboardCLI(t)
	r := runCLI(t, d, "help")
	if r.code != 0 {
		t.Fatalf("code=%d", r.code)
	}
	if !strings.Contains(r.stdout, "onboard [--yes]") {
		t.Fatalf("onboard missing from help:\n%s", r.stdout)
	}
}

// TestNewOnboardUIDispatch verifies the TTY detection dispatch: a
// terminal picks the Bubble Tea TUI, anything else the line UI.
func TestNewOnboardUIDispatch(t *testing.T) {
	if _, ok := newOnboardUI(os.Stdin, &strings.Builder{}, true).(*tui.UI); !ok {
		t.Fatal("TTY must dispatch to the Bubble Tea TUI")
	}
	if _, ok := newOnboardUI(os.Stdin, &strings.Builder{}, false).(*lineUI); !ok {
		t.Fatal("non-TTY must dispatch to the line UI")
	}
}
