package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Shreyas0047/veilbox/v3/core/internal/onboarding"
)

// welcomeModel is step 0: introduction, nothing on the machine
// changes until the review screen confirms.
type welcomeModel struct {
	abort bool
}

func newWelcomeModel() welcomeModel { return welcomeModel{} }

func (m welcomeModel) Init() tea.Cmd { return nil }

func (m welcomeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter, tea.KeySpace:
			return m, tea.Quit
		case tea.KeyCtrlC:
			m.abort = true
			return m, tea.Quit
		default:
			for _, r := range keyRunes(msg) {
				if r == 'q' || r == 'Q' {
					m.abort = true
					return m, tea.Quit
				}
			}
		}
	}
	return m, nil
}

func (m welcomeModel) View() string {
	var b strings.Builder
	b.WriteString("VEILBOX ONBOARDING\n\n")
	b.WriteString("This wizard configures your engineer role, desktop,\n")
	b.WriteString("capabilities and workspace preferences.\n\n")
	b.WriteString("Nothing on this machine changes until you confirm the\n")
	b.WriteString("plan on the review screen — and desktop activation is\n")
	b.WriteString("a separate explicit confirmation.\n\n")
	b.WriteString("Press Enter to begin.  [q] quits.")
	return b.String()
}

// pickItem is one selectable row in a single-choice screen.
type pickItem struct {
	label   string
	detail  string
	status  string
	current bool
}

// pickModel is a single-choice list (role, desktop). Enter confirms;
// the previously chosen value is marked and pre-cursorred.
type pickModel struct {
	chrome
	items      []pickItem
	values     []string
	currentIdx int
	scroll
	height int
	value  string
	abort  bool
}

func newPickModel(step int, title string, n int) pickModel {
	return pickModel{
		chrome:     chrome{step: step, title: title},
		items:      make([]pickItem, n),
		values:     make([]string, n),
		currentIdx: -1,
	}
}

// cursorToCurrent places the cursor on the previously chosen value.
func (m *pickModel) cursorToCurrent() {
	m.cursor = m.currentIdx
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m pickModel) Init() tea.Cmd { return nil }

func (m pickModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
		m.clamp(len(m.items), m.viewHeight())
		return m, nil
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyUp:
			m.move(-1, len(m.items), m.viewHeight())
			return m, nil
		case tea.KeyDown:
			m.move(1, len(m.items), m.viewHeight())
			return m, nil
		case tea.KeyEnter:
			if len(m.items) == 0 {
				return m, nil
			}
			m.value = m.values[m.cursor]
			return m, tea.Quit
		case tea.KeyCtrlC, tea.KeyEsc:
			m.abort = true
			return m, tea.Quit
		default:
			for _, r := range keyRunes(msg) {
				switch r {
				case 'q', 'Q':
					m.abort = true
					return m, tea.Quit
				case 'j':
					m.move(1, len(m.items), m.viewHeight())
				case 'k':
					m.move(-1, len(m.items), m.viewHeight())
				case 'g':
					m.cursor, m.offset = 0, 0
				case 'G':
					m.cursor = len(m.items) - 1
					m.clamp(len(m.items), m.viewHeight())
				}
			}
			return m, nil
		}
	}
	return m, nil
}

// viewHeight is the number of visible list rows.
func (m pickModel) viewHeight() int {
	h := m.height - 8
	if h < 3 {
		h = 3
	}
	return h
}

func (m pickModel) View() string {
	var b strings.Builder
	b.WriteString(m.header())
	if len(m.items) == 0 {
		b.WriteString("No choices available.\n")
		b.WriteString(m.footer())
		return b.String()
	}
	height := m.viewHeight()
	end := m.offset + height
	if end > len(m.items) {
		end = len(m.items)
	}
	for i := m.offset; i < end; i++ {
		it := m.items[i]
		marker := "  "
		if i == m.cursor {
			marker = "› "
		}
		b.WriteString(marker)
		b.WriteString(it.label)
		if it.current {
			b.WriteString("  (current)")
		}
		if it.status != "" {
			b.WriteString("  [")
			b.WriteString(it.status)
			b.WriteString("]")
		}
		b.WriteString("\n")
		if i == m.cursor && it.detail != "" {
			b.WriteString("     ")
			b.WriteString(it.detail)
			b.WriteString("\n")
		}
	}
	if len(m.items) > height {
		b.WriteString(fmt.Sprintf("     · %d of %d ·\n", m.offset+1, len(m.items)))
	}
	b.WriteString(m.footer())
	return b.String()
}

