package onboarding

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// scriptedUI is a fake UI driven by queued answers.
type scriptedUI struct {
	role        string
	desktop     string
	experiences func(current []string) []string
	workspace   WorkspacePrefs
	decisions   []ReviewDecision

	roleCalls   int
	reviewCalls int
	report      string
	seeded      []string
}

func (u *scriptedUI) Welcome() error { return nil }

func (u *scriptedUI) SelectRole(choices []RoleChoice, current string) (string, error) {
	u.roleCalls++
	if u.role != "" {
		return u.role, nil
	}
	if current != "" {
		return current, nil
	}
	if len(choices) > 0 {
		return choices[0].Name, nil
	}
	return "", nil
}

func (u *scriptedUI) SelectDesktop(choices []DesktopChoice, current string) (string, error) {
	if u.desktop != "" {
		return u.desktop, nil
	}
	return current, nil
}

func (u *scriptedUI) SelectExperiences(groups []ExperienceGroup, current []string) ([]string, error) {
	u.seeded = append([]string{}, current...)
	if u.experiences != nil {
		return u.experiences(current), nil
	}
	return current, nil
}

func (u *scriptedUI) SelectWorkspace(opts WorkspaceOptions, current WorkspacePrefs) (WorkspacePrefs, error) {
	if u.workspace.Editor != "" || u.workspace.Prompt != "" || u.workspace.Terminal != "" || u.workspace.Tmux != nil {
		return u.workspace, nil
	}
	return current, nil
}

func (u *scriptedUI) Review(info ReviewInfo) (ReviewDecision, error) {
	u.reviewCalls++
	if len(u.decisions) > 0 {
		d := u.decisions[0]
		u.decisions = u.decisions[1:]
		return d, nil
	}
	return ReviewDecision{Apply: true, ActivateDesktop: true}, nil
}

func (u *scriptedUI) ShowReport(report string) error {
	u.report = report
	return nil
}

func addTerminalOps(t *testing.T, root string) {
	t.Helper()
	os.WriteFile(filepath.Join(root, "experiences", "terminal-ops.yaml"), []byte(
		"name: terminal-ops\ndescription: Terminal improvements.\nrpm: veilbox-experience-terminal-ops\npackages:\n  - tmux\n"), 0o644)
}

func TestWizardFullRun(t *testing.T) {
	in, f := setupOnboarding(t)
	addTerminalOps(t, os.Getenv("VEILBOX_ROOT"))
	f.Responses[f.Key("systemctl", "get-default")] = "multi-user.target"

	ui := &scriptedUI{
		role:      "cloud-engineer",
		desktop:   "niri-desktop",
		decisions: []ReviewDecision{{Apply: true, ActivateDesktop: true}},
	}
	w := &Wizard{UI: ui, Inputs: in, Existing: false}
	sel, res, err := w.Run()
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res == nil || !res.Success {
		t.Fatalf("wizard did not apply: %+v", res)
	}
	if sel.Profile != "cloud-engineer" || sel.Desktop != "niri-desktop" {
		t.Fatalf("selection wrong: %+v", sel)
	}
	// Fresh run must seed from the profile recommendations.
	if !contains(sel.Experiences, "networking-tools") {
		t.Fatalf("recommendations not seeded: %v", sel.Experiences)
	}
	if ui.report == "" || !strings.Contains(ui.report, "APPLY RESULT") {
		t.Fatalf("report not shown: %q", ui.report)
	}
	joined := strings.Join(f.Joined(), "\n")
	if !strings.Contains(joined, "interactive:sudo dnf install -y veilbox-experience-networking-tools") {
		t.Fatalf("expected install transaction:\n%s", joined)
	}
	if !strings.Contains(joined, "interactive:sudo systemctl enable sddm") {
		t.Fatalf("expected desktop activation:\n%s", joined)
	}
	// The selection must persist the ledger.
	saved, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if saved.LastApply == nil || !saved.LastApply.Success {
		t.Fatalf("ledger not saved: %+v", saved.LastApply)
	}
}

func TestWizardAbortAtReview(t *testing.T) {
	in, f := setupOnboarding(t)
	ui := &scriptedUI{role: "cloud-engineer", decisions: []ReviewDecision{{Apply: false}}}
	w := &Wizard{UI: ui, Inputs: in, Existing: false}
	sel, res, err := w.Run()
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res != nil {
		t.Fatalf("aborted wizard must not apply")
	}
	if sel.Profile != "cloud-engineer" {
		t.Fatalf("selection lost: %+v", sel)
	}
	for _, cmd := range f.Joined() {
		if strings.HasPrefix(cmd, "interactive:sudo") || strings.HasPrefix(cmd, "sudo") {
			t.Fatalf("nothing may change the machine on abort, got: %v", cmd)
		}
	}
}

