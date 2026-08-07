package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyCleanMachine(t *testing.T) {
	eng, home, wsDir := setupTest(t, "user")
	writeFile(t, filepath.Join(home, ".bashrc"), "# my dotfile\nalias ll='ls -la'\n")
	eng.LookPath = presentLookPath("vim", "tmux")
	p := prefsWith("vim", "tmux")

	res := mustApply(t, eng, p, "devops", false)
	if len(res.Applied) < 4 {
		t.Fatalf("expected >=4 applied actions, got %+v", res.Applied)
	}
	// Generated files exist.
	if _, err := os.Stat(filepath.Join(wsDir, ShellScriptName)); err != nil {
		t.Fatal("shell.sh missing")
	}
	if _, err := os.Stat(filepath.Join(wsDir, TmuxConfigName)); err != nil {
		t.Fatal("tmux.conf missing")
	}
	// .bashrc preserves user content and gains exactly one block.
	bashrc, _ := os.ReadFile(filepath.Join(home, ".bashrc"))
	content := string(bashrc)
	if !strings.HasPrefix(content, "# my dotfile\nalias ll='ls -la'\n") {
		t.Fatalf("user content not preserved:\n%s", content)
	}
	if strings.Count(content, BlockStart) != 1 {
		t.Fatalf("expected exactly one managed block:\n%s", content)
	}
	if _, err := os.Stat(filepath.Join(home, ".tmux.conf")); err != nil {
		t.Fatal(".tmux.conf missing")
	}
	// State records the profile.
	st, _ := LoadState()
	if st.AppliedProfile != "devops" || st.Generation != 1 {
		t.Fatalf("bad state: %+v", st)
	}
	if len(st.Backups) != 1 || st.Backups[0].OriginalPath != filepath.Join(home, ".bashrc") {
		t.Fatalf("expected one backup of .bashrc: %+v", st.Backups)
	}
	// Backup metadata exists on disk.
	meta := filepath.Join(filepath.Dir(st.Backups[0].BackupPath), "backup.json")
	if _, err := os.Stat(meta); err != nil {
		t.Fatalf("backup metadata missing: %v", err)
	}
}

func TestApplyIdempotent(t *testing.T) {
	eng, home, _ := setupTest(t, "user")
	writeFile(t, filepath.Join(home, ".bashrc"), "user stuff\n")
	eng.LookPath = presentLookPath("vim", "tmux")
	p := prefsWith("vim", "tmux")

	mustApply(t, eng, p, "devops", false)
	bashrcAfterFirst, _ := os.ReadFile(filepath.Join(home, ".bashrc"))

	res := mustApply(t, eng, p, "devops", false)
	if len(res.Applied) != 0 {
		t.Fatalf("second apply must be a no-op, got %+v", res.Applied)
	}
	bashrcAfterSecond, _ := os.ReadFile(filepath.Join(home, ".bashrc"))
	if string(bashrcAfterFirst) != string(bashrcAfterSecond) {
		t.Fatal("second apply changed the file")
	}
	st, _ := LoadState()
	if st.Generation != 1 {
		t.Fatalf("no-op apply must not bump generation: %+v", st)
	}
	if len(st.Backups) != 1 {
		t.Fatalf("backups must stay bounded: %+v", st.Backups)
	}
}

