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
	"github.com/Shreyas0047/veilbox/v3/core/internal/settings"
)

// CompositionFile is the applied product record file name (ADR-0010).
// Unlike the onboarding draft (Selection), the composition is written
// only by the apply path, is recreated (never edited) on every apply,
// and is the source of truth for 'veil status' and 'veil doctor'.
const CompositionFile = "composition.json"

// CompositionSchemaVersion of the composition record.
const CompositionSchemaVersion = 1

// EnvironmentRecord snapshots the applied environment experience at
// apply time: the manifest identity and its concrete component stack.
type EnvironmentRecord struct {
	Name        string            `json:"name"`
	DisplayName string            `json:"display_name,omitempty"`
	RPM         string            `json:"rpm,omitempty"`
	Components  map[string]string `json:"components,omitempty"`
}

// WorkspaceRecord is the merged workspace preferences of the applied
// product (profile baseline overlaid with the engineer's choices).
type WorkspaceRecord struct {
	Shell       string            `json:"shell,omitempty"`
	Editor      string            `json:"editor,omitempty"`
	Terminal    string            `json:"terminal,omitempty"`
	Prompt      string            `json:"prompt,omitempty"`
	Tmux        bool              `json:"tmux,omitempty"`
	Aliases     map[string]string `json:"aliases,omitempty"`
	Environment map[string]string `json:"environment,omitempty"`
}

// ValidationRecord records the compatibility validation result of the
// applied composition (ADR-0010: the record carries it; ADR-0014: the
// composition is re-validated before any engine call).
type ValidationRecord struct {
	Valid       bool     `json:"valid"`
	Notes       []string `json:"notes,omitempty"`
	ValidatedAt string   `json:"validated_at,omitempty"`
}

// Composition is the engineer's complete, applied Veilbox product
// (ADR-0010). It is produced by apply, never pre-authored, and
// replaced atomically on every successful apply.
type Composition struct {
	SchemaVersion int                `json:"schema_version"`
	AppliedAt     string             `json:"applied_at"`
	Profile       string             `json:"profile,omitempty"`
	Capabilities  []string           `json:"capabilities,omitempty"`
	Experiences   []string           `json:"experiences,omitempty"`
	Environment   *EnvironmentRecord `json:"environment,omitempty"`
	Workspace     WorkspaceRecord    `json:"workspace,omitempty"`
	Validation    ValidationRecord   `json:"validation,omitempty"`
}

// CompositionPath returns the composition record file path.
func CompositionPath() (string, error) {
	dir, err := settings.StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, CompositionFile), nil
}

// LoadComposition reads the composition record. A missing file is an
// empty record (zero SchemaVersion), not an error; a corrupt file is
// reported so 'veil status' never guesses about it.
func LoadComposition() (Composition, error) {
	var c Composition
	path, err := CompositionPath()
	if err != nil {
		return c, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil
		}
		return c, fmt.Errorf("read composition %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return c, fmt.Errorf("parse composition %s: %w", path, err)
	}
	return c, nil
}

// Save atomically persists the composition record: the previous
// record is replaced only when the new one is fully written
// (write-temp-rename). An aborted apply therefore never leaves a
// half-written product record behind (ADR-0010).
func (c *Composition) Save() error {
	c.SchemaVersion = CompositionSchemaVersion
	path, err := CompositionPath()
	if err != nil {
		return err
	}
	if _, err := settings.EnsureStateDir(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("encode composition: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write composition %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("commit composition %s: %w", path, err)
	}
	return nil
}

// recordComposition builds the product record from an applied
// selection and the engines' authoritative state, re-validating the
// composition before it is recorded (ADR-0014 apply-time validation).
func recordComposition(sel Selection, in PlanInputs) (Composition, error) {
	c := Composition{
		AppliedAt:    time.Now().UTC().Format(time.RFC3339),
		Profile:      sel.Profile,
		Capabilities: sortedCopy(sel.Capabilities),
		Experiences:  sortedCopy(sel.Experiences),
	}

	if sel.Environment != "" {
		m, err := in.Catalog.Load(sel.Environment)
		if err != nil {
			return c, fmt.Errorf("record composition: environment %q: %w", sel.Environment, err)
		}
		c.Environment = &EnvironmentRecord{
			Name:        m.Name,
			DisplayName: m.DisplayName,
			RPM:         m.RPM,
			Components:  m.Components,
		}
	}

	if sel.Profile != "" {
		if m, err := in.Registry.Load(sel.Profile); err == nil {
			prefs := MergeWorkspace(m.Workspace, sel.Workspace)
			c.Workspace = WorkspaceRecord{
				Shell:       prefs.Shell,
				Editor:      prefs.Editor,
				Terminal:    prefs.Terminal,
				Prompt:      prefs.Prompt,
				Tmux:        prefs.Tmux,
				Aliases:     prefs.Aliases,
				Environment: prefs.Environment,
			}
		}
	}

	c.Validation = validateComposition(sel, in)
	return c, nil
}

// validateComposition re-validates the applied composition against
// the registries and the environment data contract (ADR-0014,
// ADR-0015) and records the outcome on the composition record.
// Selection problems invalidate the record; the environment contract
// declaration is informational (what the record stands on).
func validateComposition(sel Selection, in PlanInputs) ValidationRecord {
	v := ValidationRecord{ValidatedAt: time.Now().UTC().Format(time.RFC3339)}
	for _, p := range sel.Problems(in.Registry, in.Capabilities, in.Catalog) {
		v.Notes = append(v.Notes, "problem: "+p)
	}
	if sel.Environment != "" {
		m, err := in.Catalog.Load(sel.Environment)
		switch {
		case err != nil:
			v.Notes = append(v.Notes, fmt.Sprintf("problem: environment %q: %v", sel.Environment, err))
		case m.Type != experience.TypeEnvironment:
			v.Notes = append(v.Notes, fmt.Sprintf("problem: environment %q is not an environment experience", sel.Environment))
		default:
			contract := contractCount(m.Environment)
			v.Notes = append(v.Notes, fmt.Sprintf("environment %q contract: %d config file(s), %d managed file(s), %d validation declaration(s)",
				m.Name, contract[0], contract[1], contract[2]))
		}
	}
	if len(v.Notes) == 0 {
		v.Valid = true
		v.Notes = []string{"composition re-validated against the registries; no drift"}
		return v
	}
	v.Valid = true
	for _, n := range v.Notes {
		if strings.HasPrefix(n, "problem:") {
			v.Valid = false
			break
		}
	}
	return v
}

// contractCount returns the declared config/managed/validation-hook
// counts of an environment's data contract (ADR-0015).
func contractCount(env *experience.EnvSpec) [3]int {
	if env == nil {
		return [3]int{}
	}
	return [3]int{len(env.Config), len(env.Managed), len(env.Validate.Files) + len(env.Validate.Commands)}
}

func sortedCopy(in []string) []string {
	out := append([]string{}, in...)
	sort.Strings(out)
	return out
}
