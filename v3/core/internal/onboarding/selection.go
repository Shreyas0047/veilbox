// Package onboarding implements the Veilbox composition flow: a
// terminal wizard that turns engineer intent into a complete,
// previewable workstation plan.
//
// Onboarding is an orchestration layer, never a second state system:
// the Profile, Experience, Workspace and Desktop engines remain the
// authoritative state holders. The onboarding selection file
// (~/.config/veilbox/onboarding.json) is a draft of intent plus an
// apply log — engines decide truth, onboarding only sequences.
package onboarding

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Shreyas0047/veilbox/v3/core/internal/experience"
	"github.com/Shreyas0047/veilbox/v3/core/internal/profile"
	"github.com/Shreyas0047/veilbox/v3/core/internal/settings"
	"github.com/Shreyas0047/veilbox/v3/core/internal/workspace"
)

// SchemaVersion of the onboarding selection file.
const SchemaVersion = 1

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
	SchemaVersion int            `json:"schema_version"`
	UpdatedAt     string         `json:"updated_at,omitempty"`
	Profile       string         `json:"profile,omitempty"`
	Desktop       string         `json:"desktop,omitempty"`
	Experiences   []string       `json:"experiences,omitempty"`
	Workspace     WorkspacePrefs `json:"workspace"`
	LastApply     *ApplyLog      `json:"last_apply,omitempty"`
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
	return sel, nil
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

// ProfileRecommendations returns the recommended ∪ optional
// experiences of a profile manifest, filtered to installable tooling
// experiences. This is the initial capability selection; the engineer
// may toggle anything afterwards.
func ProfileRecommendations(reg *profile.Registry, cat *experience.Catalog, profileName string) ([]string, error) {
	m, err := reg.Load(profileName)
	if err != nil {
		return nil, err
	}
	entries, err := cat.List()
	if err != nil {
		return nil, err
	}
	installable := make(map[string]bool)
	for _, e := range entries {
		if e.Type != experience.TypeDesktop && e.RPM != "" {
			installable[e.Name] = true
		}
	}
	seen := make(map[string]bool)
	var out []string
	for _, name := range append(append([]string{}, m.Recommended...), m.Optional...) {
		if installable[name] && !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out, nil
}

// Problems validates the selection against the registries: unknown
// profiles, unknown or non-installable experiences, desktop references
// in the capability step, and invalid workspace subset values. The
// plan refuses to apply while problems exist.
func (s Selection) Problems(reg *profile.Registry, cat *experience.Catalog) []string {
	var out []string
	if s.Profile == "" {
		out = append(out, "no profile selected")
	} else if _, err := reg.Load(s.Profile); err != nil {
		out = append(out, fmt.Sprintf("profile %q is not known", s.Profile))
	}

	if s.Desktop != "" {
		m, err := cat.Load(s.Desktop)
		switch {
		case err != nil:
			out = append(out, fmt.Sprintf("desktop %q is not a known experience", s.Desktop))
		case m.Type != experience.TypeDesktop:
			out = append(out, fmt.Sprintf("desktop %q is not a desktop experience", s.Desktop))
		case m.RPM == "":
			out = append(out, fmt.Sprintf("desktop %q is planned and not installable", s.Desktop))
		}
	}

	for _, name := range s.Experiences {
		m, err := cat.Load(name)
		switch {
		case err != nil:
			out = append(out, fmt.Sprintf("experience %q is not known", name))
		case m.Type == experience.TypeDesktop:
			out = append(out, fmt.Sprintf("experience %q is a desktop; it belongs in the desktop step", name))
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
