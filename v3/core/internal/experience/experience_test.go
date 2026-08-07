package experience

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Shreyas0047/veilbox/v3/core/internal/dnfops"
)

const networkingToolsYAML = `name: networking-tools
description: Networking diagnostics toolkit for engineers.
rpm: veilbox-experience-networking-tools
packages:
  - bind-utils
  - traceroute
  - nmap-ncat
  - iproute
  - tcpdump
`

const terminalOpsYAML = `name: terminal-ops
description: Terminal operations toolkit (planned).
`

func writeCatalog(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	os.MkdirAll(dir, 0o755)
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// fakeRunner is a configurable dnfops.Runner that never touches the
// system. It records invocations and serves canned responses.
type fakeRunner struct {
	responses map[string]string
	errByCmd  map[string]error
	calls     []string
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{responses: map[string]string{}, errByCmd: map[string]error{}}
}

func (f *fakeRunner) key(name string, args ...string) string {
	return strings.Join(append([]string{name}, args...), " ")
}

func (f *fakeRunner) Run(name string, args ...string) (string, error) {
	key := f.key(name, args...)
	f.calls = append(f.calls, "run:"+key)
	if err := f.errByCmd[key]; err != nil {
		return f.responses[key], err
	}
	return f.responses[key], nil
}

func (f *fakeRunner) RunInteractive(name string, args ...string) error {
	key := f.key(name, args...)
	f.calls = append(f.calls, "interactive:"+key)
	return f.errByCmd[key]
}

func (f *fakeRunner) installed(pkg string) {
	f.responses[f.key("rpm", "-q", pkg)] = pkg + "-0.1.0-1.fc44.noarch"
}

func (f *fakeRunner) notInstalled(pkg string) {
	key := f.key("rpm", "-q", pkg)
	f.responses[key] = "package " + pkg + " is not installed"
	f.errByCmd[key] = errors.New("exit status 1")
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	writeCatalog(t, dir, map[string]string{"networking-tools.yaml": networkingToolsYAML})
	m, err := NewCatalogWith(dir, dnfops.NewWithRunner(newFakeRunner())).Load("networking-tools")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if m.Name != "networking-tools" || m.RPM != "veilbox-experience-networking-tools" {
		t.Fatalf("bad manifest: %+v", m)
	}
	if len(m.Packages) != 5 {
		t.Fatalf("packages: %v", m.Packages)
	}
}

func TestLoadUnknown(t *testing.T) {
	dir := t.TempDir()
	if _, err := NewCatalogWith(dir, dnfops.NewWithRunner(newFakeRunner())).Load("nope"); err == nil {
		t.Fatal("expected error for unknown experience")
	}
}

func TestLoadNameMismatch(t *testing.T) {
	dir := t.TempDir()
	writeCatalog(t, dir, map[string]string{"networking-tools.yaml": "name: other\n"})
	if _, err := NewCatalogWith(dir, dnfops.NewWithRunner(newFakeRunner())).Load("networking-tools"); err == nil {
		t.Fatal("expected name mismatch error")
	}
}

func TestListStatuses(t *testing.T) {
	dir := t.TempDir()
	writeCatalog(t, dir, map[string]string{
		"networking-tools.yaml": networkingToolsYAML,
		"terminal-ops.yaml":     terminalOpsYAML,
	})
	f := newFakeRunner()
	f.responses[f.key("rpm", "-qa", "--queryformat", "%{NAME}\n")] = "veilbox-core\nbash\n"
	c := NewCatalogWith(dir, dnfops.NewWithRunner(f))

	entries, err := c.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries: %v", entries)
	}
	byName := map[string]Status{}
	for _, e := range entries {
		byName[e.Name] = e.Status
	}
	if byName["networking-tools"] != StatusAvailable {
		t.Fatalf("networking-tools should be available, got %s", byName["networking-tools"])
	}
	if byName["terminal-ops"] != StatusPlanned {
		t.Fatalf("terminal-ops should be planned, got %s", byName["terminal-ops"])
	}
}

func TestListInstalledStatus(t *testing.T) {
	dir := t.TempDir()
	writeCatalog(t, dir, map[string]string{"networking-tools.yaml": networkingToolsYAML})
	f := newFakeRunner()
	f.responses[f.key("rpm", "-qa", "--queryformat", "%{NAME}\n")] = "veilbox-experience-networking-tools\n"
	c := NewCatalogWith(dir, dnfops.NewWithRunner(f))

	entries, err := c.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if entries[0].Status != StatusInstalled {
		t.Fatalf("expected installed, got %s", entries[0].Status)
	}
}

func TestInstallUsesPackageNameThroughDNF(t *testing.T) {
	dir := t.TempDir()
	writeCatalog(t, dir, map[string]string{"networking-tools.yaml": networkingToolsYAML})
	f := newFakeRunner()
	f.notInstalled("veilbox-experience-networking-tools")
	c := NewCatalogWith(dir, dnfops.NewWithRunner(f))

	if err := c.Install("networking-tools"); err != nil {
		t.Fatalf("install: %v", err)
	}
	want := "interactive:sudo dnf install -y veilbox-experience-networking-tools"
	if got := f.calls; len(got) != 2 || got[1] != want {
		t.Fatalf("got %v, want last call %q", got, want)
	}
}

