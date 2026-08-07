package workspace

import (
	"strings"
	"testing"
)

func contains(s, sub string) bool { return strings.Contains(s, sub) }

func validPrefs() Preferences {
	return Preferences{
		Shell:    ShellBash,
		Editor:   "vim",
		Terminal: TerminalAuto,
		Prompt:   PromptVeilbox,
		Tmux:     true,
		Aliases: map[string]string{
			"k":  "kubectl",
			"ll": "ls -la",
		},
		Environment: map[string]string{
			"KUBECONFIG": "~/.kube/config",
		},
	}
}

func TestValidateAcceptsPracticalPrefs(t *testing.T) {
	if err := validPrefs().Validate(); err != nil {
		t.Fatalf("expected valid prefs, got: %v", err)
	}
}

func TestValidateRejectsInjection(t *testing.T) {
	cases := map[string]Preferences{
		"alias with semicolon":    {Aliases: map[string]string{"a": "kubectl; rm -rf /"}},
		"alias with pipe":         {Aliases: map[string]string{"a": "ls | grep x"}},
		"alias with substitution": {Aliases: map[string]string{"a": "$(evil)"}},
		"alias with backticks":    {Aliases: map[string]string{"a": "`evil`"}},
		"alias with redirection":  {Aliases: map[string]string{"a": "ls > /tmp/x"}},
		"alias with ampersand":    {Aliases: map[string]string{"a": "cmd & cmd"}},
		"alias with parens":       {Aliases: map[string]string{"a": "(evil)"}},
		"alias name with space":   {Aliases: map[string]string{"bad name": "ls"}},
		"env with substitution":   {Environment: map[string]string{"A": "$(evil)"}},
		"env with pipe":           {Environment: map[string]string{"A": "x|y"}},
		"env name with dash":      {Environment: map[string]string{"A-B": "x"}},
		"env value with quotes":   {Environment: map[string]string{"A": `x"y`}},
		"editor with args":        {Editor: "vim --norc"},
		"editor with metachar":    {Editor: "vim;x"},
		"unsupported shell":       {Shell: "zsh"},
		"unknown prompt":          {Prompt: "custom"},
		"unknown terminal":        {Terminal: "xterm-evil"},
	}
	for name, p := range cases {
		t.Run(name, func(t *testing.T) {
			if err := p.Validate(); err == nil {
				t.Fatalf("expected validation error for %+v", p)
			}
		})
	}
}

func TestValidateAcceptsEmpty(t *testing.T) {
	var p Preferences
	if err := p.Validate(); err != nil {
		t.Fatalf("empty prefs must validate: %v", err)
	}
}

func TestSortedOrdering(t *testing.T) {
	p := Preferences{Aliases: map[string]string{"b": "x", "a": "y"}, Environment: map[string]string{"Z": "1", "A": "2"}}
	got := p.SortedAliasNames()
	if got[0] != "a" || got[1] != "b" {
		t.Fatalf("alias order: %v", got)
	}
	got = p.SortedEnvNames()
	if got[0] != "A" || got[1] != "Z" {
		t.Fatalf("env order: %v", got)
	}
}

func TestExpandHome(t *testing.T) {
	cases := map[string]string{
		"~/.kube/config": "$HOME/.kube/config",
		"~":              "$HOME",
		"/etc/hosts":     "/etc/hosts",
		"":               "",
	}
	for in, want := range cases {
		if got := ExpandHome(in); got != want {
			t.Fatalf("ExpandHome(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGenerateDeterministic(t *testing.T) {
	p := validPrefs()
	a := ShellScriptContent(p)
	b := ShellScriptContent(p)
	if a != b {
		t.Fatal("generation must be deterministic")
	}
	for _, want := range []string{"export EDITOR=\"vim\"", "alias k='kubectl'", "export KUBECONFIG=\"$HOME/.kube/config\"", "PS1="} {
		if !contains(a, want) {
			t.Fatalf("generated script missing %q:\n%s", want, a)
		}
	}
	if contains(a, ";") {
		t.Fatalf("generated script contains shell metacharacters:\n%s", a)
	}
}
