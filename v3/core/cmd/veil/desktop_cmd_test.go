package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Shreyas0047/veilbox/v3/core/internal/desktop"
	"github.com/Shreyas0047/veilbox/v3/core/internal/dnfops"
	"github.com/Shreyas0047/veilbox/v3/core/internal/experience"
)

const cliNiriDesktopYAML = `name: niri-desktop
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

const cliNiriTemplate = `// {{.DisplayName}}
spawn-at-startup "noctalia"
binds {
    Mod+T { spawn "{{.Terminal}}" }
    Mod+E { spawn "{{.Terminal}}" "-e" "{{.Editor}}" }
}
`

// setupDesktopCLI wires a desktop engine over an isolated root:
// VEILBOX_ROOT, HOME, XDG_CONFIG_HOME, templates, session dir, and a
// fake system facade. Returns deps plus the fake runner.
func setupDesktopCLI(t *testing.T) (deps, *fakeCLIRunner) {
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
		os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))

	sessions := t.TempDir()
	t.Setenv("VEILBOX_SESSION_DIR", sessions)
	os.WriteFile(filepath.Join(sessions, "niri.desktop"), []byte("[Desktop Entry]\n"), 0o644)

	catDir := filepath.Join(root, "experiences")
	os.MkdirAll(catDir, 0o755)
	os.WriteFile(filepath.Join(catDir, "niri-desktop.yaml"), []byte(cliNiriDesktopYAML), 0o644)
	os.WriteFile(filepath.Join(catDir, "networking-tools.yaml"),
		[]byte("name: networking-tools\ndescription: Networking diagnostics.\nrpm: veilbox-experience-networking-tools\npackages:\n  - bind-utils\n"), 0o644)

	tmplDir := filepath.Join(root, "desktop", "niri")
	os.MkdirAll(tmplDir, 0o755)
	os.WriteFile(filepath.Join(tmplDir, "niri.config.kdl"), []byte(cliNiriTemplate), 0o644)
	os.WriteFile(filepath.Join(tmplDir, "noctalia.config.toml"),
		[]byte("[include]\nfiles = [\"~/.config/veilbox/desktop/{{.Name}}/noctalia-veilbox.toml\"]\n"), 0o644)
	os.WriteFile(filepath.Join(tmplDir, "noctalia-veilbox.toml"),
		[]byte("[wallpaper.default]\npath = \"/usr/share/veilbox/desktop/{{.Name}}/wallpaper.png\"\n"), 0o644)

	f := &fakeCLIRunner{responses: map[string]string{}, errByCmd: map[string]error{}}
	dnf := dnfops.NewWithRunner(f)
	d := deps{
		newCatalog: func() *experience.Catalog {
			return experience.NewCatalogWith(catDir, dnf)
		},
		newDNF: func() *dnfops.System { return dnf },
		newDesktop: func(c *experience.Catalog) *desktop.Engine {
			return desktop.NewWith(c, desktop.NewSystemWith(f))
		},
	}
	return d, f
}

// installed simulates rpm -q reporting a present package.
func (f *fakeCLIRunner) installed(pkg string) {
	f.responses[f.key("rpm", "-q", pkg)] = pkg + "-0.1.0-1.fc44.noarch"
}

func TestDesktopListShowsDesktop(t *testing.T) {
	d, f := setupDesktopCLI(t)
	f.responses[f.key("rpm", "-qa", "--queryformat", "%{NAME}\n")] = ""
	r := runCLI(t, d, "desktop", "list")
	if r.code != 0 {
		t.Fatalf("code=%d err=%q", r.code, r.stderr)
	}
	for _, want := range []string{"niri-desktop", "Niri Experience", "available", "niri"} {
		if !strings.Contains(r.stdout, want) {
			t.Fatalf("missing %q in:\n%s", want, r.stdout)
		}
	}
	if strings.Contains(r.stdout, "networking-tools") {
		t.Fatalf("tooling experience leaked into desktop list:\n%s", r.stdout)
	}
}

func TestDesktopOverviewSessionFromTTY(t *testing.T) {
	d, f := setupDesktopCLI(t)
	f.responses[f.key("rpm", "-qa", "--queryformat", "%{NAME}\n")] = ""
	r := runCLI(t, d, "desktop")
	if r.code != 0 {
		t.Fatalf("code=%d err=%q", r.code, r.stderr)
	}
	if !strings.Contains(r.stdout, "no graphical Veilbox desktop session detected") {
		t.Fatalf("must not guess session state:\n%s", r.stdout)
	}
}

func TestDesktopInfo(t *testing.T) {
	d, f := setupDesktopCLI(t)
	f.responses[f.key("rpm", "-qa", "--queryformat", "%{NAME}\n")] = ""
	r := runCLI(t, d, "desktop", "info", "niri-desktop")
	if r.code != 0 {
		t.Fatalf("code=%d err=%q", r.code, r.stderr)
	}
	for _, want := range []string{"compositor", "niri", "shell", "noctalia", "display_manager", "sddm", "provided by shell"} {
		if !strings.Contains(r.stdout, want) {
			t.Fatalf("missing %q in:\n%s", want, r.stdout)
		}
	}
}

func TestDesktopInfoUnknown(t *testing.T) {
	d, _ := setupDesktopCLI(t)
	r := runCLI(t, d, "desktop", "info", "nope")
	if r.code != 1 {
		t.Fatalf("code=%d", r.code)
	}
}

func TestDesktopInstallActivates(t *testing.T) {
	d, f := setupDesktopCLI(t)
	f.responses[f.key("rpm", "-qa", "--queryformat", "%{NAME}\n")] = "veilbox-experience-niri\n"
	f.notInstalled("veilbox-experience-niri")
	f.responses[f.key("systemctl", "get-default")] = "multi-user.target"

	r := runCLI(t, d, "desktop", "install", "niri-desktop")
	if r.code != 0 {
		t.Fatalf("code=%d err=%q", r.code, r.stderr)
	}
	joined := strings.Join(f.joined(), "\n")
	for _, want := range []string{
		"interactive:sudo systemctl enable sddm",
		"interactive:sudo systemctl set-default graphical.target",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "interactive:sudo dnf install -y veilbox-experience-niri") {
		t.Fatalf("package already installed — DNF step must be skipped:\n%s", joined)
	}
	for _, want := range []string{
		"already installed",
		"enabled display manager: sddm",
		"set-default multi-user.target",
		"activated. Reboot to log in.",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Fatalf("missing %q in stdout:\n%s", want, r.stdout)
		}
	}
	cfg := os.Getenv("XDG_CONFIG_HOME")
	for _, p := range []string{
		filepath.Join(cfg, "niri", "config.kdl"),
		filepath.Join(cfg, "noctalia", "config.toml"),
		filepath.Join(cfg, "veilbox", "desktop", "niri-desktop", "noctalia-veilbox.toml"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected %s: %v", p, err)
		}
	}
}

func TestDesktopInstallPreservesUserConfig(t *testing.T) {
	d, f := setupDesktopCLI(t)
	f.responses[f.key("rpm", "-qa", "--queryformat", "%{NAME}\n")] = "veilbox-experience-niri\n"
	f.notInstalled("veilbox-experience-niri")
	f.responses[f.key("systemctl", "get-default")] = "multi-user.target"

	cfg := os.Getenv("XDG_CONFIG_HOME")
	os.MkdirAll(filepath.Join(cfg, "niri"), 0o755)
	userConfig := filepath.Join(cfg, "niri", "config.kdl")
	os.WriteFile(userConfig, []byte("my precious config\n"), 0o644)

	r := runCLI(t, d, "desktop", "install", "niri-desktop")
	if r.code != 0 {
		t.Fatalf("code=%d err=%q", r.code, r.stderr)
	}
	if !strings.Contains(r.stdout, "preserved") {
		t.Fatalf("must report preserved config:\n%s", r.stdout)
	}
	data, err := os.ReadFile(userConfig)
	if err != nil || string(data) != "my precious config\n" {
		t.Fatalf("user config overwritten: %v %q", err, data)
	}
}

func TestDesktopRemoveIsConservative(t *testing.T) {
	d, f := setupDesktopCLI(t)
	f.responses[f.key("rpm", "-qa", "--queryformat", "%{NAME}\n")] = "veilbox-experience-niri\n"
	f.installed("veilbox-experience-niri")
	cfg := os.Getenv("XDG_CONFIG_HOME")
	os.MkdirAll(filepath.Join(cfg, "niri"), 0o755)
	os.WriteFile(filepath.Join(cfg, "niri", "config.kdl"), []byte("user config\n"), 0o644)

	r := runCLI(t, d, "desktop", "remove", "niri-desktop")
	if r.code != 0 {
		t.Fatalf("code=%d err=%q", r.code, r.stderr)
	}
	if !strings.Contains(strings.Join(f.joined(), "\n"), "interactive:sudo dnf remove -y veilbox-experience-niri") {
		t.Fatalf("calls: %v", f.joined())
	}
	for _, want := range []string{"preserved", "config.kdl", "set-default"} {
		if !strings.Contains(r.stdout, want) {
			t.Fatalf("missing %q in:\n%s", want, r.stdout)
		}
	}
	joined := strings.Join(f.joined(), "\n")
	if strings.Contains(joined, "systemctl disable") || strings.Contains(joined, "set-default graphical") {
		t.Fatalf("removal must not deactivate the display manager:\n%s", joined)
	}
}

func TestDesktopProvisionRegenerates(t *testing.T) {
	d, f := setupDesktopCLI(t)
	f.responses[f.key("rpm", "-qa", "--queryformat", "%{NAME}\n")] = "veilbox-experience-niri\n"
	r := runCLI(t, d, "desktop", "provision", "niri-desktop")
	if r.code != 0 {
		t.Fatalf("code=%d err=%q", r.code, r.stderr)
	}
	for _, want := range []string{"created", "regenerated", "config.kdl"} {
		if !strings.Contains(r.stdout, want) {
			t.Fatalf("missing %q in:\n%s", want, r.stdout)
		}
	}
}

func TestDesktopProvisionNotInstalled(t *testing.T) {
	d, f := setupDesktopCLI(t)
	f.responses[f.key("rpm", "-qa", "--queryformat", "%{NAME}\n")] = ""
	r := runCLI(t, d, "desktop", "provision", "niri-desktop")
	if r.code != 1 {
		t.Fatalf("code=%d", r.code)
	}
	if !strings.Contains(r.stderr, "not installed") {
		t.Fatalf("stderr=%q", r.stderr)
	}
}

func TestDesktopUnknownCommand(t *testing.T) {
	d, _ := setupDesktopCLI(t)
	if r := runCLI(t, d, "desktop", "frobnicate"); r.code != 2 {
		t.Fatalf("code=%d", r.code)
	}
}

func TestDesktopInstallRejectsToolingExperience(t *testing.T) {
	d, f := setupDesktopCLI(t)
	f.responses[f.key("rpm", "-qa", "--queryformat", "%{NAME}\n")] = ""
	r := runCLI(t, d, "desktop", "install", "networking-tools")
	if r.code != 1 {
		t.Fatalf("code=%d err=%q", r.code, r.stderr)
	}
	if !strings.Contains(r.stderr, "not a desktop experience") {
		t.Fatalf("stderr=%q", r.stderr)
	}
}
