package desktop

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Shreyas0047/veilbox/v3/core/internal/dnfops"
	"github.com/Shreyas0047/veilbox/v3/core/internal/experience"
)

const niriDesktopYAML = `name: niri-desktop
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

const terminalOpsYAML = `name: terminal-ops
description: Terminal operations toolkit.
rpm: veilbox-experience-terminal-ops
`

// fakeRunner is a stateful dnfops.Runner that never touches the
// system. It models the RPM database: dnf install/remove transactions
// mutate it, so the Experience/Desktop Engine sees the same state
// transitions production DNF would produce. systemctl/pgrep responses
// are canned.
type fakeRunner struct {
	db        map[string]bool
	responses map[string]string
	errByCmd  map[string]error
	calls     []string
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{db: map[string]bool{"veilbox-core": true}, responses: map[string]string{}, errByCmd: map[string]error{}}
}

func (f *fakeRunner) key(name string, args ...string) string {
	return strings.Join(append([]string{name}, args...), " ")
}

func (f *fakeRunner) Run(name string, args ...string) (string, error) {
	key := f.key(name, args...)
	f.calls = append(f.calls, "run:"+key)
	if name == "rpm" && len(args) == 2 && args[0] == "-q" {
		pkg := args[1]
		if f.db[pkg] {
			return pkg + "-0.1.0-1.fc44.noarch", nil
		}
		return "package " + pkg + " is not installed", errors.New("exit status 1")
	}
	if key == f.key("rpm", "-qa", "--queryformat", "%{NAME}\n") {
		var pkgs []string
		for p := range f.db {
			pkgs = append(pkgs, p)
		}
		sort.Strings(pkgs)
		return strings.Join(pkgs, "\n") + "\n", nil
	}
	if err := f.errByCmd[key]; err != nil {
		return f.responses[key], err
	}
	return f.responses[key], nil
}

func (f *fakeRunner) RunInteractive(name string, args ...string) error {
	key := f.key(name, args...)
	f.calls = append(f.calls, "interactive:"+key)
	if name == "sudo" && len(args) == 4 && args[0] == "dnf" && args[1] == "install" && args[2] == "-y" {
		f.db[args[3]] = true
	}
	if name == "sudo" && len(args) == 4 && args[0] == "dnf" && args[1] == "remove" && args[2] == "-y" {
		delete(f.db, args[3])
	}
	return f.errByCmd[key]
}

// installed marks a package present in the (fake) RPM database.
func (f *fakeRunner) installed(pkg string) {
	f.db[pkg] = true
}

// setupEngine builds a desktop Engine over a fake catalog, with the
// system facade, templates, session dir and home all isolated in
// temp dirs.
func setupEngine(t *testing.T, manifests map[string]string) (*Engine, *fakeRunner, string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("VEILBOX_ROOT", root)
	catDir := filepath.Join(root, "experiences")
	os.MkdirAll(catDir, 0o755)
	for name, content := range manifests {
		os.WriteFile(filepath.Join(catDir, name), []byte(content), 0o644)
	}
	home := filepath.Join(t.TempDir(), "home")
	os.MkdirAll(home, 0o755)
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	f := newFakeRunner()
	sys := NewSystemWith(f)
	sys.sessionDir = filepath.Join(t.TempDir(), "sessions")
	os.MkdirAll(sys.sessionDir, 0o755)
	sys.lookPath = func(name string) (string, error) {
		if name == "kitty" || name == "vim" {
			return "/usr/bin/" + name, nil
		}
		return "", errors.New("not found")
	}
	return NewWith(experience.NewCatalogWith(catDir, dnfops.NewWithRunner(f)), sys), f, root
}

// writeTemplates installs the RPM-owned templates for an experience
// under the VEILBOX_ROOT/desktop/<name> directory.
func writeTemplates(t *testing.T, root, name string) {
	t.Helper()
	dir := filepath.Join(root, "desktop", name)
	os.MkdirAll(dir, 0o755)
	files := map[string]string{
		"niri.config.kdl": `// {{.DisplayName}}
spawn-at-startup "noctalia"
binds {
    Mod+T { spawn "{{.Terminal}}" }
    Mod+E { spawn "{{.Terminal}}" "-e" "{{.Editor}}" }
}
`,
		"noctalia.config.toml": `[include]
files = ["~/.config/veilbox/desktop/{{.Name}}/noctalia-veilbox.toml"]
`,
		"noctalia-veilbox.toml": `[theme]
