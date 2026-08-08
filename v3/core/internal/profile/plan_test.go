package profile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Shreyas0047/veilbox/v3/core/internal/capability"
	"github.com/Shreyas0047/veilbox/v3/core/internal/dnfops"
	"github.com/Shreyas0047/veilbox/v3/core/internal/experience"
)

// fakeRunner records invocations and returns canned rpm output, so plan
// tests can drive catalog statuses without touching the system.
type fakeRunner struct {
	responses map[string]string
	errByCmd  map[string]error
}

func (f *fakeRunner) key(name string, args ...string) string {
	return strings.Join(append([]string{name}, args...), " ")
}

func (f *fakeRunner) Run(name string, args ...string) (string, error) {
	k := f.key(name, args...)
	if err := f.errByCmd[k]; err != nil {
		return f.responses[k], err
	}
	return f.responses[k], nil
}

func (f *fakeRunner) RunInteractive(name string, args ...string) error {
	return f.errByCmd[f.key(name, args...)]
}

const planDevopsYAML = `name: devops
description: Builds and operates delivery pipelines.
recommended_capabilities:
  - base-operations
  - networking
  - terminal-operations
optional_capabilities:
  - observability
`

// planFixture builds a capability registry + profile registry + catalog
// for plan tests.
//
// Catalog experiences:
//
//	base-ops, networking-tools, terminal-ops, observability-cli  installable
//	future-tools                                                 planned (no rpm)
//
// The returned runner simulates the RPM database: tests set
// responses["rpm -qa --queryformat %{NAME}\n"] to the installed
// veilbox-experience-* package list before calling Diff.
func planFixture(t *testing.T) (*Registry, *capability.Registry, *experience.Catalog, *fakeRunner) {
	t.Helper()
	root := t.TempDir()
	profilesDir := filepath.Join(root, "profiles")
	os.MkdirAll(profilesDir, 0o755)
	os.WriteFile(filepath.Join(profilesDir, "devops.yaml"), []byte(planDevopsYAML), 0o644)
	os.WriteFile(filepath.Join(profilesDir, "minimal.yaml"), []byte("name: minimal\ndescription: x\nrecommended_capabilities: [base-operations]\n"), 0o644)
	os.WriteFile(filepath.Join(profilesDir, "broken.yaml"), []byte("name: broken\ndescription: x\nrecommended_capabilities: [ghost-capability]\n"), 0o644)

	capDir := filepath.Join(root, "capabilities")
	os.MkdirAll(capDir, 0o755)
	for _, name := range []string{"base-operations", "networking", "terminal-operations", "observability", "future-capability"} {
		os.WriteFile(filepath.Join(capDir, name+".yaml"),
			[]byte("name: "+name+"\ndomain: test\ntier: core\ndescription: x.\n"), 0o644)
	}

	catDir := filepath.Join(root, "experiences")
	os.MkdirAll(catDir, 0o755)
	for name, def := range map[string][2]string{
		"base-ops":          {"base-operations", "veilbox-experience-base-ops"},
		"networking-tools":  {"networking", "veilbox-experience-networking-tools"},
		"terminal-ops":      {"terminal-operations", "veilbox-experience-terminal-ops"},
		"observability-cli": {"observability", "veilbox-experience-observability-cli"},
		"future-tools":      {"future-capability", ""},
	} {
		cap, rpm := def[0], def[1]
		content := "name: " + name + "\ndescription: x\ncapabilities:\n  - " + cap + "\n"
		if rpm != "" {
			content += "rpm: " + rpm + "\n"
		}
		os.WriteFile(filepath.Join(catDir, name+".yaml"), []byte(content), 0o644)
	}

	f := &fakeRunner{responses: map[string]string{}, errByCmd: map[string]error{}}
	f.responses["rpm -qa --queryformat %{NAME}\n"] = ""
	dnf := dnfops.NewWithRunner(f)
	return NewRegistryDir(profilesDir), capability.NewRegistryDir(capDir), experience.NewCatalogWith(catDir, dnf), f
}

// setInstalled simulates the RPM database containing the given
// veilbox-experience-* packages.
func setInstalled(f *fakeRunner, pkgs ...string) {
	f.responses["rpm -qa --queryformat %{NAME}\n"] = strings.Join(pkgs, "\n")
}

func TestDiffNoneInstalled(t *testing.T) {
	reg, capReg, cat, f := planFixture(t)
	setInstalled(f)

	p, err := Diff(reg, capReg, cat, "devops")
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if p.Profile != "devops" {
		t.Fatalf("profile: %q", p.Profile)
	}
	if len(p.RecommendedCaps) != 3 || p.RecommendedCaps[0] != "base-operations" {
		t.Fatalf("recommended caps: %v", p.RecommendedCaps)
	}
	if len(p.MissingRecommended) != 3 || p.MissingRecommended[0] != "base-ops" {
		t.Fatalf("missing: %v", p.MissingRecommended)
	}
	if len(p.Satisfied) != 0 || len(p.OptionalInstalled) != 0 {
		t.Fatalf("unexpected satisfied: %+v", p)
	}
	if len(p.OptionalMissing) != 1 || p.OptionalMissing[0] != "observability-cli" {
		t.Fatalf("optional missing: %v", p.OptionalMissing)
	}
	if p.Synced() {
		t.Fatal("should not be synced")
	}
	if got := SyncPlan(p); len(got) != 3 || got[0] != "base-ops" {
		t.Fatalf("sync plan: %v", got)
	}
}

