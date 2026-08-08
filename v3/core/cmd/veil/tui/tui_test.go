package tui

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Shreyas0047/veilbox/v3/core/internal/onboarding"
)

func key(t tea.KeyType, runes ...rune) tea.Msg {
	return tea.KeyMsg{Type: t, Runes: runes}
}

func enter() tea.Msg { return key(tea.KeyEnter) }
func up() tea.Msg    { return key(tea.KeyUp) }
func down() tea.Msg  { return key(tea.KeyDown) }
func runeMsg(r rune) tea.Msg {
	return key(tea.KeyRunes, r)
}

func apply(m tea.Model, msgs ...tea.Msg) tea.Model {
	for _, msg := range msgs {
		m, _ = m.Update(msg)
	}
	return m
}

func TestPickModelSelectsAndShowsStep(t *testing.T) {
	m := newPickModel(1, "Role", 2)
	m.items[0] = pickItem{label: "Alpha", detail: "first"}
	m.items[1] = pickItem{label: "Beta", detail: "second"}
	m.values[0], m.values[1] = "alpha", "beta"

	view := m.View()
	for _, want := range []string{"Step 1/5", "Role", "Alpha", "Beta"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q in:\n%s", want, view)
		}
	}

	m = apply(m, down(), enter()).(pickModel)
	if m.value != "beta" {
		t.Fatalf("value = %q, want beta", m.value)
	}
}

func TestPickModelKeepsCurrentCursor(t *testing.T) {
	m := newPickModel(2, "Desktop", 3)
	m.items[0] = pickItem{label: "None"}
	m.items[1] = pickItem{label: "Niri"}
	m.items[2] = pickItem{label: "Other"}
	m.values[0], m.values[1], m.values[2] = "", "niri", "other"
	m.currentIdx = 1
	m.cursorToCurrent()
	m = apply(m, enter()).(pickModel)
	if m.value != "niri" {
		t.Fatalf("value = %q, want niri (cursor must land on current)", m.value)
	}
}

func TestPickModelAbortKeys(t *testing.T) {
	m := newPickModel(1, "Role", 1)
	m = apply(m, runeMsg('q')).(pickModel)
	if !m.abort {
		t.Fatal("q must abort")
	}
}

func TestMultiModelTogglesAndFinishes(t *testing.T) {
	m := newMultiModel(3)
	m.selected = map[string]bool{}
	m.items = append(m.items, multiItem{header: "Networking"})
	m.items = append(m.items, multiItem{value: "a", label: "A tools"})
	m.items = append(m.items, multiItem{value: "b", label: "B tools"})
	m.items = append(m.items, multiItem{done: true, label: "Done"})

	view := m.View()
	if !strings.Contains(view, "[x]") || !strings.Contains(view, "[r]") || !strings.Contains(view, "Networking") {
		t.Fatalf("capability chrome missing:\n%s", view)
	}

	// Toggle A on, move to B, toggle B on, finish.
	m = apply(m, key(tea.KeySpace), down(), key(tea.KeySpace), down(), enter()).(multiModel)
	if !m.selected["a"] || !m.selected["b"] {
		t.Fatalf("selections = %v", m.selected)
	}

	// The selected state must be obvious in the view: the two item
	// lines carry [x] (the help line mentions it, so count item
	// markers by non-help lines).
	itemMarks := 0
	for _, line := range strings.Split(m.View(), "\n") {
		if strings.Contains(line, "[x]") && !strings.Contains(line, "marked") {
			itemMarks++
		}
	}
	if itemMarks != 2 {
		t.Fatalf("selected state not shown for both items:\n%s", m.View())
	}
}

func TestMultiModelSkipsHeaders(t *testing.T) {
	m := newMultiModel(3)
	m.selected = map[string]bool{}
	m.items = append(m.items, multiItem{header: "First"})
	m.items = append(m.items, multiItem{value: "x", label: "X"})
	m.items = append(m.items, multiItem{done: true, label: "Done"})
	m.cursor = 0
	for m.cursor < len(m.items) && m.items[m.cursor].header != "" {
		m.cursor++
	}
	m = apply(m, down()).(multiModel)
	if !m.items[m.cursor].done {
		t.Fatalf("down from the last capability must land on Done, got item %d", m.cursor)
	}
}

