package onboarding

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/Shreyas0047/veilbox/v3/core/internal/experience"
	"github.com/Shreyas0047/veilbox/v3/core/internal/profile"
	"github.com/Shreyas0047/veilbox/v3/core/internal/workspace"
)

// RoleChoice is one selectable engineer role in the role step.
type RoleChoice struct {
	Name        string
	DisplayName string
	Description string
	Recommended []string // display names of recommended experiences
	Optional    []string
	Applied     bool
}

// DesktopChoice is one selectable desktop experience.
type DesktopChoice struct {
	Name        string
	DisplayName string
	Description string
	Status      string // available / installed
	Active      bool   // currently the active desktop
}

// ExperienceChoice is one selectable capability.
type ExperienceChoice struct {
	Name        string
	DisplayName string
	Domain      string
	Description string
	Status      string // available / installed
	Recommended bool   // recommended by the selected profile
}

// ExperienceGroup groups capabilities by concept (domain).
type ExperienceGroup struct {
	Domain  string
	Choices []ExperienceChoice
}

// WorkspaceOptions is the safe subset the workspace step exposes.
type WorkspaceOptions struct {
	Prompts   []string
	Editors   []string
	Terminals []string
}

// ReviewInfo carries everything the review step renders.
type ReviewInfo struct {
	Plan      *Plan
	Selection Selection
	Existing  bool
	Text      string
}

// ReviewDecision is what the engineer decides at the review step.
type ReviewDecision struct {
	// Apply is true when the engineer confirmed the plan.
	Apply bool
	// Restart is true when the engineer went back to change choices.
	Restart bool
	// ActivateDesktop confirms desktop activation when a desktop is
	// selected. Declining keeps the desktop in the selection but
	// skips the activation stage for this run.
	ActivateDesktop bool
}

// ErrAborted is the shared abort signal between the UI contract and
// its implementations: a wizard UI returns it when the engineer
// quits. Nothing on the machine changes when it is returned.
var ErrAborted = fmt.Errorf("onboarding aborted — no changes were made")

// UI renders the wizard steps. Implementations: a bubbletea TUI for
// terminals and a line-based fallback for scripts and tests.
type UI interface {
	Welcome() error
	SelectRole(choices []RoleChoice, current string) (string, error)
	SelectDesktop(choices []DesktopChoice, current string) (string, error)
	SelectExperiences(groups []ExperienceGroup, current []string) ([]string, error)
	SelectWorkspace(opts WorkspaceOptions, current WorkspacePrefs) (WorkspacePrefs, error)
	Review(info ReviewInfo) (ReviewDecision, error)
	ShowReport(report string) error
}

// Wizard walks the onboarding steps over a UI. The wizard owns the
// conversation; the engines own every implementation.
type Wizard struct {
	UI        UI
	Inputs    PlanInputs
	Selection Selection
	// Existing reports whether a prior selection was loaded.
	Existing bool
}

// LoadWizard loads the saved selection (or a fresh one) and prepares
// the wizard.
func LoadWizard(ui UI, in PlanInputs) (*Wizard, error) {
	sel, err := Load()
	if err != nil {
		return nil, err
	}
	existing := sel.Profile != "" || len(sel.Experiences) > 0 || sel.Desktop != ""
	return &Wizard{UI: ui, Inputs: in, Selection: sel, Existing: existing}, nil
}

// Run walks all steps and returns the final selection plus the apply
// result. A nil result means the engineer aborted or exited.
func (w *Wizard) Run() (*Selection, *ApplyResult, error) {
	if err := w.UI.Welcome(); err != nil {
		return nil, nil, err
	}

	for {
		if err := w.stepRole(); err != nil {
			return nil, nil, err
		}
		if err := w.stepDesktop(); err != nil {
			return nil, nil, err
		}
		if err := w.stepCapabilities(); err != nil {
			return nil, nil, err
		}
		if err := w.stepWorkspace(); err != nil {
			return nil, nil, err
		}

		plan, err := BuildPlan(w.Selection, w.Inputs)
		if err != nil {
			return nil, nil, fmt.Errorf("build plan: %w", err)
		}
		var buf bytes.Buffer
		RenderPlan(&buf, *plan, w.Selection)
		decision, err := w.UI.Review(ReviewInfo{
			Plan:      plan,
			Selection: w.Selection,
			Existing:  w.Existing,
			Text:      buf.String(),
		})
		if err != nil {
			return nil, nil, err
		}
		if decision.Restart {
			continue
		}
		if !decision.Apply {
			return &w.Selection, nil, nil
		}

		applySel := w.Selection
		if !decision.ActivateDesktop && applySel.Desktop != "" {
			applySel.Desktop = ""
		}
		res, aerr := Apply(applySel, w.Inputs)
		if aerr != nil && res == nil {
			// Unrecoverable: the plan never built. Nothing ran.
			return &w.Selection, nil, aerr
		}
		_ = w.UI.ShowReport(RenderReport(res, aerr))

		// Persist the full selection (including a declined desktop) so
		// the next run resumes exactly where the engineer left off.
		if res != nil && res.Log != nil {
			w.Selection.LastApply = res.Log
		}
		_ = w.Selection.Save()
		return &w.Selection, res, aerr
	}
}