// multiItem is one row in the capability list: either a group header
// (not selectable), a toggleable capability, or the trailing Done
// item.
type multiItem struct {
	header string // group title when set
	done   bool   // trailing "Done" item
	value  string
	label  string
	detail string
	status string
	rec    bool
}

// multiModel is the capabilities screen: grouped, multi-select with
// obvious selected state.
type multiModel struct {
	chrome
	items    []multiItem
	selected map[string]bool
	scroll
	height int
	abort  bool
}

func newMultiModel(step int) multiModel {
	return multiModel{
		chrome: chrome{
			step:  step,
			title: "Capabilities",
			help:  "↑/↓ move · space toggle · enter toggle / finish · q quit",
		},
	}
}

func (m multiModel) Init() tea.Cmd { return nil }

// selectable reports whether the row at i accepts a toggle.
func (m multiModel) selectable(i int) bool {
	if i < 0 || i >= len(m.items) {
		return false
	}
	it := m.items[i]
	return it.header == "" && !it.done
}

// moveSkips advances the cursor past group headers.
func (m *multiModel) moveSkips(delta int) {
	n := len(m.items)
	if n == 0 {
		return
	}
	for i := 0; i < n; i++ {
		m.cursor += delta
		if m.cursor < 0 {
			m.cursor = 0
		}
		if m.cursor >= n {
			m.cursor = n - 1
		}
		if m.selectable(m.cursor) || m.items[m.cursor].done {
			break
		}
	}
	m.clamp(n, m.viewHeight())
}

func (m multiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
		m.clamp(len(m.items), m.viewHeight())
		return m, nil
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyUp:
			m.moveSkips(-1)
			return m, nil
		case tea.KeyDown:
			m.moveSkips(1)
			return m, nil
		case tea.KeyEnter, tea.KeySpace:
			if len(m.items) == 0 {
				return m, nil
			}
			it := m.items[m.cursor]
			switch {
			case it.done:
				return m, tea.Quit
			case it.value != "":
				m.selected[it.value] = !m.selected[it.value]
			}
			return m, nil
		case tea.KeyCtrlC, tea.KeyEsc:
			m.abort = true
			return m, tea.Quit
		default:
			for _, r := range keyRunes(msg) {
				switch r {
				case 'q', 'Q':
					m.abort = true
					return m, tea.Quit
				case 'j':
					m.moveSkips(1)
				case 'k':
					m.moveSkips(-1)
				case 'g':
					m.cursor, m.offset = 0, 0
				case 'G':
					m.cursor = len(m.items) - 1
					m.clamp(len(m.items), m.viewHeight())
				}
			}
			return m, nil
		}
	}
	return m, nil
}

func (m multiModel) viewHeight() int {
	h := m.height - 9
	if h < 3 {
		h = 3
	}
	return h
}

func (m multiModel) View() string {
	var b strings.Builder
	b.WriteString(m.header())
	b.WriteString("Selected capabilities are marked with [x]. Recommended\n")
	b.WriteString("experiences are marked [r].\n")
	height := m.viewHeight()
	end := m.offset + height
	if end > len(m.items) {
		end = len(m.items)
	}
	for i := m.offset; i < end; i++ {
		it := m.items[i]
		switch {
		case it.header != "":
			b.WriteString("\n  ")
			b.WriteString(it.header)
			b.WriteString("\n")
		case it.done:
			marker := "  "
			if i == m.cursor {
				marker = "› "
			}
			b.WriteString(marker)
			b.WriteString(it.label)
			b.WriteString("\n")
		default:
			marker := "  "
			if i == m.cursor {
				marker = "› "
			}
			sel := "  "
			if m.selected[it.value] {
				sel = "[x]"
			}
			rec := "  "
			if it.rec {
				rec = "[r]"
			}
			b.WriteString(marker)
			b.WriteString(sel)
			b.WriteString(" ")
			b.WriteString(rec)
			b.WriteString(" ")
			b.WriteString(it.label)
			if it.status != "" {
				b.WriteString("  [")
				b.WriteString(it.status)
				b.WriteString("]")
			}
			b.WriteString("\n")
			if i == m.cursor && it.detail != "" {
				b.WriteString("     ")
				b.WriteString(it.detail)
				b.WriteString("\n")
			}
		}
	}
	if len(m.items) > height {
		b.WriteString(fmt.Sprintf("     · %d of %d ·\n", m.offset+1, len(m.items)))
	}
	b.WriteString(m.footer())
	return b.String()
}

