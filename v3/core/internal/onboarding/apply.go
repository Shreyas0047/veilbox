package onboarding

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/Shreyas0047/veilbox/v3/core/internal/experience"
	"github.com/Shreyas0047/veilbox/v3/core/internal/profile"
)

// ApplyResult reports the executed stages of one apply.
type ApplyResult struct {
	Stages  []StageResult
	Log     *ApplyLog
	Success bool
}

// Apply executes the selection through the engines in fixed order:
// profile → experiences → workspace → desktop. The onboarding layer
// owns sequencing only; every engine owns its own implementation.
//
// The apply is deliberately not transactional: each stage that
// completed stays completed (engines are idempotent, so rerunning the
// wizard resumes safely). On failure the apply stops, records exactly
// what succeeded and what failed, and returns the ledgers for the
// caller to report with recovery guidance.
func Apply(sel Selection, in PlanInputs) (*ApplyResult, error) {
	// Normalize to the capability model first: a capability selection
	// has no stored experience list yet (or carries a stale one), so
	// derive the experiences now. After this, the whole apply, verify
	// and persist pipeline runs on the derived list (ADR-0011).
	if err := sel.Derive(in.Resolver()); err != nil {
		return nil, err
	}
	plan, err := BuildPlan(sel, in)
	if err != nil {
		return nil, err
	}
	if len(plan.Problems) > 0 {
		return nil, fmt.Errorf("selection has problems; nothing was changed:\n  - %s",
			strings.Join(plan.Problems, "\n  - "))
	}

	log := &ApplyLog{
		StartedAt: time.Now().UTC().Format(time.RFC3339),
		Stages:    []StageResult{},
	}
	sel.LastApply = log
	res := &ApplyResult{Log: log}

	record := func(name, status, detail string) {
		stage := StageResult{Name: name, Status: status, Detail: detail}
		log.Stages = append(log.Stages, stage)
		res.Stages = append(res.Stages, stage)
	}

	// Stage 1: profile (intent persists; always applied, idempotent).
	if _, err := profile.ApplyWith(in.Registry, sel.Profile); err != nil {
		return failApply(sel, log, res, record, "profile", err)
	}
	record("profile", StageOK, sel.Profile)

	// Stage 2: capability experiences (never removes anything).
	entries, err := in.Catalog.List()
	if err != nil {
		return failApply(sel, log, res, record, "experiences", err)
	}
	installedSet := make(map[string]bool)
	for _, e := range entries {
		if e.Status == experience.StatusInstalled {
			installedSet[e.Name] = true
		}
	}
	names := append([]string{}, sel.Experiences...)
	sort.Strings(names)
	for _, name := range names {
		if installedSet[name] {
			record("experiences", StageSkip, name+" already installed")
			continue
		}
		if err := in.Catalog.Install(name); err != nil {
			return failApply(sel, log, res, record, "experiences", fmt.Errorf("install %s: %w", name, err))
		}
		record("experiences", StageOK, name)
	}

	// Stage 3: workspace preferences (merged over the profile baseline).
	if sel.Profile != "" {
		m, err := in.Registry.Load(sel.Profile)
		if err != nil {
			return failApply(sel, log, res, record, "workspace", err)
		}
		prefs := MergeWorkspace(m.Workspace, sel.Workspace)
		if _, err := in.Workspace.Apply(prefs, sel.Profile, false); err != nil {
			return failApply(sel, log, res, record, "workspace", err)
		}
		record("workspace", StageOK, fmt.Sprintf("prompt %q, tmux %t, editor %q, terminal %q",
			orDefault(prefs.Prompt, "inherited"), prefs.Tmux,
			orDefault(prefs.Editor, "inherited"), orDefault(prefs.Terminal, "inherited")))
	}

	// Stage 4: desktop install + activation (explicitly confirmed at
	// the review step; the Desktop Engine stays idempotent).
	if sel.Desktop != "" {
		switch plan.Desktop.Action {
		case DesktopAlreadyActive:
			record("desktop", StageSkip, plan.Desktop.DisplayName+" already active")
		default:
			ires, derr := in.Desktop.Install(sel.Desktop)
			if derr != nil {
				return failApply(sel, log, res, record, "desktop", derr)
			}
			record("desktop", StageOK, strings.Join(ires.Steps, "; "))
		}
	} else {
		record("desktop", StageSkip, "no desktop selected")
	}

	// Stage 5: verification against the engines' authoritative state.
	if err := Verify(sel, in); err != nil {
		return failApply(sel, log, res, record, "verify", err)
	}
	record("verify", StageOK, "engine state consistent")

	log.Success = true
	log.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	res.Success = true
	if err := sel.Save(); err != nil {
		return nil, fmt.Errorf("apply completed but the selection could not be saved: %w", err)
	}
	return res, nil
}

