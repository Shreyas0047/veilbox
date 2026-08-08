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
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Shreyas0047/veilbox/v3/core/internal/dnfops"
	"github.com/Shreyas0047/veilbox/v3/core/internal/safetoken"
	"github.com/Shreyas0047/veilbox/v3/core/internal/settings"
)

// PackagePrefix is the name prefix of all Veilbox experience RPMs.
const PackagePrefix = "veilbox-experience-"

// namePattern matches manifest names and capability references:
// lowercase letters, digits, and hyphens.
var namePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// Status of an experience.
type Status string

const (
	StatusPlanned   Status = "planned"
	StatusAvailable Status = "available"
	StatusInstalled Status = "installed"
)

// Type classifies an experience. Experiences of type environment are
// complete graphical environments, activated through the Environment
// Engine ('veil environment'); all other experiences are type tooling.
type Type string

const (
	// TypeTooling is the default: a capability that changes the
	// machine (packages, tooling, services of the engineer's craft).
	TypeTooling Type = "tooling"
	// TypeEnvironment is a complete environment experience:
	// compositor, desktop shell, terminal, integration and defaults —
	// a working environment at first login, not a bare compositor
	// (ADR-0012).
	TypeEnvironment Type = "environment"
	// TypeDesktop is the legacy name of the environment type
	// (ADR-0012 rename). Manifests and state are migrated to
	// TypeEnvironment by the loader on read; nothing writes it.
	TypeDesktop Type = "desktop"
)

