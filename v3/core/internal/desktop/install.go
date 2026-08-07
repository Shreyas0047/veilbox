package desktop

import (
	"fmt"
	"path/filepath"

	"github.com/Shreyas0047/veilbox/v3/core/internal/experience"
	"github.com/Shreyas0047/veilbox/v3/core/internal/workspace"
)

// InstallPlan describes what activating a desktop experience would
// change, computed without executing anything.
type InstallPlan struct {
	Name        string
	DisplayName string
	RPM         string
	Packages    []string

	// SessionFile is the wayland-sessions entry the compositor must
	// register.
	SessionFile string
	// SessionRegistered reports whether the session file already
	// exists.
	SessionRegistered bool

	// DeclaredDM is the display manager the experience declares.
	DeclaredDM string
	// EnabledDM is the display manager currently enabled ("" when
	// none). May be a different DM than DeclaredDM; in that case it is
	// left untouched.
	EnabledDM string
	// WillEnableDM reports whether the declared DM will be enabled.
	WillEnableDM bool

	// CurrentTarget is the current systemd default target.
	CurrentTarget string
	// WillSetTarget reports whether the default target will be
	// switched to graphical.target.
	WillSetTarget bool
	// RollbackTarget is the target to restore to undo activation.
	RollbackTarget string

	// CreatedConfig lists user-side files that will be created
	// (first-touch only).
	CreatedConfig []string
	// ExistingConfig lists user-side files that already exist and will
	// be preserved untouched.
	ExistingConfig []string
}

// PlanInstall computes the activation plan for a desktop experience.
// It is pure read-only: nothing is installed or changed.
func (e *Engine) PlanInstall(name string) (*InstallPlan, error) {
	m, err := e.loadDesktopManifest(name)
	if err != nil {
		return nil, err
	}
	plan := &InstallPlan{
		Name:          m.Name,
		DisplayName:   m.DisplayName,
		RPM:           m.RPM,
		Packages:      m.Packages,
		SessionFile:   e.sessionFilePath(m.Components[experience.CompCompositor]),
		DeclaredDM:    m.Components[experience.CompDisplayManager],
		CurrentTarget: e.sys.defaultTarget(),
	}
	plan.SessionRegistered = e.sys.fileExists(plan.SessionFile)
	plan.WillSetTarget = plan.CurrentTarget != "graphical.target"
	plan.RollbackTarget = plan.CurrentTarget

	for _, dm := range knownDMs {
		if e.sys.unitEnabled(dm) {
			plan.EnabledDM = dm
			break
		}
	}
	if plan.DeclaredDM != "" && plan.EnabledDM == "" {
		plan.WillEnableDM = true
	}

	cfg, err := userConfigDir()
	if err != nil {
		return nil, err
	}
	plan.CreatedConfig, plan.ExistingConfig = e.firstTouchAnalysis(cfg, m)
	return plan, nil
}

// firstTouchAnalysis reports which user-side config files would be
// created and which already exist (and are therefore preserved).
func (e *Engine) firstTouchAnalysis(cfg string, m experience.Manifest) (created, existing []string) {
	paths := []string{
		filepath.Join(cfg, "niri", "config.kdl"),
		filepath.Join(cfg, "noctalia", "config.toml"),
	}
	for _, p := range paths {
		if e.sys.fileExists(p) {
			existing = append(existing, p)
		} else {
			created = append(created, p)
		}
	}
	return created, existing
}

// InstallResult reports the executed activation steps.
type InstallResult struct {
	Plan  *InstallPlan
	Steps []string
}

// Install activates a desktop experience. Package installation and
// desktop activation are separate responsibilities: this is the only
// place where the display manager is enabled and the default target
// is switched. The sequence is explicit:
//
//  1. install the experience through the Experience Engine (DNF)
//  2. provision Veilbox-owned desktop configuration (first-touch)
//  3. verify session registration
//  4. enable the display manager
//  5. set the graphical target
func (e *Engine) Install(name string) (*InstallResult, error) {
	plan, err := e.PlanInstall(name)
	if err != nil {
		return nil, err
	}
	res := &InstallResult{Plan: plan}

	installed, err := e.catalogIsInstalled(name)
	if err != nil {
		return nil, err
	}
	if installed {
		res.Steps = append(res.Steps, fmt.Sprintf("experience %s already installed (%s) — package step skipped", name, plan.RPM))
	} else if err := e.catalog.Install(name); err != nil {
		return nil, fmt.Errorf("install experience %s: %w", name, err)
	} else {
		res.Steps = append(res.Steps, fmt.Sprintf("installed experience %s (%s) through DNF", name, plan.RPM))
	}

	prov, err := e.Provision(name)
	if err != nil {
		return nil, fmt.Errorf("provision desktop configuration: %w", err)
	}
	for _, f := range prov.Created {
		res.Steps = append(res.Steps, "created "+f)
	}
	for _, f := range prov.Preserved {
		res.Steps = append(res.Steps, "preserved existing "+f)
	}
	for _, f := range prov.Regenerated {
		res.Steps = append(res.Steps, "regenerated "+f)
	}

	if !e.sys.fileExists(plan.SessionFile) {
		return nil, fmt.Errorf("session file %s missing after install — the desktop is not registered with the display manager", plan.SessionFile)
	}
	res.Steps = append(res.Steps, "session registered: "+plan.SessionFile)

	switch {
	case plan.DeclaredDM == "":
		res.Steps = append(res.Steps, "no display manager declared — skipped")
	case !plan.WillEnableDM && plan.EnabledDM != "" && plan.EnabledDM != plan.DeclaredDM:
		res.Steps = append(res.Steps, fmt.Sprintf("display manager %s already enabled — left as-is", plan.EnabledDM))
	case !plan.WillEnableDM:
		res.Steps = append(res.Steps, fmt.Sprintf("display manager %s already enabled", plan.DeclaredDM))
	default:
		if err := e.sys.enableUnit(plan.DeclaredDM); err != nil {
			return nil, fmt.Errorf("enable display manager %s: %w", plan.DeclaredDM, err)
		}
		res.Steps = append(res.Steps, "enabled display manager: "+plan.DeclaredDM)
	}

	if plan.WillSetTarget {
		if err := e.sys.setDefaultTarget("graphical.target"); err != nil {
			return nil, fmt.Errorf("set default target: %w", err)
		}
		res.Steps = append(res.Steps, fmt.Sprintf("set default target to graphical.target (rollback: systemctl set-default %s)", plan.RollbackTarget))
	} else {
		res.Steps = append(res.Steps, "default target already graphical.target")
	}
	return res, nil
}

