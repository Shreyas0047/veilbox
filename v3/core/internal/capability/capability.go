// Package capability implements the Capability Engine (ADR-0011).
//
// A capability is engineer intent at the concept level: what the
// engineer wants to be able to do (networking, kubernetes, security).
// Capabilities are first-class manifests shipped by veilbox-core and
// are the only thing the onboarding wizard and profiles express.
// Experiences — the concrete RPM meta-packages — implement
// capabilities through an N→M mapping declared on experience
// manifests; the Resolver derives the required experience set from a
// selected capability set. Users pick capabilities, never RPMs.
//
// BaseName is the one structural exception: the universal engineering
// base is implicitly included in every composition and cannot be
// deselected.
package capability

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Shreyas0047/veilbox/v3/core/internal/experience"
	"github.com/Shreyas0047/veilbox/v3/core/internal/safetoken"
	"github.com/Shreyas0047/veilbox/v3/core/internal/settings"
)

// namePattern matches manifest names and capability references:
// lowercase letters, digits, and hyphens.
var namePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// Tier of a capability: how fundamental it is to an engineering role.
type Tier string

const (
	// TierCore is the foundation of every composition (role-level
	// capabilities: networking, containers, kubernetes, ...).
	TierCore Tier = "core"
	// TierTooling is additive craftsmanship: comfort and productivity
	// beyond the core role.
	TierTooling Tier = "tooling"
	// TierExpert is deep specialization, always opt-in.
	TierExpert Tier = "expert"
)

// BaseName is the universally included capability: version control,
// editor, transfer and debugging tools. Every composition includes
// it; the wizard renders it as required and locked.
const BaseName = "base-operations"

// Manifest is a capability definition (capabilities/<name>.yaml).
type Manifest struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	// Domain groups capabilities in the onboarding capabilities step
	// (concept grouping, presentation text).
	Domain string `yaml:"domain"`
	// Tier classifies how fundamental the capability is.
	Tier Tier `yaml:"tier,omitempty"`
	// Provides lists the interface tokens the capability contributes
	// to the composition (validated against the compatibility model
	// in a later phase; ADR-0014).
	Provides []string `yaml:"provides,omitempty"`
}

// Validate checks the manifest structurally: naming, required fields,
// tier enum and safe tokens.
func (m Manifest) Validate() error {
	if !namePattern.MatchString(m.Name) {
		return fmt.Errorf("invalid capability name %q (want lowercase letters, digits, hyphens)", m.Name)
	}
	if m.Description == "" {
		return fmt.Errorf("capability %q: description is required", m.Name)
	}
	if m.Domain == "" {
		return fmt.Errorf("capability %q: domain is required", m.Name)
	}
	if !safetoken.ValidCommand(m.Domain) {
		return fmt.Errorf("capability %q: domain %q uses invalid characters (presentation text, safe tokens only)", m.Name, m.Domain)
	}
	switch m.Tier {
	case "", TierCore:
		m.Tier = TierCore
	case TierTooling, TierExpert:
	default:
		return fmt.Errorf("capability %q: unknown tier %q (want %q, %q or %q)", m.Name, m.Tier, TierCore, TierTooling, TierExpert)
	}
	for _, t := range m.Provides {
		if !safetoken.Valid(t) {
			return fmt.Errorf("capability %q: provides token %q uses invalid characters (safe tokens only)", m.Name, t)
		}
	}
	return nil
}

// Registry loads capability manifests from a directory.
type Registry struct {
	dir string
}

// NewRegistry returns a Registry rooted at the system capabilities dir.
func NewRegistry() *Registry {
	return &Registry{dir: settings.SystemCapabilitiesDir()}
}

// NewRegistryDir returns a Registry over an explicit directory (tests).
func NewRegistryDir(dir string) *Registry {
	return &Registry{dir: dir}
}

// Dir returns the registry directory.
func (r *Registry) Dir() string { return r.dir }

// Load reads and validates a single capability manifest by name.
func (r *Registry) Load(name string) (Manifest, error) {
	if !namePattern.MatchString(name) {
		return Manifest{}, fmt.Errorf("invalid capability name %q", name)
	}
	data, err := os.ReadFile(filepath.Join(r.dir, name+".yaml"))
	if err != nil {
		if os.IsNotExist(err) {
			return Manifest{}, fmt.Errorf("capability %q not found in %s", name, r.dir)
		}
		return Manifest{}, fmt.Errorf("read capability %s: %w", name, err)
	}
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("parse capability %s: %w", name, err)
	}
	if m.Name != name {
		return Manifest{}, fmt.Errorf("capability %s declares name %q", name, m.Name)
	}
	if err := m.Validate(); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

