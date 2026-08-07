package workspace

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Action classifies one planned change to the workspace.
type Action string

const (
	// ActionCreate writes a new Veilbox-owned file or inserts a
	// managed block.
	ActionCreate Action = "create"
	// ActionUpdate rewrites Veilbox-owned content to match intent.
	ActionUpdate Action = "update"
	// ActionUnchanged means nothing to do.
	ActionUnchanged Action = "unchanged"
	// ActionRemove deletes Veilbox-owned content only (generated
	// files, managed blocks, or whole files Veilbox created).
	ActionRemove Action = "remove"
	// ActionConflict is a file that needs a human decision.
	ActionConflict Action = "conflict"
	// ActionSkip is a preference that cannot be satisfied today
	// (e.g. a missing capability binary). Nothing is done.
	ActionSkip Action = "skip"
)

// entryKind tells apply how to execute an entry. It is derived at
// planning time so the plan is the single source of truth for apply.
type entryKind int

const (
	kindGenerated       entryKind = iota // write a generated file (content)
	kindGeneratedRemove                  // delete a generated file
	kindBlockInsert                      // insert a managed block (content = block text)
	kindBlockUpdate                      // replace a managed block interior (content = block text)
	kindBlockRemove                      // strip a managed block, keep the file
	kindFileDelete                       // delete a user file containing only Veilbox content
)

// Entry is one planned change.
type Entry struct {
	Action Action
	Path   string
	Detail string
	// Drift marks conflicts caused by external modification of
	// Veilbox-managed content; apply --force is allowed to resolve
	// them by restoring Veilbox content.
	Drift bool
	// Fatal marks structural conflicts (symlinked user files,
	// ambiguous blocks) that apply --force must never resolve.
	Fatal bool

	kind    entryKind
	content string
}

// Plan is the complete, deterministic description of what apply would
// do. Building a plan never touches the filesystem beyond reading.
type Plan struct {
	Entries []Entry
	// Capabilities lists unmet preference requirements (missing
	// binaries). The engine reports these; it never installs.
	Capabilities []string
}

// IsClean reports whether the plan requires no changes. Skip entries
// (unmet capabilities) are reported but are not pending changes.
func (p *Plan) IsClean() bool {
	for _, e := range p.Entries {
		if e.Action != ActionUnchanged && e.Action != ActionSkip {
			return false
		}
	}
	return true
}

// Conflicts returns the conflicting entries.
func (p *Plan) Conflicts() []Entry {
	var out []Entry
	for _, e := range p.Entries {
		if e.Action == ActionConflict {
			out = append(out, e)
		}
	}
	return out
}

// HasFatalConflict reports whether any conflict is structural and can
// never be resolved by --force.
func (p *Plan) HasFatalConflict() bool {
	for _, e := range p.Entries {
		if e.Action == ActionConflict && e.Fatal {
			return true
		}
	}
	return false
}

// LookPathFunc resolves a command to its path (exec.LookPath shape),
// injectable so tests can simulate missing capabilities.
type LookPathFunc func(string) (string, error)

// Engine executes the workspace lifecycle. It never performs DNF
// transactions and never writes outside Veilbox-owned paths plus the
// managed blocks it maintains.
type Engine struct {
	LookPath LookPathFunc
}

// NewEngine returns an Engine using the real PATH.
func NewEngine() *Engine {
	return &Engine{LookPath: exec.LookPath}
}

// planner is the context shared while building a plan.
type planner struct {
	eng      *Engine
	st       State
	hasState bool
	home     string
	wsDir    string
	prefs    Preferences
	cap      map[string]bool
	entries  []Entry
}

func (e *Engine) buildPlanner(prefs Preferences, st State) (*planner, error) {
	if err := prefs.Validate(); err != nil {
		return nil, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home directory: %w", err)
	}
	wsDir, err := WorkspaceDir()
	if err != nil {
		return nil, err
	}
	p := &planner{
		eng:      e,
		st:       st,
		hasState: HasState(),
		home:     home,
		wsDir:    wsDir,
		prefs:    prefs,
		cap:      map[string]bool{},
	}
	return p, nil
}