mode = "dark"
[wallpaper.default]
path = "{{.SystemDir}}/wallpaper.png"
`,
	}
	for fn, content := range files {
		os.WriteFile(filepath.Join(dir, fn), []byte(content), 0o644)
	}
}

func sessionFile(t *testing.T, eng *Engine) string {
	t.Helper()
	p := filepath.Join(eng.sys.sessionDir, "niri.desktop")
	os.WriteFile(p, []byte("[Desktop Entry]\nType=Application\nName=Niri\nExec=niri-session\n"), 0o644)
	return p
}

func TestListFiltersDesktop(t *testing.T) {
	eng, _, _ := setupEngine(t, map[string]string{
		"niri-desktop.yaml": niriDesktopYAML,
		"terminal-ops.yaml": terminalOpsYAML,
	})
	entries, err := eng.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("want only desktop entries, got %d: %+v", len(entries), entries)
	}
	if entries[0].Name != "niri-desktop" {
		t.Fatalf("entry: %s", entries[0].Name)
	}
	if entries[0].Status != experience.StatusAvailable {
		t.Fatalf("status: %s", entries[0].Status)
	}
}

func TestInfoUnknown(t *testing.T) {
	eng, _, _ := setupEngine(t, map[string]string{"niri-desktop.yaml": niriDesktopYAML})
	if _, err := eng.Info("nope"); err == nil {
		t.Fatal("expected error for unknown desktop")
	}
}

func TestDetectSessionFromTTY(t *testing.T) {
	t.Setenv("XDG_SESSION_TYPE", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	eng, _, _ := setupEngine(t, map[string]string{"niri-desktop.yaml": niriDesktopYAML})
	s := eng.DetectSession()
	if s.Graphical {
		t.Fatal("must not claim a graphical session from a TTY")
	}
	if !strings.Contains(s.Message, "no graphical Veilbox desktop session detected") {
		t.Fatalf("message: %q", s.Message)
	}
}

func TestDetectSessionWayland(t *testing.T) {
	t.Setenv("XDG_SESSION_TYPE", "wayland")
	t.Setenv("WAYLAND_DISPLAY", "wayland-0")
	eng, f, _ := setupEngine(t, map[string]string{"niri-desktop.yaml": niriDesktopYAML})
	f.installed("veilbox-experience-niri")
	s := eng.DetectSession()
	if !s.Graphical {
		t.Fatal("expected graphical session")
	}
	if s.Compositor != "niri" {
		t.Fatalf("compositor: %q", s.Compositor)
	}
	if !strings.Contains(s.Message, "graphical Veilbox desktop session detected") {
		t.Fatalf("message: %q", s.Message)
	}
}

func TestDetectSessionWaylandNoCompositor(t *testing.T) {
	t.Setenv("XDG_SESSION_TYPE", "wayland")
	t.Setenv("WAYLAND_DISPLAY", "wayland-0")
	eng, _, _ := setupEngine(t, map[string]string{"niri-desktop.yaml": niriDesktopYAML})
	s := eng.DetectSession()
	if s.Compositor != "" {
		t.Fatalf("must not report a compositor that is not running, got %q", s.Compositor)
	}
	if s.Message == "" {
		t.Fatal("expected an honest message")
	}
}

func TestPlanInstallFirstTouch(t *testing.T) {
	eng, f, root := setupEngine(t, map[string]string{"niri-desktop.yaml": niriDesktopYAML})
	writeTemplates(t, root, "niri")
	f.responses[f.key("systemctl", "get-default")] = "multi-user.target"
	f.errByCmd[f.key("systemctl", "is-enabled", "sddm")] = errors.New("not enabled")
	plan, err := eng.PlanInstall("niri-desktop")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if len(plan.CreatedConfig) != 2 {
		t.Fatalf("created: %v", plan.CreatedConfig)
	}
	if plan.WillEnableDM != true || plan.DeclaredDM != "sddm" {
		t.Fatalf("dm: declared=%s enabled=%s will=%v", plan.DeclaredDM, plan.EnabledDM, plan.WillEnableDM)
	}
	if !plan.WillSetTarget || plan.RollbackTarget != "multi-user.target" {
		t.Fatalf("target: %+v", plan)
	}
	if plan.SessionFile == "" || !strings.HasSuffix(plan.SessionFile, "sessions/niri.desktop") {
		t.Fatalf("session file: %q", plan.SessionFile)
	}
}

func TestPlanInstallPreservesExistingConfig(t *testing.T) {
	eng, f, root := setupEngine(t, map[string]string{"niri-desktop.yaml": niriDesktopYAML})
	writeTemplates(t, root, "niri")
	cfg := os.Getenv("XDG_CONFIG_HOME")
	os.MkdirAll(filepath.Join(cfg, "niri"), 0o755)
	os.WriteFile(filepath.Join(cfg, "niri", "config.kdl"), []byte("user config\n"), 0o644)
	plan, err := eng.PlanInstall("niri-desktop")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if len(plan.CreatedConfig) != 1 || len(plan.ExistingConfig) != 1 {
		t.Fatalf("created=%v existing=%v", plan.CreatedConfig, plan.ExistingConfig)
	}
	if !strings.HasSuffix(plan.ExistingConfig[0], "niri/config.kdl") {
		t.Fatalf("existing: %v", plan.ExistingConfig)
	}
	_ = f
}

func TestPlanInstallOtherDMEnabled(t *testing.T) {
	eng, f, root := setupEngine(t, map[string]string{"niri-desktop.yaml": niriDesktopYAML})
	writeTemplates(t, root, "niri")
	f.responses[f.key("systemctl", "is-enabled", "gdm")] = "enabled"
	f.responses[f.key("systemctl", "is-enabled", "sddm")] = "disabled"
	plan, err := eng.PlanInstall("niri-desktop")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if plan.EnabledDM != "gdm" || plan.WillEnableDM {
		t.Fatalf("must not enable sddm while gdm is enabled: %+v", plan)
	}
}

func TestInstallFullSequence(t *testing.T) {
	eng, f, root := setupEngine(t, map[string]string{"niri-desktop.yaml": niriDesktopYAML})
	writeTemplates(t, root, "niri")
	f.responses[f.key("systemctl", "get-default")] = "multi-user.target"
	f.errByCmd[f.key("systemctl", "is-enabled", "sddm")] = errors.New("not enabled")
	sessionFile(t, eng)

	res, err := eng.Install("niri-desktop")
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if !strings.Contains(strings.Join(res.Steps, "\n"), "enabled display manager: sddm") {
		t.Fatalf("steps: %v", res.Steps)
	}
	if !strings.Contains(strings.Join(res.Steps, "\n"), "set-default multi-user.target") {
		t.Fatalf("rollback hint missing: %v", res.Steps)
	}
	joined := strings.Join(f.calls, "\n")
	for _, want := range []string{
		"interactive:sudo dnf install -y veilbox-experience-niri",
		"interactive:sudo systemctl enable sddm",
		"interactive:sudo systemctl set-default graphical.target",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing call %q in %v", want, f.calls)
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

func TestInstallMissingSessionFileFails(t *testing.T) {
	eng, f, root := setupEngine(t, map[string]string{"niri-desktop.yaml": niriDesktopYAML})
	writeTemplates(t, root, "niri")
	f.responses[f.key("systemctl", "get-default")] = "graphical.target"
	if _, err := eng.Install("niri-desktop"); err == nil {
		t.Fatal("expected error when session file missing")
	} else if !strings.Contains(err.Error(), "session file") {
		t.Fatalf("error: %v", err)
	}
}

func TestInstallNonDesktopRejected(t *testing.T) {
	eng, _, _ := setupEngine(t, map[string]string{"terminal-ops.yaml": terminalOpsYAML})
	if _, err := eng.Install("terminal-ops"); err == nil {
		t.Fatal("expected non-desktop rejection")
	}
}

func TestProvisionConsumesPreferences(t *testing.T) {
	eng, f, root := setupEngine(t, map[string]string{"niri-desktop.yaml": niriDesktopYAML})
	writeTemplates(t, root, "niri")
	f.installed("veilbox-experience-niri")
	os.MkdirAll(filepath.Join(root, "profiles"), 0o755)
	os.WriteFile(filepath.Join(root, "profiles", "devops.yaml"), []byte(`name: devops
