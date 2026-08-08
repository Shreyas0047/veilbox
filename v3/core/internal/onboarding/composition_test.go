package onboarding

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Shreyas0047/veilbox/v3/core/internal/capability"
	"github.com/Shreyas0047/veilbox/v3/core/internal/dnfops"
	"github.com/Shreyas0047/veilbox/v3/core/internal/environment"
	"github.com/Shreyas0047/veilbox/v3/core/internal/experience"
	"github.com/Shreyas0047/veilbox/v3/core/internal/onboarding/onboardingtest"
	"github.com/Shreyas0047/veilbox/v3/core/internal/profile"
	"github.com/Shreyas0047/veilbox/v3/core/internal/settings"
	"github.com/Shreyas0047/veilbox/v3/core/internal/workspace"
)

// setupCompositionOnboarding mirrors setupOnboarding: isolated roots
// and fake runners, with the environment engine wired so Apply can
// run its full stage list.
func setupCompositionOnboarding(t *testing.T) (PlanInputs, *onboardingtest.Runner) {
	t.Helper()
	env := onboardingtest.SetupEnv(t)
	f := onboardingtest.New()
	dnf := dnfops.NewWithRunner(f)
	in := PlanInputs{
		Registry:     profile.NewRegistryDir(env.ProfDir),
		Catalog:      experience.NewCatalogWith(env.CatDir, dnf),
		Capabilities: capability.NewRegistryDir(env.CapDir),
		Workspace:    &workspace.Engine{LookPath: func(string) (string, error) { return "/usr/bin/vim", nil }},
		Environment:  environment.NewWith(experience.NewCatalogWith(env.CatDir, dnf), environment.NewSystemWith(f)),
	}
	return in, f
}

