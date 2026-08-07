package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupTest builds an isolated HOME/XDG_CONFIG_HOME and returns an
// engine whose LookPath resolves nothing (capability tests opt in
// explicitly).
func setupTest(t *testing.T, homeName string) (*Engine, string, string) {
	t.Helper()
	home := filepath.Join(t.TempDir(), homeName)
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	eng := &Engine{LookPath: func(string) (string, error) { return "", errors.New("not found") }}
	wsDir, err := WorkspaceDir()
	if err != nil {
		t.Fatal(err)
	}
	return eng, home, wsDir
}

// presentLookPath resolves the named binary when allowed.
func presentLookPath(allowed ...string) LookPathFunc {
	return func(name string) (string, error) {
		for _, a := range allowed {
			if name == a {
				return "/usr/bin/" + name, nil
			}
		}
		return "", errors.New("not found")
	}
}

func prefsWith(allowed ...string) Preferences {
	p := Preferences{
		Shell:  ShellBash,
		Editor: "vim",
		Prompt: PromptVeilbox,
	}
	for _, a := range allowed {
		switch a {
		case "vim":
			p.Editor = "vim"
		case "tmux":
			p.Tmux = true
		}
	}
	return p
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustApply(t *testing.T, eng *Engine, p Preferences, profile string, force bool) *ApplyResult {
	t.Helper()
	res, err := eng.Apply(p, profile, force)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	return res
}

func snapshotTree(t *testing.T, roots ...string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, root := range roots {
		filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if !info.IsDir() {
				data, _ := os.ReadFile(path)
				out[path] = string(data)
			}
			return nil
		})
	}
	return out
}

func assertTreeEqual(t *testing.T, before, after map[string]string) {
	t.Helper()
	if len(before) != len(after) {
		t.Fatalf("filesystem changed: before %d files, after %d", len(before), len(after))
	}
	for p, c := range before {
		if after[p] != c {
			t.Fatalf("file %s changed during read-only operation", p)
		}
	}
}

func TestPlanMakesNoFilesystemChanges(t *testing.T) {
	eng, home, wsDir := setupTest(t, "user")
	eng.LookPath = presentLookPath("vim", "tmux")
	writeFile(t, filepath.Join(home, ".bashrc"), "user stuff\n")
	p := prefsWith("vim", "tmux")

	before := snapshotTree(t, wsDir, filepath.Join(home, ".bashrc"))
	st, err := LoadState()
	if err != nil {
		t.Fatal(err)
	}
	plan, err := eng.BuildPlan(p, st)
	if err != nil {
		t.Fatal(err)
	}
	after := snapshotTree(t, wsDir, filepath.Join(home, ".bashrc"))
	assertTreeEqual(t, before, after)

	if plan.IsClean() {
		t.Fatal("clean machine must not plan as clean")
	}
	if len(plan.Entries) < 4 {
		t.Fatalf("expected entries for shell.sh, tmux.conf, .bashrc, .tmux.conf; got %d", len(plan.Entries))
	}
	if plan.Entries[0].Action != ActionCreate || plan.Entries[0].Path != filepath.Join(wsDir, ShellScriptName) {
		t.Fatalf("first entry wrong: %+v", plan.Entries[0])
	}
}

func TestPlanActionsAfterApply(t *testing.T) {
	eng, home, wsDir := setupTest(t, "user")
	eng.LookPath = presentLookPath("vim", "tmux")
	writeFile(t, filepath.Join(home, ".bashrc"), "user stuff\n")
	p := prefsWith("vim", "tmux")
	mustApply(t, eng, p, "devops", false)

	st, _ := LoadState()
	plan, err := eng.BuildPlan(p, st)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.IsClean() {
		for _, e := range plan.Entries {
			if e.Action != ActionUnchanged {
				t.Errorf("expected UNCHANGED, got %s %s (%s)", e.Action, e.Path, e.Detail)
			}
		}
	}
	_ = wsDir
}

func TestPlanPrefChangeUpdates(t *testing.T) {
	eng, home, wsDir := setupTest(t, "user")
	eng.LookPath = presentLookPath("vim", "tmux")
	writeFile(t, filepath.Join(home, ".bashrc"), "user stuff\n")
	mustApply(t, eng, prefsWith("vim", "tmux"), "devops", false)

	// Different prompt + tmux off.
	changed := Preferences{Shell: ShellBash, Editor: "vim", Prompt: PromptPlain}
	st, _ := LoadState()
	plan, err := eng.BuildPlan(changed, st)
	if err != nil {
		t.Fatal(err)
	}
	actions := map[string]Action{}
	for _, e := range plan.Entries {
		actions[e.Path] = e.Action
	}
	if actions[filepath.Join(wsDir, ShellScriptName)] != ActionUpdate {
		t.Errorf("shell.sh should UPDATE: %v", actions)
	}
	if actions[filepath.Join(wsDir, TmuxConfigName)] != ActionRemove {
		t.Errorf("tmux.conf should REMOVE (stale): %v", actions)
	}
	if actions[filepath.Join(home, ".tmux.conf")] != ActionRemove {
		t.Errorf(".tmux.conf should REMOVE (created by Veilbox): %v", actions)
	}
	if actions[filepath.Join(home, ".bashrc")] != ActionUnchanged {
		t.Errorf(".bashrc block should stay UNCHANGED (payload unchanged): %v", actions)
	}
}

