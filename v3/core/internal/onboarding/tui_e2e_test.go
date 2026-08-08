package onboarding_test

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Shreyas0047/veilbox/v3/core/cmd/veil/tui"
	"github.com/Shreyas0047/veilbox/v3/core/internal/capability"
	"github.com/Shreyas0047/veilbox/v3/core/internal/desktop"
	"github.com/Shreyas0047/veilbox/v3/core/internal/dnfops"
	"github.com/Shreyas0047/veilbox/v3/core/internal/experience"
	"github.com/Shreyas0047/veilbox/v3/core/internal/onboarding"
	"github.com/Shreyas0047/veilbox/v3/core/internal/onboarding/onboardingtest"
	"github.com/Shreyas0047/veilbox/v3/core/internal/profile"
	"github.com/Shreyas0047/veilbox/v3/core/internal/workspace"
)

// setupE2E builds the wizard's PlanInputs over the shared fake
// environment, returning the wiring plus the scriptable runner.
func setupE2E(t *testing.T) (onboarding.PlanInputs, *onboardingtest.Runner) {
	t.Helper()
	env := onboardingtest.SetupEnv(t)
	f := onboardingtest.New()
	dnf := dnfops.NewWithRunner(f)
	in := onboarding.PlanInputs{
		Registry:     profile.NewRegistryDir(env.ProfDir),
		Catalog:      experience.NewCatalogWith(env.CatDir, dnf),
		Capabilities: capability.NewRegistryDir(env.CapDir),
		Workspace:    &workspace.Engine{LookPath: func(string) (string, error) { return "/usr/bin/vim", nil }},
		Desktop:      desktop.NewWith(experience.NewCatalogWith(env.CatDir, dnf), desktop.NewSystemWith(f)),
	}
	return in, f
}

// syncBuffer is a goroutine-safe writer used as the TUI's output so
// the driver can watch the rendered screens while the wizard runs.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// driver drives a wizard conversation over a pipe input: keys are
// released when the screen that consumes them has rendered.
type driver struct {
	t   *testing.T
	out *syncBuffer
	pw  *os.File
}

func newDriver(t *testing.T) (*driver, *tui.UI) {
	t.Helper()
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	out := &syncBuffer{}
	d := &driver{t: t, out: out, pw: pw}
	t.Cleanup(func() { pw.Close(); pr.Close() })
	return d, tui.NewWith(pr, out)
}

func (d *driver) press(keys string) {
	if _, err := d.pw.WriteString(keys); err != nil {
		d.t.Fatal(err)
	}
}

func (d *driver) waitFor(marker string, occ int) {
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Count(d.out.String(), marker) >= occ {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	d.t.Fatalf("timeout waiting for %q (occurrence %d) in output:\n%s", marker, occ, d.out.String())
}

// runResult carries the wizard outcome out of the driver goroutine.
type runResult struct {
	sel *onboarding.Selection
	res *onboarding.ApplyResult
	err error
}

// run starts the wizard in a goroutine and returns its result channel.
func (d *driver) run(w *onboarding.Wizard) chan runResult {
	done := make(chan runResult, 1)
	go func() {
		sel, res, err := w.Run()
		done <- runResult{sel: sel, res: res, err: err}
	}()
	return done
}

// fullFlow presses the scripted keys for a complete successful run.
func (d *driver) fullFlow() {
	d.press("\r") // welcome: begin
	d.waitFor("Step 1/5 — Role", 1)
	d.press("\r") // role: cloud-engineer
	d.waitFor("Step 2/5 — Desktop", 1)
	d.press("\x1b[B\r") // desktop: niri-desktop
	d.waitFor("Step 3/5 — Capabilities", 1)
	d.press("jj\r") // capabilities: both seeded rows selected, Done
	d.waitFor("Step 4/5 — Workspace", 1)
	d.press("\rvim\rj\r\rj\rj\rj\r") // workspace: vim, veilbox, system, tmux on
	d.waitFor("Step 5/5 — Review", 1)
	d.press("\r") // review: Apply (activation confirmed by default)
	d.waitFor("APPLY RESULT", 1)
	d.press("\r") // report: exit
}

// TestTUIFullWizardRun drives the complete onboarding flow through
// the real wizard and the Bubble Tea UI over a scripted key stream.
func TestTUIFullWizardRun(t *testing.T) {
	in, f := setupE2E(t)
	f.Responses[f.Key("systemctl", "get-default")] = "multi-user.target"

	d, ui := newDriver(t)
	w, err := onboarding.LoadWizard(ui, in)
	if err != nil {
		t.Fatalf("load wizard: %v", err)
	}
	done := d.run(w)
	d.fullFlow()

	var res runResult
	select {
	case res = <-done:
		if res.err != nil {
			t.Fatalf("run: %v", res.err)
		}
	case <-time.After(15 * time.Second):
		t.Fatalf("wizard did not finish; output so far:\n%s", d.out.String())
	}

	if res.res == nil || !res.res.Success {
		t.Fatalf("apply not successful: %+v", res.res)
	}
	if res.sel == nil {
		t.Fatalf("no selection returned")
	}
	sel := *res.sel
	if sel.Profile != "cloud-engineer" || sel.Desktop != "niri-desktop" {
		t.Fatalf("selection wrong: %+v", sel)
	}
	if len(sel.Experiences) != 1 || sel.Experiences[0] != "networking-tools" {
		t.Fatalf("experiences = %v\n%s", sel.Experiences, d.out.String())
	}
	if sel.Workspace.Editor != "vim" || sel.Workspace.Prompt != "veilbox" ||
		sel.Workspace.Terminal != "system" || sel.Workspace.Tmux == nil || !*sel.Workspace.Tmux {
		t.Fatalf("workspace prefs = %+v", sel.Workspace)
	}

	view := d.out.String()
	for _, want := range []string{
		"Step 1/5 — Role",
		"Step 2/5 — Desktop",
		"Step 3/5 — Capabilities",
		"Step 4/5 — Workspace",
		"Step 5/5 — Review",
		"Activate niri-desktop now: yes",
		"APPLY RESULT",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q in TUI output:\n%s", want, view)
		}
	}

	joined := strings.Join(f.Joined(), "\n")
	for _, want := range []string{
		"interactive:sudo dnf install -y veilbox-experience-networking-tools",
		"interactive:sudo systemctl enable sddm",
		"interactive:sudo systemctl set-default graphical.target",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in executed commands:\n%s", want, joined)
		}
	}
}

