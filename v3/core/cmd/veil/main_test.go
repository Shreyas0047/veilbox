package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Shreyas0047/veilbox/v3/core/internal/capability"
	"github.com/Shreyas0047/veilbox/v3/core/internal/dnfops"
	"github.com/Shreyas0047/veilbox/v3/core/internal/experience"
)

type cliResult struct {
	code   int
	stdout string
	stderr string
}

func runCLI(t *testing.T, d deps, args ...string) cliResult {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(args, &out, &errb, d)
	return cliResult{code: code, stdout: out.String(), stderr: errb.String()}
}

func fakeDeps(t *testing.T) (deps, *fakeCLIRunner) {
	t.Helper()
	root := t.TempDir()
	catalogDir := filepath.Join(root, "experiences")
	capDir := filepath.Join(root, "capabilities")
	writeTestCatalog(t, catalogDir, capDir)
	f := &fakeCLIRunner{responses: map[string]string{}, errByCmd: map[string]error{}}
	dnf := dnfops.NewWithRunner(f)
	d := deps{
		newCatalog: func() *experience.Catalog {
			return experience.NewCatalogWith(catalogDir, dnf)
		},
		newDNF: func() *dnfops.System { return dnf },
		newCapabilities: func() *capability.Registry {
			return capability.NewRegistryDir(capDir)
		},
	}
	return d, f
}

// fakeCLIRunner is a configurable runner that records every invocation
// so tests can assert what transactions Veilbox would run.
type fakeCLIRunner struct {
	responses   map[string]string
	errByCmd    map[string]error
	calls       [][]string
	interactive [][]string
}

func (f *fakeCLIRunner) key(name string, args ...string) string {
	return strings.Join(append([]string{name}, args...), " ")
}

func (f *fakeCLIRunner) Run(name string, args ...string) (string, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
	key := f.key(name, args...)
	if err := f.errByCmd[key]; err != nil {
		return f.responses[key], err
	}
	return f.responses[key], nil
}

func (f *fakeCLIRunner) RunInteractive(name string, args ...string) error {
	f.interactive = append(f.interactive, append([]string{name}, args...))
	return f.errByCmd[f.key(name, args...)]
}

// joined returns "name args..." for every recorded invocation;
// interactive transactions are prefixed with "interactive:".
func (f *fakeCLIRunner) joined() []string {
	out := make([]string, 0, len(f.calls)+len(f.interactive))
	for _, c := range f.calls {
		out = append(out, strings.Join(c, " "))
	}
	for _, c := range f.interactive {
		out = append(out, "interactive:"+strings.Join(c, " "))
	}
	return out
}

// notInstalled simulates rpm -q reporting a package as missing.
func (f *fakeCLIRunner) notInstalled(pkg string) {
	f.responses[f.key("rpm", "-q", pkg)] = "package " + pkg + " is not installed"
	f.errByCmd[f.key("rpm", "-q", pkg)] = errors.New("exit status 1")
}