// failApply records the failed stage, persists the ledger and returns
// the staged result with an error describing exactly what happened.
func failApply(sel Selection, log *ApplyLog, res *ApplyResult, record func(string, string, string), stage string, err error) (*ApplyResult, error) {
	record(stage, StageFailed, err.Error())
	log.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	_ = sel.Save()
	return res, fmt.Errorf("stage %s failed: %w", stage, err)
}

// Verify re-reads the engines' authoritative state and reports
// inconsistencies between the selection and what is actually applied.
// It never changes anything.
func Verify(sel Selection, in PlanInputs) error {
	problems := sel.Problems(in.Registry, in.Capabilities, in.Catalog)
	if len(problems) > 0 {
		return fmt.Errorf("selection references invalid objects: %s", strings.Join(problems, "; "))
	}
	entries, err := in.Catalog.List()
	if err != nil {
		return fmt.Errorf("catalog unreadable after apply: %w", err)
	}
	installedSet := make(map[string]bool)
	for _, e := range entries {
		if e.Status == experience.StatusInstalled {
			installedSet[e.Name] = true
		}
	}
	var missing []string
	for _, name := range sel.Experiences {
		if !installedSet[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("experiences not installed after apply: %s", strings.Join(missing, ", "))
	}
	if sel.Desktop != "" && !installedSet[sel.Desktop] {
		return fmt.Errorf("desktop %q not installed after apply", sel.Desktop)
	}
	if sel.Profile != "" {
		m, lerr := in.Registry.Load(sel.Profile)
		if lerr != nil {
			return fmt.Errorf("active profile unreadable after apply: %w", lerr)
		}
		rep, serr := in.Workspace.Status(MergeWorkspace(m.Workspace, sel.Workspace), sel.Profile)
		if serr != nil {
			return fmt.Errorf("workspace unreadable after apply: %w", serr)
		}
		if rep.Applied && !rep.Clean {
			return fmt.Errorf("workspace reports pending changes or drift after apply")
		}
	}
	return nil
}

// RenderApplyReport writes the executed stage ledger plus recovery
// guidance in human-readable form.
func RenderApplyReport(w io.Writer, res *ApplyResult, err error) {
	fmt.Fprintln(w, "APPLY RESULT")
	for _, s := range res.Stages {
		mark := "OK"
		if s.Status == StageFailed {
			mark = "FAILED"
		} else if s.Status == StageSkip {
			mark = "skipped"
		}
		if s.Detail != "" {
			fmt.Fprintf(w, "  [%s] %s — %s\n", mark, s.Name, s.Detail)
		} else {
			fmt.Fprintf(w, "  [%s] %s\n", mark, s.Name)
		}
	}
	if res.Success {
		fmt.Fprintln(w, "Success: selection applied and verified.")
	} else {
		fmt.Fprintln(w, "Apply stopped at the first failed stage.")
		fmt.Fprintln(w, "Completed stages are NOT rolled back (no automatic rollback of DNF transactions).")
		fmt.Fprintln(w, "Recovery: rerun 'veil onboard' — it loads your selection and every engine is idempotent,")
		fmt.Fprintln(w, "so completed stages are no-ops and only the remaining work runs.")
		if err != nil {
			fmt.Fprintf(w, "Error: %v\n", err)
		}
	}
}
