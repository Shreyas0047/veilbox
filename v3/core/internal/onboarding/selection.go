// Package onboarding implements the Veilbox composition flow: a
// terminal wizard that turns engineer intent into a complete,
// previewable workstation plan.
//
// Onboarding is an orchestration layer, never a second state system:
// the Profile, Experience, Workspace and Environment engines remain
// the authoritative state holders. The onboarding selection file
// (~/.config/veilbox/onboarding.json) is a draft of intent plus an
// apply log; the applied product record is composition.json
// (ADR-0010). Engines decide truth, onboarding only sequences.
package onboarding

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Shreyas0047/veilbox/v3/core/internal/capability"
	"github.com/Shreyas0047/veilbox/v3/core/internal/experience"
	"github.com/Shreyas0047/veilbox/v3/core/internal/profile"
	"github.com/Shreyas0047/veilbox/v3/core/internal/settings"
	"github.com/Shreyas0047/veilbox/v3/core/internal/workspace"
)

// SchemaVersion of the onboarding selection file.
//
// Version 2 introduced the capability axis (ADR-0011): the selection
// stores the chosen capabilities; the experience list is derived state
// (wizard steps, apply and verify re-derive it). Version 1 files are
// read transparently and upgraded in place on the next save; a
// pre-capability selection without capabilities is backfilled from its
// experience list.
const SchemaVersion = 2

// SelectionFile is the onboarding draft file name.
const SelectionFile = "onboarding.json"

// DomainDefault is the capability grouping for tooling experiences
// that declare no domain. The UI and status always deal in concepts,
// never raw package names.
const DomainDefault = "Tooling"

// WorkspacePrefs is the small, safe subset of workspace preferences
// the wizard exposes. Unset fields inherit from the profile baseline
// when the plan is built (see MergeWorkspace).
type WorkspacePrefs struct {
	// Prompt selects the prompt style (plain or veilbox).
	Prompt string `json:"prompt,omitempty"`
	// Tmux enables Veilbox-managed tmux configuration; nil = inherit.
	Tmux *bool `json:"tmux,omitempty"`
	// Editor is a simple command name (EDITOR).
	Editor string `json:"editor,omitempty"`
	// Terminal is the preferred terminal emulator (validated enum).
	Terminal string `json:"terminal,omitempty"`
}

// StageResult records one apply stage in the apply log.
type StageResult struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// Stage statuses.
const (
	StageOK     = "ok"
	StageSkip   = "skipped"
	StageFailed = "failed"
)

// ApplyLog is the last apply attempt. It is a log, never authority:
// the engines' own state decides what is actually true.
type ApplyLog struct {
	StartedAt  string        `json:"started_at"`
	FinishedAt string        `json:"finished_at,omitempty"`
	Success    bool          `json:"success"`
	Stages     []StageResult `json:"stages"`
}

// Selection is the onboarding draft: what the engineer chose.
type Selection struct {
	SchemaVersion int    `json:"schema_version"`
	UpdatedAt     string `json:"updated_at,omitempty"`
	Profile       string `json:"profile,omitempty"`
	Environment   string `json:"environment,omitempty"`
	// Desktop is the legacy key for the environment selection
	// (ADR-0012 rename). Load migrates it into Environment on read;
	// nothing writes it.
	Desktop string `json:"desktop,omitempty"`
	// Capabilities are the selected capability concepts (ADR-0011).
	// The experience list is derived from them; an empty list marks a
	// pre-capability (v1) selection, which apply still honors via its
	// stored Experiences.
	Capabilities []string       `json:"capabilities,omitempty"`
	Experiences  []string       `json:"experiences,omitempty"`
	Workspace    WorkspacePrefs `json:"workspace"`
	LastApply    *ApplyLog      `json:"last_apply,omitempty"`
}

// SelectionPath returns the onboarding selection file path.
func SelectionPath() (string, error) {
	dir, err := settings.StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, SelectionFile), nil
}

// Load reads the onboarding selection. A missing file is an empty
// selection, not an error; a corrupt file is reported as an error.
func Load() (Selection, error) {
	var sel Selection
	path, err := SelectionPath()
	if err != nil {
		return sel, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return sel, nil
		}
		return sel, fmt.Errorf("read onboarding selection %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &sel); err != nil {
		return sel, fmt.Errorf("parse onboarding selection %s: %w", path, err)
	}
	sel.migrate()
	return sel, nil
}

// migrate moves the legacy "desktop" selection key into
// "environment" (ADR-0012). The loader is the single migration point;
// saves always write the canonical key.
func (s *Selection) migrate() {
	if s.Environment == "" && s.Desktop != "" {
		s.Environment = s.Desktop
	}
	s.Desktop = ""
}

