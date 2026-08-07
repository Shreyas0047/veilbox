package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Shreyas0047/veilbox/v3/core/internal/profile"
	"github.com/Shreyas0047/veilbox/v3/core/internal/settings"
	"github.com/Shreyas0047/veilbox/v3/core/internal/workspace"
)

const workspaceUsage = `Usage:
  veil workspace                  Show workspace overview
  veil workspace plan             Show what apply would do (no changes)
  veil workspace apply [--yes] [--force]
                                  Apply the active profile's workspace
  veil workspace status           Report applied state, drift, conflicts
  veil workspace reset [--yes]    Remove only Veilbox-managed workspace config
`

// cmdWorkspace dispatches the workspace subcommands.
func cmdWorkspace(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return workspaceOverview(stdout, stderr)
	}
	switch args[0] {
	case "plan":
		return workspacePlan(stdout, stderr)
	case "apply":
		return workspaceApply(args[1:], stdout, stderr)
	case "status":
		return workspaceStatus(stdout, stderr)
	case "reset":
		return workspaceReset(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprint(stdout, workspaceUsage)
		return 0
	default:
		fmt.Fprintf(stderr, "veil workspace: unknown command %q\n\n%s", args[0], workspaceUsage)
		return 2
	}
}

// activeManifest loads the active profile and its manifest. It errors
// when no profile has been applied.
func activeManifest() (profile.Manifest, settings.State, error) {
	st, err := settings.LoadState()
	if err != nil {
		return profile.Manifest{}, st, err
	}
	if st.ActiveProfile == "" {
		return profile.Manifest{}, st, fmt.Errorf("no active profile — apply one first with 'veil profile apply <name>'")
	}
	m, err := profile.NewRegistry().Load(st.ActiveProfile)
	return m, st, err
}

func newWorkspaceEngine() *workspace.Engine {
	return workspace.NewEngine()
}

