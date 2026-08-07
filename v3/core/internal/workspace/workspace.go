// Package workspace manages the personalized operations workspace:
// the user-owned Veilbox state directories and the layout Veilbox
// provisions for the engineer.
package workspace

import (
	"fmt"

	"github.com/Shreyas0047/veilbox/v3/core/internal/settings"
)

// Provision creates the user-owned Veilbox state directory if missing.
// It is safe to call repeatedly.
func Provision() error {
	dir, err := settings.EnsureStateDir()
	if err != nil {
		return fmt.Errorf("provision workspace: %w", err)
	}
	_ = dir
	return nil
}

// StateDir returns the user-owned state directory path.
func StateDir() (string, error) {
	return settings.StateDir()
}
