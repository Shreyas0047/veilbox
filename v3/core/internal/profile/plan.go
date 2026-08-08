package profile

import (
	"sort"

	"github.com/Shreyas0047/veilbox/v3/core/internal/capability"
	"github.com/Shreyas0047/veilbox/v3/core/internal/experience"
)

// Plan is the deterministic difference between current machine state
// and a profile's intent. It is computed purely from catalog statuses
// provided by the Experience Engine; the Profile Engine never touches
// DNF directly. Recommended capabilities are resolved to the
// experiences that implement them through the capability mapping; the
// machine state is compared at the experience level.
type Plan struct {
	Profile string

	// RecommendedCaps are the profile's recommended capabilities
	// (intent concepts, for presentation).
	RecommendedCaps []string
	// OptionalCaps are the profile's optional capabilities.
	OptionalCaps []string
	// UnknownCapabilities are recommended capabilities that do not
	// exist in the capability registry; their experiences cannot be
	// derived and sync skips them with a warning.
	UnknownCapabilities []string

	// MissingRecommended are derived experiences that are
	// installable and not installed — the sync candidate set.
	MissingRecommended []string
	// NotInstallable are derived experiences that are only
	// 'planned' in the catalog (no package exists yet); sync skips
	// them with a warning.
	NotInstallable []string
	// UnknownRecommended are derived experiences absent from the
	// catalog (typos or not yet shipped); sync skips them.
	UnknownRecommended []string
	// Satisfied are derived experiences already installed.
	Satisfied []string
	// OptionalInstalled are optional derived experiences already
	// installed.
	OptionalInstalled []string
	// OptionalMissing are optional derived experiences not installed.
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
		len(p.UnknownRecommended) == 0 &&
		len(p.UnknownCapabilities) == 0
}

// Diff computes the plan for the named profile against the current
// catalog state. Output order is deterministic (sorted).
func Diff(reg *Registry, capReg *capability.Registry, cat *experience.Catalog, name string) (Plan, error) {
	m, err := reg.Load(name)
	if err != nil {
		return Plan{}, err
	}
	res, err := capability.NewResolver(capReg, cat)
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

	recExp, unknownCaps := resolveCaps(res, m.Recommended)
	optExp, _ := resolveCaps(res, m.Optional)

	p := Plan{
		Profile:             m.Name,
		RecommendedCaps:     append([]string{}, m.Recommended...),
		OptionalCaps:        append([]string{}, m.Optional...),
		UnknownCapabilities: unknownCaps,
	}

	for _, e := range recExp {
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
	for _, e := range optExp {
		if installed[e] {
			p.OptionalInstalled = append(p.OptionalInstalled, e)
		} else {
			p.OptionalMissing = append(p.OptionalMissing, e)
		}
	}

	refs := make(map[string]bool)
	for _, e := range append(append([]string{}, recExp...), optExp...) {
		refs[e] = true
	}
	for name, isInstalled := range installed {
		if isInstalled && !refs[name] {
			p.Extras = append(p.Extras, name)
		}
	}

	p.sort()
	return p, nil
}

// resolveCaps resolves capability names to the experiences that
// implement them. Capabilities unknown to the registry are returned
// separately; their experiences cannot be derived.
func resolveCaps(res *capability.Resolver, caps []string) (exps, unknown []string) {
	seenExp := make(map[string]bool)
	for _, c := range caps {
		derived, err := res.ExperiencesFor([]string{c})
		if err != nil {
			unknown = append(unknown, c)
			continue
		}
		for _, e := range derived {
			if !seenExp[e] {
				seenExp[e] = true
				exps = append(exps, e)
			}
		}
	}
	return exps, unknown
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

// CheckCapabilities returns the recommended/optional capability names
// referenced by the profile that do not exist in the capability
// registry, plus experience capability references that are unknown.
// This is the cross-engine consistency check for doctor and apply.
func CheckCapabilities(reg *Registry, capReg *capability.Registry, cat *experience.Catalog, name string) ([]string, error) {
	m, err := reg.Load(name)
	if err != nil {
		return nil, err
	}
	res, err := capability.NewResolver(capReg, cat)
	if err != nil {
		return nil, err
	}
	missing, err := res.Validate(m.AllReferences())
	if err != nil {
		return missing, nil
	}
	return nil, nil
}

func (p *Plan) sort() {
	sort.Strings(p.RecommendedCaps)
	sort.Strings(p.OptionalCaps)
	sort.Strings(p.UnknownCapabilities)
	sort.Strings(p.MissingRecommended)
	sort.Strings(p.NotInstallable)
	sort.Strings(p.UnknownRecommended)
	sort.Strings(p.Satisfied)
	sort.Strings(p.OptionalInstalled)
	sort.Strings(p.OptionalMissing)
	sort.Strings(p.Extras)
}
