// Package environment implements the Environment Engine: the
// activation layer for complete graphical environment experiences.
//
// It deliberately contains no package management of its own.
// Installation goes through the Experience Engine, which goes through
// DNF; the Environment Engine only:
//
//   - presents environment catalog views (list, info)
//   - detects the current graphical session (or its absence)
//   - provisions Veilbox-owned environment configuration (first-touch)
//   - enables the selected display manager and the graphical target
//
// Package installation and environment activation are separate
// responsibilities: installing veilbox-experience-<name> with DNF
// never changes the systemd default target and never enables
// services. 'veil environment install' is the only activation path.
package environment

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Shreyas0047/veilbox/v3/core/internal/dnfops"
	"github.com/Shreyas0047/veilbox/v3/core/internal/experience"
	"github.com/Shreyas0047/veilbox/v3/core/internal/profile"
	"github.com/Shreyas0047/veilbox/v3/core/internal/settings"
	"github.com/Shreyas0047/veilbox/v3/core/internal/workspace"
)

// System is the host-control facade used by the Environment Engine.
// Production uses real systemd/rpm/pgrep binaries; tests substitute a
// fake runner.
type System struct {
	runner   dnfops.Runner
	lookPath func(name string) (string, error)
	// sessionDir is where Wayland sessions register their .desktop
	// files (overrideable in tests).
	sessionDir string
}

// NewSystem returns a System backed by the real host.
func NewSystem() *System {
	// VEILBOX_SESSION_DIR overrides the wayland-sessions directory
	// (tests and development runs), mirroring settings.VEILBOX_ROOT.
	sd := os.Getenv("VEILBOX_SESSION_DIR")
	if sd == "" {
		sd = "/usr/share/wayland-sessions"
	}
	return &System{runner: dnfops.ExecRunner{}, lookPath: exec.LookPath, sessionDir: sd}
}

// NewSystemWith returns a System using the given runner (tests).
func NewSystemWith(r dnfops.Runner) *System {
	s := NewSystem()
	s.runner = r
	return s
}

func (s *System) run(name string, args ...string) (string, error) {
	return s.runner.Run(name, args...)
}

// runPriv runs a privileged host command through sudo (systemd
// enable/set-default are root operations).
func (s *System) runPriv(name string, args ...string) error {
	return s.runner.RunInteractive("sudo", append([]string{name}, args...)...)
}

func (s *System) unitEnabled(unit string) bool {
	out, err := s.run("systemctl", "is-enabled", unit)
	return err == nil && strings.HasPrefix(strings.TrimSpace(out), "enabled")
}

func (s *System) unitActive(unit string) bool {
	out, err := s.run("systemctl", "is-active", unit)
	return err == nil && strings.TrimSpace(out) == "active"
}

func (s *System) enableUnit(unit string) error {
	return s.runPriv("systemctl", "enable", unit)
}