func writeTestCatalog(t *testing.T, dir, capDir string) {
	t.Helper()
	os.MkdirAll(dir, 0o755)
	os.MkdirAll(capDir, 0o755)
	for name, content := range map[string]string{
		"base-operations.yaml":     "name: base-operations\ndomain: fundamentals\ntier: core\ndescription: Essential system operations.\n",
		"networking.yaml":          "name: networking\ndomain: networking\ntier: core\ndescription: Network diagnostics.\n",
		"terminal-operations.yaml": "name: terminal-operations\ndomain: terminal\ntier: core\ndescription: Terminal operations.\n",
	} {
		if err := os.WriteFile(filepath.Join(capDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	content := `name: networking-tools
description: Networking diagnostics toolkit for engineers.
rpm: veilbox-experience-networking-tools
capabilities:
  - networking
packages:
  - bind-utils
  - tcpdump
`
	if err := os.WriteFile(filepath.Join(dir, "networking-tools.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	planned := `name: terminal-ops
description: Terminal operations toolkit. Planned.
capabilities:
  - terminal-operations
`
	if err := os.WriteFile(filepath.Join(dir, "terminal-ops.yaml"), []byte(planned), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeProfile writes a profile manifest under the given VEILBOX_ROOT.
func writeProfile(t *testing.T, root, name, content string) {
	t.Helper()
	dir := filepath.Join(root, "profiles")
	os.MkdirAll(dir, 0o755)
	if err := os.WriteFile(filepath.Join(dir, name+".yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const cliDevopsYAML = `name: devops
display_name: DevOps Engineer
description: Builds and operates delivery pipelines.
role: devops
recommended_capabilities:
  - networking
optional_capabilities:
  - terminal-operations
tags: [ci, delivery]
workspace_preferences:
  shell: bash
  prompt: veilbox
  aliases:
    k: kubectl
`

func TestVersion(t *testing.T) {
	r := runCLI(t, deps{}, "version")
	if r.code != 0 || !strings.Contains(r.stdout, "veil "+version) {
		t.Fatalf("code=%d out=%q", r.code, r.stdout)
	}
}

func TestUnknownCommand(t *testing.T) {
	r := runCLI(t, deps{}, "frobnicate")
	if r.code != 2 {
		t.Fatalf("code=%d", r.code)
	}
}

func TestNoProfileReported(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	r := runCLI(t, deps{}, "profile")
	if r.code != 0 || !strings.Contains(r.stdout, "No profile configured") {
		t.Fatalf("code=%d out=%q", r.code, r.stdout)
	}
}

func TestProfileApply(t *testing.T) {
	root := t.TempDir()
	writeProfile(t, root, "devops", cliDevopsYAML)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("VEILBOX_ROOT", root)
	d, f := fakeDeps(t)
	f.responses[f.key("rpm", "-qa", "--queryformat", "%{NAME}\n")] = ""

	r := runCLI(t, d, "profile", "apply", "devops")
	if r.code != 0 || !strings.Contains(r.stdout, "Profile \"devops\" applied") {
		t.Fatalf("code=%d out=%q err=%q", r.code, r.stdout, r.stderr)
	}
	// apply must present the recommendation summary without installing.
	if !strings.Contains(r.stdout, "recommends the following capabilities") {
		t.Fatalf("no recommendation summary in %q", r.stdout)
	}
	if !strings.Contains(r.stdout, "Nothing was installed") {
		t.Fatalf("apply must not install: %q", r.stdout)
	}

	r2 := runCLI(t, d, "profile")
	if !strings.Contains(r2.stdout, "Profile: devops") {
		t.Fatalf("out=%q", r2.stdout)
	}
}

func TestProfileApplyUnknown(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	r := runCLI(t, deps{}, "profile", "apply", "nope")
	if r.code != 1 || r.stderr == "" {
		t.Fatalf("code=%d stderr=%q", r.code, r.stderr)
	}
}

func TestProfileListMarksActive(t *testing.T) {
	root := t.TempDir()
	writeProfile(t, root, "devops", cliDevopsYAML)
	writeProfile(t, root, "sre", "name: sre\ndescription: keeps systems reliable.\n")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("VEILBOX_ROOT", root)
	d, f := fakeDeps(t)
	f.responses[f.key("rpm", "-qa", "--queryformat", "%{NAME}\n")] = ""

	if r := runCLI(t, d, "profile", "apply", "devops"); r.code != 0 {
		t.Fatalf("apply: %q", r.stderr)
	}
	r := runCLI(t, d, "profile", "list")
	if r.code != 0 {
		t.Fatalf("code=%d", r.code)
	}
	if !strings.Contains(r.stdout, "devops (active)") || !strings.Contains(r.stdout, "sre") {
		t.Fatalf("out=%q", r.stdout)
	}
}

func TestProfileShow(t *testing.T) {
	root := t.TempDir()
	writeProfile(t, root, "devops", cliDevopsYAML)
	t.Setenv("VEILBOX_ROOT", root)
	d, f := fakeDeps(t)
	f.responses[f.key("rpm", "-qa", "--queryformat", "%{NAME}\n")] = ""

	r := runCLI(t, d, "profile", "show", "devops")
	if r.code != 0 {
		t.Fatalf("code=%d err=%q", r.code, r.stderr)
	}
	for _, want := range []string{
		"DevOps Engineer",
		"Builds and operates delivery pipelines.",
		"Recommended capabilities:",
		"networking",
		"Optional capabilities:",
		"shell: bash",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Fatalf("missing %q in %q", want, r.stdout)
		}
	}
}

func TestProfileShowUnknown(t *testing.T) {
	r := runCLI(t, deps{}, "profile", "show", "nope")
	if r.code != 1 || r.stderr == "" {
		t.Fatalf("code=%d stderr=%q", r.code, r.stderr)
	}
}

func TestProfileDiff(t *testing.T) {
	root := t.TempDir()
	writeProfile(t, root, "devops", cliDevopsYAML)
	t.Setenv("VEILBOX_ROOT", root)
	d, f := fakeDeps(t)
	f.responses[f.key("rpm", "-qa", "--queryformat", "%{NAME}\n")] = ""

	r := runCLI(t, d, "profile", "diff", "devops")
	if r.code != 0 {
		t.Fatalf("code=%d err=%q", r.code, r.stderr)
	}
	if !strings.Contains(r.stdout, "Missing recommended experiences:") ||
		!strings.Contains(r.stdout, "- networking-tools") {
		t.Fatalf("missing section in %q", r.stdout)
	}
	if !strings.Contains(r.stdout, "1 recommended experience(s) missing") {
		t.Fatalf("missing summary in %q", r.stdout)
	}

	// Deterministic: identical output on re-run.
	r2 := runCLI(t, d, "profile", "diff", "devops")
	if r.stdout != r2.stdout {
		t.Fatal("diff output not deterministic")
	}
}

func TestProfileDiffSatisfied(t *testing.T) {
	root := t.TempDir()
	writeProfile(t, root, "devops", cliDevopsYAML)
	t.Setenv("VEILBOX_ROOT", root)
	d, f := fakeDeps(t)
	f.responses[f.key("rpm", "-qa", "--queryformat", "%{NAME}\n")] = "veilbox-experience-networking-tools\n"

	r := runCLI(t, d, "profile", "diff", "devops")
	if r.code != 0 {
		t.Fatalf("code=%d err=%q", r.code, r.stderr)
	}
	if !strings.Contains(r.stdout, "Already satisfied:") ||
		!strings.Contains(r.stdout, "- networking-tools") {
		t.Fatalf("out=%q", r.stdout)
	}
	if !strings.Contains(r.stdout, "Profile is synced.") {
		t.Fatalf("out=%q", r.stdout)
	}
}

func TestProfileDiffUnknown(t *testing.T) {
	r := runCLI(t, deps{}, "profile", "diff", "nope")
	if r.code != 1 || r.stderr == "" {
		t.Fatalf("code=%d stderr=%q", r.code, r.stderr)
	}
}

func TestProfileSyncNoActive(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	r := runCLI(t, deps{}, "profile", "sync", "--yes")
	if r.code != 1 || !strings.Contains(r.stderr, "no active profile") {
		t.Fatalf("code=%d stderr=%q", r.code, r.stderr)
	}
}

func TestProfileSyncInstalls(t *testing.T) {
	root := t.TempDir()
	writeProfile(t, root, "devops", cliDevopsYAML)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("VEILBOX_ROOT", root)
	d, f := fakeDeps(t)
	f.responses[f.key("rpm", "-qa", "--queryformat", "%{NAME}\n")] = ""
	f.notInstalled("veilbox-experience-networking-tools")

	if r := runCLI(t, d, "profile", "apply", "devops"); r.code != 0 {
		t.Fatalf("apply: %q", r.stderr)
	}
	r := runCLI(t, d, "profile", "sync", "--yes")
	if r.code != 0 {
		t.Fatalf("code=%d err=%q", r.code, r.stderr)
	}
	if !strings.Contains(r.stdout, "Installed networking-tools") ||
		!strings.Contains(r.stdout, "Profile sync complete: 1 installed") {
		t.Fatalf("out=%q", r.stdout)
	}
	joined := f.joined()
	found := false
	for _, c := range joined {
		if c == "interactive:sudo dnf install -y veilbox-experience-networking-tools" {
			found = true
		}
	}
	if !found {
		t.Fatalf("no DNF install transaction recorded: %v", joined)
	}
}

func TestProfileSyncDoesNotRemoveExtras(t *testing.T) {
	root := t.TempDir()
	writeProfile(t, root, "devops", cliDevopsYAML)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("VEILBOX_ROOT", root)
	d, f := fakeDeps(t)
	// networking-tools installed plus an unrelated extra experience
	// (not in the test catalog, so invisible to the plan — but the
	// assertion below is that no remove ever happens).
	f.responses[f.key("rpm", "-qa", "--queryformat", "%{NAME}\n")] =
		"veilbox-experience-networking-tools\nveilbox-experience-extra\n"

	if r := runCLI(t, d, "profile", "apply", "devops"); r.code != 0 {
		t.Fatalf("apply: %q", r.stderr)
	}
	r := runCLI(t, d, "profile", "sync", "--yes")
	if r.code != 0 {
		t.Fatalf("code=%d err=%q", r.code, r.stderr)
	}
	if !strings.Contains(r.stdout, "already synced") {
		t.Fatalf("out=%q", r.stdout)
	}
	for _, c := range f.joined() {
		if strings.Contains(c, "remove") {
			t.Fatalf("sync must never remove: %v", f.joined())
		}
	}
}

func TestExperienceList(t *testing.T) {
	d, _ := fakeDeps(t)
	r := runCLI(t, d, "experience", "list")
	if r.code != 0 || !strings.Contains(r.stdout, "networking-tools") || !strings.Contains(r.stdout, "available") {
		t.Fatalf("code=%d out=%q", r.code, r.stdout)
	}
}

func TestExperienceInstallUsage(t *testing.T) {
	d, _ := fakeDeps(t)
	r := runCLI(t, d, "experience", "install")
	if r.code != 2 {
		t.Fatalf("code=%d", r.code)
	}
}

func TestExperienceListShowsInstalled(t *testing.T) {
	d, f := fakeDeps(t)
	f.responses[f.key("rpm", "-qa", "--queryformat", "%{NAME}\n")] = "veilbox-experience-networking-tools\n"
	r := runCLI(t, d, "experience", "list")
	if r.code != 0 || !strings.Contains(r.stdout, "installed") {
		t.Fatalf("code=%d out=%q", r.code, r.stdout)
	}
}

func TestCapabilityList(t *testing.T) {
	root := t.TempDir()
	writeProfile(t, root, "devops", cliDevopsYAML)
	t.Setenv("VEILBOX_ROOT", root)
	d, f := fakeDeps(t)
	f.responses[f.key("rpm", "-qa", "--queryformat", "%{NAME}\n")] = ""

	r := runCLI(t, d, "capability", "list")
	if r.code != 0 {
		t.Fatalf("code=%d err=%q", r.code, r.stderr)
	}
	for _, want := range []string{"base-operations", "networking", "networking-tools", "CAPABILITY"} {
		if !strings.Contains(r.stdout, want) {
			t.Fatalf("missing %q in %q", want, r.stdout)
		}
	}
}

func TestCapabilityInfo(t *testing.T) {
	root := t.TempDir()
	writeProfile(t, root, "devops", cliDevopsYAML)
	t.Setenv("VEILBOX_ROOT", root)
	d, f := fakeDeps(t)
	f.responses[f.key("rpm", "-qa", "--queryformat", "%{NAME}\n")] = ""

	r := runCLI(t, d, "capability", "info", "networking")
	if r.code != 0 {
		t.Fatalf("code=%d err=%q", r.code, r.stderr)
	}
	for _, want := range []string{
		"Capability: networking",
		"Domain: networking",
		"Implementing experiences:",
		"networking-tools",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Fatalf("missing %q in %q", want, r.stdout)
		}
	}
}

func TestCapabilityInfoUnknown(t *testing.T) {
	d, _ := fakeDeps(t)
	r := runCLI(t, d, "capability", "info", "nope")
	if r.code != 1 || r.stderr == "" {
		t.Fatalf("code=%d stderr=%q", r.code, r.stderr)
	}
}

func TestExperienceInfo(t *testing.T) {
	root := t.TempDir()
	writeProfile(t, root, "devops", cliDevopsYAML)
	writeProfile(t, root, "sre", "name: sre\ndescription: x\n")
	t.Setenv("VEILBOX_ROOT", root)
	d, f := fakeDeps(t)
	f.responses[f.key("rpm", "-qa", "--queryformat", "%{NAME}\n")] = ""

	r := runCLI(t, d, "experience", "info", "networking-tools")
	if r.code != 0 {
		t.Fatalf("code=%d err=%q", r.code, r.stderr)
	}
	for _, want := range []string{
		"Experience: networking-tools",
		"Status: available",
		"Package: veilbox-experience-networking-tools",
		"Recommended by profiles:",
		"- devops",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Fatalf("missing %q in %q", want, r.stdout)
		}
	}
	if strings.Contains(r.stdout, "- sre") {
		t.Fatalf("sre must not recommend networking-tools: %q", r.stdout)
	}
}

func TestExperienceInfoUnknown(t *testing.T) {
	d, _ := fakeDeps(t)
	r := runCLI(t, d, "experience", "info", "nope")
	if r.code != 1 || !strings.Contains(r.stderr, "not found") {
		t.Fatalf("code=%d stderr=%q", r.code, r.stderr)
	}
}

func TestStatusSyncLine(t *testing.T) {
	root := t.TempDir()
	writeProfile(t, root, "devops", cliDevopsYAML)
	t.Setenv("VEILBOX_ROOT", root)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	d, f := fakeDeps(t)
	f.responses[f.key("rpm", "-qa", "--queryformat", "%{NAME}\n")] = ""

	if r := runCLI(t, d, "profile", "apply", "devops"); r.code != 0 {
		t.Fatalf("apply: %q", r.stderr)
	}
	r := runCLI(t, d, "status")
	if r.code != 0 {
		t.Fatalf("code=%d err=%q", r.code, r.stderr)
	}
	if !strings.Contains(r.stdout, "Profile sync:   missing 1 recommended experience(s)") {
		t.Fatalf("out=%q", r.stdout)
	}

	// Once installed, the same status reports synced.
	f.responses[f.key("rpm", "-qa", "--queryformat", "%{NAME}\n")] = "veilbox-experience-networking-tools\n"
	r = runCLI(t, d, "status")
	if !strings.Contains(r.stdout, "Profile sync:   synced") {
		t.Fatalf("out=%q", r.stdout)
	}
}

func TestDoctorPassesWithProfileChecks(t *testing.T) {
	root := t.TempDir()
	writeProfile(t, root, "devops", cliDevopsYAML)
	t.Setenv("VEILBOX_ROOT", root)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	d, f := fakeDeps(t)
	f.responses[f.key("rpm", "-qa", "--queryformat", "%{NAME}\n")] = ""
	f.notInstalled("veilbox-experience-networking-tools")
	f.responses[f.key("dnf", "repolist")] = "repo id    repo name\nfedora     Fedora 44 - x86_64\nveilbox-dev Veilbox Development Repository\n"

	if r := runCLI(t, d, "profile", "apply", "devops"); r.code != 0 {
		t.Fatalf("apply: %q", r.stderr)
	}
	r := runCLI(t, d, "doctor")
	// The DNF-available check probes the real host PATH; inside a mock
	// buildroot dnf is absent, so the doctor legitimately fails there.
	// When dnf exists the doctor must pass cleanly.
	for _, want := range []string{
		"Profile state parses",
		"Active profile exists",
		"Profile manifests valid",
		"Profile capabilities resolve to experiences",
		"DNF repositories reachable",
		"Veilbox repository configured",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Fatalf("missing %q in %q", want, r.stdout)
		}
	}
	if dnfops.Available(dnfops.DNFBinary) {
		if r.code != 0 {
			t.Fatalf("code=%d out=%q err=%q", r.code, r.stdout, r.stderr)
		}
		if !strings.Contains(r.stdout, "All critical checks passed") {
			t.Fatalf("out=%q", r.stdout)
		}
	}
}