func TestWizardRestartRevisitsSteps(t *testing.T) {
	in, _ := setupOnboarding(t)
	ui := &scriptedUI{
		role:      "cloud-engineer",
		decisions: []ReviewDecision{{Restart: true}, {Apply: true, ActivateDesktop: false}},
	}
	w := &Wizard{UI: ui, Inputs: in, Existing: false}
	_, res, err := w.Run()
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res == nil || !res.Success {
		t.Fatalf("restart must end in apply: %+v", res)
	}
	if ui.roleCalls != 2 {
		t.Fatalf("role must be asked again after restart, got %d calls", ui.roleCalls)
	}
}

func TestWizardDesktopActivationDeclined(t *testing.T) {
	in, f := setupOnboarding(t)
	ui := &scriptedUI{
		role:      "cloud-engineer",
		desktop:   "niri-desktop",
		decisions: []ReviewDecision{{Apply: true, ActivateDesktop: false}},
	}
	w := &Wizard{UI: ui, Inputs: in, Existing: false}
	sel, res, err := w.Run()
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res == nil || !res.Success {
		t.Fatalf("apply: %+v", res)
	}
	// The selection keeps the desktop; the machine never activated it.
	if sel.Desktop != "niri-desktop" {
		t.Fatalf("declined activation must keep the desktop choice: %+v", sel)
	}
	joined := strings.Join(f.Joined(), "\n")
	if strings.Contains(joined, "systemctl enable sddm") {
		t.Fatalf("declined activation must not enable the display manager:\n%s", joined)
	}
	if strings.Contains(joined, "set-default graphical.target") {
		t.Fatalf("declined activation must not change the boot target:\n%s", joined)
	}
	if strings.Contains(joined, "sudo dnf install -y veilbox-experience-niri") {
		t.Fatalf("declined activation must not install the desktop:\n%s", joined)
	}
}

func TestWizardAltersOneCapability(t *testing.T) {
	in, f := setupOnboarding(t)
	addTerminalOps(t, os.Getenv("VEILBOX_ROOT"))

	// Prior run: networking-tools installed, terminal-ops selected but
	// not yet installed. The engineer now removes networking-tools
	// from the selection — everything else stays.
	f.Installed("veilbox-experience-networking-tools")
	prev := Selection{
		SchemaVersion: SchemaVersion,
		Profile:       "cloud-engineer",
		Experiences:   []string{"networking-tools", "terminal-ops"},
	}
	if err := prev.Save(); err != nil {
		t.Fatalf("save prior selection: %v", err)
	}

	ui := &scriptedUI{
		role:      "cloud-engineer",
		desktop:   "",
		workspace: WorkspacePrefs{Editor: "vim", Prompt: "veilbox", Terminal: "system"},
		experiences: func(current []string) []string {
			// Alter exactly one capability: drop networking-tools.
			var out []string
			for _, c := range current {
				if c != "networking-tools" {
					out = append(out, c)
				}
			}
			return out
		},
		decisions: []ReviewDecision{{Apply: true, ActivateDesktop: false}},
	}
	w, err := LoadWizard(ui, in)
	if err != nil {
		t.Fatalf("load wizard: %v", err)
	}
	if !w.Existing {
		t.Fatalf("prior selection must mark the wizard as existing")
	}
	sel, res, err := w.Run()
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res == nil || !res.Success {
		t.Fatalf("apply: %+v", res)
	}
	// One capability altered, the other kept.
	if len(sel.Experiences) != 1 || sel.Experiences[0] != "terminal-ops" {
		t.Fatalf("altered selection wrong: %v", sel.Experiences)
	}
	joined := strings.Join(f.Joined(), "\n")
	if strings.Contains(joined, "sudo dnf install -y veilbox-experience-networking-tools") {
		t.Fatalf("kept capability must not be reinstalled:\n%s", joined)
	}
	if !strings.Contains(joined, "sudo dnf install -y veilbox-experience-terminal-ops") {
		t.Fatalf("altered-in capability must be installed:\n%s", joined)
	}
}

func TestWizardRoleChangeKeepsCustomization(t *testing.T) {
	in, _ := setupOnboarding(t)
	addTerminalOps(t, os.Getenv("VEILBOX_ROOT"))

	prev := Selection{
		SchemaVersion: SchemaVersion,
		Profile:       "cloud-engineer",
		Experiences:   []string{"terminal-ops"},
	}
	if err := prev.Save(); err != nil {
		t.Fatalf("save prior selection: %v", err)
	}

	// Changing roles on an existing customization must never reseed
	// the recommendations: the engineer's explicit choice stays.
	ui := &scriptedUI{
		role:      "cloud-engineer",
		workspace: WorkspacePrefs{Editor: "vim", Prompt: "veilbox", Terminal: "system"},
		experiences: func(current []string) []string {
			if contains(current, "networking-tools") {
				t.Fatal("role change must not reseed profile recommendations")
			}
			return current
		},
		decisions: []ReviewDecision{{Apply: true, ActivateDesktop: false}},
	}
	w, err := LoadWizard(ui, in)
	if err != nil {
		t.Fatalf("load wizard: %v", err)
	}
	_, res, err := w.Run()
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res == nil || !res.Success {
		t.Fatalf("apply: %+v", res)
	}
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