func (s *System) defaultTarget() string {
	out, err := s.run("systemctl", "get-default")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func (s *System) setDefaultTarget(target string) error {
	return s.runPriv("systemctl", "set-default", target)
}

func (s *System) processRunning(name string) bool {
	_, err := s.run("pgrep", "-x", name)
	return err == nil
}

func (s *System) hasBinary(name string) bool {
	if name == "" {
		return false
	}
	_, err := s.lookPath(name)
	return err == nil
}

func (s *System) fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (s *System) env(name string) string {
	return os.Getenv(name)
}

// knownDMs is the set of display managers Veilbox detects.
var knownDMs = []string{"sddm", "gdm", "lightdm", "lxdm", "xdm"}

// Session describes the detected graphical session state.
type Session struct {
	// Graphical is true when a Wayland graphical session is active in
	// this environment.
	Graphical bool
	// Compositor is the running compositor of an installed Veilbox
	// environment experience ("" when none).
	Compositor string
	// DisplayManager is the currently active display manager ("" when
	// none).
	DisplayManager string
	// Message is a human-readable description, never guessed.
	Message string
}

// DetectSession inspects the environment and session metadata. From a
// TTY/SSH session it reports "no graphical Veilbox environment session
// detected" rather than guessing at state.
func (e *Engine) DetectSession() Session {
	s := Session{}
	if e.sys.env("XDG_SESSION_TYPE") != "wayland" && e.sys.env("WAYLAND_DISPLAY") == "" {
		s.Message = "no graphical Veilbox environment session detected"
		return s
	}
	s.Graphical = true
	entries, err := e.catalog.List()
	if err == nil {
		for _, en := range entries {
			if en.Type != experience.TypeEnvironment || en.Status != experience.StatusInstalled {
				continue
			}
			if comp := en.Components[experience.CompCompositor]; comp != "" && e.sys.processRunning(comp) {
				s.Compositor = comp
				s.Message = fmt.Sprintf("graphical Veilbox environment session detected — %s (%s)", comp, en.Name)
				break
			}
		}
	}
	if s.Compositor == "" {
		s.Message = "graphical session active, but no Veilbox environment compositor detected"
	}
	for _, dm := range knownDMs {
		if e.sys.unitActive(dm) {
			s.DisplayManager = dm
			break
		}
	}
	return s
}

// Component is an environment stack component with its role resolved.
type Component struct {
	Key     string
	Value   string
	Builtin bool
}

// displayOrder renders components in a stable, meaningful order.
var displayOrder = []string{
	experience.CompCompositor, experience.CompDesktopShell, experience.CompTerminal,
	experience.CompLauncher, experience.CompNotifications, experience.CompLock,
	experience.CompIdle, experience.CompWallpaper, experience.CompClipboard,
	experience.CompScreenshot, experience.CompDisplayManager,
}

// Entry is an environment experience with catalog state and session
// info.
type Entry struct {
	experience.Entry
	Session Session
}

// ComponentList returns the declared components in display order.
func (e Entry) ComponentList() []Component {
	var out []Component
	for _, key := range displayOrder {
		if val, ok := e.Components[key]; ok {
			out = append(out, Component{Key: key, Value: val, Builtin: val == "builtin"})
		}
	}
	return out
}

// Engine is the Environment Engine.
type Engine struct {
	catalog *experience.Catalog
	sys     *System
}

// New returns an Environment Engine over the given experience catalog.
func New(catalog *experience.Catalog) *Engine {
	return &Engine{catalog: catalog, sys: NewSystem()}
}

// NewWith returns an Environment Engine with explicit system facade
// (tests).
func NewWith(catalog *experience.Catalog, sys *System) *Engine {
	return &Engine{catalog: catalog, sys: sys}
}

// List returns installed/available environment experiences with
// session state.
func (e *Engine) List() ([]Entry, error) {
	entries, err := e.catalog.List()
	if err != nil {
		return nil, err
	}
	session := e.DetectSession()
	var out []Entry
	for _, en := range entries {
		if en.Type != experience.TypeEnvironment {
			continue
		}
		out = append(out, Entry{Entry: en, Session: session})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Info returns a single environment experience by name.
func (e *Engine) Info(name string) (*Entry, error) {
	entries, err := e.List()
	if err != nil {
		return nil, err
	}
	for i := range entries {
		if entries[i].Name == name {
			return &entries[i], nil
		}
	}
	return nil, fmt.Errorf("environment experience %q not found", name)
}

// loadEnvironmentManifest loads a manifest and enforces it is an
// environment experience.
func (e *Engine) loadEnvironmentManifest(name string) (experience.Manifest, error) {
	m, err := e.catalog.Load(name)
	if err != nil {
		return experience.Manifest{}, err
	}
	if m.Type != experience.TypeEnvironment {
		return experience.Manifest{}, fmt.Errorf("experience %q is not an environment experience", name)
	}
	return m, nil
}

// userConfigDir returns the user config root (XDG_CONFIG_HOME or
// ~/.config).
func userConfigDir() (string, error) {
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".config"), nil
}

// ConfigDir exposes the user config root for external health checks.
func ConfigDir() (string, error) {
	return userConfigDir()
}

// activePreferences returns the workspace preferences of the active
// profile (zero value when none is set). Profiles are authoritative
// for general user preferences; the environment experience consumes
// only the fields it can use.
func (e *Engine) activePreferences() workspace.Preferences {
	st, err := profile.Active()
	if err != nil || st.ActiveProfile == "" {
		return workspace.Preferences{}
	}
	m, err := profile.NewRegistry().Load(st.ActiveProfile)
	if err != nil {
		return workspace.Preferences{}
	}
	return m.Workspace
}

// sessionFilePath returns the wayland-sessions desktop file for a
// compositor binary.
func (e *Engine) sessionFilePath(compositor string) string {
	return filepath.Join(e.sys.sessionDir, compositor+".desktop")
}

// templateDir returns the RPM-owned template directory of an
// environment experience. The directory mirrors the RPM package name
// (the veilbox-experience- prefix is stripped): the RPM owns its
// layout, so the engine discovers it from the manifest rather than
// assuming it matches the experience name.
func templateDir(m experience.Manifest) string {
	dir := m.RPM
	if dir == "" {
		dir = m.Name
	}
	dir = strings.TrimPrefix(dir, "veilbox-experience-")
	return filepath.Join(settings.SystemEnvironmentDir(), dir)
}

// TemplateDir exposes templateDir for external health checks.
func (e *Engine) TemplateDir(m experience.Manifest) string {
	return templateDir(m)
}

// SessionDir exposes the wayland-sessions registration directory
// (shared engine mechanics; doctor's session-file check must look in
// the same place the engine registers sessions).
func (e *Engine) SessionDir() string {
	return e.sys.sessionDir
}
