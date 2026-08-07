// Package profile implements the Profile Engine.
//
// A profile declares engineer intent: a role plus selected
// capabilities. Profiles are configuration — YAML manifests shipped by
// veilbox-core and state persisted by Veilbox. Profiles are never
// represented as RPMs and never install packages directly; capability
// implementation belongs to experiences (see ADR-0001).
package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Shreyas0047/veilbox/v3/core/internal/settings"
)

// Capability domains used by profile manifests.
const (
	DomainCloud          = "cloud"
	DomainContainers     = "containers"
	DomainKubernetes     = "kubernetes"
	DomainInfrastructure = "infrastructure"
	DomainObservability  = "observability"
)

// Manifest is a profile definition file (profiles/<name>.yaml).
type Manifest struct {
	Name        string              `yaml:"name"`
	Description string              `yaml:"description"`
	Capabilities map[string][]string `yaml:"capabilities"`
}

// CapabilityNames returns the set of capability names declared by the
// manifest, in deterministic order.
func (m Manifest) CapabilityNames() []string {
	var out []string
	for _, caps := range m.Capabilities {
		out = append(out, caps...)
	}
	sort.Strings(out)
	return out
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

// Load reads a single profile manifest by name.
func (r *Registry) Load(name string) (Manifest, error) {
	if name == "" || strings.Contains(name, string(filepath.Separator)) {
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
	return m, nil
}

// List returns all profile names in the registry.
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
