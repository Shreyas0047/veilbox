// Package experience implements the Experience Engine.
//
// An experience is an installable capability delivered as an RPM
// meta-package (veilbox-experience-<name>). The experience catalog —
// YAML manifests shipped by veilbox-core — describes what each
// experience provides. The RPM database is the source of truth for
// whether an experience is installed; Veilbox never installs packages
// outside DNF.
//
// Status meanings:
//
//	planned    declared in the catalog but not yet packaged/installable
//	available  installable from a configured Veilbox repository
//	installed  present in the RPM database
package experience

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Shreyas0047/veilbox/v3/core/internal/dnfops"
	"github.com/Shreyas0047/veilbox/v3/core/internal/safetoken"
	"github.com/Shreyas0047/veilbox/v3/core/internal/settings"
)

// PackagePrefix is the name prefix of all Veilbox experience RPMs.
const PackagePrefix = "veilbox-experience-"

// Status of an experience.
type Status string

const (
	StatusPlanned   Status = "planned"
	StatusAvailable Status = "available"
	StatusInstalled Status = "installed"
)

// Type classifies an experience. Experiences of type desktop are
// complete desktop stacks, activated through the Desktop Engine
// ('veil desktop'); all other experiences are type tooling.
type Type string

const (
	// TypeTooling is the default: a capability that changes the
	// machine (packages, tooling, services of the engineer's craft).
	TypeTooling Type = "tooling"
	// TypeDesktop is a complete desktop experience: compositor, shell,
	// terminal, integration and defaults — a working desktop at first
	// login, not a bare compositor.
	TypeDesktop Type = "desktop"
)

// Component keys allowed in desktop experience manifests. Values are
// either "builtin" (provided by the shell itself) or a safe-token
// package/binary name — never shell commands.
const (
	CompCompositor     = "compositor"
	CompShell          = "shell"
	CompTerminal       = "terminal"
	CompLauncher       = "launcher"
	CompNotifications  = "notifications"
	CompLock           = "lock"
	CompIdle           = "idle"
	CompWallpaper      = "wallpaper"
	CompClipboard      = "clipboard"
	CompScreenshot     = "screenshot"
	CompDisplayManager = "display_manager"
)

// componentKeys is the complete set of recognized component keys.
var componentKeys = map[string]bool{
	CompCompositor: true, CompShell: true, CompTerminal: true,
	CompLauncher: true, CompNotifications: true, CompLock: true,
	CompIdle: true, CompWallpaper: true, CompClipboard: true,
	CompScreenshot: true, CompDisplayManager: true,
}

// builtinAllowed reports whether a component key may be satisfied by
// "builtin". Structural components (compositor, shell, terminal,
// display manager) must name a real package/binary.
func builtinAllowed(key string) bool {
	switch key {
	case CompCompositor, CompShell, CompTerminal, CompDisplayManager:
		return false
	}
	return true
}

// desktopRequiredComponents are the component keys every desktop
// experience must declare.
var desktopRequiredComponents = []string{
	CompCompositor, CompShell, CompTerminal, CompDisplayManager,
}

// Manifest is an experience catalog entry (experiences/<name>.yaml).
type Manifest struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	// DisplayName is a human-readable name for the experience.
	DisplayName string `yaml:"display_name,omitempty"`
	// Domain is the capability concept the experience belongs to
	// (e.g. "Networking", "Observability"). It groups experiences in
	// the onboarding capability step and in 'veil status' — the UI
	// deals in concepts, never raw package names. Empty means the
	// experience is uncategorized tooling.
	Domain string `yaml:"domain,omitempty"`
	// Type classifies the experience (tooling or desktop); empty means
	// tooling. Desktop experiences declare a components map and are
	// activated only through 'veil desktop' — never by the RPM itself.
	Type Type `yaml:"type,omitempty"`
	// RPM is the package name that implements this experience. Empty
	// means the experience is planned and not yet installable.
	RPM string `yaml:"rpm,omitempty"`
	// Packages lists the concrete Fedora packages the meta-package
	// pulls in (informational; the RPM Requires are authoritative).
	Packages []string `yaml:"packages,omitempty"`
	// Components declares the desktop stack for type: desktop
	// experiences. Values are "builtin" or a safe-token name.
	Components map[string]string `yaml:"components,omitempty"`
}

