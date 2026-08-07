// Package settings owns Veilbox paths and persistent state.
//
// Layout:
//
//	/usr/share/veilbox/            system-owned data shipped by RPMs (read-only)
//	/usr/share/veilbox/profiles/   profile manifests (intent definitions)
//	/usr/share/veilbox/experiences/ experience catalog (capability definitions)
//	~/.config/veilbox/             user-owned Veilbox state (written by veil)
//	~/.config/veilbox/state.json   machine-written state (active profile, ...)
//
// VEILBOX_ROOT overrides the system data root (used by tests and dev runs).
package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	// SystemRoot is where Veilbox RPMs install their read-only data.
	SystemRoot = "/usr/share/veilbox"
	// EnvRoot overrides SystemRoot, for tests and development.
	EnvRoot = "VEILBOX_ROOT"

	ProfilesDir    = "profiles"
	ExperiencesDir = "experiences"

	StateDirName = "veilbox"
	StateFile    = "state.json"
)

// StateDir returns the user-owned Veilbox state directory.
func StateDir() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, StateDirName), nil
}

// StateFile returns the path of the Veilbox state file.
func StateFilePath() (string, error) {
	dir, err := StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, StateFile), nil
}

// SystemProfilesDir returns the system profile manifests directory.
func SystemProfilesDir() string {
	return filepath.Join(Root(), ProfilesDir)
}

// SystemExperiencesDir returns the system experience catalog directory.
func SystemExperiencesDir() string {
	return filepath.Join(Root(), ExperiencesDir)
}

// Root returns the system data root (VEILBOX_ROOT override when set).
func Root() string {
	if v := os.Getenv(EnvRoot); v != "" {
		return v
	}
	return SystemRoot
}

// EnsureStateDir creates the user state directory if missing.
func EnsureStateDir() (string, error) {
	dir, err := StateDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create state dir %s: %w", dir, err)
	}
	return dir, nil
}

// State is the machine-written Veilbox state persisted to state.json.
type State struct {
	// ActiveProfile is the name of the currently applied engineer profile.
	ActiveProfile string `json:"active_profile,omitempty"`
	// AppliedAt is an RFC3339 timestamp of the last profile application.
	AppliedAt string `json:"applied_at,omitempty"`
	// Version is the Veilbox Core version that last wrote this state.
	Version string `json:"version,omitempty"`
}

// LoadState reads state.json; a missing file is an empty state, not an error.
func LoadState() (State, error) {
	var st State
	path, err := StateFilePath()
	if err != nil {
		return st, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return st, nil
		}
		return st, fmt.Errorf("read state %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &st); err != nil {
		return st, fmt.Errorf("parse state %s: %w", path, err)
	}
	return st, nil
}

// SaveState atomically writes state.json.
func SaveState(st State) error {
	path, err := StateFilePath()
	if err != nil {
		return err
	}
	if _, err := EnsureStateDir(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write state %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("commit state %s: %w", path, err)
	}
	return nil
}
