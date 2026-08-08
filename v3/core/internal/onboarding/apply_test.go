package onboarding

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Shreyas0047/veilbox/v3/core/internal/capability"
	"github.com/Shreyas0047/veilbox/v3/core/internal/desktop"
	"github.com/Shreyas0047/veilbox/v3/core/internal/dnfops"
	"github.com/Shreyas0047/veilbox/v3/core/internal/experience"
	"github.com/Shreyas0047/veilbox/v3/core/internal/onboarding/onboardingtest"
	"github.com/Shreyas0047/veilbox/v3/core/internal/profile"
	"github.com/Shreyas0047/veilbox/v3/core/internal/workspace"
)

// setupOnboarding builds an isolated environment and the wiring for
// the wizard's engines, backed by a fake command runner.
func setupOnboarding(t *testing.T) (PlanInputs, *onboardingtest.Runner) {
	t.Helper()
	env := onboardingtest.SetupEnv(t)
	f := onboardingtest.New()
	dnf := dnfops.NewWithRunner(f)
	in := PlanInputs{
		Registry:     profile.NewRegistryDir(env.ProfDir),
		Catalog:      experience.NewCatalogWith(env.CatDir, dnf),
		Capabilities: capability.NewRegistryDir(env.CapDir),
		Workspace:    &workspace.Engine{LookPath: func(string) (string, error) { return "/usr/bin/vim", nil }},
		Desktop:      desktop.NewWith(experience.NewCatalogWith(env.CatDir, dnf), desktop.NewSystemWith(f)),
	}
	return in, f
}

func selectionAll(in PlanInputs) Selection {
	return Selection{
		Profile:     "cloud-engineer",
		Experiences: []string{"networking-tools"},
		Desktop:     "niri-desktop",
		Workspace:   WorkspacePrefs{Editor: "vim", Prompt: "veilbox", Terminal: "system"},
	}
}

func TestApplyFullFlow(t *testing.T) {
	in, f := setupOnboarding(t)
	f.Responses[f.Key("systemctl", "get-default")] = "multi-user.target"

	sel := selectionAll(in)
	res, err := Apply(sel, in)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !res.Success {
		t.Fatalf("apply not successful: %+v", res.Stages)
	}
	names := stageNames(res)
	want := []string{"profile", "experiences", "workspace", "desktop", "verify"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("stage order %v, want %v", names, want)
	}
	for _, s := range res.Stages {
		if s.Status != StageOK {
			t.Fatalf("stage %s not OK: %+v", s.Name, s)
		}
	}

	saved, err := Load()
	if err != nil {
		t.Fatalf("load saved selection: %v", err)
	}
	if saved.LastApply == nil || !saved.LastApply.Success {
		t.Fatalf("selection ledger not persisted with success")
	}

	// Verify standalone against the same (now-consistent) state.
	if err := Verify(sel, in); err != nil {
		t.Fatalf("verify after apply: %v", err)
	}
}

