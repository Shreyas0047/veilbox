// Package tui implements the onboarding wizard as an interactive
// Bubble Tea terminal application.
//
// It is a pure presenter: the wizard (internal/onboarding) owns the
// flow, the selection, the plan and the apply; this package only
// renders choices, navigates the keyboard and returns the engineer's
// answers. No package, system or state logic lives here.
package tui

import (
	"io"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Shreyas0047/veilbox/v3/core/internal/onboarding"
)

// stepTotal is the number of numbered wizard steps (review included).
// Welcome is step 0 and the report screen is not numbered.
const stepTotal = 5

// UI implements onboarding.UI over Bubble Tea. One instance is reused
// across the whole wizard conversation; the step counter lets the
// review screen go back to the role step without restarting the
// program.
type UI struct {
	in   io.Reader
	out  io.Writer
	step int
}

// New returns a TUI wired to the process terminal.
func New() *UI {
	return &UI{in: os.Stdin, out: os.Stdout}
}

// NewWith returns a TUI over explicit streams (tests and PTY runs).
func NewWith(in io.Reader, out io.Writer) *UI {
	return &UI{in: in, out: out}
}

// run executes one interactive screen and returns the final model
// (the screen quits itself when the engineer decides).
func (u *UI) run(m tea.Model) (tea.Model, error) {
	return tea.NewProgram(m, tea.WithInput(u.in), tea.WithOutput(u.out)).Run()
}

// nextStep advances the step counter. After the review step the
// wizard restarts at the role step (go-back path), so the counter
// wraps to 1.
func (u *UI) nextStep() int {
	u.step++
	if u.step > stepTotal {
		u.step = 1
	}
	return u.step
}

// Welcome implements onboarding.UI.
func (u *UI) Welcome() error {
	final, err := u.run(newWelcomeModel())
	if err != nil {
		return err
	}
	m := final.(welcomeModel)
	if m.abort {
		return onboarding.ErrAborted
	}
	return nil
}

// SelectRole implements onboarding.UI.
func (u *UI) SelectRole(choices []onboarding.RoleChoice, current string) (string, error) {
	step := u.nextStep()
	m := newPickModel(step, "Role", len(choices))
	m.currentIdx = indexOfChoice(choices, current)
	for i, c := range choices {
		m.items[i] = pickItem{
			label:   c.DisplayName,
			detail:  c.Description,
			status:  strings.Join(c.Recommended, ", "),
			current: c.Applied,
		}
		m.values[i] = c.Name
	}
	m.cursorToCurrent()
	final, err := u.run(m)
	if err != nil {
		return "", err
	}
	m = final.(pickModel)
	if m.abort {
		return "", onboarding.ErrAborted
	}
	return m.value, nil
}

// SelectDesktop implements onboarding.UI.
func (u *UI) SelectDesktop(choices []onboarding.DesktopChoice, current string) (string, error) {
	step := u.nextStep()
	n := len(choices) + 1
	m := newPickModel(step, "Desktop", n)
	m.items[0] = pickItem{
		label:  "No desktop",
		detail: "Skip the desktop step; keep the machine headless.",
	}
	m.currentIdx = 0
	for i, c := range choices {
		it := pickItem{
			label:  c.DisplayName,
			detail: c.Description,
		}
		if c.Status != "" {
			it.status = c.Status
		}
		if c.Active {
			it.status = "active"
		}
		m.items[i+1] = it
		m.values[i+1] = c.Name
		if c.Name == current {
			m.currentIdx = i + 1
		}
	}
	m.cursorToCurrent()
	final, err := u.run(m)
	if err != nil {
		return "", err
	}
	m = final.(pickModel)
	if m.abort {
		return "", onboarding.ErrAborted
	}
	return m.value, nil
}