func TestApplyDriftRefusedThenForced(t *testing.T) {
	eng, home, wsDir := setupTest(t, "user")
	writeFile(t, filepath.Join(home, ".bashrc"), "user stuff\n")
	eng.LookPath = presentLookPath("vim", "tmux")
	p := prefsWith("vim", "tmux")
	mustApply(t, eng, p, "devops", false)

	// User edits a Veilbox-generated file.
	writeFile(t, filepath.Join(wsDir, ShellScriptName), "# user hacked this\n")

	if _, err := eng.Apply(p, "devops", false); err == nil {
		t.Fatal("apply without --force must refuse drifted content")
	}
	// Status reports drift.
	rep, err := eng.Status(p, "devops")
	if err != nil {
		t.Fatal(err)
	}
	foundDrift := false
	for _, it := range rep.Items {
		if it.Verdict == VerdictDrifted {
			foundDrift = true
		}
	}
	if !foundDrift {
		t.Fatalf("expected drift in status: %+v", rep.Items)
	}
	if rep.Clean || !rep.Applied {
		t.Fatalf("drifted workspace must not be clean: %+v", rep)
	}

	// --force restores Veilbox-owned content only.
	res := mustApply(t, eng, p, "devops", true)
	if len(res.ResolvedConflicts) != 1 {
		t.Fatalf("expected one resolved conflict: %+v", res)
	}
	shell, _ := os.ReadFile(filepath.Join(wsDir, ShellScriptName))
	if strings.Contains(string(shell), "user hacked") {
		t.Fatalf("generated file not restored:\n%s", shell)
	}
	bashrc, _ := os.ReadFile(filepath.Join(home, ".bashrc"))
	if !strings.Contains(string(bashrc), "user stuff\n") {
		t.Fatal("user content must be untouched by --force")
	}
	rep, _ = eng.Status(p, "devops")
	if !rep.Clean {
		t.Fatalf("workspace must be clean after --force: %+v", rep.Items)
	}
}

func TestApplyDriftedBlockRestored(t *testing.T) {
	eng, home, _ := setupTest(t, "user")
	writeFile(t, filepath.Join(home, ".bashrc"), "user stuff\n")
	eng.LookPath = presentLookPath("vim")
	p := prefsWith("vim")
	mustApply(t, eng, p, "devops", false)

	// User edits inside the managed block.
	bashrc := filepath.Join(home, ".bashrc")
	data, _ := os.ReadFile(bashrc)
	blk, err := FindBlock(string(data))
	if err != nil || blk == nil {
		t.Fatal("expected managed block")
	}
	edited := string(data[:blk.Start]) + strings.Replace(blk.Content, "shell.sh", "shell.sh-evil", 1) + string(data[blk.End:])
	writeFile(t, bashrc, edited)

	if _, err := eng.Apply(p, "devops", false); err == nil {
		t.Fatal("apply must refuse drifted block")
	}
	mustApply(t, eng, p, "devops", true)
	data, _ = os.ReadFile(bashrc)
	if strings.Contains(string(data), "hacked") {
		t.Fatalf("block not restored:\n%s", data)
	}
	if !strings.Contains(string(data), "user stuff\n") {
		t.Fatal("user content lost")
	}
}

func TestApplyBlockInsertedWhenMissing(t *testing.T) {
	eng, home, _ := setupTest(t, "user")
	writeFile(t, filepath.Join(home, ".bashrc"), "user stuff\n")
	eng.LookPath = presentLookPath("vim")
	p := prefsWith("vim")
	mustApply(t, eng, p, "devops", false)

	// User deletes the managed block (but keeps their content).
	data, _ := os.ReadFile(filepath.Join(home, ".bashrc"))
	stripped := strings.ReplaceAll(string(data), BlockStart, "")
	stripped = strings.ReplaceAll(stripped, BlockEnd, "")
	stripped = strings.ReplaceAll(stripped, "  \n", "\n")
	lines := strings.Split(stripped, "\n")
	var kept []string
	for _, l := range lines {
		if !strings.Contains(l, "shell.sh") && strings.TrimSpace(l) != "" {
			kept = append(kept, l)
		}
	}
	writeFile(t, filepath.Join(home, ".bashrc"), strings.Join(kept, "\n")+"\n")

	if _, err := eng.Apply(p, "devops", false); err == nil {
		t.Fatal("removed managed block must be reported as drift/conflict")
	}
	mustApply(t, eng, p, "devops", true)
	data, _ = os.ReadFile(filepath.Join(home, ".bashrc"))
	if strings.Count(string(data), BlockStart) != 1 {
		t.Fatalf("block not reinserted:\n%s", data)
	}
}

