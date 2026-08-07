package profile

import (
	"sort"

	"github.com/Shreyas0047/veilbox/v3/core/internal/experience"
)

// Plan is the deterministic difference between current machine state
// and a profile's intent. It is computed purely from catalog statuses
// provided by the Experience Engine; the Profile Engine never touches
// DNF directly.
type Plan struct {
	Profile string

	// MissingRecommended are recommended experiences that are
	// installable and not installed — the sync candidate set.
	MissingRecommended []string
	// NotInstallable are recommended experiences that are only
	// 'planned' in the catalog (no package exists yet); sync skips
	// them with a warning.
	NotInstallable []string
	// UnknownRecommended are recommended experiences absent from the
	// catalog (typos or not yet shipped); sync skips them.
	UnknownRecommended []string
	// Satisfied are recommended experiences already installed.
	Satisfied []string
	// OptionalInstalled are optional experiences already installed.
	OptionalInstalled []string
	// OptionalMissing are optional experiences not installed.
	OptionalMissing []string
	// Extras are installed experiences not referenced by the profile.
	// They are reported for information and never removed: the profile
	// is a desired baseline, not an enforced prison (ADR-0004).
	Extras []string
}

// Synced reports whether the machine meets the profile baseline:
// nothing installable is missing and nothing recommended is deferred
// or unknown. Extra installed experiences do not affect sync state.
func (p Plan) Synced() bool {
	return len(p.MissingRecommended) == 0 &&
		len(p.NotInstallable) == 0 &&
		len(p.UnknownRecommended) == 0
}

// Diff computes the plan for the named profile against the current
// catalog state. Output order is deterministic (sorted).
func Diff(reg *Registry, cat *experience.Catalog, name string) (Plan, error) {
	m, err := reg.Load(name)
	if err != nil {
		return Plan{}, err
	}
	entries, err := cat.List()
	if err != nil {
		return Plan{}, err
	}

	byName := make(map[string]experience.Entry, len(entries))
	installed := make(map[string]bool, len(entries))
	for _, e := range entries {
		byName[e.Name] = e
		if e.Status == experience.StatusInstalled {
			installed[e.Name] = true
		}
	}

	refs := make(map[string]bool, len(m.Recommended)+len(m.Optional))
	for _, e := range m.AllReferences() {
		refs[e] = true
	}

	p := Plan{Profile: m.Name}

	for _, e := range m.Recommended {
		entry, known := byName[e]
		switch {
		case !known:
			p.UnknownRecommended = append(p.UnknownRecommended, e)
		case entry.Status == experience.StatusPlanned:
			p.NotInstallable = append(p.NotInstallable, e)
		case installed[e]:
			p.Satisfied = append(p.Satisfied, e)
		default:
			p.MissingRecommended = append(p.MissingRecommended, e)
		}
	}
	for _, e := range m.Optional {
		if installed[e] {
			p.OptionalInstalled = append(p.OptionalInstalled, e)
		} else {
			p.OptionalMissing = append(p.OptionalMissing, e)
		}
	}
	for name, isInstalled := range installed {
		if isInstalled && !refs[name] {
			p.Extras = append(p.Extras, name)
		}
	}

	p.sort()
	return p, nil
}

// SyncPlan returns the ordered list of experiences to install to reach
// the profile baseline: exactly the missing installable recommended
// experiences. Not-installable and unknown recommendations are never
// included; callers surface them as warnings.
func SyncPlan(p Plan) []string {
	out := make([]string, len(p.MissingRecommended))
	copy(out, p.MissingRecommended)
	return out
}

// CheckReferences returns the recommended/optional experience names
// referenced by the profile that do not exist in the catalog, sorted.
// This is the cross-engine consistency check for doctor and apply.
func CheckReferences(reg *Registry, cat *experience.Catalog, name string) ([]string, error) {
	m, err := reg.Load(name)
	if err != nil {
		return nil, err
	}
	entries, err := cat.List()
	if err != nil {
		return nil, err
	}
	known := make(map[string]bool, len(entries))
	for _, e := range entries {
		known[e.Name] = true
	}
	var missing []string
	for _, e := range m.AllReferences() {
		if !known[e] {
			missing = append(missing, e)
		}
	}
	sort.Strings(missing)
	return missing, nil
}

func (p *Plan) sort() {
	sort.Strings(p.MissingRecommended)
	sort.Strings(p.NotInstallable)
	sort.Strings(p.UnknownRecommended)
	sort.Strings(p.Satisfied)
	sort.Strings(p.OptionalInstalled)
	sort.Strings(p.OptionalMissing)
	sort.Strings(p.Extras)
}
