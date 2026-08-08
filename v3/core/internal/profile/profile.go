// Package profile implements the Profile Engine.
//
// A profile declares engineer intent: who the engineer is (role), what
// they likely need (recommended capabilities), what they might want
// (optional capabilities), and workspace preferences. Capabilities are
// concepts (see the capability package); experiences implement them
// and are derived by the capability mapping. Profiles are
// configuration — YAML manifests shipped by veilbox-core and state
// persisted by Veilbox. Profiles are never represented as RPMs and
// never install packages directly (ADR-0001, ADR-0011).
//
// The profile is a desired baseline, not an enforced prison: applying
// or syncing a profile never removes experiences the engineer chose
// manually (see ADR-0004). Profiles never declare an environment:
// role and environment are independent axes (ADR-0013).
package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Shreyas0047/veilbox/v3/core/internal/settings"
	"github.com/Shreyas0047/veilbox/v3/core/internal/workspace"
)

// namePattern matches manifest names and capability references:
// lowercase letters, digits, and hyphens.
var namePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// Manifest is a profile definition file (profiles/<name>.yaml).
//
// DisplayName and Role are presentation metadata; RecommendedCaps and
// OptionalCaps are the intent: capability concepts the engineer likely
// needs (recommended) or may want (optional). Profiles never declare
// experiences directly — the capability mapping derives them — and
// never declare an environment (ADR-0013). Workspace holds the
// preferences the Workspace Engine translates into user-level
// workspace configuration.
type Manifest struct {
	Name        string                `yaml:"name"`
	DisplayName string                `yaml:"display_name,omitempty"`
	Description string                `yaml:"description"`
	Role        string                `yaml:"role,omitempty"`
	Recommended []string              `yaml:"recommended_capabilities,omitempty"`
	Optional    []string              `yaml:"optional_capabilities,omitempty"`
	Tags        []string              `yaml:"tags,omitempty"`
	Workspace   workspace.Preferences `yaml:"workspace_preferences,omitempty"`
}

// AllReferences returns the sorted union of recommended and optional
// capability names referenced by the manifest.
func (m Manifest) AllReferences() []string {
	seen := make(map[string]bool, len(m.Recommended)+len(m.Optional))
	for _, e := range m.Recommended {
		seen[e] = true
	}
	for _, e := range m.Optional {
		seen[e] = true
	}
	out := make([]string, 0, len(seen))
	for e := range seen {
		out = append(out, e)
	}
	sort.Strings(out)
	return out
}

// Validate checks the manifest structurally: naming, required fields,
// and duplicate-free capability reference lists. It does not verify
// that referenced capabilities exist in the registry (see
// CheckCapabilities).
func (m Manifest) Validate() error {
	if !namePattern.MatchString(m.Name) {
		return fmt.Errorf("invalid profile name %q (want lowercase letters, digits, hyphens)", m.Name)
	}
	if m.Description == "" {
		return fmt.Errorf("profile %q: description is required", m.Name)
	}
	for _, list := range [][]string{m.Recommended, m.Optional} {
		seen := make(map[string]bool, len(list))
		for _, e := range list {
			if !namePattern.MatchString(e) {
				return fmt.Errorf("profile %q: invalid capability reference %q", m.Name, e)
			}
			if seen[e] {
				return fmt.Errorf("profile %q: duplicate capability reference %q", m.Name, e)
			}
			seen[e] = true
		}
	}
	return nil
}

// fillDefaults applies display-name and role defaults after loading.
func (m *Manifest) fillDefaults() {
	if m.DisplayName == "" {
		m.DisplayName = m.Name
	}
	if m.Role == "" {
		m.Role = m.Name
	}
}

// Registry loads profile manifests from a directory.
type Registry struct {
	dir string
}

// NewRegistry returns a Registry rooted at the system profiles dir.
func NewRegistry() *Registry {
	return &Registry{dir: settings.SystemProfilesDir()}
}

// NewRegistryDir returns a Registry over an explicit directory (tests).
func NewRegistryDir(dir string) *Registry {
	return &Registry{dir: dir}
}

// Dir returns the registry directory.
func (r *Registry) Dir() string { return r.dir }

// Load reads and validates a single profile manifest by name.
func (r *Registry) Load(name string) (Manifest, error) {
	if !namePattern.MatchString(name) {
		return Manifest{}, fmt.Errorf("invalid profile name %q", name)
	}
	data, err := os.ReadFile(filepath.Join(r.dir, name+".yaml"))
	if err != nil {
		if os.IsNotExist(err) {
			return Manifest{}, fmt.Errorf("profile %q not found in %s", name, r.dir)
		}
		return Manifest{}, fmt.Errorf("read profile %s: %w", name, err)
	}
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("parse profile %s: %w", name, err)
	}
	if m.Name != name {
		return Manifest{}, fmt.Errorf("profile %s declares name %q", name, m.Name)
	}
	if err := m.Validate(); err != nil {
		return Manifest{}, err
	}
	m.fillDefaults()
	return m, nil
}

// List returns all profile names in the registry, sorted.
func (r *Registry) List() ([]string, error) {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return nil, fmt.Errorf("read profiles dir %s: %w", r.dir, err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		names = append(names, strings.TrimSuffix(e.Name(), ".yaml"))
	}
	sort.Strings(names)
	return names, nil
}

// Apply sets the active profile in Veilbox state. It validates that the
// profile exists but does not install anything: profiles are intent.
func Apply(name string) (settings.State, error) {
	return ApplyWith(NewRegistry(), name)
}

// ApplyWith applies a profile against an explicit registry (tests).
func ApplyWith(reg *Registry, name string) (settings.State, error) {
	if _, err := reg.Load(name); err != nil {
		return settings.State{}, err
	}
	st, err := settings.LoadState()
	if err != nil {
		return settings.State{}, err
	}
	st.ActiveProfile = name
	st.AppliedAt = time.Now().UTC().Format(time.RFC3339)
	st.Version = "0.1.0"
	if err := settings.SaveState(st); err != nil {
		return settings.State{}, err
	}
	return st, nil
}

// Active returns the current state (which may have no active profile).
func Active() (settings.State, error) {
	return settings.LoadState()
}
