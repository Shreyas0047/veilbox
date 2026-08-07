package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	catalogDir := t.TempDir()
	writeTestCatalog(t, catalogDir)
	f := &fakeCLIRunner{responses: map[string]string{}, errByCmd: map[string]error{}}
	dnf := dnfops.NewWithRunner(f)
	d := deps{
		newCatalog: func() *experience.Catalog {
			return experience.NewCatalogWith(catalogDir, dnf)
		},
		newDNF: func() *dnfops.System { return dnf },
	}
	return d, f
}

// fakeCLIRunner is a configurable runner for CLI-level tests.
type fakeCLIRunner struct {
	responses map[string]string
	errByCmd  map[string]error
}

func (f *fakeCLIRunner) key(name string, args ...string) string {
	return strings.Join(append([]string{name}, args...), " ")
}

func (f *fakeCLIRunner) Run(name string, args ...string) (string, error) {
	key := f.key(name, args...)
	if err := f.errByCmd[key]; err != nil {
		return "", err
	}
	return f.responses[key], nil
}

func (f *fakeCLIRunner) RunInteractive(name string, args ...string) error {
	return f.errByCmd[f.key(name, args...)]
}

func writeTestCatalog(t *testing.T, dir string) {
	t.Helper()
	os.MkdirAll(dir, 0o755)
	content := `name: networking-tools
description: Networking diagnostics toolkit for engineers.
rpm: veilbox-experience-networking-tools
`
	if err := os.WriteFile(filepath.Join(dir, "networking-tools.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

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
	regDir := filepath.Join(root, "profiles")
	os.MkdirAll(regDir, 0o755)
	os.WriteFile(filepath.Join(regDir, "devops.yaml"), []byte("name: devops\ndescription: x\n"), 0o644)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// Apply via the real registry dir by overriding VEILBOX_ROOT.
	t.Setenv("VEILBOX_ROOT", root)
	r := runCLI(t, deps{}, "profile", "apply", "devops")
	if r.code != 0 || !strings.Contains(r.stdout, "Profile \"devops\" applied") {
		t.Fatalf("code=%d out=%q err=%q", r.code, r.stdout, r.stderr)
	}

	r2 := runCLI(t, deps{}, "profile")
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