func TestPlanReportsDriftConflict(t *testing.T) {
	eng, home, wsDir := setupTest(t, "user")
	writeFile(t, filepath.Join(home, ".bashrc"), "user stuff\n")
	p := prefsWith("vim", "tmux")
	mustApply(t, eng, p, "devops", false)

	writeFile(t, filepath.Join(wsDir, ShellScriptName), "# tampered by user\n")

	st, _ := LoadState()
	plan, err := eng.BuildPlan(p, st)
	if err != nil {
		t.Fatal(err)
	}
	cf := plan.Conflicts()
	if len(cf) != 1 || cf[0].Action != ActionConflict || !cf[0].Drift {
		t.Fatalf("expected one drift conflict, got %+v", cf)
	}
}

func TestPlanConflictMultipleBlocks(t *testing.T) {
	eng, home, _ := setupTest(t, "user")
	blk := blockText()
	writeFile(t, filepath.Join(home, ".bashrc"), blk+"\n"+blk)

	st, _ := LoadState()
	plan, err := eng.BuildPlan(prefsWith("vim"), st)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range plan.Entries {
		if e.Action == ActionConflict && e.Fatal && strings.Contains(e.Path, ".bashrc") {
			return
		}
	}
	t.Fatalf("expected fatal conflict for .bashrc, got %+v", plan.Entries)
}

func TestPlanConflictSymlinkedBashrc(t *testing.T) {
	eng, home, _ := setupTest(t, "user")
	target := filepath.Join(home, "real-bashrc")
	writeFile(t, target, "user stuff\n")
	if err := os.Symlink(target, filepath.Join(home, ".bashrc")); err != nil {
		t.Fatal(err)
	}
	st, _ := LoadState()
	plan, err := eng.BuildPlan(prefsWith("vim"), st)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range plan.Entries {
		if e.Action == ActionConflict && e.Fatal {
			return
		}
	}
	t.Fatalf("expected fatal conflict for symlinked .bashrc, got %+v", plan.Entries)
}

func TestPlanSkipsMissingCapability(t *testing.T) {
	eng, home, wsDir := setupTest(t, "user")
	writeFile(t, filepath.Join(home, ".bashrc"), "user stuff\n")
	// tmux and vim both missing (LookPath resolves nothing).
	p := Preferences{Shell: ShellBash, Editor: "vim", Tmux: true, Prompt: PromptPlain}
	st, _ := LoadState()
	plan, err := eng.BuildPlan(p, st)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Capabilities) == 0 {
		t.Fatal("expected capability report")
	}
	for _, e := range plan.Entries {
		if strings.Contains(e.Path, TmuxConfigName) && e.Action == ActionCreate {
			t.Fatalf("tmux.conf must not be planned when tmux is missing: %+v", e)
		}
		if strings.Contains(e.Path, ".tmux.conf") && e.Action == ActionCreate {
			t.Fatalf(".tmux.conf must not be planned when tmux is missing: %+v", e)
		}
	}
	// shell.sh and .bashrc are still planned: shell integration does
	// not depend on the missing binaries.
	createdShell := false
	for _, e := range plan.Entries {
		if strings.Contains(e.Path, ShellScriptName) && e.Action == ActionCreate {
			createdShell = true
		}
	}
	if !createdShell {
		t.Fatalf("shell.sh should still be planned: %+v", plan.Entries)
	}
	_ = wsDir
}

func TestPlanCapabilityPresent(t *testing.T) {
	eng, home, wsDir := setupTest(t, "user")
	eng.LookPath = presentLookPath("vim", "tmux")
	writeFile(t, filepath.Join(home, ".bashrc"), "user stuff\n")
	p := Preferences{Shell: ShellBash, Editor: "vim", Tmux: true, Prompt: PromptPlain}
	st, _ := LoadState()
	plan, err := eng.BuildPlan(p, st)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Capabilities) != 0 {
		t.Fatalf("no capability issues expected: %v", plan.Capabilities)
	}
	foundTmux := false
	for _, e := range plan.Entries {
		if strings.Contains(e.Path, TmuxConfigName) && e.Action == ActionCreate {
			foundTmux = true
		}
	}
	if !foundTmux {
		t.Fatalf("expected tmux.conf CREATE: %+v", plan.Entries)
	}
	_ = wsDir
}
