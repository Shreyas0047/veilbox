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
	"github.com/Shreyas0047/veilbox/v3/core/internal/settings"
)

// PackagePrefix is the name prefix of all Veilbox experience RPMs.
const PackagePrefix = "veilbox-experience-"

// Status of an experience.
type Status string

const (
	StatusPlanned    Status = "planned"
	StatusAvailable  Status = "available"
	StatusInstalled  Status = "installed"
)

// Manifest is an experience catalog entry (experiences/<name>.yaml).
type Manifest struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	// RPM is the package name that implements this experience. Empty
	// means the experience is planned and not yet installable.
	RPM string `yaml:"rpm,omitempty"`
	// Packages lists the concrete Fedora packages the meta-package
	// pulls in (informational; the RPM Requires are authoritative).
	Packages []string `yaml:"packages,omitempty"`
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