description: DevOps engineer.
workspace_preferences:
  shell: bash
  editor: vim
  terminal: kitty
`), 0o644)
	os.WriteFile(filepath.Join(root, "state.json"), []byte(`{"active_profile":"devops"}`), 0o644)

	res, err := eng.Provision("niri-desktop")
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	if len(res.Created) != 2 || len(res.Regenerated) != 1 {
		t.Fatalf("result: %+v", res)
	}
	cfg := os.Getenv("XDG_CONFIG_HOME")
	data, err := os.ReadFile(filepath.Join(cfg, "niri", "config.kdl"))
	if err != nil {
		t.Fatalf("read niri config: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, `Mod+T { spawn "kitty" }`) {
		t.Fatalf("terminal preference not applied:\n%s", content)
	}
	if !strings.Contains(content, `Mod+E { spawn "kitty" "-e" "vim" }`) {
		t.Fatalf("editor preference not applied:\n%s", content)
	}
}

func TestProvisionRegeneratesVeilboxFile(t *testing.T) {
	eng, f, root := setupEngine(t, map[string]string{"niri-desktop.yaml": niriDesktopYAML})
	writeTemplates(t, root, "niri")
	f.installed("veilbox-experience-niri")
	cfg := os.Getenv("XDG_CONFIG_HOME")
	os.MkdirAll(filepath.Join(cfg, "niri"), 0o755)
	os.MkdirAll(filepath.Join(cfg, "noctalia"), 0o755)
	os.WriteFile(filepath.Join(cfg, "niri", "config.kdl"), []byte("user niri config\n"), 0o644)
	os.WriteFile(filepath.Join(cfg, "noctalia", "config.toml"), []byte("user noctalia config\n"), 0o644)
	veilFile := filepath.Join(cfg, "veilbox", "desktop", "niri-desktop", "noctalia-veilbox.toml")
	os.MkdirAll(filepath.Dir(veilFile), 0o755)
	os.WriteFile(veilFile, []byte("stale\n"), 0o644)

	res, err := eng.Provision("niri-desktop")
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	if len(res.Created) != 0 || len(res.Preserved) != 2 || len(res.Regenerated) != 1 {
		t.Fatalf("result: %+v", res)
	}
	data, err := os.ReadFile(veilFile)
	if err != nil {
		t.Fatalf("read veilbox file: %v", err)
	}
	if !strings.Contains(string(data), "wallpaper.png") {
		t.Fatalf("veilbox file not regenerated:\n%s", data)
	}
	keep, _ := os.ReadFile(filepath.Join(cfg, "niri", "config.kdl"))
	if string(keep) != "user niri config\n" {
		t.Fatalf("user config overwritten: %q", keep)
	}
}

func TestProvisionNotInstalled(t *testing.T) {
	eng, _, root := setupEngine(t, map[string]string{"niri-desktop.yaml": niriDesktopYAML})
	writeTemplates(t, root, "niri")
	if _, err := eng.Provision("niri-desktop"); err == nil {
		t.Fatal("expected error when not installed")
	}
}

func TestPlanRemovePreservesAndReports(t *testing.T) {
	eng, f, root := setupEngine(t, map[string]string{"niri-desktop.yaml": niriDesktopYAML})
	writeTemplates(t, root, "niri")
	cfg := os.Getenv("XDG_CONFIG_HOME")
	os.MkdirAll(filepath.Join(cfg, "niri"), 0o755)
	os.WriteFile(filepath.Join(cfg, "niri", "config.kdl"), []byte("user config\n"), 0o644)
	veilFile := filepath.Join(cfg, "veilbox", "desktop", "niri-desktop", "noctalia-veilbox.toml")
	os.MkdirAll(filepath.Dir(veilFile), 0o755)
	os.WriteFile(veilFile, []byte("x\n"), 0o644)
	f.responses[f.key("systemctl", "get-default")] = "graphical.target"

	plan, err := eng.PlanRemove("niri-desktop")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if len(plan.PreservedFiles) != 1 || len(plan.VeilboxFiles) != 1 {
		t.Fatalf("files: %+v", plan)
	}
	if !strings.Contains(plan.DeactivateHint, "sddm") ||
		!strings.Contains(plan.DeactivateHint, "set-default graphical.target") {
		t.Fatalf("hint: %q", plan.DeactivateHint)
	}
	if strings.Contains(strings.Join(f.calls, "\n"), "systemctl disable") {
		t.Fatal("planning must not touch the display manager")
	}
}

func TestRemoveIsNonDestructive(t *testing.T) {
	eng, f, root := setupEngine(t, map[string]string{"niri-desktop.yaml": niriDesktopYAML})
	writeTemplates(t, root, "niri")
	f.installed("veilbox-experience-niri")
	cfg := os.Getenv("XDG_CONFIG_HOME")
	os.MkdirAll(filepath.Join(cfg, "niri"), 0o755)
	userFile := filepath.Join(cfg, "niri", "config.kdl")
	os.WriteFile(userFile, []byte("user config\n"), 0o644)

	plan, err := eng.Remove("niri-desktop")
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if plan.RPM != "veilbox-experience-niri" {
		t.Fatalf("rpm: %s", plan.RPM)
	}
	if !strings.Contains(strings.Join(f.calls, "\n"), "interactive:sudo dnf remove -y veilbox-experience-niri") {
		t.Fatalf("calls: %v", f.calls)
	}
	data, err := os.ReadFile(userFile)
	if err != nil || string(data) != "user config\n" {
		t.Fatalf("user config must survive removal: %v %q", err, data)
	}
}