func TestMultiModelHandlesCoalescedRunes(t *testing.T) {
	// Rapid typing or a held key arrives as one KeyRunes message with
	// several runes; every rune must be applied, not just the first.
	m := newMultiModel(3)
	m.selected = map[string]bool{}
	m.items = append(m.items, multiItem{header: "Base"})
	m.items = append(m.items, multiItem{header: "Networking"})
	m.items = append(m.items, multiItem{value: "a", label: "A tools"})
	m.items = append(m.items, multiItem{value: "b", label: "B tools"})
	m.items = append(m.items, multiItem{value: "c", label: "C tools"})
	m.items = append(m.items, multiItem{done: true, label: "Done"})
	m.cursor = 2

	// Two j's in one message: the cursor must skip the Networking
	// header and stop on c tools (all runes are applied).
	m = apply(m, key(tea.KeyRunes, 'j', 'j')).(multiModel)
	if m.items[m.cursor].value != "c" {
		t.Fatalf("two coalesced j's must move to c tools, cursor at %d (%q)", m.cursor, m.items[m.cursor].label)
	}

	// A third coalesced j lands on Done.
	m = apply(m, key(tea.KeyRunes, 'j')).(multiModel)
	if !m.items[m.cursor].done {
		t.Fatalf("a third j must land on Done, got %q", m.items[m.cursor].label)
	}

	// Enter on Done in the same message must finish the step.
	m = apply(m, key(tea.KeyRunes, 'q')).(multiModel)
	if !m.abort {
		t.Fatalf("a coalesced q must abort the step")
	}
}

func TestWSModelCycleAndEdit(t *testing.T) {
	m := newWSModel(4, onboarding.WorkspaceOptions{
		Prompts:   []string{"plain", "veilbox"},
		Editors:   []string{"vim", "vi", "nano", "emacs"},
		Terminals: []string{"system", "kitty"},
	}, onboarding.WorkspacePrefs{})

	// Editor: enter (edit), type vim, enter (commit).
	m = apply(m, enter()).(wsModel)
	if !m.editing {
		t.Fatal("enter on editor must start editing")
	}
	m = apply(m, runeMsg('v'), runeMsg('i'), runeMsg('m'), enter()).(wsModel)
	if m.editing || m.fields[0].value != "vim" {
		t.Fatalf("editor = %q editing=%t", m.fields[0].value, m.editing)
	}

	// Prompt: cycle once ("" -> plain), twice -> veilbox.
	m = apply(m, down(), enter()).(wsModel)
	if m.fields[1].value != "plain" {
		t.Fatalf("prompt = %q", m.fields[1].value)
	}
	m = apply(m, enter()).(wsModel)
	if m.fields[1].value != "veilbox" {
		t.Fatalf("prompt = %q", m.fields[1].value)
	}

	// Terminal: one cycle -> system.
	m = apply(m, down(), enter()).(wsModel)
	if m.fields[2].value != "system" {
		t.Fatalf("terminal = %q", m.fields[2].value)
	}

	// Tmux: cycle once -> on.
	m = apply(m, down(), enter()).(wsModel)
	if m.fields[3].value != "on" {
		t.Fatalf("tmux = %q", m.fields[3].value)
	}

	prefs := m.prefs()
	if prefs.Editor != "vim" || prefs.Prompt != "veilbox" || prefs.Terminal != "system" || prefs.Tmux == nil || !*prefs.Tmux {
		t.Fatalf("prefs = %+v", prefs)
	}

	// Continue row finishes the step.
	m = apply(m, down(), enter()).(wsModel)
	_ = m
}

func TestReviewModelDecisionAndActivation(t *testing.T) {
	info := onboarding.ReviewInfo{
		Selection: onboarding.Selection{Profile: "cloud", Desktop: "niri-desktop"},
		Text:      "PLAN: profile cloud\ndesktop niri-desktop\n",
	}
	m := newReviewModel(5, info)

	view := m.View()
	for _, want := range []string{"Step 5/5", "PLAN: profile cloud", "Activate niri-desktop now: yes", "Apply", "Restart", "Abort"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q in:\n%s", want, view)
		}
	}

	// Decline activation, then apply.
	m = apply(m, runeMsg('a'), enter()).(reviewModel)
	if !m.decision.Apply || m.decision.ActivateDesktop {
		t.Fatalf("decision = %+v, want apply without activation", m.decision)
	}
	if !strings.Contains(m.View(), "Activate niri-desktop now: no") {
		t.Fatalf("activation toggle state must be visible:\n%s", m.View())
	}
}