func TestDiffSomeInstalled(t *testing.T) {
	reg, capReg, cat, f := planFixture(t)
	setInstalled(f, "veilbox-experience-base-ops")

	p, err := Diff(reg, capReg, cat, "devops")
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if len(p.Satisfied) != 1 || p.Satisfied[0] != "base-ops" {
		t.Fatalf("satisfied: %v", p.Satisfied)
	}
	if len(p.MissingRecommended) != 2 {
		t.Fatalf("missing: %v", p.MissingRecommended)
	}
}

func TestDiffNotInstallableAndUnknownCapability(t *testing.T) {
	reg, capReg, cat, f := planFixture(t)
	setInstalled(f)
	// A profile recommending the planned future-capability (its
	// experience has no package yet) and a nonexistent capability.
	os.WriteFile(filepath.Join(reg.Dir(), "edge.yaml"),
		[]byte("name: edge\ndescription: x\nrecommended_capabilities: [future-capability, ghost-capability]\n"), 0o644)

	p, err := Diff(reg, capReg, cat, "edge")
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if len(p.NotInstallable) != 1 || p.NotInstallable[0] != "future-tools" {
		t.Fatalf("not installable: %v", p.NotInstallable)
	}
	if len(p.UnknownCapabilities) != 1 || p.UnknownCapabilities[0] != "ghost-capability" {
		t.Fatalf("unknown capabilities: %v", p.UnknownCapabilities)
	}
	if p.Synced() {
		t.Fatal("edge profile must not be synced")
	}
	if got := SyncPlan(p); len(got) != 0 {
		t.Fatalf("sync plan must exclude planned/unknown, got %v", got)
	}
}

func TestDiffExtrasPreserved(t *testing.T) {
	reg, capReg, cat, f := planFixture(t)
	// minimal references only base-operations; observability-cli is
	// installed but unreferenced. A package without a catalog entry is
	// invisible to the plan (the catalog is the universe of
	// experiences).
	setInstalled(f,
		"veilbox-experience-base-ops",
		"veilbox-experience-observability-cli",
		"veilbox-experience-no-catalog-entry",
	)

	p, err := Diff(reg, capReg, cat, "minimal")
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if len(p.Extras) != 1 || p.Extras[0] != "observability-cli" {
		t.Fatalf("extras: %v", p.Extras)
	}
	// Extras never affect sync state: everything recommended is installed.
	if !p.Synced() {
		t.Fatal("should be synced despite extras")
	}
	if got := SyncPlan(p); len(got) != 0 {
		t.Fatalf("sync plan must not include extras: %v", got)
	}
}

func TestDiffDeterministic(t *testing.T) {
	reg, capReg, cat, f := planFixture(t)
	setInstalled(f, "veilbox-experience-base-ops", "veilbox-experience-observability-cli")

	a, err := Diff(reg, capReg, cat, "devops")
	if err != nil {
		t.Fatal(err)
	}
	b, err := Diff(reg, capReg, cat, "devops")
	if err != nil {
		t.Fatal(err)
	}
	if a.String() != b.String() {
		t.Fatalf("diff not deterministic:\n%+v\n%+v", a, b)
	}
}

func TestCheckCapabilities(t *testing.T) {
	reg, capReg, cat, f := planFixture(t)
	setInstalled(f)

	missing, err := CheckCapabilities(reg, capReg, cat, "devops")
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 0 {
		t.Fatalf("devops should have no unknown refs: %v", missing)
	}

	missing, err = CheckCapabilities(reg, capReg, cat, "broken")
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 1 || missing[0] != "ghost-capability" {
		t.Fatalf("broken refs: %v", missing)
	}
}

// String renders the plan fields in a stable order for comparisons.
func (p Plan) String() string {
	return strings.Join([]string{
		p.Profile,
		"reccaps:" + strings.Join(p.RecommendedCaps, ","),
		"missing:" + strings.Join(p.MissingRecommended, ","),
		"notinstallable:" + strings.Join(p.NotInstallable, ","),
		"unknowncaps:" + strings.Join(p.UnknownCapabilities, ","),
		"unknownrec:" + strings.Join(p.UnknownRecommended, ","),
		"satisfied:" + strings.Join(p.Satisfied, ","),
		"optinst:" + strings.Join(p.OptionalInstalled, ","),
		"optmiss:" + strings.Join(p.OptionalMissing, ","),
		"extras:" + strings.Join(p.Extras, ","),
	}, "|")
}