// wsField is one editable workspace preference.
type wsField struct {
	name  string
	opts  []string // enum options (first = inherit); nil = free text
	value string   // raw value; "" means inherit from the profile
	cycle int      // current index into opts for enum fields
}

// wsModel is the workspace step: a small form of four preferences
// plus a Continue row. Enum fields cycle through their options on
// Enter (first option is "inherit"); the editor is free text with
// inline input.
type wsModel struct {
	chrome
	fields  []wsField
	cursor  int
	editing bool
	buffer  string
	height  int
	abort   bool
}

// wsRows is the number of cursor positions (4 fields + Continue).
const wsRows = 5

func newWSModel(step int, opts onboarding.WorkspaceOptions, current onboarding.WorkspacePrefs) wsModel {
	m := wsModel{
		chrome: chrome{
			step:  step,
			title: "Workspace",
			help:  "↑/↓ or j/k field · enter edit/cycle · q quit",
		},
	}
	m.fields = append(m.fields, wsField{
		name:  "Editor",
		value: current.Editor,
	})
	m.fields = append(m.fields, wsField{
		name:  "Prompt",
		opts:  append([]string{""}, opts.Prompts...),
		value: current.Prompt,
	})
	m.fields = append(m.fields, wsField{
		name:  "Terminal",
		opts:  append([]string{""}, opts.Terminals...),
		value: current.Terminal,
	})
	tmux := ""
	if current.Tmux != nil {
		if *current.Tmux {
			tmux = "on"
		} else {
			tmux = "off"
		}
	}
	m.fields = append(m.fields, wsField{
		name:  "Tmux",
		opts:  []string{"", "on", "off"},
		value: tmux,
	})
	for i := range m.fields {
		if m.fields[i].opts != nil {
			for j, o := range m.fields[i].opts {
				if o == m.fields[i].value {
					m.fields[i].cycle = j
				}
			}
		}
	}
	return m
}

func (m wsModel) Init() tea.Cmd { return nil }

func (m wsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		if m.editing {
			nm := m.updateEditing(msg)
			return *nm, nil
		}
		switch msg.Type {
		case tea.KeyUp:
			m.cursor--
			if m.cursor < 0 {
				m.cursor = wsRows - 1
			}
			return m, nil
		case tea.KeyDown:
			m.cursor++
			if m.cursor >= wsRows {
				m.cursor = 0
			}
			return m, nil
		case tea.KeyEnter:
			if m.cursor == wsRows-1 {
				return m, tea.Quit
			}
			f := &m.fields[m.cursor]
			if f.opts == nil {
				m.editing = true
				m.buffer = f.value
			} else {
				f.cycle = (f.cycle + 1) % len(f.opts)
				f.value = f.opts[f.cycle]
			}
			return m, nil
		case tea.KeyCtrlC, tea.KeyEsc:
			m.abort = true
			return m, tea.Quit
		default:
			for _, r := range keyRunes(msg) {
				switch r {
				case 'q', 'Q':
					m.abort = true
					return m, tea.Quit
				case 'j':
					m.cursor++
					if m.cursor >= wsRows {
						m.cursor = 0
					}
				case 'k':
					m.cursor--
					if m.cursor < 0 {
						m.cursor = wsRows - 1
					}
				}
			}
			return m, nil
		}
	}
	return m, nil
}

