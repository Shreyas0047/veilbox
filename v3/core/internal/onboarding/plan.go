package onboarding

import (
	"fmt"
	"io"
	"sort"

	"github.com/Shreyas0047/veilbox/v3/core/internal/capability"
	"github.com/Shreyas0047/veilbox/v3/core/internal/environment"
	"github.com/Shreyas0047/veilbox/v3/core/internal/experience"
	"github.com/Shreyas0047/veilbox/v3/core/internal/profile"
	"github.com/Shreyas0047/veilbox/v3/core/internal/settings"
	"github.com/Shreyas0047/veilbox/v3/core/internal/workspace"
)

// Plan action values.
const (
	ActionApply     = "apply"
	ActionUnchanged = "unchanged"
	ActionInstall   = "install"
	ActionInstalled = "installed"
	ActionNone      = "none"

	EnvironmentInstallActivate = "install-activate"
	EnvironmentActivateOnly    = "activate-only"
	EnvironmentAlreadyActive   = "already-active"
)

// ProfileAction describes what applying the selected profile would do.
type ProfileAction struct {
	Name        string
	DisplayName string
	Action      string
}

// ExperienceItem is one selected capability and what the plan would do.
type ExperienceItem struct {
	Name        string
	DisplayName string
	Domain      string
	Status      string
	Action      string
}

// EnvironmentAction describes what the plan would do for the environment.
type EnvironmentAction struct {
	Name        string
	DisplayName string
	Action      string
	// Steps lists concrete activation steps when the environment will be
	// installed or activated (session, display manager, boot target).
	Steps []string
}

// WorkspaceAction describes the workspace delta.
type WorkspaceAction struct {
	Action string
	Prefs  workspace.Preferences
}

// Plan is the complete, read-only description of what applying the
// selection would do. Building a plan never changes anything.
type Plan struct {
	Profile     ProfileAction
	Experiences []ExperienceItem
	Environment EnvironmentAction
	Workspace   WorkspaceAction
	// Problems are invalid references or values in the selection;
	// apply refuses to run while any exist.
	Problems []string
}

// HasChanges reports whether the plan contains any pending work.
func (p Plan) HasChanges() bool {
	if len(p.Problems) > 0 {
		return true
	}
	if p.Profile.Action != ActionUnchanged {
		return true
	}
	if p.Workspace.Action != ActionUnchanged {
		return true
	}
	if p.Environment.Action != ActionNone && p.Environment.Action != EnvironmentAlreadyActive {
		return true
	}
	for _, e := range p.Experiences {
		if e.Action != ActionInstalled {
			return true
		}
	}
	return false
}

// SystemActions renders the plan's concrete system-level effects in
// human terms — never raw package lists as the primary presentation.
func (p Plan) SystemActions() []string {
	var out []string
	if p.Profile.Action == ActionApply {
		out = append(out, fmt.Sprintf("apply profile: %s", display(p.Profile.DisplayName, p.Profile.Name)))
	}
	installCount := 0
	for _, e := range p.Experiences {
		if e.Action == ActionInstall {
			installCount++
		}
	}
	if installCount > 0 {
		out = append(out, fmt.Sprintf("install %d capability experience(s)", installCount))
	} else if len(p.Experiences) > 0 {
		out = append(out, "capability experiences already installed")
	}
	if p.Workspace.Action == ActionApply {
		out = append(out, "apply workspace preferences")
	}
	switch p.Environment.Action {
	case EnvironmentInstallActivate:
		out = append(out, fmt.Sprintf("install %s", display(p.Environment.DisplayName, p.Environment.Name)))
		out = append(out, p.Environment.Steps...)
	case EnvironmentActivateOnly:
		out = append(out, fmt.Sprintf("activate %s", display(p.Environment.DisplayName, p.Environment.Name)))
		out = append(out, p.Environment.Steps...)
	case EnvironmentAlreadyActive:
		out = append(out, fmt.Sprintf("%s already active", display(p.Environment.DisplayName, p.Environment.Name)))
	case ActionNone:
		out = append(out, "no environment selected")
	}
	if len(out) == 0 {
		out = append(out, "nothing to do — selection matches the machine")
	}
	return out
}

