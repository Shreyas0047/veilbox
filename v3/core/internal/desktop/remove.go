package desktop

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Shreyas0047/veilbox/v3/core/internal/experience"
)

// RemovePlan describes a conservative desktop removal before it is
// executed. Removal is intentionally non-destructive: only the
// experience package is removed; user-side configuration (including
// Veilbox-generated files) is preserved and reported, and the display
// manager and boot target are never touched.
type RemovePlan struct {
	Name           string
	RPM            string
	PreservedFiles []string
	VeilboxFiles   []string
	DeactivateHint string
}

// PlanRemove computes the removal plan for a desktop experience.
// Pure read-only.
func (e *Engine) PlanRemove(name string) (*RemovePlan, error) {
	m, err := e.loadDesktopManifest(name)
	if err != nil {
		return nil, err
	}
	plan := &RemovePlan{Name: m.Name, RPM: m.RPM}
	cfg, err := userConfigDir()
	if err != nil {
		return nil, err
	}
	for _, p := range []string{
		filepath.Join(cfg, "niri", "config.kdl"),
		filepath.Join(cfg, "noctalia", "config.toml"),
	} {
		if _, err := os.Stat(p); err == nil {
			plan.PreservedFiles = append(plan.PreservedFiles, p)
		}
	}
	veilDir := filepath.Join(cfg, "veilbox", "desktop", name)
	if entries, err := os.ReadDir(veilDir); err == nil {
		for _, en := range entries {
			plan.VeilboxFiles = append(plan.VeilboxFiles, filepath.Join(veilDir, en.Name()))
		}
	}
	if dm := m.Components[experience.CompDisplayManager]; dm != "" {
		target := e.sys.defaultTarget()
		if target == "" {
			target = "multi-user.target"
		}
		plan.DeactivateHint = fmt.Sprintf(
			"Display manager and boot target are untouched by removal. To deactivate the desktop: sudo systemctl disable --now %s; sudo systemctl set-default %s",
			dm, target)
	}
	return plan, nil
}

// Remove removes a desktop experience: the experience meta-package
// through DNF only. User configuration, the display manager and the
// boot target are preserved; the caller reports what remains.
func (e *Engine) Remove(name string) (*RemovePlan, error) {
	plan, err := e.PlanRemove(name)
	if err != nil {
		return nil, err
	}
	if err := e.catalog.Remove(name); err != nil {
		return nil, fmt.Errorf("remove experience %s: %w", name, err)
	}
	return plan, nil
}

// JoinFiles renders a file list for reporting.
func JoinFiles(files []string) string {
	if len(files) == 0 {
		return "(none)"
	}
	return strings.Join(files, "\n  ")
}