func TestApplyProfileSwitch(t *testing.T) {
	eng, home, wsDir := setupTest(t, "user")
	writeFile(t, filepath.Join(home, ".bashrc"), "user stuff\n")
	eng.LookPath = presentLookPath("vim", "tmux")

	devops := prefsWith("vim", "tmux")
	mustApply(t, eng, devops, "devops", false)
	if _, err := os.Stat(filepath.Join(wsDir, TmuxConfigName)); err != nil {
		t.Fatal("tmux.conf should exist under devops")
	}

	sre := Preferences{Shell: ShellBash, Editor: "vim", Prompt: PromptPlain}
	res := mustApply(t, eng, sre, "sre", false)

	// Stale tmux config removed; no stale blocks left behind.
	if _, err := os.Stat(filepath.Join(wsDir, TmuxConfigName)); !os.IsNotExist(err) {
		t.Fatal("tmux.conf must be removed after profile switch")
	}
	if _, err := os.Stat(filepath.Join(home, ".tmux.conf")); !os.IsNotExist(err) {
		t.Fatal(".tmux.conf (Veilbox-created) must be removed after profile switch")
	}
	found := false
	for _, a := range res.Applied {
		if strings.Contains(a.Path, TmuxConfigName) && strings.Contains(a.Path, wsDir) {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected tmux.conf removal in applied actions: %+v", res.Applied)
	}
	st, _ := LoadState()
	if st.AppliedProfile != "sre" {
		t.Fatalf("state must record the new profile: %+v", st)
	}
	// Clean and no stale items.
	rep, _ := eng.Status(sre, "sre")
	if !rep.Clean {
		t.Fatalf("workspace must be clean after switch: %+v", rep.Items)
	}
}

func TestApplyMissingCapabilitySkipped(t *testing.T) {
	eng, home, wsDir := setupTest(t, "user")
	writeFile(t, filepath.Join(home, ".bashrc"), "user stuff\n")
	p := Preferences{Shell: ShellBash, Editor: "vim", Tmux: true, Prompt: PromptPlain}
	// Neither vim nor tmux resolve.

	res := mustApply(t, eng, p, "devops", false)
	if len(res.Capabilities) == 0 {
		t.Fatal("expected capability report")
	}
	if _, err := os.Stat(filepath.Join(wsDir, TmuxConfigName)); !os.IsNotExist(err) {
		t.Fatal("tmux.conf must NOT be created when tmux is missing")
	}
	if _, err := os.Stat(filepath.Join(home, ".tmux.conf")); !os.IsNotExist(err) {
		t.Fatal(".tmux.conf must NOT be created when tmux is missing")
	}
	// shell.sh must not reference the missing editor.
	shell, _ := os.ReadFile(filepath.Join(wsDir, ShellScriptName))
	if strings.Contains(string(shell), "EDITOR") {
		t.Fatalf("no EDITOR export when editor is missing:\n%s", shell)
	}
}

func TestResetPurity(t *testing.T) {
	eng, home, wsDir := setupTest(t, "user")
	original := "# my dotfile\nalias ll='ls -la'\nexport FOO=bar\n"
	writeFile(t, filepath.Join(home, ".bashrc"), original)
	eng.LookPath = presentLookPath("vim", "tmux")
	p := prefsWith("vim", "tmux")
	mustApply(t, eng, p, "devops", false)

	res, err := eng.Reset()
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Removed) == 0 {
		t.Fatalf("expected removals: %+v", res)
	}
	if _, err := os.Stat(filepath.Join(wsDir, ShellScriptName)); !os.IsNotExist(err) {
		t.Fatal("shell.sh must be removed")
	}
	if _, err := os.Stat(filepath.Join(wsDir, TmuxConfigName)); !os.IsNotExist(err) {
		t.Fatal("tmux.conf must be removed")
	}
	bashrc, _ := os.ReadFile(filepath.Join(home, ".bashrc"))
	if string(bashrc) != original {
		t.Fatalf("user .bashrc must be byte-identical after reset:\n%q\nvs\n%q", string(bashrc), original)
	}
	// .tmux.conf was created by Veilbox and must be gone entirely.
	if _, err := os.Stat(filepath.Join(home, ".tmux.conf")); !os.IsNotExist(err) {
		t.Fatal(".tmux.conf must be deleted (Veilbox-created)")
	}
	// Backups preserved as recovery trail.
	st, _ := LoadState()
	if len(st.Backups) != 1 {
		t.Fatalf("backups must be kept: %+v", st.Backups)
	}
	if st.AppliedProfile != "" || len(st.Files) != 0 || len(st.Blocks) != 0 {
		t.Fatalf("state must be cleared: %+v", st)
	}
	// Second reset is a no-op.
	res2, err := eng.Reset()
	if err != nil {
		t.Fatal(err)
	}
	if len(res2.Removed) != 0 {
		t.Fatalf("second reset must remove nothing: %+v", res2)
	}
}

