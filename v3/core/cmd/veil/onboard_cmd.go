package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/Shreyas0047/veilbox/v3/core/cmd/veil/tui"
	"github.com/Shreyas0047/veilbox/v3/core/internal/capability"
	"github.com/Shreyas0047/veilbox/v3/core/internal/onboarding"
	"github.com/Shreyas0047/veilbox/v3/core/internal/profile"
	"github.com/Shreyas0047/veilbox/v3/core/internal/workspace"
)

// cmdOnboard implements 'veil onboard'.
//
// Interactive mode walks the wizard (role → environment → capabilities →
// workspace → review). In a terminal it runs the Bubble Tea TUI; from
// a script or a pipe it falls back to the line UI. --yes applies the
// saved selection without any prompting; on a fresh machine it seeds
// the selection with the first profile and its recommended
// experiences.
func cmdOnboard(args []string, stdout, stderr io.Writer, d deps) int {
	fs := flag.NewFlagSet("onboard", flag.ContinueOnError)
	fs.SetOutput(stderr)
	yes := fs.Bool("yes", false, "apply the saved selection without prompting (seeds defaults on first run)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if len(fs.Args()) > 0 {
		fmt.Fprintf(stderr, "veil onboard: unknown argument %q\n", fs.Args()[0])
		return 2
	}

	inputs := onboarding.PlanInputs{
		Registry:     profile.NewRegistry(),
		Catalog:      d.catalog(),
		Capabilities: capability.NewRegistry(),
		Workspace:    workspace.NewEngine(),
		Environment:  d.environmentEngine(),
	}

	if *yes {
		return onboardYes(stdout, stderr, inputs)
	}

	ui := newOnboardUI(os.Stdin, stdout, isTerminal(os.Stdin) && isTerminal(os.Stdout))
	w, err := onboarding.LoadWizard(ui, inputs)
	if err != nil {
		fmt.Fprintf(stderr, "veil: %v\n", err)
		return 1
	}
	sel, res, runErr := w.Run()
	if runErr != nil {
		if errors.Is(runErr, onboarding.ErrAborted) {
			fmt.Fprintln(stdout, "Aborted. No changes were made.")
			return 0
		}
		if res == nil {
			fmt.Fprintf(stderr, "veil: %v\n", runErr)
			return 1
		}
	}
	if res == nil {
		fmt.Fprintln(stdout, "Nothing applied.")
		if sel != nil && (sel.Profile != "" || sel.Environment != "" || len(sel.Experiences) > 0) {
			fmt.Fprintln(stdout, "Your selection is saved. Rerun 'veil onboard' to continue.")
		}
		return 0
	}
	if !res.Success {
		return 1
	}
	// An environment kept in the selection but not activated this run is
	// worth an explicit pointer; the selection already says so.
	if sel != nil && sel.Environment != "" && !stageRan(res, "environment") {
		fmt.Fprintf(stdout, "Environment %q is selected but NOT activated. Rerun 'veil onboard' and confirm activation.\n", sel.Environment)
	}
	return 0
}

func stageRan(res *onboarding.ApplyResult, name string) bool {
	for _, s := range res.Stages {
		if s.Name == name && s.Status == onboarding.StageOK {
			return true
		}
	}
	return false
}