func (m *wsModel) updateEditing(msg tea.KeyMsg) *wsModel {
	f := &m.fields[m.cursor]
	switch msg.Type {
	case tea.KeyEnter:
		f.value = m.buffer
		m.editing = false
	case tea.KeyEsc:
		m.editing = false
	case tea.KeyBackspace:
		if len(m.buffer) > 0 {
			m.buffer = m.buffer[:len(m.buffer)-1]
		}
	case tea.KeyCtrlC:
		m.abort = true
		m.editing = false
	case tea.KeyRunes:
		m.buffer += string(msg.Runes)
	}
	return m
}

// prefs builds the chosen workspace preferences (empty = inherit).
func (m wsModel) prefs() onboarding.WorkspacePrefs {
	p := onboarding.WorkspacePrefs{
		Editor:   m.fields[0].value,
		Prompt:   m.fields[1].value,
		Terminal: m.fields[2].value,
	}
	switch m.fields[3].value {
	case "on":
		t := true
		p.Tmux = &t
	case "off":
		f := false
		p.Tmux = &f
	}
	return p
}

func (m wsModel) View() string {
	var b strings.Builder
	b.WriteString(m.header())
	b.WriteString("Empty values inherit from the profile's preferences.\n\n")
	for i, f := range m.fields {
		marker := "  "
		if i == m.cursor {
			marker = "› "
		}
		b.WriteString(marker)
		b.WriteString(f.name)
		b.WriteString(": ")
		if m.editing && i == m.cursor {
			b.WriteString("[edit] ")
			b.WriteString(m.buffer)
			b.WriteString("_")
		} else if f.value == "" {
			b.WriteString("(inherit)")
		} else {
			b.WriteString(f.value)
		}
		b.WriteString("\n")
	}
	marker := "  "
	if m.cursor == wsRows-1 {
		marker = "› "
	}
	b.WriteString(marker)
	b.WriteString("Continue\n")
	b.WriteString(m.footer())
	return b.String()
}

// reviewModel is the review screen: the complete plan (scrollable),
// the desktop-activation confirmation and the Apply / Restart /
// Abort actions.
type reviewModel struct {
	chrome
	info       onboarding.ReviewInfo
	hasDesktop bool
	activate   bool
	action     int // 0 apply, 1 restart, 2 abort
	scroll
	height   int
	decision onboarding.ReviewDecision
	abort    bool
}

func newReviewModel(step int, info onboarding.ReviewInfo) reviewModel {
	return reviewModel{
		chrome: chrome{
			step:  step,
			title: "Review",
			help:  "↑/↓ scroll · ←/→ action · a activation · enter confirm · b restart · q quit",
		},
		info:       info,
		hasDesktop: info.Selection.Desktop != "",
		activate:   info.Selection.Desktop != "",
	}
}

func (m reviewModel) Init() tea.Cmd { return nil }

func (m reviewModel) planLines() []string {
	return strings.Split(m.info.Text, "\n")
}

func (m reviewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
		m.clamp(len(m.planLines()), m.viewHeight())
		return m, nil
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyUp:
			m.move(-1, len(m.planLines()), m.viewHeight())
			return m, nil
		case tea.KeyDown:
			m.move(1, len(m.planLines()), m.viewHeight())
			return m, nil
		case tea.KeyLeft:
			if m.hasDesktop && m.action == 0 {
				// The activation line sits before the actions.
				m.action = -1
			} else {
				m.action--
				if m.action < 0 {
					m.action = 2
				}
			}
			return m, nil
		case tea.KeyRight:
			if m.action == -1 {
				m.action = 0
			} else {
				m.action++
				if m.action > 2 {
					m.action = 0
				}
			}
			return m, nil
		case tea.KeyEnter:
			switch m.action {
			case 0:
				m.decision = onboarding.ReviewDecision{
					Apply:           true,
					ActivateDesktop: m.activate,
				}
			case 1:
				m.decision = onboarding.ReviewDecision{Restart: true}
			default:
				m.abort = true
			}
			return m, tea.Quit
		case tea.KeyCtrlC:
			m.abort = true
			return m, tea.Quit
		default:
			for _, r := range keyRunes(msg) {
				switch r {
				case 'q', 'Q':
					m.abort = true
					return m, tea.Quit
				case 'b', 'B':
					m.decision = onboarding.ReviewDecision{Restart: true}
					return m, tea.Quit
				case 'a', 'A':
					if m.hasDesktop {
						m.activate = !m.activate
					}
				case 'j':
					m.move(1, len(m.planLines()), m.viewHeight())
				case 'k':
					m.move(-1, len(m.planLines()), m.viewHeight())
				}
			}
			return m, nil
		}
	}
	return m, nil
}

