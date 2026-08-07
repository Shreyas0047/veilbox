package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Verdict describes the health of one managed item.
type Verdict string

const (
	VerdictClean      Verdict = "clean"
	VerdictDrifted    Verdict = "drifted"
	VerdictMissing    Verdict = "missing"
	VerdictConflict   Verdict = "conflict"
	VerdictOutdated   Verdict = "outdated"
	VerdictNotTracked Verdict = "not-tracked"
	VerdictRemoved    Verdict = "removed"
)

// Item is one managed file or block and its health.
type Item struct {
	Path    string
	Verdict Verdict
	Detail  string
}

// StatusReport is the full status view of the workspace.
type StatusReport struct {
	// Applied is true when the workspace state exists and the applied
	// profile matches the active profile.
	Applied bool
	// Clean is true when there is nothing to do (no drift, conflicts,
	// missing content, or pending changes).
	Clean          bool
	AppliedProfile string
	Generation     int
	ActiveProfile  string
	Items          []Item
	Capabilities   []string
}

// Status inspects the workspace without writing anything.
// activeProfile is the profile currently applied via `veil profile
// apply`; it is only compared against the workspace's recorded profile.
func (e *Engine) Status(prefs Preferences, activeProfile string) (*StatusReport, error) {
	report := &StatusReport{ActiveProfile: activeProfile}
	st, err := LoadState()
	if err != nil {
		return nil, err
	}
	wsDir, err := WorkspaceDir()
	if err != nil {
		return nil, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	if !HasState() {
		report.Generation = 0
		report.Applied = false
		report.Clean = false
		report.AppliedProfile = ""
		report.Items = append(report.Items, Item{Path: "(workspace)", Verdict: VerdictNotTracked, Detail: "no workspace state — run 'veil workspace apply' after applying a profile"})
		// Files present without state are orphans Veilbox cannot
		// attribute; surface them but never touch them.
		wsFiles := listWorkspaceFiles(wsDir)
		for _, p := range wsFiles {
			report.Items = append(report.Items, Item{Path: p, Verdict: VerdictConflict, Detail: "file present but no workspace state tracks it"})
		}
		return report, nil
	}

	report.Generation = st.Generation
	report.AppliedProfile = st.AppliedProfile
	if st.AppliedProfile != "" && st.AppliedProfile == activeProfile {
		report.Applied = true
	}

	// Capability checks (report-only; never install).
	checkBin := func(name string) {
		if _, err := e.LookPath(name); err != nil {
			report.Capabilities = append(report.Capabilities, fmt.Sprintf("%s is not installed", name))
		}
	}
	if prefs.Editor != "" {
		checkBin(prefs.Editor)
	}
	if prefs.Tmux {
		checkBin("tmux")
	}

	// Generated files.
	expected := map[string]string{
		filepath.Join(wsDir, ShellScriptName): ShellScriptContent(prefs),
	}
	if prefs.Tmux {
		expected[filepath.Join(wsDir, TmuxConfigName)] = TmuxConfigContent()
	}
	for path, want := range expected {
		report.Items = append(report.Items, e.itemGenerated(path, want, st))
	}
	for path := range st.Files {
		if _, stillWanted := expected[path]; !stillWanted {
			if _, err := os.Stat(path); err == nil {
				report.Items = append(report.Items, Item{Path: path, Verdict: VerdictRemoved, Detail: "no longer requested by the active profile (apply to remove)"})
			}
		}
	}

	// Managed blocks.
	blocks := map[string][]string{}
	if prefs.Shell != "" {
		blocks[filepath.Join(home, ".bashrc")] = []string{ShellIncludeLine(wsDir)}
	}
	if prefs.Tmux {
		blocks[filepath.Join(home, ".tmux.conf")] = []string{TmuxIncludeLine(wsDir)}
	}
	for path, payload := range blocks {
		report.Items = append(report.Items, e.itemBlock(path, BlockText(wsDir, payload), st))
	}
	// Stale blocks still present from a previous profile.
	for path := range st.Blocks {
		if _, stillWanted := blocks[path]; !stillWanted {
			if _, err := os.Stat(path); err == nil {
				report.Items = append(report.Items, Item{Path: path, Verdict: VerdictRemoved, Detail: "no longer requested by the active profile (apply to remove)"})
			}
		}
	}

	report.Clean = true
	sort.Slice(report.Items, func(i, j int) bool { return report.Items[i].Path < report.Items[j].Path })
	for _, it := range report.Items {
		switch it.Verdict {
		case VerdictClean, VerdictRemoved:
			continue
		default:
			report.Clean = false
		}
	}
	if report.Applied && len(report.Capabilities) == 0 && report.Clean {
		report.Clean = true
	} else {
		report.Clean = false
	}
	return report, nil
}

func (e *Engine) itemGenerated(path, want string, st State) Item {
	data, err := os.ReadFile(path)
	if !os.IsNotExist(err) && err != nil {
		return Item{Path: path, Verdict: VerdictConflict, Detail: "cannot read: " + err.Error()}
	}
	rec, hasRec := st.Files[path]
	if err != nil {
		if hasRec {
			return Item{Path: path, Verdict: VerdictMissing, Detail: "Veilbox-managed file was deleted (apply --force to restore)"}
		}
		return Item{Path: path, Verdict: VerdictNotTracked, Detail: "not created yet"}
	}
	if !hasRec {
		return Item{Path: path, Verdict: VerdictConflict, Detail: "file present but not tracked by Veilbox state"}
	}
	if sha256Hex(data) != rec.SHA256 {
		return Item{Path: path, Verdict: VerdictDrifted, Detail: "modified outside Veilbox (apply --force to restore)"}
	}
	if string(data) != want {
		return Item{Path: path, Verdict: VerdictOutdated, Detail: "needs update to match profile preferences"}
	}
	return Item{Path: path, Verdict: VerdictClean, Detail: ""}
}

func (e *Engine) itemBlock(path, want string, st State) Item {
	data, err := os.ReadFile(path)
	rec, hasRec := st.Blocks[path]
	if err != nil {
		if hasRec {
			return Item{Path: path, Verdict: VerdictMissing, Detail: "file with a managed block was deleted (apply --force to restore)"}
		}
		return Item{Path: path, Verdict: VerdictNotTracked, Detail: "not managed yet"}
	}
	info, lerr := os.Lstat(path)
	if lerr == nil && info.Mode()&os.ModeSymlink != 0 {
		return Item{Path: path, Verdict: VerdictConflict, Detail: "symlinked user file; Veilbox refuses to modify it"}
	}
	blk, ferr := FindBlock(string(data))
	if ferr != nil {
		return Item{Path: path, Verdict: VerdictConflict, Detail: ferr.Error()}
	}
	if blk == nil {
		if hasRec {
			return Item{Path: path, Verdict: VerdictDrifted, Detail: "managed block was removed (apply --force to restore)"}
		}
		return Item{Path: path, Verdict: VerdictNotTracked, Detail: "no managed block yet"}
	}
	if !hasRec {
		return Item{Path: path, Verdict: VerdictConflict, Detail: "managed block present but not tracked by Veilbox state"}
	}
	if sha256Hex([]byte(blk.Interior())) != rec.SHA256 {
		return Item{Path: path, Verdict: VerdictDrifted, Detail: "managed block edited outside Veilbox (apply --force to restore)"}
	}
	if blk.Content != want {
		return Item{Path: path, Verdict: VerdictOutdated, Detail: "managed block needs update to match profile preferences"}
	}
	return Item{Path: path, Verdict: VerdictClean, Detail: ""}
}

// listWorkspaceFiles returns regular files directly inside wsDir.
func listWorkspaceFiles(wsDir string) []string {
	entries, err := os.ReadDir(wsDir)
	if err != nil {
		return nil
	}
	var out []string
	for _, en := range entries {
		if !en.IsDir() {
			out = append(out, filepath.Join(wsDir, en.Name()))
		}
	}
	sort.Strings(out)
	return out
}
