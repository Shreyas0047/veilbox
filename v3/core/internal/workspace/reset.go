package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ResetResult summarizes a reset.
type ResetResult struct {
	// Removed are the Veilbox-managed paths removed.
	Removed []string
	// Preserved are user-owned paths that were left untouched.
	Preserved []string
}

// Reset removes ONLY Veilbox-managed workspace configuration:
// generated files, managed blocks, and whole files Veilbox created.
// User-owned content is never destroyed: ambiguous states (multiple or
// malformed blocks, symlinked user files) abort the reset instead.
// Backups are kept as the recovery trail.
func (e *Engine) Reset() (*ResetResult, error) {
	st, err := LoadState()
	if err != nil {
		return nil, err
	}
	wsDir, err := WorkspaceDir()
	if err != nil {
		return nil, err
	}
	res := &ResetResult{}

	// Refuse destructive ambiguity up front.
	for path := range st.Blocks {
		data, err := os.ReadFile(path)
		if err != nil {
			continue // already gone; nothing to strip
		}
		info, lerr := os.Lstat(path)
		if lerr == nil && info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("refusing to reset: %s is a symlink; remove it manually", path)
		}
		if _, ferr := FindBlock(string(data)); ferr != nil {
			return nil, fmt.Errorf("refusing to reset: %s: %w (resolve manually)", path, ferr)
		}
	}

	// Strip managed blocks; delete whole files Veilbox created when
	// they still contain only Veilbox content.
	for path, rec := range st.Blocks {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		blk, err := FindBlock(string(data))
		if err != nil {
			continue // pre-checked; unreachable
		}
		if blk == nil {
			res.Preserved = append(res.Preserved, path)
			continue
		}
		if rec.Created && string(data) == blk.Content {
			if err := os.Remove(path); err == nil {
				res.Removed = append(res.Removed, path)
			}
			continue
		}
		if err := modifyUserFile(path, func(content string) (string, error) {
			return RemoveBlock(content, blk), nil
		}); err == nil {
			res.Removed = append(res.Removed, path+" (managed block)")
		} else {
			return nil, fmt.Errorf("reset %s: %w", path, err)
		}
	}

	// Remove generated files.
	removedGenerated := false
	for _, name := range []string{ShellScriptName, TmuxConfigName} {
		p := filepath.Join(wsDir, name)
		if err := os.Remove(p); err == nil {
			removedGenerated = true
			res.Removed = append(res.Removed, p)
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("reset %s: %w", p, err)
		}
	}
	if len(res.Removed) == 0 && !removedGenerated && !HasState() {
		res.Preserved = []string{"(nothing managed by Veilbox)"}
	}

	// State: keep the backup ledger, drop the configuration records.
	st.Generation++
	st.AppliedProfile = ""
	st.AppliedAt = time.Now().UTC().Format(time.RFC3339)
	st.Files = nil
	st.Blocks = nil
	if err := SaveState(st); err != nil {
		return nil, err
	}
	return res, nil
}