func (m reviewModel) viewHeight() int {
	h := m.height - 9
	if h < 3 {
		h = 3
	}
	return h
}

func (m reviewModel) View() string {
	var b strings.Builder
	b.WriteString(m.header())
	if m.info.Existing {
		b.WriteString("Continuing your existing selection.\n")
	}
	lines := m.planLines()
	height := m.viewHeight()
	end := m.offset + height
	if end > len(lines) {
		end = len(lines)
	}
	for i := m.offset; i < end; i++ {
		b.WriteString(lines[i])
		b.WriteString("\n")
	}
	if len(lines) > height {
		b.WriteString(fmt.Sprintf("     · %d of %d ·\n", m.offset+1, len(lines)))
	}
	b.WriteString("\n")
	if m.hasDesktop {
		state := "yes"
		if !m.activate {
			state = "no"
		}
		b.WriteString("Activate ")
		b.WriteString(m.info.Selection.Desktop)
		b.WriteString(" now: ")
		b.WriteString(state)
		b.WriteString("  [a toggles — activation installs and enables the desktop]\n")
	}
	names := []string{"Apply", "Restart", "Abort"}
	for i, n := range names {
		if i == m.action {
			b.WriteString("[")
			b.WriteString(n)
			b.WriteString("]")
		} else {
			b.WriteString(" ")
			b.WriteString(n)
			b.WriteString(" ")
		}
	}
	b.WriteString(m.footer())
	return b.String()
}

// reportModel is the final screen: the apply report (scrollable).
type reportModel struct {
	chrome
	text string
	scroll
	height int
}

func newReportModel(text string) reportModel {
	return reportModel{
		chrome: chrome{title: "Result", help: "↑/↓ scroll · enter or q to exit"},
		text:   text,
	}
}

func (m reportModel) Init() tea.Cmd { return nil }

func (m reportModel) lines() []string {
	return strings.Split(strings.TrimRight(m.text, "\n"), "\n")
}

func (m reportModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
		m.clamp(len(m.lines()), m.viewHeight())
		return m, nil
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyUp:
			m.move(-1, len(m.lines()), m.viewHeight())
			return m, nil
		case tea.KeyDown:
			m.move(1, len(m.lines()), m.viewHeight())
			return m, nil
		case tea.KeyEnter, tea.KeyEsc:
			return m, tea.Quit
		case tea.KeyCtrlC:
			return m, tea.Quit
		default:
			for _, r := range keyRunes(msg) {
				switch r {
				case 'q', 'Q':
					return m, tea.Quit
				case 'j':
					m.move(1, len(m.lines()), m.viewHeight())
				case 'k':
					m.move(-1, len(m.lines()), m.viewHeight())
				}
			}
			return m, nil
		}
	}
	return m, nil
}

func (m reportModel) viewHeight() int {
	h := m.height - 4
	if h < 3 {
		h = 3
	}
	return h
}

func (m reportModel) View() string {
	var b strings.Builder
	b.WriteString(m.header())
	lines := m.lines()
	height := m.viewHeight()
	end := m.offset + height
	if end > len(lines) {
		end = len(lines)
	}
	for i := m.offset; i < end; i++ {
		b.WriteString(lines[i])
		b.WriteString("\n")
	}
	if len(lines) > height {
		b.WriteString(fmt.Sprintf("     · %d of %d ·\n", m.offset+1, len(lines)))
	}
	b.WriteString(m.footer())
	return b.String()
}