func TestCompositionRoundTripAtomic(t *testing.T) {
	onboardingtest.SetupEnv(t)
	dir, err := settings.StateDir()
	if err != nil {
		t.Fatal(err)
	}
	c := Composition{
		SchemaVersion: CompositionSchemaVersion,
		AppliedAt:     "2026-08-08T00:00:00Z",
		Profile:       "cloud-engineer",
		Capabilities:  []string{"networking", capability.BaseName},
		Experiences:   []string{"networking-tools"},
		Environment: &EnvironmentRecord{
			Name:       "niri-desktop",
			RPM:        "veilbox-experience-niri",
			Components: map[string]string{"compositor": "niri"},
		},
		Validation: ValidationRecord{Valid: true, Notes: []string{"ok"}},
	}
	if err := c.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, CompositionFile)); err != nil {
		t.Fatalf("composition file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, CompositionFile+".tmp")); !os.IsNotExist(err) {
		t.Fatalf("temp file left behind: %v", err)
	}

	got, err := LoadComposition()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.SchemaVersion != CompositionSchemaVersion || got.Profile != "cloud-engineer" ||
		got.Environment == nil || got.Environment.RPM != "veilbox-experience-niri" {
		t.Fatalf("round trip mismatch: %+v", got)
	}
	if !got.Validation.Valid || len(got.Validation.Notes) == 0 {
		t.Fatalf("validation lost: %+v", got.Validation)
	}

	// Corrupt records must be reported, never guessed at.
	if err := os.WriteFile(filepath.Join(dir, CompositionFile), []byte("{nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadComposition(); err == nil {
		t.Fatal("corrupt composition must error")
	}
}

func TestCompositionMissingIsEmpty(t *testing.T) {
	onboardingtest.SetupEnv(t)
	if err := os.Remove(CompositionPathMust(t)); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	got, err := LoadComposition()
	if err != nil {
		t.Fatalf("missing composition must not error: %v", err)
	}
	if got.SchemaVersion != 0 || got.AppliedAt != "" || got.Environment != nil {
		t.Fatalf("missing composition must be empty: %+v", got)
	}
}

func TestCompositionRecordedByApply(t *testing.T) {
	in, f := setupCompositionOnboarding(t)
	f.Responses[f.Key("systemctl", "get-default")] = "multi-user.target"

	sel := Selection{
		Profile:      "cloud-engineer",
		Capabilities: []string{"networking", capability.BaseName},
		Environment:  "niri-desktop",
		Workspace:    WorkspacePrefs{Editor: "vim", Prompt: "veilbox", Terminal: "system"},
	}
	if err := sel.Derive(in.Resolver()); err != nil {
		t.Fatal(err)
	}
	res, err := Apply(sel, in)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !res.Success {
		t.Fatalf("apply not successful: %+v", res.Stages)
	}

	comp, err := LoadComposition()
	if err != nil {
		t.Fatalf("composition not readable: %v", err)
	}
	if comp.SchemaVersion != CompositionSchemaVersion {
		t.Fatalf("schema version: %d", comp.SchemaVersion)
	}
	if comp.Profile != "cloud-engineer" {
		t.Fatalf("profile: %q", comp.Profile)
	}
	if !contains(comp.Experiences, "networking-tools") {
		t.Fatalf("experiences: %v", comp.Experiences)
	}
	if comp.Environment == nil || comp.Environment.Name != "niri-desktop" ||
		comp.Environment.Components[experience.CompCompositor] != "niri" {
		t.Fatalf("environment record: %+v", comp.Environment)
	}
	if comp.Workspace.Editor != "vim" || comp.Workspace.Prompt != "veilbox" {
		t.Fatalf("workspace record: %+v", comp.Workspace)
	}
	if !comp.Validation.Valid {
		t.Fatalf("composition must validate: %+v", comp.Validation.Notes)
	}
}

func TestCompositionSurvivesFailedApply(t *testing.T) {
	in, f := setupCompositionOnboarding(t)
	f.Responses[f.Key("systemctl", "get-default")] = "multi-user.target"

	ok := Selection{Profile: "cloud-engineer", Capabilities: []string{capability.BaseName}, Environment: "niri-desktop"}
	if err := ok.Derive(in.Resolver()); err != nil {
		t.Fatal(err)
	}
	if res, err := Apply(ok, in); err != nil || !res.Success {
		t.Fatalf("first apply: %v %+v", err, res)
	}
	before, err := LoadComposition()
	if err != nil {
		t.Fatal(err)
	}
	if before.AppliedAt == "" {
		t.Fatal("composition missing after first apply")
	}

	// Second apply fails at the experience stage: the previous
	// product record must stay untouched (zero-change guarantee).
	f.Fail("sudo", "dnf", "install", "veilbox-experience-networking-tools")
	bad := Selection{Profile: "cloud-engineer", Capabilities: []string{"networking", capability.BaseName}}
	if err := bad.Derive(in.Resolver()); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(bad, in); err == nil {
		t.Fatal("second apply must fail")
	}
	after, err := LoadComposition()
	if err != nil {
		t.Fatal(err)
	}
	if after.AppliedAt != before.AppliedAt || after.Profile != before.Profile {
		t.Fatalf("failed apply must not touch the composition record: before %+v after %+v", before, after)
	}
}

func TestCompositionValidationRecordsProblems(t *testing.T) {
	in, _ := setupCompositionOnboarding(t)
	sel := Selection{Profile: "cloud-engineer", Experiences: []string{"does-not-exist"}}
	v := validateComposition(sel, in)
	if v.Valid {
		t.Fatal("invalid selection must not validate")
	}
	found := false
	for _, n := range v.Notes {
		if strings.Contains(n, "problem:") {
			found = true
		}
	}
	if !found {
		t.Fatalf("problems not recorded: %v", v.Notes)
	}
}

func TestCompositionJSONShape(t *testing.T) {
	in, f := setupCompositionOnboarding(t)
	f.Responses[f.Key("systemctl", "get-default")] = "multi-user.target"
	sel := Selection{Profile: "cloud-engineer", Capabilities: []string{"networking", capability.BaseName}, Environment: "niri-desktop"}
	if err := sel.Derive(in.Resolver()); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(sel, in); err != nil {
		t.Fatalf("apply: %v", err)
	}
	dir, err := settings.StateDir()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, CompositionFile))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("composition not valid JSON: %v", err)
	}
	for _, key := range []string{"schema_version", "applied_at", "profile", "capabilities", "experiences", "workspace", "validation"} {
		if _, ok := raw[key]; !ok {
			t.Fatalf("composition missing %q: %s", key, data)
		}
	}
	if _, ok := raw["environment"]; !ok {
		t.Fatalf("composition missing environment record: %s", data)
	}
}

func CompositionPathMust(t *testing.T) string {
	t.Helper()
	p, err := CompositionPath()
	if err != nil {
		t.Fatal(err)
	}
	return p
}