// Validate checks manifest constraints, including the declarative
// grammar of desktop components.
func (m Manifest) Validate() error {
	if m.Domain != "" && !safetoken.ValidCommand(m.Domain) {
		return fmt.Errorf("experience %q: domain %q uses invalid characters (presentation text, safe tokens only)", m.Name, m.Domain)
	}
	switch m.Type {
	case "", TypeTooling:
		if len(m.Components) > 0 {
			return fmt.Errorf("experience %q: components are only valid for type: desktop", m.Name)
		}
		return nil
	case TypeDesktop:
	default:
		return fmt.Errorf("experience %q: unknown type %q (want %q or %q)", m.Name, m.Type, TypeTooling, TypeDesktop)
	}
	if m.RPM == "" {
		return fmt.Errorf("experience %q: a desktop experience must declare an installable rpm", m.Name)
	}
	if len(m.Components) == 0 {
		return fmt.Errorf("experience %q: a desktop experience must declare components", m.Name)
	}
	for _, key := range desktopRequiredComponents {
		if m.Components[key] == "" {
			return fmt.Errorf("experience %q: desktop component %q is required", m.Name, key)
		}
	}
	for key, val := range m.Components {
		if !componentKeys[key] {
			return fmt.Errorf("experience %q: unknown component %q", m.Name, key)
		}
		if !safetoken.Valid(val) {
			return fmt.Errorf("experience %q: component %q value %q uses invalid characters (safe tokens only)", m.Name, key, val)
		}
		if val == "builtin" && !builtinAllowed(key) {
			return fmt.Errorf("experience %q: component %q must name a package (builtin is not valid here)", m.Name, key)
		}
	}
	return nil
}

// Catalog holds the known experiences merged with system state.
type Catalog struct {
	dir string
	dnf *dnfops.System
}

// NewCatalog returns a Catalog rooted at the system experiences dir.
func NewCatalog() *Catalog {
	return &Catalog{dir: settings.SystemExperiencesDir(), dnf: dnfops.New()}
}

// NewCatalogWith returns a Catalog with explicit dir and DNF facade (tests).
func NewCatalogWith(dir string, dnf *dnfops.System) *Catalog {
	return &Catalog{dir: dir, dnf: dnf}
}

// Entry is a catalog manifest plus its resolved system status.
type Entry struct {
	Manifest
	Status Status
}

// Load reads a catalog entry by name.
func (c *Catalog) Load(name string) (Manifest, error) {
	if name == "" || strings.Contains(name, string(filepath.Separator)) {
		return Manifest{}, fmt.Errorf("invalid experience name %q", name)
	}
	data, err := os.ReadFile(filepath.Join(c.dir, name+".yaml"))
	if err != nil {
		if os.IsNotExist(err) {
			return Manifest{}, fmt.Errorf("experience %q not found in %s", name, c.dir)
		}
		return Manifest{}, fmt.Errorf("read experience %s: %w", name, err)
	}
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("parse experience %s: %w", name, err)
	}
	if m.Name != name {
		return Manifest{}, fmt.Errorf("experience %s declares name %q", name, m.Name)
	}
	if err := m.Validate(); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

// List returns catalog entries with resolved status.
func (c *Catalog) List() ([]Entry, error) {
	manifests, err := c.loadAll()
	if err != nil {
		return nil, err
	}
	installed, err := c.dnf.ListInstalledByPrefix(PackagePrefix)
	if err != nil {
		return nil, err
	}
	installedSet := make(map[string]bool, len(installed))
	for _, p := range installed {
		installedSet[p] = true
	}
	entries := make([]Entry, 0, len(manifests))
	for _, m := range manifests {
		e := Entry{Manifest: m, Status: StatusAvailable}
		if m.RPM == "" {
			e.Status = StatusPlanned
		}
		if m.RPM != "" && installedSet[m.RPM] {
			e.Status = StatusInstalled
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// Install resolves an experience to its RPM and runs a DNF
// transaction by package name through the configured repositories.
// It is idempotent: installing an already-installed experience is a
// no-op report, and DNF itself is a no-op when packages are present.
func (c *Catalog) Install(name string) error {
	m, err := c.Load(name)
	if err != nil {
		return err
	}
	if m.RPM == "" {
		return fmt.Errorf("experience %q is planned; no installable package exists yet", name)
	}
	installed, err := c.dnf.IsInstalled(m.RPM)
	if err != nil {
		return err
	}
	if installed {
		return fmt.Errorf("experience %q is already installed (%s)", name, m.RPM)
	}
	return c.dnf.Transaction("install", "-y", m.RPM)
}

// Remove runs a DNF transaction removing the experience meta-package.
// Only the experience package itself is targeted; the base packages it
// depends on are left to DNF's normal dependency handling, so the
// experience's packages are kept when other packages need them and are
// never removed below base.
func (c *Catalog) Remove(name string) error {
	m, err := c.Load(name)
	if err != nil {
		return err
	}
	if m.RPM == "" {
		return fmt.Errorf("experience %q is planned; nothing is installed to remove", name)
	}
	installed, err := c.dnf.IsInstalled(m.RPM)
	if err != nil {
		return err
	}
	if !installed {
		return fmt.Errorf("experience %q is not installed (%s)", name, m.RPM)
	}
	return c.dnf.Transaction("remove", "-y", m.RPM)
}

func (c *Catalog) loadAll() ([]Manifest, error) {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return nil, fmt.Errorf("read experiences dir %s: %w", c.dir, err)
	}
	var manifests []Manifest
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		m, err := c.Load(strings.TrimSuffix(e.Name(), ".yaml"))
		if err != nil {
			return nil, err
		}
		manifests = append(manifests, m)
	}
	sort.Slice(manifests, func(i, j int) bool { return manifests[i].Name < manifests[j].Name })
	return manifests, nil
}