func TestInstallPlannedRejected(t *testing.T) {
	dir := t.TempDir()
	writeCatalog(t, dir, map[string]string{"terminal-ops.yaml": terminalOpsYAML})
	c := NewCatalogWith(dir, dnfops.NewWithRunner(newFakeRunner()))

	if err := c.Install("terminal-ops"); err == nil {
		t.Fatal("expected error installing planned experience")
	}
}

func TestInstallIdempotent(t *testing.T) {
	dir := t.TempDir()
	writeCatalog(t, dir, map[string]string{"networking-tools.yaml": networkingToolsYAML})
	f := newFakeRunner()
	f.installed("veilbox-experience-networking-tools")
	c := NewCatalogWith(dir, dnfops.NewWithRunner(f))

	if err := c.Install("networking-tools"); err == nil {
		t.Fatal("expected already-installed error")
	}
	if got := f.calls; len(got) != 1 {
		t.Fatalf("no transaction should run for installed experience, got %v", got)
	}
}

func TestRemoveUsesPackageNameThroughDNF(t *testing.T) {
	dir := t.TempDir()
	writeCatalog(t, dir, map[string]string{"networking-tools.yaml": networkingToolsYAML})
	f := newFakeRunner()
	f.installed("veilbox-experience-networking-tools")
	c := NewCatalogWith(dir, dnfops.NewWithRunner(f))

	if err := c.Remove("networking-tools"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	want := "interactive:sudo dnf remove -y veilbox-experience-networking-tools"
	if got := f.calls; len(got) != 2 || got[1] != want {
		t.Fatalf("got %v, want last call %q", got, want)
	}
}

func TestRemoveNotInstalled(t *testing.T) {
	dir := t.TempDir()
	writeCatalog(t, dir, map[string]string{"networking-tools.yaml": networkingToolsYAML})
	f := newFakeRunner()
	f.notInstalled("veilbox-experience-networking-tools")
	c := NewCatalogWith(dir, dnfops.NewWithRunner(f))

	if err := c.Remove("networking-tools"); err == nil {
		t.Fatal("expected not-installed error")
	}
}

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

func TestDesktopManifestValid(t *testing.T) {
	dir := t.TempDir()
	writeCatalog(t, dir, map[string]string{"niri-desktop.yaml": niriDesktopYAML})
	m, err := NewCatalogWith(dir, dnfops.NewWithRunner(newFakeRunner())).Load("niri-desktop")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if m.Type != TypeDesktop {
		t.Fatalf("type: %s", m.Type)
	}
	if m.DisplayName != "Niri Experience" {
		t.Fatalf("display_name: %q", m.DisplayName)
	}
	if m.Components[CompCompositor] != "niri" || m.Components[CompShell] != "noctalia" {
		t.Fatalf("components: %v", m.Components)
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestManifestValidateRejects(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want string
	}{
		{
			name: "unknown type",
			yaml: `name: x
type: de
`,
			want: "unknown type",
		},
		{
			name: "components on tooling",
			yaml: `name: x
type: tooling
components:
  compositor: niri
`,
			want: "components are only valid for type: desktop",
		},
		{
			name: "desktop without rpm",
			yaml: `name: x
type: desktop
components:
  compositor: niri
  shell: noctalia
  terminal: kitty
  display_manager: sddm
`,
			want: "must declare an installable rpm",
		},
		{
			name: "missing shell component",
			yaml: `name: x
type: desktop
rpm: veilbox-experience-x
components:
  compositor: niri
  terminal: kitty
  display_manager: sddm
`,
			want: `component "shell" is required`,
		},
		{
			name: "unknown component key",
			yaml: `name: x
type: desktop
rpm: veilbox-experience-x
components:
  compositor: niri
  shell: noctalia
  terminal: kitty
  display_manager: sddm
  composatator: niri
`,
			want: "unknown component",
		},
		{
			name: "shell metacharacter injection",
			yaml: `name: x
type: desktop
rpm: veilbox-experience-x
components:
  compositor: niri
  shell: "noctalia; rm -rf /"
  terminal: kitty
  display_manager: sddm
`,
			want: "invalid characters",
		},
		{
			name: "builtin structural component",
			yaml: `name: x
type: desktop
rpm: veilbox-experience-x
components:
  compositor: builtin
  shell: noctalia
  terminal: kitty
  display_manager: sddm
`,
			want: "must name a package",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeCatalog(t, dir, map[string]string{"x.yaml": tc.yaml})
			_, err := NewCatalogWith(dir, dnfops.NewWithRunner(newFakeRunner())).Load("x")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestDesktopCatalogEntryType(t *testing.T) {
	dir := t.TempDir()
	writeCatalog(t, dir, map[string]string{
		"networking-tools.yaml": networkingToolsYAML,
		"niri-desktop.yaml":     niriDesktopYAML,
	})
	f := newFakeRunner()
	f.responses[f.key("rpm", "-qa", "--queryformat", "%{NAME}\n")] = ""
	c := NewCatalogWith(dir, dnfops.NewWithRunner(f))

	entries, err := c.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	byName := map[string]Type{}
	for _, e := range entries {
		byName[e.Name] = e.Type
	}
	if byName["networking-tools"] == TypeDesktop {
		t.Fatalf("tooling experience must not be desktop type")
	}
	if byName["niri-desktop"] != TypeDesktop {
		t.Fatalf("desktop type: %s", byName["niri-desktop"])
	}
}
