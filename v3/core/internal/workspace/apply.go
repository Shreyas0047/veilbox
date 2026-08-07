package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ApplyResult summarizes one apply execution.
type ApplyResult struct {
	// Applied are the entries that changed the workspace.
	Applied []Entry
	// Skipped are the entries Veilbox declined to act on.
	Skipped []Entry
	// ResolvedConflicts are drift conflicts that --force restored.
	ResolvedConflicts []Entry
	// Capabilities lists unmet preference requirements (reported,
	// never installed).
	Capabilities []string
	// BackedUp lists first-time backups created during this apply.
	BackedUp []string
}

// Apply executes the plan for prefs. profileName is recorded in the
// workspace state so status can detect a profile switch that has not
// been applied yet. It refuses to touch anything when a conflict
// exists unless force is set; force authorizes restoring drifted
// Veilbox-managed content only — structural conflicts (symlinked user
// files, ambiguous blocks) abort even with force, and user-owned files
// are never replaced wholesale.
func (e *Engine) Apply(prefs Preferences, profileName string, force bool) (*ApplyResult, error) {
	st, err := LoadState()
	if err != nil {
		return nil, err
	}
	plan, err := e.BuildPlan(prefs, st)
	if err != nil {
		return nil, err
	}

	if plan.HasFatalConflict() {
		return nil, fmt.Errorf("refusing to apply: structural conflicts require a decision, not --force")
	}
	conflicts := plan.Conflicts()
	if len(conflicts) > 0 && !force {
		return nil, fmt.Errorf("workspace has conflicts (%d); nothing was changed — inspect with 'veil workspace status' and resolve, or run 'veil workspace apply --force' to restore Veilbox-managed content only", len(conflicts))
	}

	res := &ApplyResult{Capabilities: plan.Capabilities}
	wsDir, err := WorkspaceDir()
	if err != nil {
		return nil, err
	}
	// Resolved drift conflicts become their execution kind.
	entries := make([]Entry, 0, len(plan.Entries))
	for _, en := range plan.Entries {
		if en.Action == ActionConflict && en.Drift {
			res.ResolvedConflicts = append(res.ResolvedConflicts, en)
		}
		if en.Action == ActionConflict && en.Drift && force {
			en.Action = ActionUpdate
			entries = append(entries, en)
			continue
		}
		if en.Action == ActionConflict {
			continue // fatal: already rejected; drift without force: rejected above
		}
		entries = append(entries, en)
	}

	// Phase 1: back up every user-owned file we are about to modify.
	changed := 0
	for _, en := range entries {
		if en.Action == ActionCreate || en.Action == ActionUpdate || en.Action == ActionRemove {
			changed++
		}
	}
	if changed == 0 {
		res.Skipped = []Entry{{Action: ActionUnchanged, Path: "(workspace)", Detail: "already up to date"}}
		return res, nil
	}
	for _, en := range entries {
		if !isUserFile(en.Path, wsDir) || en.kind == kindFileDelete {
			continue
		}
		if en.kind != kindBlockInsert && en.kind != kindBlockUpdate && en.kind != kindBlockRemove {
			continue
		}
		info, err := os.Lstat(en.Path)
		if err == nil && !info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
			br, err := EnsureBackup(&st, en.Path, "pre-workspace-apply")
			if err != nil {
				return nil, err
			}
			if br.Created {
				res.BackedUp = append(res.BackedUp, en.Path)
			}
		}
	}

	// Phase 2: execute in plan order.
	createdWhole := map[string]bool{}
	for _, en := range entries {
		switch en.Action {
		case ActionCreate, ActionUpdate:
			if en.kind == kindGenerated {
				if err := writeAtomic(en.Path, []byte(en.content), 0o600); err != nil {
					return nil, fmt.Errorf("write %s: %w", en.Path, err)
				}
			} else if en.kind == kindGeneratedRemove {
				if err := os.Remove(en.Path); err != nil && !os.IsNotExist(err) {
					return nil, fmt.Errorf("remove %s: %w", en.Path, err)
				}
			} else if en.kind == kindBlockInsert {
				if _, err := os.Lstat(en.Path); err != nil {
					createdWhole[en.Path] = true
					if err := writeAtomic(en.Path, []byte(en.content), 0o644); err != nil {
						return nil, fmt.Errorf("create %s: %w", en.Path, err)
					}
				} else if err := modifyUserFile(en.Path, func(content string) (string, error) {
					return InsertBlock(content, en.content), nil
				}); err != nil {
					return nil, fmt.Errorf("insert managed block in %s: %w", en.Path, err)
				}
			} else if en.kind == kindBlockUpdate {
				if err := modifyUserFile(en.Path, func(content string) (string, error) {
					blk, err := FindBlock(content)
					if err != nil {
						return "", err
					}
					if blk == nil {
						return "", fmt.Errorf("managed block missing (restore with apply --force)")
					}
					return ReplaceBlock(content, blk, en.content), nil
				}); err != nil {
					return nil, fmt.Errorf("update managed block in %s: %w", en.Path, err)
				}
			}
		case ActionRemove:
			switch en.kind {
			case kindGeneratedRemove:
				if err := os.Remove(en.Path); err != nil && !os.IsNotExist(err) {
					return nil, fmt.Errorf("remove %s: %w", en.Path, err)
				}
			case kindBlockRemove:
				if err := modifyUserFile(en.Path, func(content string) (string, error) {
					blk, err := FindBlock(content)
					if err != nil {
						return "", err
					}
					if blk == nil {
						return content, nil
					}
					return RemoveBlock(content, blk), nil
				}); err != nil {
					return nil, fmt.Errorf("remove managed block from %s: %w", en.Path, err)
				}
			case kindFileDelete:
				if err := modifyUserFile(en.Path, func(content string) (string, error) {
					blk, err := FindBlock(content)
					if err != nil {
						return "", err
					}
					if blk != nil && content == blk.Content {
						return "", nil
					}
					return content, nil
				}); err != nil {
					return nil, fmt.Errorf("clean %s: %w", en.Path, err)
				}
			}
		case ActionUnchanged, ActionSkip, ActionConflict:
			continue
		}
		res.Applied = append(res.Applied, en)
	}

	// Phase 3: persist state.
	st.Generation++
	st.AppliedProfile = profileName
	st.AppliedAt = time.Now().UTC().Format(time.RFC3339)
	if st.Files == nil {
		st.Files = map[string]FileRecord{}
	}
	if st.Blocks == nil {
		st.Blocks = map[string]BlockRecord{}
	}
	for _, en := range entries {
		if en.kind == kindGenerated {
			if en.Action == ActionCreate || en.Action == ActionUpdate {
				if data, err := os.ReadFile(en.Path); err == nil {
					st.Files[en.Path] = FileRecord{SHA256: sha256Hex(data)}
				}
			}
		}
		if en.kind == kindGeneratedRemove && (en.Action == ActionRemove || en.Action == ActionUpdate) {
			delete(st.Files, en.Path)
		}
		if isUserFile(en.Path, wsDir) {
			switch en.kind {
			case kindBlockInsert:
				if data, err := os.ReadFile(en.Path); err == nil {
					blk, _ := FindBlock(string(data))
					if blk != nil {
						st.Blocks[en.Path] = BlockRecord{Created: createdWhole[en.Path], SHA256: sha256Hex([]byte(blk.Interior()))}
					}
				}
			case kindBlockUpdate:
				if data, err := os.ReadFile(en.Path); err == nil {
					blk, _ := FindBlock(string(data))
					if blk != nil {
						prev := st.Blocks[en.Path]
						st.Blocks[en.Path] = BlockRecord{Created: prev.Created, SHA256: sha256Hex([]byte(blk.Interior()))}
					}
				}
			case kindBlockRemove, kindFileDelete:
				if en.Action == ActionRemove {
					delete(st.Blocks, en.Path)
				}
			}
		}
	}
	if err := SaveState(st); err != nil {
		return nil, err
	}
	return res, nil
}

// isUserFile reports whether path is a user-owned file outside the
// Veilbox workspace directory.
func isUserFile(path, wsDir string) bool {
	return !hasPrefixPath(path, wsDir)
}

func hasPrefixPath(p, dir string) bool {
	return p == dir || len(p) > len(dir) && p[:len(dir)+1] == dir+string(filepath.Separator)
}

// modifyUserFile reads a user-owned file, applies fn to its content,
// and writes the result back with the original mode preserved. An
// empty result deletes the file.
func modifyUserFile(path string, fn func(string) (string, error)) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	out, err := fn(string(data))
	if err != nil {
		return err
	}
	if out == "" {
		return os.Remove(path)
	}
	return writeAtomic(path, []byte(out), info.Mode().Perm())
}

// writeAtomic writes data to path via a temporary file and rename.
func writeAtomic(path string, data []byte, perm os.FileMode) error {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".veilbox-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