// Component keys allowed in environment experience manifests. Values
// are either "builtin" (provided by the shell itself) or a safe-token
// package/binary name — never shell commands.
const (
	CompCompositor     = "compositor"
	CompDesktopShell   = "desktop_shell"
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

// legacyCompShell is the pre-ADR-0012 name of the desktop shell
// component slot ("shell" collided with the workspace's login shell
// concept). The loader migrates it on read; nothing writes it.
const legacyCompShell = "shell"

// componentKeys is the complete set of recognized component keys.
var componentKeys = map[string]bool{
	CompCompositor: true, CompDesktopShell: true, CompTerminal: true,
	CompLauncher: true, CompNotifications: true, CompLock: true,
	CompIdle: true, CompWallpaper: true, CompClipboard: true,
	CompScreenshot: true, CompDisplayManager: true,
}

// builtinAllowed reports whether a component key may be satisfied by
// "builtin". Structural components (compositor, desktop shell,
// terminal, display manager) must name a real package/binary.
func builtinAllowed(key string) bool {
	switch key {
	case CompCompositor, CompDesktopShell, CompTerminal, CompDisplayManager:
		return false
	}
	return true
}

// environmentRequiredComponents are the component keys every
// environment experience must declare.
var environmentRequiredComponents = []string{
	CompCompositor, CompDesktopShell, CompTerminal, CompDisplayManager,
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
	// Type classifies the experience (tooling or environment); empty
	// means tooling. Environment experiences declare a components map
	// and are activated only through 'veil environment' — never by the
	// RPM itself. The legacy name "desktop" (ADR-0012) is accepted on
	// read and migrated by the loader.
	Type Type `yaml:"type,omitempty"`
	// RPM is the package name that implements this experience. Empty
	// means the experience is planned and not yet installable.
	RPM string `yaml:"rpm,omitempty"`
	// Packages lists the concrete Fedora packages the meta-package
	// pulls in (informational; the RPM Requires are authoritative).
	Packages []string `yaml:"packages,omitempty"`
	// Capabilities lists the capability names this experience
	// implements (the N→M mapping of ADR-0011). The Capability Engine
	// derives the required experience set from a capability selection;
	// the mapping is validated by doctor.
	Capabilities []string `yaml:"capabilities,omitempty"`
	// Components declares the graphical environment stack for
	// type: environment experiences. Values are "builtin" or a
	// safe-token name.
	Components map[string]string `yaml:"components,omitempty"`
	// Environment declares the environment data contract (ADR-0015):
	// which templates render to which user-side paths and what doctor
	// validates. Mechanics live in the Environment Engine; every
	// environment-specific fact is this data. Empty means no
	// provisioning or validation is declared.
	Environment *EnvSpec `yaml:"environment,omitempty"`
}

// EnvFile is one environment-managed config file: a template under
// the RPM-owned template directory rendered to a user-side path.
type EnvFile struct {
	// Src is the template file name under the environment's RPM-owned
	// template directory.
	Src string `yaml:"src"`
	// Dest is the destination path relative to the user config dir
	// (XDG_CONFIG_HOME or ~/.config), e.g. "compositor/config.kdl".
	// Config destinations are user-owned first-touch (never
	// overwritten); managed destinations are Veilbox-owned and always
	// regenerated.
	Dest string `yaml:"dest"`
}

// EnvValidate declares the doctor validation expectations of an
// environment (ADR-0015 validation hooks: file expectations and
// command hooks).
type EnvValidate struct {
	// Files are config paths (relative to the user config dir) that
	// must exist when the environment is installed.
	Files []string `yaml:"files,omitempty"`
	// Commands are safe-token argv lists run to validate the
	// environment's own config (e.g. "shell config validate").
	Commands [][]string `yaml:"commands,omitempty"`
}

// EnvSpec is the environment data contract (ADR-0015).
type EnvSpec struct {
	// Config lists first-touch user config files: rendered from
	// templates, never overwriting existing user files.
	Config []EnvFile `yaml:"config,omitempty"`
	// Managed lists Veilbox-owned files under
	// <config>/veilbox/environment/<name>/ that are always
	// regenerated.
	Managed []EnvFile `yaml:"managed,omitempty"`
	// Validate declares doctor validation expectations.
	Validate EnvValidate `yaml:"validate,omitempty"`
}

// Validate checks the environment data contract.
func (s *EnvSpec) validate(name string) error {
	for _, f := range s.Config {
		if err := validateEnvFile(name, "config", f); err != nil {
			return err
		}
	}
	for _, f := range s.Managed {
		if err := validateEnvFile(name, "managed", f); err != nil {
			return err
		}
	}
	seen := make(map[string]bool, len(s.Validate.Files))
	for _, p := range s.Validate.Files {
		if err := validateConfigPath(p); err != nil {
			return fmt.Errorf("experience %q: environment validate file %q: %v", name, p, err)
		}
		if seen[p] {
			return fmt.Errorf("experience %q: duplicate validate file %q", name, p)
		}
		seen[p] = true
	}
	for _, cmd := range s.Validate.Commands {
		if len(cmd) == 0 {
			return fmt.Errorf("experience %q: empty validate command", name)
		}
		for _, tok := range cmd {
			if !safetoken.Valid(tok) {
				return fmt.Errorf("experience %q: validate command element %q uses invalid characters (safe tokens only)", name, tok)
			}
		}
	}
	return nil
}

func validateEnvFile(name, kind string, f EnvFile) error {
	if f.Src == "" || f.Dest == "" {
		return fmt.Errorf("experience %q: environment %s entry needs both src and dest", name, kind)
	}
	if !safetoken.Valid(f.Src) {
		return fmt.Errorf("experience %q: environment %s src %q uses invalid characters (safe tokens only)", name, kind, f.Src)
	}
	if err := validateConfigPath(f.Dest); err != nil {
		return fmt.Errorf("experience %q: environment %s dest %q: %v", name, kind, f.Dest, err)
	}
	return nil
}

// validateConfigPath checks a manifest-declared path is relative to
// the user config dir and free of traversal.
func validateConfigPath(p string) error {
	if p == "" {
		return fmt.Errorf("empty path")
	}
	if strings.HasPrefix(p, "/") || strings.HasPrefix(p, "~") {
		return fmt.Errorf("must be relative to the user config dir")
	}
	if strings.Contains(p, "..") {
		return fmt.Errorf("must not contain path traversal")
	}
	if !safetoken.Valid(p) {
		return fmt.Errorf("uses invalid characters (safe tokens only)")
	}
	return nil
}

// Validate checks manifest constraints, including the declarative
// grammar of environment components.
func (m Manifest) Validate() error {
	if m.Domain != "" && !safetoken.ValidCommand(m.Domain) {
		return fmt.Errorf("experience %q: domain %q uses invalid characters (presentation text, safe tokens only)", m.Name, m.Domain)
	}
	switch m.Type {
	case "", TypeTooling:
		if len(m.Components) > 0 {
			return fmt.Errorf("experience %q: components are only valid for type: environment", m.Name)
		}
		return m.validateCapabilities()
	case TypeEnvironment, TypeDesktop:
	default:
		return fmt.Errorf("experience %q: unknown type %q (want %q, %q or %q)", m.Name, m.Type, TypeTooling, TypeEnvironment, TypeDesktop)
	}
	if m.RPM == "" {
		return fmt.Errorf("experience %q: an environment experience must declare an installable rpm", m.Name)
	}
	if len(m.Components) == 0 {
		return fmt.Errorf("experience %q: an environment experience must declare components", m.Name)
	}
	for _, key := range environmentRequiredComponents {
		if m.Components[key] == "" {
			return fmt.Errorf("experience %q: environment component %q is required", m.Name, key)
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
	if m.Environment != nil {
		if err := m.Environment.validate(m.Name); err != nil {
			return err
		}
	}
	return m.validateCapabilities()
}

// validateCapabilities checks capability reference names on the
// manifest. Cross-registry existence is checked by the Capability
// Engine (see capability.Resolver.CheckMapping).
func (m Manifest) validateCapabilities() error {
	seen := make(map[string]bool, len(m.Capabilities))
	for _, c := range m.Capabilities {
		if !namePattern.MatchString(c) {
			return fmt.Errorf("experience %q: invalid capability reference %q", m.Name, c)
		}
		if seen[c] {
			return fmt.Errorf("experience %q: duplicate capability reference %q", m.Name, c)
		}
		seen[c] = true
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
	m.normalize()
	if err := m.Validate(); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

// normalize migrates legacy manifest spellings to the current schema
// (ADR-0012): the type name "desktop" becomes "environment" and the
// component slot "shell" becomes "desktop_shell". The loader is the
// single migration point; manifests never sit on a forked path.
func (m *Manifest) normalize() {
	if m.Type == TypeDesktop {
		m.Type = TypeEnvironment
	}
	if v, ok := m.Components[legacyCompShell]; ok {
		if _, exists := m.Components[CompDesktopShell]; !exists {
			m.Components[CompDesktopShell] = v
		}
		delete(m.Components, legacyCompShell)
	}
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

// Manifests returns all catalog manifests, sorted, with no system
// state attached. Used by the Capability Engine's mapping (no DNF
// queries); List is the status-bearing variant.
func (c *Catalog) Manifests() ([]Manifest, error) {
	return c.loadAll()
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
