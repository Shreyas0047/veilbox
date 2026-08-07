package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// BackupResult reports whether a backup was created for a file.
type BackupResult struct {
	Created    bool
	BackupPath string
}

// EnsureBackup backs up a user-owned file before Veilbox modifies it
// for the first time. The original backup is never overwritten: once a
// backup record exists for a path, later applies skip it.
//
// The backup lives under the Veilbox-owned backup root with a JSON
// sidecar describing the original path, creation time, and reason —
// never as a .bak scattered through the user's home directory.
func EnsureBackup(st *State, targetPath, reason string) (BackupResult, error) {
	for _, b := range st.Backups {
		if b.OriginalPath == targetPath {
			return BackupResult{Created: false, BackupPath: b.BackupPath}, nil
		}
	}
	info, err := os.Stat(targetPath)
	if err != nil || info.IsDir() {
		return BackupResult{}, nil // nothing to back up (or not a regular file)
	}
	data, err := os.ReadFile(targetPath)
	if err != nil {
		return BackupResult{}, fmt.Errorf("read %s for backup: %w", targetPath, err)
	}
	root, err := BackupRoot()
	if err != nil {
		return BackupResult{}, err
	}
	ts := time.Now().UTC().Format("20060102T150405Z")
	dir := filepath.Join(root, ts)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return BackupResult{}, fmt.Errorf("create backup dir: %w", err)
	}
	backupPath := filepath.Join(dir, filepath.Base(targetPath)+".bak")
	if err := os.WriteFile(backupPath, data, 0o600); err != nil {
		return BackupResult{}, fmt.Errorf("write backup %s: %w", backupPath, err)
	}
	rec := BackupRecord{
		OriginalPath: targetPath,
		BackupPath:   backupPath,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
		Reason:       reason,
		SHA256:       sha256Hex(data),
	}
	meta, err := jsonMarshal(rec)
	if err != nil {
		return BackupResult{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, "backup.json"), meta, 0o600); err != nil {
		return BackupResult{}, fmt.Errorf("write backup metadata: %w", err)
	}
	st.Backups = append(st.Backups, rec)
	return BackupResult{Created: true, BackupPath: backupPath}, nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func jsonMarshal(v any) ([]byte, error) {
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode json: %w", err)
	}
	return out, nil
}
