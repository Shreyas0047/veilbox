package capability

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Shreyas0047/veilbox/v3/core/internal/dnfops"
	"github.com/Shreyas0047/veilbox/v3/core/internal/experience"
)

const testNetworkingYAML = `name: networking
domain: networking
tier: core
description: Network diagnostics.
`

const testContainersYAML = `name: containers
domain: containers
tier: core
description: Container tooling.
`

// testRunner is a trivial dnfops.Runner returning no packages.
type testRunner struct{}

func (testRunner) Run(name string, args ...string) (string, error)  { return "", nil }
func (testRunner) RunInteractive(name string, args ...string) error { return nil }

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func testCatalog(t *testing.T, dir string) *experience.Catalog {
	t.Helper()
	writeFiles(t, dir, map[string]string{
		"networking-tools.yaml": `name: networking-tools
description: Networking tools.
rpm: veilbox-experience-networking-tools
capabilities:
  - networking
packages:
  - bind-utils
`,
		"container-tools.yaml": `name: container-tools
description: Container tools.
rpm: veilbox-experience-container-tools
capabilities:
  - containers
packages:
  - podman
`,
		"future-tools.yaml": `name: future-tools
description: Planned experience.
capabilities:
  - networking
`,
		"niri-desktop.yaml": `name: niri-desktop
type: environment
description: Environment experience.
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
capabilities:
  - desktop
packages:
  - niri
`,
	})
	return experience.NewCatalogWith(dir, dnfops.NewWithRunner(testRunner{}))
}

func TestRegistryListAndLoad(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"networking.yaml": testNetworkingYAML,
		"containers.yaml": testContainersYAML,
	})
	reg := NewRegistryDir(dir)
	names, err := reg.List()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(names, ",") != "containers,networking" {
		t.Fatalf("list: %v", names)
	}
	m, err := reg.Load("networking")
	if err != nil {
		t.Fatal(err)
	}
	if m.Domain != "networking" || m.Tier != "core" {
		t.Fatalf("manifest: %+v", m)
	}
}

func TestRegistryLoadUnknown(t *testing.T) {
	if _, err := NewRegistryDir(t.TempDir()).Load("nope"); err == nil {
		t.Fatal("expected error for unknown capability")
	}
}

func TestRegistryMissingBase(t *testing.T) {
	// A catalog without base-operations must fail the resolver's base
	// check (doctor surfaces it).
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"networking.yaml": testNetworkingYAML})
	if _, err := NewResolver(NewRegistryDir(dir), testCatalog(t, t.TempDir())); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRegistryDir(dir).Load(BaseName); err == nil {
		t.Fatal("base-operations must be absent here")
	}
}

func TestResolverExperiencesFor(t *testing.T) {
	capDir := t.TempDir()
	writeFiles(t, capDir, map[string]string{
		"base-operations.yaml": "name: base-operations\ndomain: fundamentals\ntier: core\ndescription: base.\n",
		"networking.yaml":      testNetworkingYAML,
		"containers.yaml":      testContainersYAML,
	})
	cat := testCatalog(t, t.TempDir())
	res, err := NewResolver(NewRegistryDir(capDir), cat)
	if err != nil {
		t.Fatal(err)
	}

	// networking maps to two experiences: the installable one plus the
	// planned one (both derivable; status handled downstream).
	got, err := res.ExperiencesFor([]string{"networking"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "future-tools,networking-tools" {
		t.Fatalf("experiences: %v", got)
	}

	// Environment experiences are never derived.
	got, err = res.ExperiencesFor([]string{"desktop"})
	if err == nil {
		t.Fatal("desktop capability has no manifest; expected validation error")
	}
}

func TestResolverInverseAndSeed(t *testing.T) {
	capDir := t.TempDir()
	writeFiles(t, capDir, map[string]string{
		"base-operations.yaml": "name: base-operations\ndomain: fundamentals\ntier: core\ndescription: base.\n",
		"networking.yaml":      testNetworkingYAML,
		"containers.yaml":      testContainersYAML,
	})
	res, err := NewResolver(NewRegistryDir(capDir), testCatalog(t, t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}

	// Inverse: experiences → capabilities (v1 backfill).
	caps := res.CapabilitiesOf([]string{"networking-tools", "container-tools"})
	if strings.Join(caps, ",") != "containers,networking" {
		t.Fatalf("capabilities of: %v", caps)
	}

	// Seed: recommended + base only. Optional capabilities are offered,
	// never pre-selected (the SRE demo: security is optional and the
	// engineer adds it).
	seeded, err := res.Seed([]string{"containers", "networking"}, []string{"networking"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{BaseName, "containers", "networking"}
	if strings.Join(seeded, ",") != strings.Join(want, ",") {
		t.Fatalf("seed: %v", seeded)
	}
}

func TestResolverValidateUnknown(t *testing.T) {
	capDir := t.TempDir()
	writeFiles(t, capDir, map[string]string{
		"base-operations.yaml": "name: base-operations\ndomain: fundamentals\ntier: core\ndescription: base.\n",
	})
	res, err := NewResolver(NewRegistryDir(capDir), testCatalog(t, t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	unknown, err := res.Validate([]string{BaseName, "ghost"})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if len(unknown) != 1 || unknown[0] != "ghost" {
		t.Fatalf("unknown: %v", unknown)
	}
}

func TestCheckMappingPlanned(t *testing.T) {
	capDir := t.TempDir()
	writeFiles(t, capDir, map[string]string{
		"base-operations.yaml": "name: base-operations\ndomain: fundamentals\ntier: core\ndescription: base.\n",
		"networking.yaml":      testNetworkingYAML,
		"containers.yaml":      testContainersYAML,
		"desktop.yaml":         "name: desktop\ndomain: environment\ntier: core\ndescription: desktop.\n",
	})
	res, err := NewResolver(NewRegistryDir(capDir), testCatalog(t, t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	planned, unknown := res.CheckMapping()
	// networking-tools and future-tools both carry the networking
	// reference; all referenced capabilities exist, so no unknowns.
	if len(unknown) != 0 {
		t.Fatalf("unknown refs: %v", unknown)
	}
	// networking has an installable experience; base-operations and
	// desktop (desktop experiences excluded) have none.
	if containsStr(planned, "networking") {
		t.Fatalf("networking must not be planned: %v", planned)
	}
	for _, want := range []string{BaseName, "desktop"} {
		if !containsStr(planned, want) {
			t.Fatalf("expected %s planned: %v", want, planned)
		}
	}
}

func containsStr(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