func TestReviewModelRestartAndAbort(t *testing.T) {
	m := newReviewModel(5, onboarding.ReviewInfo{Text: "plan"})
	m = apply(m, runeMsg('b')).(reviewModel)
	if !m.decision.Restart {
		t.Fatalf("b must restart, got %+v", m.decision)
	}

	m = newReviewModel(5, onboarding.ReviewInfo{Text: "plan"})
	m = apply(m, runeMsg('q')).(reviewModel)
	if !m.abort {
		t.Fatal("q must abort")
	}

	m = newReviewModel(5, onboarding.ReviewInfo{Text: "plan"})
	m = apply(m, key(tea.KeyLeft), key(tea.KeyLeft), enter()).(reviewModel)
	if !m.decision.Restart {
		t.Fatalf("left,left,enter must select Restart, got %+v", m.decision)
	}
	if m.abort {
		t.Fatal("restart must not abort")
	}
}

func TestReviewModelActionNavigation(t *testing.T) {
	m := newReviewModel(5, onboarding.ReviewInfo{Text: "plan"})
	m = apply(m, key(tea.KeyLeft), key(tea.KeyRight)).(reviewModel)
	if m.action != 0 {
		t.Fatalf("action = %d, want 0", m.action)
	}
}

func TestReportModelShowsReport(t *testing.T) {
	m := newReportModel("APPLY RESULT\n[x] profile ok\n")
	view := m.View()
	for _, want := range []string{"Result", "APPLY RESULT"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q in:\n%s", want, view)
		}
	}
	m = apply(m, enter()).(reportModel)
	_ = m
}

func TestWelcomeModel(t *testing.T) {
	m := newWelcomeModel()
	if !strings.Contains(m.View(), "VEILBOX ONBOARDING") {
		t.Fatalf("welcome view:\n%s", m.View())
	}
	m = apply(m, enter()).(welcomeModel)
	if m.abort {
		t.Fatal("enter must begin")
	}
	m = newWelcomeModel()
	m = apply(m, runeMsg('q')).(welcomeModel)
	if !m.abort {
		t.Fatal("q must abort")
	}
}

// TestSelectRoleViaReader drives a real UI session over a scripted
// (non-TTY) key stream, the same path the TUI takes in production.
func TestSelectRoleViaReader(t *testing.T) {
	var out bytes.Buffer
	ui := NewWith(strings.NewReader("\x1b[B\r"), &out)
	got, err := ui.SelectRole([]onboarding.RoleChoice{
		{Name: "a", DisplayName: "Alpha"},
		{Name: "b", DisplayName: "Beta"},
	}, "")
	if err != nil {
		t.Fatalf("select role: %v", err)
	}
	if got != "b" {
		t.Fatalf("role = %q, want b", got)
	}
	if !strings.Contains(out.String(), "Step 1/5") {
		t.Fatalf("step indicator missing:\n%s", out.String())
	}
}

func TestSelectRoleAbortViaReader(t *testing.T) {
	var out bytes.Buffer
	ui := NewWith(strings.NewReader("q"), &out)
	_, err := ui.SelectRole([]onboarding.RoleChoice{{Name: "a", DisplayName: "Alpha"}}, "")
	if !errors.Is(err, onboarding.ErrAborted) {
		t.Fatalf("err = %v, want ErrAborted", err)
	}
}

func TestSelectDesktopNoneViaReader(t *testing.T) {
	var out bytes.Buffer
	ui := NewWith(strings.NewReader("\r"), &out)
	got, err := ui.SelectDesktop([]onboarding.DesktopChoice{{Name: "niri", DisplayName: "Niri"}}, "")
	if err != nil {
		t.Fatalf("select desktop: %v", err)
	}
	if got != "" {
		t.Fatalf("desktop = %q, want none", got)
	}
}