func (w *Wizard) stepRole() error {
	names, err := w.Inputs.Registry.List()
	if err != nil {
		return fmt.Errorf("list profiles: %w", err)
	}
	choices := make([]RoleChoice, 0, len(names))
	for _, n := range names {
		m, err := w.Inputs.Registry.Load(n)
		if err != nil {
			continue
		}
		choices = append(choices, RoleChoice{
			Name:        m.Name,
			DisplayName: m.DisplayName,
			Description: m.Description,
			Recommended: experienceDisplays(w.Inputs.Catalog, m.Recommended),
			Optional:    experienceDisplays(w.Inputs.Catalog, m.Optional),
			Applied:     m.Name == appliedProfile(),
		})
	}
	sort.Slice(choices, func(i, j int) bool { return choices[i].Name < choices[j].Name })
	chosen, err := w.UI.SelectRole(choices, w.Selection.Profile)
	if err != nil {
		return err
	}
	w.Selection.Profile = chosen
	// Seed the capability selection with the profile recommendations
	// only on a fresh wizard; afterwards the saved selection is the
	// engineer's explicit customization and is never re-seeded.
	if !w.Existing && len(w.Selection.Experiences) == 0 && chosen != "" {
		rec, rerr := ProfileRecommendations(w.Inputs.Registry, w.Inputs.Catalog, chosen)
		if rerr == nil {
			w.Selection.Experiences = rec
		}
	}
	return nil
}

func (w *Wizard) stepDesktop() error {
	entries, err := w.Inputs.Catalog.List()
	if err != nil {
		return fmt.Errorf("list experiences: %w", err)
	}
	active := activeCompositor(w.Inputs)
	var choices []DesktopChoice
	for _, e := range entries {
		if e.Type != experience.TypeDesktop || e.RPM == "" {
			continue
		}
		choices = append(choices, DesktopChoice{
			Name:        e.Name,
			DisplayName: display(e.DisplayName, e.Name),
			Description: e.Description,
			Status:      string(e.Status),
			Active:      active != "" && e.Components[experience.CompCompositor] == active,
		})
	}
	sort.Slice(choices, func(i, j int) bool { return choices[i].Name < choices[j].Name })
	chosen, err := w.UI.SelectDesktop(choices, w.Selection.Desktop)
	if err != nil {
		return err
	}
	w.Selection.Desktop = chosen
	return nil
}

func (w *Wizard) stepCapabilities() error {
	entries, err := w.Inputs.Catalog.List()
	if err != nil {
		return fmt.Errorf("list experiences: %w", err)
	}
	byDomain := make(map[string][]ExperienceChoice)
	var domains []string
	for _, e := range entries {
		if e.Type == experience.TypeDesktop || e.RPM == "" {
			continue
		}
		dom := Domain(e)
		if _, ok := byDomain[dom]; !ok {
			domains = append(domains, dom)
		}
		byDomain[dom] = append(byDomain[dom], ExperienceChoice{
			Name:        e.Name,
			DisplayName: display(e.DisplayName, e.Name),
			Domain:      dom,
			Description: e.Description,
			Status:      string(e.Status),
			Recommended: recommendedBy(w.Selection.Profile, w.Inputs.Registry, e.Name),
		})
	}
	sort.Strings(domains)
	groups := make([]ExperienceGroup, 0, len(domains))
	for _, d := range domains {
		choices := byDomain[d]
		sort.Slice(choices, func(i, j int) bool { return choices[i].Name < choices[j].Name })
		groups = append(groups, ExperienceGroup{Domain: d, Choices: choices})
	}
	chosen, err := w.UI.SelectExperiences(groups, w.Selection.Experiences)
	if err != nil {
		return err
	}
	w.Selection.Experiences = chosen
	return nil
}

func (w *Wizard) stepWorkspace() error {
	opts := WorkspaceOptions{
		Prompts:   []string{workspace.PromptPlain, workspace.PromptVeilbox},
		Editors:   []string{"vim", "vi", "nano", "emacs"},
		Terminals: []string{workspace.TerminalAuto, workspace.TerminalKitty, workspace.TerminalWezterm, workspace.TerminalGhostty, workspace.TerminalAlacritty},
	}
	prefs, err := w.UI.SelectWorkspace(opts, w.Selection.Workspace)
	if err != nil {
		return err
	}
	w.Selection.Workspace = prefs
	return nil
}

// RenderReport renders the apply report for the UI.
func RenderReport(res *ApplyResult, err error) string {
	var buf bytes.Buffer
	RenderApplyReport(&buf, res, err)
	return buf.String()
}

func appliedProfile() string {
	st, err := profile.Active()
	if err != nil {
		return ""
	}
	return st.ActiveProfile
}

func activeCompositor(in PlanInputs) string {
	entries, err := in.Desktop.List()
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.Session.Compositor != "" {
			return e.Session.Compositor
		}
	}
	return ""
}

func experienceDisplays(cat *experience.Catalog, names []string) []string {
	var out []string
	for _, n := range names {
		if m, err := cat.Load(n); err == nil {
			out = append(out, display(m.DisplayName, m.Name))
		} else {
			out = append(out, n)
		}
	}
	return out
}

func recommendedBy(profileName string, reg *profile.Registry, experienceName string) bool {
	if profileName == "" {
		return false
	}
	m, err := reg.Load(profileName)
	if err != nil {
		return false
	}
	for _, r := range m.Recommended {
		if r == experienceName {
			return true
		}
	}
	return false
}