// Save atomically persists the selection and refreshes UpdatedAt.
func (s *Selection) Save() error {
	s.SchemaVersion = SchemaVersion
	s.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	path, err := SelectionPath()
	if err != nil {
		return err
	}
	if _, err := settings.EnsureStateDir(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("encode onboarding selection: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write onboarding selection %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("commit onboarding selection %s: %w", path, err)
	}
	return nil
}

// MergeWorkspace overlays the wizard subset over the profile baseline:
// explicitly chosen fields win, everything else is inherited. This is
// the customization semantic: customizing the prompt never silently
// drops the profile's aliases or environment.
func MergeWorkspace(base workspace.Preferences, sel WorkspacePrefs) workspace.Preferences {
	out := base
	if sel.Prompt != "" {
		out.Prompt = sel.Prompt
	}
	if sel.Tmux != nil {
		out.Tmux = *sel.Tmux
	}
	if sel.Editor != "" {
		out.Editor = sel.Editor
	}
	if sel.Terminal != "" {
		out.Terminal = sel.Terminal
	}
	return out
}

// SeedCapabilities returns the canonical fresh-wizard seeding for a
// profile: its recommended capabilities plus the always included base
// (capability.Seed). Optional capabilities are offered, not
// pre-selected. This is the initial capability selection; the engineer
// may toggle anything except the base.
func SeedCapabilities(reg *profile.Registry, capReg *capability.Registry, cat *experience.Catalog, profileName string) ([]string, error) {
	m, err := reg.Load(profileName)
	if err != nil {
		return nil, err
	}
	res, err := capability.NewResolver(capReg, cat)
	if err != nil {
		return nil, err
	}
	return res.Seed(m.Recommended, m.Optional)
}

// Derive normalizes the selection to the capability model: a
// pre-capability selection (no capabilities, experiences stored) is
// backfilled with the capabilities its experiences implement, and the
// experience list is re-derived from the capabilities. After Derive,
// Experiences is always the derived state and the two never disagree.
// An empty capability set is left untouched (pure v1 selections still
// apply by their stored experiences).
func (s *Selection) Derive(res *capability.Resolver) error {
	if len(s.Capabilities) == 0 && len(s.Experiences) > 0 {
		s.Capabilities = res.CapabilitiesOf(s.Experiences)
	}
	if len(s.Capabilities) == 0 {
		return nil
	}
	exps, err := res.ExperiencesFor(s.Capabilities)
	if err != nil {
		return err
	}
	s.Experiences = exps
	return nil
}

// Problems validates the selection against the registries: unknown
// profiles, unknown capabilities, unknown or non-installable
// experiences, environment references in the capability step, and
// invalid workspace subset values. The plan refuses to apply while
// problems exist.
func (s Selection) Problems(reg *profile.Registry, capReg *capability.Registry, cat *experience.Catalog) []string {
	var out []string
	if s.Profile == "" {
		out = append(out, "no profile selected")
	} else if _, err := reg.Load(s.Profile); err != nil {
		out = append(out, fmt.Sprintf("profile %q is not known", s.Profile))
	}

	if len(s.Capabilities) > 0 {
		res, err := capability.NewResolver(capReg, cat)
		if err != nil {
			out = append(out, fmt.Sprintf("capability mapping unavailable: %v", err))
		} else if unknown, verr := res.Validate(s.Capabilities); verr != nil {
			out = append(out, fmt.Sprintf("unknown capability reference(s): %s", strings.Join(unknown, ", ")))
		}
	}

	if s.Environment != "" {
		m, err := cat.Load(s.Environment)
		switch {
		case err != nil:
			out = append(out, fmt.Sprintf("environment %q is not a known experience", s.Environment))
		case m.Type != experience.TypeEnvironment:
			out = append(out, fmt.Sprintf("environment %q is not an environment experience", s.Environment))
		case m.RPM == "":
			out = append(out, fmt.Sprintf("environment %q is planned and not installable", s.Environment))
		}
	}

	for _, name := range s.Experiences {
		m, err := cat.Load(name)
		switch {
		case err != nil:
			out = append(out, fmt.Sprintf("experience %q is not known", name))
		case m.Type == experience.TypeEnvironment:
			out = append(out, fmt.Sprintf("experience %q is an environment; it belongs in the environment step", name))
		case m.RPM == "":
			out = append(out, fmt.Sprintf("experience %q is planned and not installable", name))
		}
	}

	if s.Workspace.Prompt != "" &&
		s.Workspace.Prompt != workspace.PromptPlain &&
		s.Workspace.Prompt != workspace.PromptVeilbox {
		out = append(out, fmt.Sprintf("unknown prompt %q (want %q or %q)",
			s.Workspace.Prompt, workspace.PromptPlain, workspace.PromptVeilbox))
	}
	if s.Workspace.Editor != "" && !validEditor(s.Workspace.Editor) {
		out = append(out, fmt.Sprintf("invalid editor %q", s.Workspace.Editor))
	}
	if s.Workspace.Terminal != "" && !validTerminalChoice(s.Workspace.Terminal) {
		out = append(out, fmt.Sprintf("unknown terminal %q", s.Workspace.Terminal))
	}
	return out
}

func validEditor(e string) bool {
	return e == "" || !strings.ContainsAny(e, " \t;|&$`()<>*?[]{}\"'\\")
}

// validTerminalChoice mirrors the workspace engine's known terminals.
func validTerminalChoice(t string) bool {
	switch t {
	case workspace.TerminalAuto, workspace.TerminalKitty, workspace.TerminalWezterm,
		workspace.TerminalGhostty, workspace.TerminalAlacritty:
		return true
	}
	return false
}