// PlanInputs bundles the engines the plan builder reads. Everything is
// read-only during planning.
type PlanInputs struct {
	Registry     *profile.Registry
	Catalog      *experience.Catalog
	Capabilities *capability.Registry
	Workspace    *workspace.Engine
	Environment  *environment.Engine
}

// Resolver returns the capability→experience mapping resolver over
// the inputs' registries.
func (in PlanInputs) Resolver() *capability.Resolver {
	res, err := capability.NewResolver(in.Capabilities, in.Catalog)
	if err != nil {
		return nil
	}
	return res
}

// BuildPlan computes the complete delta between the selection and the
// current machine. Pure read-only: no state, packages or files change.
func BuildPlan(sel Selection, in PlanInputs) (*Plan, error) {
	plan := &Plan{
		Experiences: []ExperienceItem{},
		Environment: EnvironmentAction{Action: ActionNone},
	}
	plan.Problems = append(plan.Problems, sel.Problems(in.Registry, in.Capabilities, in.Catalog)...)

	// Profile.
	if sel.Profile != "" {
		m, err := in.Registry.Load(sel.Profile)
		if err == nil {
			st, _ := settings.LoadState()
			plan.Profile = ProfileAction{
				Name:        m.Name,
				DisplayName: m.DisplayName,
				Action:      ActionApply,
			}
			if st.ActiveProfile == m.Name {
				plan.Profile.Action = ActionUnchanged
			}
		}
	}

	// Capability experiences.
	entries, err := in.Catalog.List()
	if err != nil {
		return nil, fmt.Errorf("read experience catalog: %w", err)
	}
	statusBy := make(map[string]experience.Status, len(entries))
	entryBy := make(map[string]experience.Entry, len(entries))
	for _, e := range entries {
		statusBy[e.Name] = e.Status
		entryBy[e.Name] = e
	}
	names := append([]string{}, sel.Experiences...)
	sort.Strings(names)
	for _, name := range names {
		en, ok := entryBy[name]
		if !ok || en.Type == experience.TypeEnvironment || en.RPM == "" {
			continue // already reported as a problem
		}
		item := ExperienceItem{
			Name:        en.Name,
			DisplayName: display(en.DisplayName, en.Name),
			Domain:      Domain(en),
			Status:      string(en.Status),
			Action:      ActionInstall,
		}
		if en.Status == experience.StatusInstalled {
			item.Action = ActionInstalled
		}
		plan.Experiences = append(plan.Experiences, item)
	}

	// Environment.
	if sel.Environment != "" {
		en, ok := entryBy[sel.Environment]
		if ok && en.Type == experience.TypeEnvironment && en.RPM != "" {
			ip, perr := in.Environment.PlanInstall(sel.Environment)
			if perr != nil {
				plan.Problems = append(plan.Problems, fmt.Sprintf("environment %q: %v", sel.Environment, perr))
			} else {
				plan.Environment = EnvironmentAction{
					Name:        en.Name,
					DisplayName: display(en.DisplayName, en.Name),
					Action:      EnvironmentInstallActivate,
					Steps:       environmentSteps(ip),
				}
				if en.Status == experience.StatusInstalled {
					switch {
					case ip.WillEnableDM || ip.WillSetTarget || !ip.SessionRegistered:
						plan.Environment.Action = EnvironmentActivateOnly
					default:
						plan.Environment.Action = EnvironmentAlreadyActive
					}
				}
			}
		}
	}

	// Workspace: merged prefs over the profile baseline.
	if sel.Profile != "" {
		if m, err := in.Registry.Load(sel.Profile); err == nil {
			prefs := MergeWorkspace(m.Workspace, sel.Workspace)
			plan.Workspace = WorkspaceAction{Action: ActionApply, Prefs: prefs}
			st, werr := workspace.LoadState()
			if werr == nil {
				wp, berr := in.Workspace.BuildPlan(prefs, st)
				if berr == nil && st.Generation > 0 && wp.IsClean() {
					plan.Workspace.Action = ActionUnchanged
				}
			}
		}
	}

	return plan, nil
}

