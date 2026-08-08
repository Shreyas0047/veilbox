// Package onboardingtest provides a fake command runner and an
// isolated environment for exercising the onboarding wizard and its
// UI without touching the machine. It intentionally does not import
// the onboarding package so tests of both onboarding itself and the
// TUI front-end can share it.
package onboardingtest

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestDesktopYAML is a minimal desktop experience catalog entry.
const TestDesktopYAML = `name: niri-desktop
display_name: Niri Experience
type: desktop
description: Complete Niri + Noctalia desktop experience.
rpm: veilbox-experience-niri
components:
  compositor: niri
  shell: noctalia
  terminal: kitty
  launcher: builtin
  notifications: builtin
  lock: builtin
  idle: builtin
  wallpaper: builtin
  clipboard: builtin
  screenshot: builtin
  display_manager: sddm
packages:
  - niri
  - noctalia
  - kitty
`

// TestToolYAML is a minimal tooling experience catalog entry.
const TestToolYAML = `name: networking-tools
display_name: Networking Tools
description: Networking diagnostics.
rpm: veilbox-experience-networking-tools
capabilities:
  - networking
packages:
  - bind-utils
`

// TestProfileYAML is a minimal profile registry entry.
const TestProfileYAML = `name: cloud-engineer
display_name: Cloud Engineer
description: Works with cloud infrastructure.
role: cloud-engineer
recommended_capabilities:
  - networking
optional_capabilities: []
workspace_preferences:
  shell: bash
  editor: vim
  terminal: system
  prompt: veilbox
  tmux: false
`

// TestCapabilityYAMLs are the capability manifests the test
// environment ships, mirroring the real catalog.
const TestCapabilityYAMLs = `name: base-operations
domain: fundamentals
tier: core
description: Essential system operations.
`

// TestNetworkingCapabilityYAML mirrors the networking capability used
// by the test profile and experience.
const TestNetworkingCapabilityYAML = `name: networking
domain: networking
tier: core
description: Network diagnostics.
`

// Runner is a command runner whose DNF/rpm behavior mirrors a real
// machine: installing through dnf mutates the rpm database.
type Runner struct {
	Responses map[string]string
	ErrByCmd  map[string]error
	state     map[string]bool // installed packages
	joined    []string
}

// New returns an empty fake runner.
func New() *Runner {
	return &Runner{
		Responses: map[string]string{},
		ErrByCmd:  map[string]error{},
		state:     map[string]bool{},
	}
}

// Run implements dnfops.Runner.
func (f *Runner) Run(name string, args ...string) (string, error) {
	f.joined = append(f.joined, strings.Join(append([]string{name}, args...), " "))
	if name == "rpm" && len(args) == 2 && args[0] == "-q" {
		pkg := args[1]
		if f.state[pkg] {
			return pkg + "-0.1.0-1.fc44.noarch\n", nil
		}
		return "package " + pkg + " is not installed\n", errors.New("exit status 1")
	}
	if name == "rpm" && len(args) >= 1 && args[0] == "-qa" {
		var names []string
		for p := range f.state {
			names = append(names, p)
		}
		sort.Strings(names)
		return strings.Join(names, "\n") + "\n", nil
	}
	key := strings.Join(append([]string{name}, args...), "\x00")
	if err := f.ErrByCmd[key]; err != nil {
		return "", err
	}
	if out, ok := f.Responses[key]; ok {
		return out, nil
	}
	return "", nil
}

// RunInteractive implements dnfops.Runner.
func (f *Runner) RunInteractive(name string, args ...string) error {
	f.joined = append(f.joined, "interactive:"+strings.Join(append([]string{name}, args...), " "))
	if name == "sudo" && len(args) >= 3 && args[0] == "dnf" && args[1] == "install" {
		pkg := args[len(args)-1]
		if err := f.ErrByCmd[f.Key("sudo", "dnf", "install", pkg)]; err != nil {
			return err
		}
		f.state[pkg] = true
		return nil
	}
	key := strings.Join(append([]string{name}, args...), "\x00")
	return f.ErrByCmd[key]
}

// Key joins a command into the fake's lookup key.
func (f *Runner) Key(name string, args ...string) string {
	return strings.Join(append([]string{name}, args...), "\x00")
}

// Installed marks a package as present in the fake rpm database.
func (f *Runner) Installed(pkg string) {
	f.state[pkg] = true
}

// Fail makes the given command fail with a generic error.
func (f *Runner) Fail(name string, args ...string) {
	f.ErrByCmd[f.Key(name, args...)] = errors.New("boom")
}

// Joined returns every command executed so far, in order.
func (f *Runner) Joined() []string {
	return f.joined
}

// Env describes the directories of the isolated onboarding
// environment built by SetupEnv.
type Env struct {
	Root    string
	CatDir  string
	CapDir  string
	ProfDir string
}

// SetupEnv builds an isolated environment: experience catalog, profile
// registry, HOME, session dir and fake terminal binaries. It returns
// the directory layout for wiring engines.
func SetupEnv(t *testing.T) Env {
	t.Helper()
	root := t.TempDir()
	t.Setenv("VEILBOX_ROOT", root)
	home := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	bin := t.TempDir()
	for _, name := range []string{"kitty", "vim"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))

	sessions := t.TempDir()
	t.Setenv("VEILBOX_SESSION_DIR", sessions)
	if err := os.WriteFile(filepath.Join(sessions, "niri.desktop"), []byte("[Desktop Entry]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	catDir := filepath.Join(root, "experiences")
	if err := os.MkdirAll(catDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"niri-desktop.yaml":     TestDesktopYAML,
		"networking-tools.yaml": TestToolYAML,
	} {
		if err := os.WriteFile(filepath.Join(catDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	capDir := filepath.Join(root, "capabilities")
	if err := os.MkdirAll(capDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"base-operations.yaml": TestCapabilityYAMLs,
		"networking.yaml":      TestNetworkingCapabilityYAML,
	} {
		if err := os.WriteFile(filepath.Join(capDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	profDir := filepath.Join(root, "profiles")
	if err := os.MkdirAll(profDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profDir, "cloud-engineer.yaml"), []byte(TestProfileYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	tmplDir := filepath.Join(root, "desktop", "niri")
	if err := os.MkdirAll(tmplDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"niri.config.kdl":       "binds { Mod+T { spawn \"{{.Terminal}}\" } }",
		"noctalia.config.toml":  "x=1",
		"noctalia-veilbox.toml": "y=2",
	} {
		if err := os.WriteFile(filepath.Join(tmplDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	return Env{Root: root, CatDir: catDir, CapDir: capDir, ProfDir: profDir}
}