// hasBinary checks and caches capability presence.
func (p *planner) hasBinary(name string) bool {
	if v, ok := p.cap[name]; ok {
		return v
	}
	_, err := p.eng.LookPath(name)
	v := err == nil
	p.cap[name] = v
	if !v {
		p.add(ActionSkip, name, "binary not installed — capability unmet; nothing will be installed automatically")
	}
	return v
}

func (p *planner) add(a Action, path, detail string) {
	p.entry(a, path, detail, false, false, 0, "")
}

// BuildPlan computes the plan for applying prefs given the current
// workspace state. Pure read-only computation.
func (e *Engine) BuildPlan(prefs Preferences, st State) (*Plan, error) {
	p, err := e.buildPlanner(prefs, st)
	if err != nil {
		return nil, err
	}
	// Capability checks first: an unmet binary turns the tmux
	// surfaces into SKIP and drops EDITOR from the generated script.
	editorOK := p.prefs.Editor == "" || p.hasBinary(p.prefs.Editor)
	if p.prefs.Editor != "" && !editorOK {
		p.add(ActionSkip, "editor:"+p.prefs.Editor, "editor binary not installed")
	}
	tmuxOK := p.prefs.Tmux && p.hasBinary("tmux")
	if p.prefs.Tmux && !tmuxOK {
		p.add(ActionSkip, "tmux", "tmux binary not installed")
	}

	p.planGenerated(ShellScriptName, ShellScriptContent(renderedPrefs(p.prefs, editorOK)), true)
	if p.prefs.Tmux {
		if tmuxOK {
			p.planGenerated(TmuxConfigName, TmuxConfigContent(), true)
		}
	} else {
		p.planStaleGenerated(TmuxConfigName)
	}
	p.planBlock(filepath.Join(p.home, ".bashrc"), []string{ShellIncludeLine(p.wsDir)})
	if p.prefs.Tmux && tmuxOK {
		p.planBlock(filepath.Join(p.home, ".tmux.conf"), []string{TmuxIncludeLine(p.wsDir)})
	} else {
		p.planStaleBlock(filepath.Join(p.home, ".tmux.conf"))
	}

	plan := &Plan{Entries: p.entries}
	if !editorOK {
		plan.Capabilities = append(plan.Capabilities, fmt.Sprintf("editor %q not installed", p.prefs.Editor))
	}
	if p.prefs.Tmux && !tmuxOK {
		plan.Capabilities = append(plan.Capabilities, "tmux not installed (recommend the terminal-ops experience)")
	}
	return plan, nil
}

// renderedPrefs drops the EDITOR line when the editor binary is
// missing; generation stays deterministic.
func renderedPrefs(prefs Preferences, editorOK bool) Preferences {
	if !editorOK {
		prefs.Editor = ""
	}
	return prefs
}

// planGenerated plans a Veilbox-owned generated file.
func (p *planner) planGenerated(name, expected string, always bool) {
	path := filepath.Join(p.wsDir, name)
	rec, hasRec := p.st.Files[path]
	data, err := os.ReadFile(path)
	exists := err == nil

	switch {
	case !exists:
		if hasRec {
			p.entry(ActionConflict, path, "Veilbox-managed file was deleted; restore with 'apply --force'", true, false, kindGenerated, expected)
		} else {
			p.entry(ActionCreate, path, "", false, false, kindGenerated, expected)
		}
	case !hasRec:
		if p.hasState {
			p.entry(ActionConflict, path, "file present but not tracked by Veilbox state", true, false, kindGenerated, expected)
		} else if string(data) == expected {
			p.entry(ActionUnchanged, path, "", false, false, kindGenerated, expected)
		} else {
			p.entry(ActionCreate, path, "", false, false, kindGenerated, expected)
		}
	default:
		hash := sha256Hex(data)
		switch {
		case hash != rec.SHA256:
			p.entry(ActionConflict, path, "modified outside Veilbox (drift); use 'apply --force' to restore", true, false, kindGenerated, expected)
		case string(data) == expected:
			p.entry(ActionUnchanged, path, "", false, false, kindGenerated, expected)
		default:
			p.entry(ActionUpdate, path, "configuration changed by profile preferences", false, false, kindGenerated, expected)
		}
	}
}