func TestApplySkipsAlreadyInstalled(t *testing.T) {
	in, f := setupOnboarding(t)
	f.Installed("veilbox-experience-networking-tools")

	sel := Selection{Profile: "cloud-engineer", Experiences: []string{"networking-tools"}}
	res, err := Apply(sel, in)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !res.Success {
		t.Fatalf("apply not successful: %+v", res.Stages)
	}
	joined := strings.Join(f.Joined(), "\n")
	if strings.Contains(joined, "sudo dnf install") {
		t.Fatalf("DNF install ran for an already-installed experience:\n%s", joined)
	}
	found := false
	for _, s := range res.Stages {
		if s.Name == "experiences" && s.Status == StageSkip {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected skip record: %+v", res.Stages)
	}
}

func TestApplyStopsAtFailedStage(t *testing.T) {
	in, f := setupOnboarding(t)
	f.Fail("sudo", "dnf", "install", "veilbox-experience-networking-tools")

	sel := Selection{Profile: "cloud-engineer", Experiences: []string{"networking-tools"}}
	res, err := Apply(sel, in)
	if err == nil {
		t.Fatal("expected apply error")
	}
	if !strings.Contains(err.Error(), "experiences") {
		t.Fatalf("error should name the stage: %v", err)
	}
	if res.Success {
		t.Fatal("res must not be successful")
	}
	if len(res.Stages) == 0 || res.Stages[0].Status != StageOK {
		t.Fatalf("profile stage should have completed first: %+v", res.Stages)
	}
	last := res.Stages[len(res.Stages)-1]
	if last.Name != "experiences" || last.Status != StageFailed {
		t.Fatalf("failed stage not recorded: %+v", last)
	}

	// The failure ledger must be persisted for the rerun recovery path.
	saved, err := Load()
	if err != nil {
		t.Fatalf("load saved selection: %v", err)
	}
	if saved.LastApply == nil || saved.LastApply.Success {
		t.Fatalf("failure ledger not persisted: %+v", saved.LastApply)
	}
}

func TestApplyDerivesExperiencesFromCapabilities(t *testing.T) {
	in, f := setupOnboarding(t)

	// A capability-only selection (no stored experiences) must derive
	// its experiences through the capability mapping.
	sel := Selection{
		Profile:      "cloud-engineer",
		Capabilities: []string{"networking", capability.BaseName},
	}
	res, err := Apply(sel, in)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !res.Success {
		t.Fatalf("apply not successful: %+v", res.Stages)
	}
	joined := strings.Join(f.Joined(), "\n")
	if !strings.Contains(joined, "sudo dnf install -y veilbox-experience-networking-tools") {
		t.Fatalf("expected derived experience install:\n%s", joined)
	}
	if strings.Contains(joined, "veilbox-experience-base-operations") {
		t.Fatalf("empty capability must not resolve an install:\n%s", joined)
	}
	saved, err := Load()
	if err != nil {
		t.Fatalf("load saved selection: %v", err)
	}
	if !contains(saved.Experiences, "networking-tools") {
		t.Fatalf("derived experiences not persisted: %+v", saved.Experiences)
	}
}

func TestApplyRejectsInvalidSelection(t *testing.T) {
	in, _ := setupOnboarding(t)
	sel := Selection{Profile: "cloud-engineer", Experiences: []string{"does-not-exist"}}
	res, err := Apply(sel, in)
	if err == nil || !strings.Contains(err.Error(), "problem") {
		t.Fatalf("expected problems error, got %v", err)
	}
	if res != nil {
		t.Fatalf("nothing may run for an invalid selection")
	}
}

func TestVerifyDetectsUninstalled(t *testing.T) {
	in, _ := setupOnboarding(t)
	sel := Selection{Profile: "cloud-engineer", Experiences: []string{"networking-tools"}}
	err := Verify(sel, in)
	if err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("expected missing-experience error, got %v", err)
	}
}

func TestVerifyDetectsWorkspaceDrift(t *testing.T) {
	in, f := setupOnboarding(t)
	f.Installed("veilbox-experience-networking-tools")

	sel := Selection{Profile: "cloud-engineer", Experiences: []string{"networking-tools"}}
	prefs := MergeWorkspace(mustManifestPrefs(t, in, "cloud-engineer"), sel.Workspace)
	if _, err := in.Workspace.Apply(prefs, "cloud-engineer", false); err != nil {
		t.Fatalf("apply workspace: %v", err)
	}
	if err := Verify(sel, in); err != nil {
		t.Fatalf("verify on clean state: %v", err)
	}

	// Drift the workspace, then Verify must fail.
	in.Workspace.LookPath = func(string) (string, error) { return "", errors.New("gone") }
	if err := Verify(sel, in); err == nil {
		t.Fatal("verify must detect workspace drift")
	}
}

func TestRenderReport(t *testing.T) {
	res := &ApplyResult{Success: false, Stages: []StageResult{
		{Name: "profile", Status: StageOK, Detail: "cloud-engineer"},
		{Name: "experiences", Status: StageFailed, Detail: "boom"},
	}}
	var sb strings.Builder
	RenderApplyReport(&sb, res, fmt.Errorf("stage experiences failed: boom"))
	out := sb.String()
	for _, want := range []string{"APPLY RESULT", "[OK]", "[FAILED]", "rerun 'veil onboard'"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in report:\n%s", want, out)
		}
	}
}

func stageNames(res *ApplyResult) []string {
	var names []string
	for _, s := range res.Stages {
		names = append(names, s.Name)
	}
	return names
}

func mustManifestPrefs(t *testing.T, in PlanInputs, name string) workspace.Preferences {
	t.Helper()
	m, err := in.Registry.Load(name)
	if err != nil {
		t.Fatal(err)
	}
	return m.Workspace
}