func environmentSteps(ip *environment.InstallPlan) []string {
	var steps []string
	if !ip.SessionRegistered {
		steps = append(steps, "register Wayland session")
	}
	if ip.WillEnableDM {
		steps = append(steps, fmt.Sprintf("enable %s", ip.DeclaredDM))
	}
	if ip.WillSetTarget {
		steps = append(steps, "set graphical.target")
	}
	return steps
}

// Domain returns the capability concept of an experience, defaulting
// uncategorized tooling to the generic Tooling group.
func Domain(en experience.Entry) string {
	if en.Domain != "" {
		return en.Domain
	}
	return DomainDefault
}

func display(d, fallback string) string {
	if d != "" {
		return d
	}
	return fallback
}

// RenderPlan writes the plan in the review format: PROFILE /
// ENVIRONMENT / EXPERIENCES / WORKSPACE / SYSTEM ACTIONS.
func RenderPlan(w io.Writer, p Plan, sel Selection) {
	fmt.Fprintln(w, "PROFILE")
	if p.Profile.Name != "" {
		fmt.Fprintf(w, "  %s\n", display(p.Profile.DisplayName, p.Profile.Name))
		if p.Profile.Action == ActionUnchanged {
			fmt.Fprintln(w, "  (already applied)")
		}
	} else {
		fmt.Fprintln(w, "  (none)")
	}

	fmt.Fprintln(w, "ENVIRONMENT")
	switch p.Environment.Action {
	case ActionNone:
		fmt.Fprintln(w, "  (none)")
	default:
		fmt.Fprintf(w, "  %s\n", display(p.Environment.DisplayName, p.Environment.Name))
		switch p.Environment.Action {
		case EnvironmentAlreadyActive:
			fmt.Fprintln(w, "  (already installed and active)")
		case EnvironmentActivateOnly:
			fmt.Fprintln(w, "  (installed; activation pending)")
		default:
			fmt.Fprintln(w, "  (will be installed and activated)")
		}
	}

	fmt.Fprintln(w, "CAPABILITIES")
	if len(sel.Capabilities) == 0 {
		fmt.Fprintln(w, "  (none selected)")
	}
	for _, c := range sel.Capabilities {
		fmt.Fprintf(w, "  [x] %s\n", c)
	}

	fmt.Fprintln(w, "EXPERIENCES")
	if len(p.Experiences) == 0 {
		fmt.Fprintln(w, "  (none selected)")
	}
	for _, e := range p.Experiences {
		mark := "+"
		if e.Action == ActionInstalled {
			mark = "="
		}
		fmt.Fprintf(w, "  %s %s (%s)\n", mark, e.DisplayName, e.Domain)
	}

	fmt.Fprintln(w, "WORKSPACE")
	if sel.Profile == "" {
		fmt.Fprintln(w, "  (no profile — preferences inherit nothing)")
	} else {
		prefs := p.Workspace.Prefs
		fmt.Fprintf(w, "  prompt: %s\n", orDefault(prefs.Prompt, "inherited"))
		fmt.Fprintf(w, "  tmux: %s\n", orDefault(boolWord(prefs.Tmux), "inherited"))
		fmt.Fprintf(w, "  editor: %s\n", orDefault(prefs.Editor, "inherited"))
		fmt.Fprintf(w, "  terminal: %s\n", orDefault(prefs.Terminal, "inherited"))
		if p.Workspace.Action == ActionUnchanged {
			fmt.Fprintln(w, "  (already applied)")
		}
	}

	fmt.Fprintln(w, "SYSTEM ACTIONS")
	for _, a := range p.SystemActions() {
		fmt.Fprintf(w, "  - %s\n", a)
	}
	if len(p.Problems) > 0 {
		fmt.Fprintln(w, "PROBLEMS")
		for _, pr := range p.Problems {
			fmt.Fprintf(w, "  - %s\n", pr)
		}
	}
}

func orDefault(v, d string) string {
	if v == "" {
		return d
	}
	return v
}

func boolWord(v bool) string {
	if v {
		return "enabled"
	}
	return "disabled"
}