// planStaleGenerated plans removal of a generated file the current
// preferences no longer want.
func (p *planner) planStaleGenerated(name string) {
	path := filepath.Join(p.wsDir, name)
	if _, hasRec := p.st.Files[path]; hasRec {
		p.entry(ActionRemove, path, "no longer requested by the active profile", false, false, kindGeneratedRemove, "")
		return
	}
	if _, err := os.Stat(path); err == nil {
		p.entry(ActionConflict, path, "file present but not tracked by Veilbox state", true, false, kindGeneratedRemove, "")
		return
	}
	p.entry(ActionUnchanged, path, "", false, false, kindGeneratedRemove, "")
}

// planBlock plans the single managed block inside a user-owned file.
func (p *planner) planBlock(path string, payload []string) {
	expected := BlockText(p.wsDir, payload)
	rec, hasRec := p.st.Blocks[path]

	info, err := os.Lstat(path)
	if err != nil {
		// File missing.
		if hasRec {
			p.entry(ActionConflict, path, "file with a managed block was deleted", true, false, kindBlockInsert, expected)
			return
		}
		p.entry(ActionCreate, path, "create file with a Veilbox managed include block", false, false, kindBlockInsert, expected)
		return
	}
	if info.Mode()&os.ModeSymlink != 0 {
		p.entry(ActionConflict, path, "refusing to modify a symlinked user file", false, true, kindBlockUpdate, expected)
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		p.entry(ActionConflict, path, "cannot read file", false, true, kindBlockUpdate, expected)
		return
	}
	blk, err := FindBlock(string(data))
	if err != nil {
		p.entry(ActionConflict, path, err.Error(), false, true, kindBlockUpdate, expected)
		return
	}
	if blk == nil {
		if hasRec {
			p.entry(ActionConflict, path, "managed block was removed; restore with 'apply --force'", true, false, kindBlockInsert, expected)
			return
		}
		p.entry(ActionCreate, path, "insert a Veilbox managed include block (existing content preserved)", false, false, kindBlockInsert, expected)
		return
	}
	actual := sha256Hex([]byte(blk.Interior()))
	switch {
	case !hasRec:
		p.entry(ActionConflict, path, "managed block present but not tracked by Veilbox state", true, false, kindBlockUpdate, expected)
	case actual != rec.SHA256:
		p.entry(ActionConflict, path, "managed block edited outside Veilbox (drift); use 'apply --force' to restore", true, false, kindBlockUpdate, expected)
	case blk.Content == expected:
		p.entry(ActionUnchanged, path, "", false, false, kindBlockUpdate, expected)
	default:
		p.entry(ActionUpdate, path, "managed block content changed by profile preferences", false, false, kindBlockUpdate, expected)
	}
}

// planStaleBlock plans removal of a managed block (and, for files
// Veilbox created, the file itself) the current preferences no longer
// want.
func (p *planner) planStaleBlock(path string) {
	rec, hasRec := p.st.Blocks[path]
	if !hasRec {
		p.entry(ActionUnchanged, path, "", false, false, kindBlockRemove, "")
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		p.entry(ActionRemove, path, "stale managed block (file already gone)", false, false, kindBlockRemove, "")
		return
	}
	blk, err := FindBlock(string(data))
	if err != nil {
		p.entry(ActionConflict, path, err.Error(), false, true, kindBlockRemove, "")
		return
	}
	if blk == nil {
		p.entry(ActionConflict, path, "managed block was removed outside Veilbox; nothing to clean", false, false, kindBlockRemove, "")
		return
	}
	if rec.Created && string(data) == blk.Content {
		p.entry(ActionRemove, path, "delete Veilbox-created file (contains only Veilbox content)", false, false, kindFileDelete, "")
		return
	}
	p.entry(ActionRemove, path, "remove the Veilbox managed include block", false, false, kindBlockRemove, "")
}

// entry appends an entry with execution metadata.
func (p *planner) entry(a Action, path, detail string, drift, fatal bool, kind entryKind, content string) {
	p.entries = append(p.entries, Entry{Action: a, Path: path, Detail: detail, Drift: drift, Fatal: fatal, kind: kind, content: content})
}
