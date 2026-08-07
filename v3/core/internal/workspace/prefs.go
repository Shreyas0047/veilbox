// Package workspace implements the Workspace Engine: it translates the
// workspace_preferences of the active profile into user-level workspace
// configuration, and manages that configuration under strict ownership.
//
// Ownership model (see docs/adr/0005-workspace-ownership.md):
//
//	Veilbox owns what Veilbox generates. It never silently destroys
//	what the user owns.
//
// Veilbox-generated configuration lives under the Veilbox-owned
// workspace directory; integration with user-owned files (e.g.
// ~/.bashrc) happens only through a single, clearly marked managed
// include block. Everything outside the block is preserved
// byte-for-byte. The engine never executes DNF: capabilities that are
// missing are reported, not installed.
package workspace

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Supported preference values. Preferences are declarative primitives:
// no field accepts arbitrary shell text (see Preferences.Validate).
const (
	// ShellBash is the only shell integrated today.
	ShellBash = "bash"

	// Terminal values.
	TerminalAuto      = "system"
	TerminalKitty     = "kitty"
	TerminalWezterm   = "wezterm"
	TerminalGhostty   = "ghostty"
	TerminalAlacritty = "alacritty"

	// Prompt styles.
	PromptPlain   = "plain"
	PromptVeilbox = "veilbox"
)

var (
	// wordPattern matches a single safe shell token: letters, digits
	// and punctuation that cannot escape a quoted word.
	wordPattern = regexp.MustCompile(`^[A-Za-z0-9_./:+\-=~]+$`)
	// commandPattern matches a simple command: safe tokens joined by
	// single spaces (e.g. "kubectl get pods -o wide"). It rejects
	// every shell metacharacter: ; | & $ ` ( ) < > ' " * ? [ ] { }.
	commandPattern = regexp.MustCompile(`^[A-Za-z0-9_./:+\-=~]+(?: [A-Za-z0-9_./:+\-=~]+)*$`)
	// aliasKeyPattern matches alias names: no spaces or metacharacters.
	aliasKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]+$`)
	// envKeyPattern matches POSIX environment variable names.
	envKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// Preferences is the structured workspace_preferences block of a
// profile manifest. It describes desired workspace behavior in
// declarative primitives; validation rejects anything that could
// become an arbitrary shell command.
type Preferences struct {
	// Shell is the login shell to integrate with. Only bash is
	// supported today.
	Shell string `yaml:"shell,omitempty"`
	// Editor is a simple command name used to set EDITOR.
	Editor string `yaml:"editor,omitempty"`
	// Terminal is the preferred terminal emulator. Validated against
	// known values; informational in this milestone.
	Terminal string `yaml:"terminal,omitempty"`
	// Prompt selects a deterministic prompt style (plain or veilbox).
	Prompt string `yaml:"prompt,omitempty"`
	// Tmux enables Veilbox-managed tmux configuration.
	Tmux bool `yaml:"tmux,omitempty"`
	// Aliases is a map of alias name to simple command.
	Aliases map[string]string `yaml:"aliases,omitempty"`
	// Environment is a map of environment variable name to value.
	Environment map[string]string `yaml:"environment,omitempty"`
}

// Validate checks every preference against its declarative grammar.
func (p Preferences) Validate() error {
	if p.Shell != "" && p.Shell != ShellBash {
		return fmt.Errorf("unsupported shell %q (only %q is supported)", p.Shell, ShellBash)
	}
	if p.Editor != "" && !wordPattern.MatchString(p.Editor) {
		return fmt.Errorf("invalid editor %q (want a simple command name)", p.Editor)
	}
	if p.Terminal != "" && !validTerminal(p.Terminal) {
		return fmt.Errorf("unknown terminal %q", p.Terminal)
	}
	if p.Prompt != "" && p.Prompt != PromptPlain && p.Prompt != PromptVeilbox {
		return fmt.Errorf("unknown prompt %q (want %q or %q)", p.Prompt, PromptPlain, PromptVeilbox)
	}
	for k, v := range p.Aliases {
		if !aliasKeyPattern.MatchString(k) {
			return fmt.Errorf("invalid alias name %q", k)
		}
		if !commandPattern.MatchString(v) {
			return fmt.Errorf("invalid alias %q: %q uses shell syntax that is not allowed (simple command with spaces only)", k, v)
		}
	}
	for k, v := range p.Environment {
		if !envKeyPattern.MatchString(k) {
			return fmt.Errorf("invalid environment variable name %q", k)
		}
		if !commandPattern.MatchString(v) {
			return fmt.Errorf("invalid environment value for %q: %q uses shell syntax that is not allowed", k, v)
		}
	}
	return nil
}

func validTerminal(t string) bool {
	switch t {
	case TerminalAuto, TerminalKitty, TerminalWezterm, TerminalGhostty, TerminalAlacritty:
		return true
	}
	return false
}

// SortedAliasNames returns alias names in deterministic order.
func (p Preferences) SortedAliasNames() []string {
	out := make([]string, 0, len(p.Aliases))
	for k := range p.Aliases {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// SortedEnvNames returns environment keys in deterministic order.
func (p Preferences) SortedEnvNames() []string {
	out := make([]string, 0, len(p.Environment))
	for k := range p.Environment {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ExpandHome converts a leading ~/ into $HOME/ for shell emission.
func ExpandHome(v string) string {
	if strings.HasPrefix(v, "~/") {
		return "$HOME/" + v[2:]
	}
	if v == "~" {
		return "$HOME"
	}
	return v
}