// isTerminal reports whether the stream is attached to a terminal.
func isTerminal(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

// newOnboardUI picks the interactive Bubble Tea TUI when both streams
// are terminals; scripts, pipes and tests keep the line UI.
func newOnboardUI(in *os.File, out io.Writer, tty bool) onboarding.UI {
	if tty {
		return tui.NewWith(in, out)
	}
	return &lineUI{in: bufio.NewReader(in), out: out}
}

// onboardYes applies the saved selection non-interactively.
func onboardYes(stdout, stderr io.Writer, inputs onboarding.PlanInputs) int {
	sel, err := onboarding.Load()
	if err != nil {
		fmt.Fprintf(stderr, "veil: %v\n", err)
		return 1
	}
	fresh := sel.Profile == "" && sel.Environment == "" && len(sel.Experiences) == 0 && len(sel.Capabilities) == 0
	if fresh {
		names, lerr := inputs.Registry.List()
		if lerr != nil {
			fmt.Fprintf(stderr, "veil: %v\n", lerr)
			return 1
		}
		if len(names) == 0 {
			fmt.Fprintln(stderr, "veil: no profiles installed — nothing to default to")
			return 1
		}
		sort.Strings(names)
		sel.Profile = names[0]
		if seed, serr := onboarding.SeedCapabilities(inputs.Registry, inputs.Capabilities, inputs.Catalog, sel.Profile); serr == nil {
			sel.Capabilities = seed
		}
		if err := sel.Derive(inputs.Resolver()); err != nil {
			fmt.Fprintf(stderr, "veil: %v\n", err)
			return 1
		}
		if err := sel.Save(); err != nil {
			fmt.Fprintf(stderr, "veil: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "First run: defaulting to profile %q with its recommended capabilities.\n", sel.Profile)
	} else {
		fmt.Fprintln(stdout, "Applying your saved selection.")
		if err := sel.Derive(inputs.Resolver()); err != nil {
			fmt.Fprintf(stderr, "veil: %v\n", err)
			return 1
		}
	}
	res, aerr := onboarding.Apply(sel, inputs)
	fmt.Fprint(stdout, onboarding.RenderReport(res, aerr))
	if aerr != nil {
		return 1
	}
	return 0
}

// lineUI is the non-interactive fallback UI: numbered prompts, one
// line at a time. It is also what tests and non-TTY terminals use.
type lineUI struct {
	in  *bufio.Reader
	out io.Writer
}

func (u *lineUI) Welcome() error {
	fmt.Fprintln(u.out, "Veilbox onboarding — pick your engineer role, environment and capabilities.")
	fmt.Fprintln(u.out, "Enter a number to choose. Empty input keeps the current value. 'q' quits.")
	return nil
}

// read returns the trimmed input line or onboarding.ErrAborted on
// 'q' or EOF.
func (u *lineUI) read(prompt string) (string, error) {
	fmt.Fprint(u.out, prompt)
	line, err := u.in.ReadString('\n')
	if err != nil && line == "" {
		return "", onboarding.ErrAborted
	}
	line = strings.TrimSpace(line)
	if strings.EqualFold(line, "q") {
		return "", onboarding.ErrAborted
	}
	return line, nil
}

func (u *lineUI) pickNumber(prompt string, count int, current int) (int, error) {
	for {
		line, err := u.read(prompt)
		if err != nil {
			return 0, err
		}
		if line == "" {
			return current, nil
		}
		n, cerr := strconv.Atoi(line)
		if cerr != nil || n < 0 || n >= count {
			fmt.Fprintf(u.out, "Enter a number between 0 and %d, or leave empty for %d.\n", count-1, current)
			continue
		}
		return n, nil
	}
}

func (u *lineUI) SelectRole(choices []onboarding.RoleChoice, current string) (string, error) {
	if len(choices) == 0 {
		return "", fmt.Errorf("no roles available")
	}
	cur := 0
	names := make([]string, len(choices))
	for i, c := range choices {
		names[i] = c.Name
		if c.Name == current {
			cur = i
		}
	}
	fmt.Fprintln(u.out, "\nROLE")
	for i, c := range choices {
		mark := " "
		if i == cur {
			mark = "*"
		}
		fmt.Fprintf(u.out, "%s %d. %s — %s\n", mark, i, c.DisplayName, c.Description)
		if len(c.Recommended) > 0 {
			fmt.Fprintf(u.out, "      recommends: %s\n", strings.Join(c.Recommended, ", "))
		}
	}
	n, err := u.pickNumber("Select a role: ", len(choices), cur)
	if err != nil {
		return "", err
	}
	return names[n], nil
}

func (u *lineUI) SelectEnvironment(choices []onboarding.EnvironmentChoice, current string) (string, error) {
	cur := 0
	names := make([]string, len(choices)+1)
	names[0] = ""
	for i, c := range choices {
		names[i+1] = c.Name
		if c.Name == current {
			cur = i + 1
		}
	}
	fmt.Fprintln(u.out, "\nENVIRONMENT")
	fmt.Fprintf(u.out, "%s 0. None — keep the machine headless\n", marker(cur == 0))
	for i, c := range choices {
		status := ""
		if c.Status != "" {
			status = " [" + c.Status + "]"
		}
		if c.Active {
			status += " (active)"
		}
		fmt.Fprintf(u.out, "%s %d. %s — %s%s\n", marker(cur == i+1), i+1, c.DisplayName, c.Description, status)
	}
	n, err := u.pickNumber("Select an environment: ", len(choices)+1, cur)
	if err != nil {
		return "", err
	}
	return names[n], nil
}

func (u *lineUI) SelectCapabilities(groups []onboarding.CapabilityGroup, current []string) ([]string, error) {
	selected := map[string]bool{}
	for _, c := range current {
		selected[c] = true
	}
	fmt.Fprintln(u.out, "\nCAPABILITIES")
	fmt.Fprintln(u.out, "For each group, type numbers to toggle (e.g. 1,3). Empty keeps, 'a' selects all, 'n' selects none.")
	for _, g := range groups {
		if len(g.Choices) == 0 {
			continue
		}
		fmt.Fprintf(u.out, "\n[%s]\n", g.Domain)
		requiredOnly := true
		for i, c := range g.Choices {
			recommend := " "
			if c.Recommended {
				recommend = "r"
			}
			mark := " "
			if selected[c.Name] {
				mark = "*"
			}
			status := ""
			if c.Status != "" {
				status = " [" + c.Status + "]"
			}
			if c.Required {
				selected[c.Name] = true
				fmt.Fprintf(u.out, "%s %s %d. %s — %s (required, always included)%s\n", mark, recommend, i, c.DisplayName, c.Description, status)
				continue
			}
			requiredOnly = false
			fmt.Fprintf(u.out, "%s %s %d. %s — %s%s\n", mark, recommend, i, c.DisplayName, c.Description, status)
		}
		if requiredOnly {
			fmt.Fprintln(u.out, "  (all capabilities in this group are required)")
			continue
		}
		line, err := u.read(fmt.Sprintf("Toggle for %s: ", g.Domain))
		if err != nil {
			return nil, err
		}
		switch {
		case line == "":
		case strings.EqualFold(line, "a"):
			for _, c := range g.Choices {
				if !c.Required {
					selected[c.Name] = true
				}
			}
		case strings.EqualFold(line, "n"):
			for _, c := range g.Choices {
				if !c.Required {
					selected[c.Name] = false
				}
			}
		default:
			for _, part := range strings.Split(line, ",") {
				n, cerr := strconv.Atoi(strings.TrimSpace(part))
				if cerr != nil || n < 0 || n >= len(g.Choices) {
					continue
				}
				c := g.Choices[n]
				if !c.Required {
					selected[c.Name] = !selected[c.Name]
				}
			}
		}
	}
	var out []string
	for _, g := range groups {
		for _, c := range g.Choices {
			if selected[c.Name] {
				out = append(out, c.Name)
			}
		}
	}
	return out, nil
}

func (u *lineUI) SelectWorkspace(opts onboarding.WorkspaceOptions, current onboarding.WorkspacePrefs) (onboarding.WorkspacePrefs, error) {
	prefs := current
	fmt.Fprintln(u.out, "\nWORKSPACE")
	fmt.Fprintln(u.out, "Leave empty to inherit from the profile.")
	line, err := u.read(fmt.Sprintf("Editor (%s) [%s]: ", strings.Join(opts.Editors, "/"), current.Editor))
	if err != nil {
		return prefs, err
	}
	if line != "" {
		prefs.Editor = line
	}
	line, err = u.read(fmt.Sprintf("Prompt (%s) [%s]: ", strings.Join(opts.Prompts, "/"), current.Prompt))
	if err != nil {
		return prefs, err
	}
	if line != "" {
		prefs.Prompt = line
	}
	line, err = u.read(fmt.Sprintf("Terminal (%s) [%s]: ", strings.Join(opts.Terminals, "/"), current.Terminal))
	if err != nil {
		return prefs, err
	}
	if line != "" {
		prefs.Terminal = line
	}
	line, err = u.read("Tmux integration (y/n) [keep]: ")
	if err != nil {
		return prefs, err
	}
	switch strings.ToLower(line) {
	case "y", "yes":
		b := true
		prefs.Tmux = &b
	case "n", "no":
		b := false
		prefs.Tmux = &b
	}
	return prefs, nil
}

func (u *lineUI) Review(info onboarding.ReviewInfo) (onboarding.ReviewDecision, error) {
	fmt.Fprintln(u.out, "\nREVIEW — nothing has changed on the machine yet.")
	fmt.Fprint(u.out, info.Text)
	for {
		line, err := u.read("Apply this plan? [y/N/b] (b goes back): ")
		if err != nil {
			return onboarding.ReviewDecision{}, err
		}
		switch strings.ToLower(line) {
		case "y", "yes":
			decision := onboarding.ReviewDecision{Apply: true, ActivateEnvironment: true}
			if info.Selection.Environment != "" {
				al, aerr := u.read(fmt.Sprintf("Activate %s now (enable its display manager and start at login)? [Y/n]: ", info.Selection.Environment))
				if aerr != nil {
					return onboarding.ReviewDecision{}, aerr
				}
				switch strings.ToLower(al) {
				case "", "y", "yes":
					decision.ActivateEnvironment = true
				default:
					decision.ActivateEnvironment = false
				}
			}
			return decision, nil
		case "b", "back":
			return onboarding.ReviewDecision{Restart: true}, nil
		case "":
			continue
		default:
			return onboarding.ReviewDecision{}, nil
		}
	}
}

func (u *lineUI) ShowReport(report string) error {
	fmt.Fprint(u.out, report)
	return nil
}

func marker(on bool) string {
	if on {
		return "*"
	}
	return " "
}