// catalogIsInstalled reports whether the experience backing a desktop
// manifest is installed, by consulting the Experience Engine catalog
// (which resolves installed state from the RPM database).
func (e *Engine) catalogIsInstalled(name string) (bool, error) {
	entries, err := e.catalog.List()
	if err != nil {
		return false, err
	}
	for _, en := range entries {
		if en.Name == name {
			return en.Status == experience.StatusInstalled, nil
		}
	}
	return false, nil
}

// ProvisionResult reports what provisioning wrote.
type ProvisionResult struct {
	Created     []string
	Preserved   []string
	Regenerated []string
}

// Provision generates Veilbox-owned desktop configuration under the
// user's config directories. It never overwrites user-owned files:
// the niri config and the noctalia config are first-touch only, while
// the Veilbox-owned include file under
// ~/.config/veilbox/desktop/<name>/ is always regenerated.
func (e *Engine) Provision(name string) (*ProvisionResult, error) {
	m, err := e.loadDesktopManifest(name)
	if err != nil {
		return nil, err
	}
	installed, err := e.catalogIsInstalled(name)
	if err != nil {
		return nil, err
	}
	if !installed {
		return nil, fmt.Errorf("desktop experience %q is not installed — run 'veil desktop install %s' first", name, name)
	}
	td := templateDir(m)
	if !e.sys.fileExists(td) {
		return nil, fmt.Errorf("desktop templates missing: %s", td)
	}
	cfg, err := userConfigDir()
	if err != nil {
		return nil, err
	}
	res := &ProvisionResult{}
	prefs := e.activePreferences()

	if err := e.renderNiriConfig(td, cfg, m, prefs, res); err != nil {
		return nil, err
	}
	if err := e.renderNoctaliaConfig(td, cfg, m, res); err != nil {
		return nil, err
	}

	veilDir := filepath.Join(cfg, "veilbox", "desktop", name)
	veilFile := filepath.Join(veilDir, "noctalia-veilbox.toml")
	if err := e.renderFromTemplate(filepath.Join(td, "noctalia-veilbox.toml"), veilFile, templateData{m, td}); err != nil {
		return nil, err
	}
	res.Regenerated = append(res.Regenerated, veilFile)
	return res, nil
}

// templateData wraps the experience manifest with the RPM-owned
// template directory, which templates reference for system-level
// paths (wallpaper etc.). The system directory follows the RPM name,
// not the experience name.
type templateData struct {
	experience.Manifest
	SystemDir string
}

func (e *Engine) renderNiriConfig(td, cfg string, m experience.Manifest, prefs workspace.Preferences, res *ProvisionResult) error {
	niriPath := filepath.Join(cfg, "niri", "config.kdl")
	if e.sys.fileExists(niriPath) {
		res.Preserved = append(res.Preserved, niriPath)
		return nil
	}
	term := m.Components[experience.CompTerminal]
	if term == "" {
		term = "kitty"
	}
	if prefs.Terminal != "" && e.sys.hasBinary(prefs.Terminal) {
		term = prefs.Terminal
	}
	editor := prefs.Editor
	if editor == "" || !e.sys.hasBinary(editor) {
		editor = "vim"
	}
	data := struct {
		Name        string
		DisplayName string
		Terminal    string
		Editor      string
	}{m.Name, m.DisplayName, term, editor}
	if err := e.renderFromTemplate(filepath.Join(td, "niri.config.kdl"), niriPath, data); err != nil {
		return err
	}
	res.Created = append(res.Created, niriPath)
	return nil
}

func (e *Engine) renderNoctaliaConfig(td, cfg string, m experience.Manifest, res *ProvisionResult) error {
	noctPath := filepath.Join(cfg, "noctalia", "config.toml")
	if e.sys.fileExists(noctPath) {
		res.Preserved = append(res.Preserved, noctPath)
		return nil
	}
	if err := e.renderFromTemplate(filepath.Join(td, "noctalia.config.toml"), noctPath, templateData{m, td}); err != nil {
		return err
	}
	res.Created = append(res.Created, noctPath)
	return nil
}