// TestTUIAbortAtReview verifies abort leaves the system untouched.
func TestTUIAbortAtReview(t *testing.T) {
	in, f := setupE2E(t)
	d, ui := newDriver(t)
	w, err := onboarding.LoadWizard(ui, in)
	if err != nil {
		t.Fatalf("load wizard: %v", err)
	}
	done := d.run(w)

	d.press("\r") // welcome
	d.waitFor("Step 1/5 — Role", 1)
	d.press("\r")
	d.waitFor("Step 2/5 — Desktop", 1)
	d.press("\x1b[B\r")
	d.waitFor("Step 3/5 — Capabilities", 1)
	d.press("jj\r")
	d.waitFor("Step 4/5 — Workspace", 1)
	d.press("\rvim\rj\r\rj\rj\rj\r")
	d.waitFor("Step 5/5 — Review", 1)
	d.press("q") // review: abort

	select {
	case res := <-done:
		if !errors.Is(res.err, onboarding.ErrAborted) {
			t.Fatalf("err = %v, want ErrAborted", res.err)
		}
	case <-time.After(15 * time.Second):
		t.Fatalf("wizard did not abort; output so far:\n%s", d.out.String())
	}
	for _, cmd := range f.Joined() {
		if strings.HasPrefix(cmd, "interactive:sudo") {
			t.Fatalf("abort must not change the machine, ran %s", cmd)
		}
	}
}

// TestTUIRestartRevisitsSteps verifies the review's restart action
// re-asks the role step (step indicator back to 1).
func TestTUIRestartRevisitsSteps(t *testing.T) {
	in, f := setupE2E(t)
	d, ui := newDriver(t)
	w, err := onboarding.LoadWizard(ui, in)
	if err != nil {
		t.Fatalf("load wizard: %v", err)
	}
	done := d.run(w)

	d.press("\r") // welcome
	d.waitFor("Step 1/5 — Role", 1)
	d.press("\r")
	d.waitFor("Step 2/5 — Desktop", 1)
	d.press("\x1b[B\r")
	d.waitFor("Step 3/5 — Capabilities", 1)
	d.press("jj\r")
	d.waitFor("Step 4/5 — Workspace", 1)
	d.press("\rvim\rj\r\rj\rj\rj\r")
	d.waitFor("Step 5/5 — Review", 1)
	d.press("b") // review: restart
	d.waitFor("Step 1/5 — Role", 2)
	d.press("q") // role: abort

	select {
	case res := <-done:
		if !errors.Is(res.err, onboarding.ErrAborted) {
			t.Fatalf("err = %v, want ErrAborted after restart+abort", res.err)
		}
	case <-time.After(15 * time.Second):
		t.Fatalf("wizard did not finish after restart; output so far:\n%s", d.out.String())
	}
	if strings.Count(d.out.String(), "Step 1/5") != 2 {
		t.Fatalf("restart must re-ask the role step, output:\n%s", d.out.String())
	}
	for _, cmd := range f.Joined() {
		if strings.HasPrefix(cmd, "interactive:sudo") {
			t.Fatalf("abort must not change the machine, ran %s", cmd)
		}
	}
}

// TestTUIApplyErrorReturnsToUI verifies a failed apply returns to the
// UI with the report and recovery guidance.
func TestTUIApplyErrorReturnsToUI(t *testing.T) {
	in, f := setupE2E(t)
	f.Fail("sudo", "dnf", "install", "veilbox-experience-networking-tools")

	d, ui := newDriver(t)
	w, err := onboarding.LoadWizard(ui, in)
	if err != nil {
		t.Fatalf("load wizard: %v", err)
	}
	done := d.run(w)
	d.fullFlow()

	var outRes *onboarding.ApplyResult
	select {
	case res := <-done:
		if res.err == nil {
			t.Fatal("expected apply error")
		}
		outRes = res.res
	case <-time.After(15 * time.Second):
		t.Fatalf("wizard did not finish; output so far:\n%s", d.out.String())
	}
	if outRes == nil || outRes.Success {
		t.Fatalf("res = %+v, want failed result", outRes)
	}
	for _, want := range []string{"APPLY RESULT", "FAILED", "experiences"} {
		if !strings.Contains(d.out.String(), want) {
			t.Fatalf("missing %q in report:\n%s", want, d.out.String())
		}
	}
}