func TestResetDeletesCreatedBashrc(t *testing.T) {
	eng, home, _ := setupTest(t, "user")
	eng.LookPath = presentLookPath("vim")
	mustApply(t, eng, prefsWith("vim"), "devops", false)
	if _, err := os.Stat(filepath.Join(home, ".bashrc")); err != nil {
		t.Fatal(".bashrc should have been created")
	}
	if _, err := eng.Reset(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".bashrc")); !os.IsNotExist(err) {
		t.Fatal("Veilbox-created .bashrc (still fully owned) must be deleted on reset")
	}
}

func TestResetRefusesAmbiguousBlock(t *testing.T) {
	eng, home, _ := setupTest(t, "user")
	blk := blockText()
	writeFile(t, filepath.Join(home, ".bashrc"), "user\n"+blk+"\n"+blk)
	// Fabricate state so reset considers this file managed.
	st, _ := LoadState()
	if st.Blocks == nil {
		st.Blocks = map[string]BlockRecord{}
	}
	st.Blocks[filepath.Join(home, ".bashrc")] = BlockRecord{Created: false, SHA256: "x"}
	if err := SaveState(st); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Reset(); err == nil {
		t.Fatal("reset must refuse multiple managed blocks")
	}
	data, _ := os.ReadFile(filepath.Join(home, ".bashrc"))
	if strings.Count(string(data), BlockStart) != 2 {
		t.Fatal("refused reset must not modify the file")
	}
}

func TestStatusNotApplied(t *testing.T) {
	eng, home, _ := setupTest(t, "user")
	writeFile(t, filepath.Join(home, ".bashrc"), "user stuff\n")
	rep, err := eng.Status(prefsWith("vim"), "devops")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Applied || rep.Clean || rep.Generation != 0 {
		t.Fatalf("unapplied workspace must report so: %+v", rep)
	}
}

func TestStatusCleanAndOutdated(t *testing.T) {
	eng, home, _ := setupTest(t, "user")
	writeFile(t, filepath.Join(home, ".bashrc"), "user stuff\n")
	eng.LookPath = presentLookPath("vim", "tmux")
	mustApply(t, eng, prefsWith("vim", "tmux"), "devops", false)

	rep, _ := eng.Status(prefsWith("vim", "tmux"), "devops")
	if !rep.Clean || !rep.Applied {
		t.Fatalf("expected clean applied status: %+v", rep)
	}
	// Active profile switched to sre (no tmux) without applying yet.
	rep, _ = eng.Status(prefsWith("vim"), "sre")
	if rep.Applied || rep.Clean {
		t.Fatalf("profile switch without apply must not be clean: %+v", rep)
	}
}

func TestStatusBlockConflict(t *testing.T) {
	eng, home, _ := setupTest(t, "user")
	blk := blockText()
	writeFile(t, filepath.Join(home, ".bashrc"), "user\n"+blk+"\n"+blk)
	eng.LookPath = presentLookPath("vim")
	p := prefsWith("vim")
	rep, err := eng.Status(p, "devops")
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range rep.Items {
		if strings.Contains(it.Path, ".bashrc") && it.Verdict != VerdictConflict {
			t.Fatalf("multiple blocks must be a conflict: %+v", it)
		}
	}
}