func workspaceOverview(stdout, stderr io.Writer) int {
	m, st, err := activeManifest()
	if err != nil {
		fmt.Fprintf(stderr, "veil: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Workspace\n---------\n")
	fmt.Fprintf(stdout, "Active profile: %s\n", m.Name)
	lines := renderPrefs(m.Workspace)
	if len(lines) > 0 {
		fmt.Fprintln(stdout, "Preferences:")
		for _, l := range lines {
			fmt.Fprintf(stdout, "  %s\n", l)
		}
	}
	eng := newWorkspaceEngine()
	rep, err := eng.Status(m.Workspace, st.ActiveProfile)
	if err != nil {
		fmt.Fprintf(stderr, "veil: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, "State:")
	if rep.Applied && rep.Clean {
		fmt.Fprintf(stdout, "  applied for %s (generation %d) — clean\n", rep.AppliedProfile, rep.Generation)
	} else if rep.Generation > 0 {
		fmt.Fprintf(stdout, "  generation %d", rep.Generation)
		if rep.AppliedProfile != "" {
			fmt.Fprintf(stdout, ", applied for %s", rep.AppliedProfile)
		}
		fmt.Fprintf(stdout, " — %s\n", workspaceHealth(rep))
	} else {
		fmt.Fprintln(stdout, "  not applied — run 'veil workspace apply'")
	}
	if len(rep.Capabilities) > 0 {
		fmt.Fprintln(stdout, "Unmet capabilities (reported, never installed):")
		for _, c := range rep.Capabilities {
			fmt.Fprintf(stdout, "  - %s\n", c)
		}
	}
	return 0
}

func workspaceHealth(rep *workspace.StatusReport) string {
	var counts []string
	for _, it := range rep.Items {
		switch it.Verdict {
		case workspace.VerdictClean, workspace.VerdictRemoved:
			continue
		case workspace.VerdictConflict:
			counts = append(counts, "conflicts")
		case workspace.VerdictDrifted:
			counts = append(counts, "drift")
		case workspace.VerdictMissing:
			counts = append(counts, "missing")
		case workspace.VerdictOutdated:
			counts = append(counts, "outdated")
		}
	}
	if len(counts) == 0 {
		return "needs apply"
	}
	seen := map[string]bool{}
	var uniq []string
	for _, c := range counts {
		if !seen[c] {
			seen[c] = true
			uniq = append(uniq, c)
		}
	}
	return strings.Join(uniq, ", ")
}

func workspacePlan(stdout, stderr io.Writer) int {
	m, _, err := activeManifest()
	if err != nil {
		fmt.Fprintf(stderr, "veil: %v\n", err)
		return 1
	}
	eng := newWorkspaceEngine()
	st, err := workspace.LoadState()
	if err != nil {
		fmt.Fprintf(stderr, "veil: %v\n", err)
		return 1
	}
	plan, err := eng.BuildPlan(m.Workspace, st)
	if err != nil {
		fmt.Fprintf(stderr, "veil: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Workspace plan for profile %q (dry run — no changes made)\n", m.Name)
	fmt.Fprintln(stdout, "Action     Path")
	fmt.Fprintln(stdout, "---------- ---------------------------------------------------")
	for _, en := range plan.Entries {
		detail := ""
		if en.Detail != "" {
			detail = "  " + en.Detail
		}
		fmt.Fprintf(stdout, "%-10s %s%s\n", strings.ToUpper(string(en.Action)), en.Path, detail)
	}
	for _, c := range plan.Capabilities {
		fmt.Fprintf(stdout, "SKIP       capability unmet: %s\n", c)
	}
	if plan.IsClean() {
		fmt.Fprintln(stdout, "\nNothing to do — workspace matches preferences.")
		return 0
	}
	if cf := plan.Conflicts(); len(cf) > 0 {
		fmt.Fprintln(stdout, "\nConflicts require a decision; apply will refuse until resolved.")
	}
	return 0
}

func workspaceApply(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("apply", flag.ContinueOnError)
	fs.SetOutput(stderr)
	yes := fs.Bool("yes", false, "skip confirmation")
	force := fs.Bool("force", false, "restore drifted Veilbox-managed content")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if len(fs.Args()) > 0 {
		fmt.Fprintf(stderr, "veil workspace apply: unknown argument %q\n", fs.Args()[0])
		return 2
	}
	m, st, err := activeManifest()
	if err != nil {
		fmt.Fprintf(stderr, "veil: %v\n", err)
		return 1
	}
	eng := newWorkspaceEngine()
	ws, err := workspace.LoadState()
	if err != nil {
		fmt.Fprintf(stderr, "veil: %v\n", err)
		return 1
	}
	plan, err := eng.BuildPlan(m.Workspace, ws)
	if err != nil {
		fmt.Fprintf(stderr, "veil: %v\n", err)
		return 1
	}
	conflicts := plan.Conflicts()
	for _, en := range plan.Entries {
		if en.Action != workspace.ActionUnchanged {
			detail := ""
			if en.Detail != "" {
				detail = "  " + en.Detail
			}
			fmt.Fprintf(stdout, "%-10s %s%s\n", strings.ToUpper(string(en.Action)), en.Path, detail)
		}
	}
	if len(conflicts) > 0 && !*force {
		fmt.Fprintf(stdout, "\n%d conflict(s) — applying now would change nothing.\n", len(conflicts))
		fmt.Fprintln(stdout, "Inspect with 'veil workspace status'. Use '--force' only to restore")
		fmt.Fprintln(stdout, "Veilbox-managed content that was modified outside Veilbox; it never")
		fmt.Fprintln(stdout, "replaces whole user-owned files.")
		return 1
	}
	if !*yes {
		fmt.Fprint(stdout, "Proceed? [y/N]: ")
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if !strings.EqualFold(strings.TrimSpace(line), "y") {
			fmt.Fprintln(stdout, "Aborted.")
			return 1
		}
	}
	res, err := eng.Apply(m.Workspace, st.ActiveProfile, *force)
	if err != nil {
		fmt.Fprintf(stderr, "veil: %v\n", err)
		return 1
	}
	if len(res.Applied) == 0 {
		fmt.Fprintf(stdout, "Workspace for profile %q is already up to date.\n", m.Name)
		return 0
	}
	fmt.Fprintf(stdout, "Workspace applied for profile %q.\n", m.Name)
	for _, en := range res.Applied {
		fmt.Fprintf(stdout, "  %s %s\n", strings.ToLower(string(en.Action)), en.Path)
	}
	if len(res.ResolvedConflicts) > 0 {
		fmt.Fprintln(stdout, "Drift restored (Veilbox-managed content):")
		for _, en := range res.ResolvedConflicts {
			fmt.Fprintf(stdout, "  %s\n", en.Path)
		}
	}
	if len(res.BackedUp) > 0 {
		fmt.Fprintln(stdout, "Backed up before first modification (kept in ~/.config/veilbox/backups/):")
		for _, p := range res.BackedUp {
			fmt.Fprintf(stdout, "  %s\n", p)
		}
	}
	if len(res.Capabilities) > 0 {
		fmt.Fprintln(stdout, "Unmet capabilities (not installed by Veilbox):")
		for _, c := range res.Capabilities {
			fmt.Fprintf(stdout, "  - %s\n", c)
		}
	}
	return 0
}

func workspaceStatus(stdout, stderr io.Writer) int {
	m, st, err := activeManifest()
	if err != nil {
		fmt.Fprintf(stderr, "veil: %v\n", err)
		return 1
	}
	eng := newWorkspaceEngine()
	rep, err := eng.Status(m.Workspace, st.ActiveProfile)
	if err != nil {
		fmt.Fprintf(stderr, "veil: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Workspace status\n----------------\n")
	fmt.Fprintf(stdout, "Active profile:  %s\n", rep.ActiveProfile)
	if rep.AppliedProfile != "" {
		fmt.Fprintf(stdout, "Applied for:     %s\n", rep.AppliedProfile)
	}
	fmt.Fprintf(stdout, "Generation:      %d\n", rep.Generation)
	if rep.Applied && rep.Clean {
		fmt.Fprintln(stdout, "State:           clean")
	} else {
		fmt.Fprintln(stdout, "State:           "+workspaceHealth(rep))
	}
	for _, it := range rep.Items {
		detail := ""
		if it.Detail != "" {
			detail = "  " + it.Detail
		}
		fmt.Fprintf(stdout, "%-9s %s%s\n", string(it.Verdict), it.Path, detail)
	}
	for _, c := range rep.Capabilities {
		fmt.Fprintf(stdout, "unmet     capability: %s\n", c)
	}
	return 0
}

func workspaceReset(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("reset", flag.ContinueOnError)
	fs.SetOutput(stderr)
	yes := fs.Bool("yes", false, "skip confirmation")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if len(fs.Args()) > 0 {
		fmt.Fprintf(stderr, "veil workspace reset: unknown argument %q\n", fs.Args()[0])
		return 2
	}
	ws, err := workspace.LoadState()
	if err != nil {
		fmt.Fprintf(stderr, "veil: %v\n", err)
		return 1
	}
	// Preview what will be removed: state records plus on-disk files.
	var toRemove []string
	for path := range ws.Files {
		if _, err := os.Stat(path); err == nil {
			toRemove = append(toRemove, path)
		}
	}
	for path := range ws.Blocks {
		if _, err := os.Stat(path); err == nil {
			toRemove = append(toRemove, path+" (managed block)")
		}
	}
	if len(toRemove) == 0 {
		fmt.Fprintln(stdout, "Nothing to reset — no Veilbox-managed workspace configuration found.")
		return 0
	}
	fmt.Fprintln(stdout, "This removes ONLY Veilbox-managed workspace configuration:")
	for _, p := range toRemove {
		fmt.Fprintf(stdout, "  - %s\n", p)
	}
	fmt.Fprintln(stdout, "User-owned file content is preserved. Backups are kept.")
	if !*yes {
		fmt.Fprint(stdout, "Proceed? [y/N]: ")
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if !strings.EqualFold(strings.TrimSpace(line), "y") {
			fmt.Fprintln(stdout, "Aborted.")
			return 1
		}
	}
	res, err := newWorkspaceEngine().Reset()
	if err != nil {
		fmt.Fprintf(stderr, "veil: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, "Workspace reset.")
	for _, p := range res.Removed {
		fmt.Fprintf(stdout, "  removed %s\n", p)
	}
	return 0
}

// renderPrefs renders preferences as deterministic "key: value" lines.
func renderPrefs(p workspace.Preferences) []string {
	var out []string
	if p.Shell != "" {
		out = append(out, fmt.Sprintf("shell: %s", p.Shell))
	}
	if p.Editor != "" {
		out = append(out, fmt.Sprintf("editor: %s", p.Editor))
	}
	if p.Terminal != "" {
		out = append(out, fmt.Sprintf("terminal: %s", p.Terminal))
	}
	if p.Prompt != "" {
		out = append(out, fmt.Sprintf("prompt: %s", p.Prompt))
	}
	if p.Tmux {
		out = append(out, "tmux: true")
	}
	for _, k := range p.SortedAliasNames() {
		out = append(out, fmt.Sprintf("alias %s: %s", k, p.Aliases[k]))
	}
	for _, k := range p.SortedEnvNames() {
		out = append(out, fmt.Sprintf("environment %s: %s", k, p.Environment[k]))
	}
	return out
}
