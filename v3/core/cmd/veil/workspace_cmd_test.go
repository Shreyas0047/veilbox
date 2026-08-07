package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupWorkspaceCLI prepares VEILBOX_ROOT (profiles), XDG_CONFIG_HOME
// (state), and HOME (user files) for workspace CLI tests.
func setupWorkspaceCLI(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("VEILBOX_ROOT", root)
	home := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	writeProfile(t, root, "devops", cliDevopsYAML)
	writeProfile(t, root, "sre", `name: sre
description: Keeps systems reliable.
workspace_preferences:
  shell: bash
  prompt: plain
  aliases:
    ll: ls -la
`)
	if err := os.WriteFile(filepath.Join(home, ".bashrc"), []byte("my existing config\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestWorkspaceRequiresProfile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	r := runCLI(t, deps{}, "workspace")
	if r.code != 1 || !strings.Contains(r.stderr, "no active profile") {
		t.Fatalf("code=%d out=%q err=%q", r.code, r.stdout, r.stderr)
	}
	r = runCLI(t, deps{}, "workspace", "plan")
	if r.code != 1 {
		t.Fatalf("plan without profile: code=%d", r.code)
	}
}

func TestWorkspaceApplyIdempotentStatusReset(t *testing.T) {
	home := setupWorkspaceCLI(t)

	r := runCLI(t, deps{}, "profile", "apply", "devops")
	if r.code != 0 {
		t.Fatalf("apply profile: code=%d err=%q", r.code, r.stderr)
	}
	r = runCLI(t, deps{}, "workspace", "plan")
	if r.code != 0 {
		t.Fatalf("plan: code=%d err=%q", r.code, r.stderr)
	}
	if !strings.Contains(r.stdout, "CREATE") {
		t.Fatalf("plan should show CREATE actions:\n%s", r.stdout)
	}

	r = runCLI(t, deps{}, "workspace", "apply", "--yes")
	if r.code != 0 {
		t.Fatalf("apply: code=%d err=%q", r.code, r.stderr)
	}
	wsDir, _ := workspaceDirForTest()
	shell := filepath.Join(wsDir, "shell.sh")
	if _, err := os.Stat(shell); err != nil {
		t.Fatal("shell.sh not created")
	}
	bashrc, _ := os.ReadFile(filepath.Join(home, ".bashrc"))
	if !strings.HasPrefix(string(bashrc), "my existing config\n") {
		t.Fatalf("user config not preserved:\n%s", bashrc)
	}
	if strings.Count(string(bashrc), "# >>> veilbox managed >>>") != 1 {
		t.Fatalf("expected one managed block:\n%s", bashrc)
	}

	// Idempotent second apply.
	r = runCLI(t, deps{}, "workspace", "apply", "--yes")
	if r.code != 0 {
		t.Fatalf("second apply: code=%d err=%q", r.code, r.stderr)
	}
	if !strings.Contains(r.stdout, "already up to date") {
		t.Fatalf("second apply should be a no-op:\n%s", r.stdout)
	}

	// Status clean.
	r = runCLI(t, deps{}, "workspace", "status")
	if r.code != 0 || !strings.Contains(r.stdout, "clean") {
		t.Fatalf("status: code=%d\n%s", r.code, r.stdout)
	}

	// Drift: tamper with a generated file, apply must refuse.
	if err := os.WriteFile(shell, []byte("# hacked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r = runCLI(t, deps{}, "workspace", "apply", "--yes")
	if r.code != 1 || !strings.Contains(r.stdout, "conflict") {
		t.Fatalf("apply on drift must refuse: code=%d out=%q", r.code, r.stdout)
	}
	r = runCLI(t, deps{}, "workspace", "status")
	if !strings.Contains(r.stdout, "drifted") {
		t.Fatalf("status should report drift:\n%s", r.stdout)
	}
	// --force restores Veilbox-managed content.
	r = runCLI(t, deps{}, "workspace", "apply", "--yes", "--force")
	if r.code != 0 {
		t.Fatalf("apply --force: code=%d err=%q", r.code, r.stderr)
	}
	r = runCLI(t, deps{}, "workspace", "status")
	if !strings.Contains(r.stdout, "clean") {
		t.Fatalf("status should be clean after --force:\n%s", r.stdout)
	}

	// Reset removes only Veilbox-managed content.
	r = runCLI(t, deps{}, "workspace", "reset", "--yes")
	if r.code != 0 {
		t.Fatalf("reset: code=%d err=%q", r.code, r.stderr)
	}
	if _, err := os.Stat(shell); !os.IsNotExist(err) {
		t.Fatal("shell.sh must be gone after reset")
	}
	bashrc, _ = os.ReadFile(filepath.Join(home, ".bashrc"))
	if string(bashrc) != "my existing config\n" {
		t.Fatalf("user config must be byte-identical after reset: %q", bashrc)
	}
	// Backups survive reset.
	backups, _ := filepath.Glob(filepath.Join(home, ".config", "veilbox", "backups", "*", "*.bak"))
	if len(backups) != 1 {
		t.Fatalf("expected one backup, got %v", backups)
	}
}

func TestWorkspaceProfileSwitch(t *testing.T) {
	home := setupWorkspaceCLI(t)
	runCLI(t, deps{}, "profile", "apply", "devops")
	if r := runCLI(t, deps{}, "workspace", "apply", "--yes"); r.code != 0 {
		t.Fatalf("apply devops: %q", r.stderr)
	}
	wsDir, _ := workspaceDirForTest()
	// Switch to sre: prompt changes from veilbox to plain.
	runCLI(t, deps{}, "profile", "apply", "sre")
	r := runCLI(t, deps{}, "workspace", "plan")
	if !strings.Contains(r.stdout, "UPDATE") {
		t.Fatalf("plan after switch should show updates:\n%s", r.stdout)
	}
	r = runCLI(t, deps{}, "workspace", "apply", "--yes")
	if r.code != 0 {
		t.Fatalf("apply sre: %q", r.stderr)
	}
	shell, _ := os.ReadFile(filepath.Join(wsDir, "shell.sh"))
	if strings.Contains(string(shell), "PS1=") {
		t.Fatalf("sre (plain prompt) must not set PS1:\n%s", shell)
	}
	r = runCLI(t, deps{}, "workspace", "status")
	if !strings.Contains(r.stdout, "Applied for:     sre") || !strings.Contains(r.stdout, "clean") {
		t.Fatalf("status after switch:\n%s", r.stdout)
	}
	_ = home
}

func workspaceDirForTest() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "veilbox", "workspace"), nil
}