// List returns all capability names in the registry, sorted.
func (r *Registry) List() ([]string, error) {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return nil, fmt.Errorf("read capabilities dir %s: %w", r.dir, err)
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

func (r *Registry) loadAll() ([]Manifest, error) {
	names, err := r.List()
	if err != nil {
		return nil, err
	}
	manifests := make([]Manifest, 0, len(names))
	for _, n := range names {
		m, err := r.Load(n)
		if err != nil {
			return nil, err
		}
		manifests = append(manifests, m)
	}
	return manifests, nil
}

// Resolver derives the required experience set from a selected
// capability set, using the experience catalog's declared
// capability references (the N→M mapping of ADR-0011).
type Resolver struct {
	reg   *Registry
	exps  []experience.Manifest
	byCap map[string][]experience.Manifest
	byExp map[string][]string // experience name -> capability names
}

// NewResolver builds a resolver over a capability registry and an
// experience catalog. The catalog is read for manifests only; no
// system state is queried.
func NewResolver(reg *Registry, cat *experience.Catalog) (*Resolver, error) {
	exps, err := cat.Manifests()
	if err != nil {
		return nil, err
	}
	r := &Resolver{reg: reg, exps: exps, byCap: map[string][]experience.Manifest{}, byExp: map[string][]string{}}
	for _, e := range exps {
		for _, c := range e.Capabilities {
			r.byCap[c] = append(r.byCap[c], e)
		}
		r.byExp[e.Name] = append([]string{}, e.Capabilities...)
	}
	return r, nil
}

// Base returns the base capability manifest.
func (r *Resolver) Base() (Manifest, error) {
	return r.reg.Load(BaseName)
}

// ExperiencesFor returns the sorted, deduplicated tooling experience
// names that implement any of the selected capabilities. Planned
// experiences (no package yet) are included: their status is reported
// downstream (plan diff, UI), and apply refuses them via validation.
// Desktop experiences are never derived here: the environment axis is
// chosen separately (ADR-0012).
func (r *Resolver) ExperiencesFor(caps []string) ([]string, error) {
	if _, err := r.Validate(caps); err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	var out []string
	for _, c := range caps {
		for _, e := range r.byCap[c] {
			if e.Type == experience.TypeDesktop {
				continue
			}
			if !seen[e.Name] {
				seen[e.Name] = true
				out = append(out, e.Name)
			}
		}
	}
	sort.Strings(out)
	return out, nil
}

// CapabilitiesOf returns the sorted capability names that any of the
// given experiences implement. This is the inverse mapping used to
// backfill capabilities from pre-capability selections.
func (r *Resolver) CapabilitiesOf(expNames []string) []string {
	seen := make(map[string]bool)
	for _, e := range expNames {
		for _, c := range r.byExp[e] {
			seen[c] = true
		}
	}
	var out []string
	for c := range seen {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

// Seed returns the canonical seeding of a fresh selection for a
// profile: its recommended capabilities plus the always included base.
// Optional capabilities are deliberately NOT pre-selected: they are
// offered in the wizard for the engineer to add (the profile's
// optional axis is a menu, not a default). Sorted and deduplicated.
func (r *Resolver) Seed(rec, opt []string) ([]string, error) {
	if _, err := r.reg.Load(BaseName); err != nil {
		return nil, fmt.Errorf("base capability unavailable: %w", err)
	}
	seen := map[string]bool{BaseName: true}
	var out []string
	for _, c := range rec {
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	out = append(out, BaseName)
	sort.Strings(out)
	return out, nil
}

// Validate checks that every named capability exists in the registry.
// Unknown names are returned sorted; the error is set when any is
// unknown.
func (r *Resolver) Validate(caps []string) ([]string, error) {
	var unknown []string
	for _, c := range caps {
		if _, err := r.reg.Load(c); err != nil {
			unknown = append(unknown, c)
		}
	}
	sort.Strings(unknown)
	if len(unknown) > 0 {
		return unknown, fmt.Errorf("unknown capability reference(s): %s", strings.Join(unknown, ", "))
	}
	return nil, nil
}

// CheckMapping returns the consistency report of the capability ↔
// experience mapping: capabilities with no installable tooling
// experience (planned) and experience capability references that are
// unknown to the registry. This is the cross-engine check for doctor
// and apply.
func (r *Resolver) CheckMapping() (planned []string, unknownRefs []string) {
	caps, err := r.reg.List()
	if err != nil {
		return nil, nil
	}
	for _, c := range caps {
		hasInstallable := false
		for _, e := range r.byCap[c] {
			if e.Type != experience.TypeDesktop && e.RPM != "" {
				hasInstallable = true
				break
			}
		}
		if !hasInstallable {
			planned = append(planned, c)
		}
	}
	for _, e := range r.exps {
		for _, c := range e.Capabilities {
			if _, err := r.reg.Load(c); err != nil {
				unknownRefs = append(unknownRefs, fmt.Sprintf("%s:%s", e.Name, c))
			}
		}
	}
	sort.Strings(planned)
	sort.Strings(unknownRefs)
	return planned, unknownRefs
}
