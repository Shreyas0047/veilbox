package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Shreyas0047/veilbox/v3/core/internal/settings"
)

// State schema version. Bump when the on-disk format changes.
const SchemaVersion = 1

// FileRecord tracks a Veilbox-generated file by its hash. The hash is
// the drift-detection anchor for Veilbox-owned content only.
type FileRecord struct {
	SHA256 string `json:"sha256"`
}

// BlockRecord tracks the expected interior of a managed block inside a
// user-owned file. The hash covers only the Veilbox-managed payload,
// never the user's surrounding content.
type BlockRecord struct {
	// Created is true when Veilbox created the whole user file (the
	// file did not exist before the first apply). Such files may be
	// deleted by reset if they still contain only Veilbox content.
	Created bool   `json:"created,omitempty"`
	SHA256  string `json:"sha256"`
}

// BackupRecord documents one Veilbox backup. Backups are stored under
// the Veilbox-owned backup root; nothing is ever scattered across the
// user's home directory.
type BackupRecord struct {
	OriginalPath string `json:"original_path"`
	BackupPath   string `json:"backup_path"`
	CreatedAt    string `json:"created_at"`
	Reason       string `json:"reason"`
	SHA256       string `json:"sha256"`
}

// State is the machine-written workspace state
// (~/.config/veilbox/workspace/state.json). It tracks Veilbox-owned
// files and Veilbox-managed blocks only — it is deliberately not a
// hash database of user-owned configuration.
type State struct {
	SchemaVersion int `json:"schema_version"`
	// Generation increments on every successful apply or reset.
	Generation int `json:"generation"`
	// AppliedProfile is the profile whose preferences generated the
	// current configuration.
	AppliedProfile string `json:"applied_profile,omitempty"`
	AppliedAt      string `json:"applied_at,omitempty"`
	// Files maps generated file path -> record.
	Files map[string]FileRecord `json:"files,omitempty"`
	// Blocks maps user-owned file path -> record of its managed block.
	Blocks map[string]BlockRecord `json:"blocks,omitempty"`
	// Backups is the append-only backup ledger.
	Backups []BackupRecord `json:"backups,omitempty"`
}

// WorkspaceDir returns the Veilbox-owned workspace directory.
func WorkspaceDir() (string, error) {
	stateDir, err := settings.StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(stateDir, "workspace"), nil
}

// BackupRoot returns the Veilbox-owned backup store root.
func BackupRoot() (string, error) {
	stateDir, err := settings.StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(stateDir, "backups"), nil
}

// StatePath returns the workspace state file path.
func StatePath() (string, error) {
	dir, err := WorkspaceDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "state.json"), nil
}

// LoadState reads workspace state; a missing file is an empty state.
func LoadState() (State, error) {
	var st State
	path, err := StatePath()
	if err != nil {
		return st, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return st, nil
		}
		return st, fmt.Errorf("read workspace state %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &st); err != nil {
		return st, fmt.Errorf("parse workspace state %s: %w", path, err)
	}
	if st.SchemaVersion != SchemaVersion {
		return st, fmt.Errorf("workspace state %s has unsupported schema version %d (want %d)", path, st.SchemaVersion, SchemaVersion)
	}
	return st, nil
}

// SaveState atomically writes workspace state.
func SaveState(st State) error {
	if st.SchemaVersion == 0 {
		st.SchemaVersion = SchemaVersion
	}
	path, err := StatePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create workspace dir: %w", err)
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("encode workspace state: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write workspace state %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("commit workspace state %s: %w", path, err)
	}
	return nil
}

// HasState reports whether a workspace state file exists on disk.
func HasState() bool {
	path, err := StatePath()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}