// SelectExperiences implements onboarding.UI.
func (u *UI) SelectExperiences(groups []onboarding.ExperienceGroup, current []string) ([]string, error) {
	step := u.nextStep()
	m := newMultiModel(step)
	m.selected = make(map[string]bool)
	for _, c := range current {
		m.selected[c] = true
	}
	for _, g := range groups {
		m.items = append(m.items, multiItem{header: g.Domain})
		for _, c := range g.Choices {
			m.items = append(m.items, multiItem{
				value:  c.Name,
				label:  c.DisplayName,
				detail: c.Description,
				status: c.Status,
				rec:    c.Recommended,
			})
		}
	}
	m.items = append(m.items, multiItem{done: true, label: "Done"})
	m.cursor = 0
	for m.cursor < len(m.items) && m.items[m.cursor].header != "" {
		m.cursor++
	}
	final, err := u.run(m)
	if err != nil {
		return nil, err
	}
	m = final.(multiModel)
	if m.abort {
		return nil, onboarding.ErrAborted
	}
	var out []string
	for _, it := range m.items {
		if it.value != "" && m.selected[it.value] {
			out = append(out, it.value)
		}
	}
	return out, nil
}

// SelectWorkspace implements onboarding.UI.
func (u *UI) SelectWorkspace(opts onboarding.WorkspaceOptions, current onboarding.WorkspacePrefs) (onboarding.WorkspacePrefs, error) {
	step := u.nextStep()
	final, err := u.run(newWSModel(step, opts, current))
	if err != nil {
		return current, err
	}
	m := final.(wsModel)
	if m.abort {
		return current, onboarding.ErrAborted
	}
	return m.prefs(), nil
}

// Review implements onboarding.UI. The decision is filled before the
// screen is shown; the screen can only confirm, restart or abort it.
func (u *UI) Review(info onboarding.ReviewInfo) (onboarding.ReviewDecision, error) {
	step := u.nextStep()
	final, err := u.run(newReviewModel(step, info))
	if err != nil {
		return onboarding.ReviewDecision{}, err
	}
	m := final.(reviewModel)
	if m.abort {
		return onboarding.ReviewDecision{}, onboarding.ErrAborted
	}
	return m.decision, nil
}

// ShowReport implements onboarding.UI.
func (u *UI) ShowReport(report string) error {
	_, err := u.run(newReportModel(report))
	return err
}

func indexOfChoice(choices []onboarding.RoleChoice, name string) int {
	for i, c := range choices {
		if c.Name == name {
			return i
		}
	}
	return -1
}

// chrome is the shared screen decoration: step indicator, title and
// the footer with the common key bindings.
type chrome struct {
	step  int
	title string
	help  string
}

func (c chrome) header() string {
	var b strings.Builder
	b.WriteString("VEILBOX ONBOARDING")
	if c.step == 0 {
		if c.title != "" {
			b.WriteString(" — ")
			b.WriteString(c.title)
		}
	} else {
		b.WriteString("   Step ")
		b.WriteString(itoa(c.step))
		b.WriteString("/")
		b.WriteString(itoa(stepTotal))
		b.WriteString(" — ")
		b.WriteString(c.title)
	}
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", 60))
	b.WriteString("\n")
	return b.String()
}

func (c chrome) footer() string {
	help := c.help
	if help == "" {
		help = "↑/↓ move · enter select"
	}
	return "\n" + help + "\n[q] quit — nothing has been changed"
}

// scroll keeps the cursor within [0, n) and derives the visible
// window for a screen of the given height.
type scroll struct {
	cursor int
	offset int
}

func (s *scroll) move(delta, n, height int) {
	if n <= 0 {
		return
	}
	s.cursor += delta
	if s.cursor < 0 {
		s.cursor = 0
	}
	if s.cursor >= n {
		s.cursor = n - 1
	}
	s.clamp(n, height)
}

func (s *scroll) clamp(n, height int) {
	if n <= 0 {
		s.cursor, s.offset = 0, 0
		return
	}
	if s.cursor < s.offset {
		s.offset = s.cursor
	}
	if s.offset+height > n {
		s.offset = n - height
	}
	if s.offset < 0 {
		s.offset = 0
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [8]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// keyRunes returns every printable rune of a key message (bubble tea
// coalesces runes that arrive in one read, so rapid typing or held keys
// can produce several runes in a single message — each must be handled).
func keyRunes(msg tea.KeyMsg) []rune {
	if msg.Type != tea.KeyRunes {
		return nil
	}
	return msg.Runes
}

// runeOf returns the single printable rune of a key message, if any.
func runeOf(msg tea.KeyMsg) (rune, bool) {
	if msg.Type != tea.KeyRunes || len(msg.Runes) != 1 {
		return 0, false
	}
	return msg.Runes[0], true
}
