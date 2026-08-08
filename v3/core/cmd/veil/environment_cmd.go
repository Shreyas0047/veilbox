package main

import (
	"flag"
	"fmt"
	"io"

	"github.com/Shreyas0047/veilbox/v3/core/internal/environment"
	"github.com/Shreyas0047/veilbox/v3/core/internal/experience"
)

// cmdEnvironment implements 'veil environment' (the legacy 'veil
// desktop' name is accepted as an alias for one release, ADR-0012).
//
// Environment activation is an explicit Environment Engine sequence —
// the RPM installs an environment, 'veil environment install' activates
// it. Installing the experience package with DNF never changes the
// boot target and never enables services.
func cmdEnvironment(args []string, stdout, stderr io.Writer, d deps) int {
	fs := flag.NewFlagSet("environment", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rest := fs.Args()
	engine := d.environmentEngine()
	if len(rest) == 0 {
		return environmentOverview(stdout, stderr, engine)
	}
	switch rest[0] {
	case "list":
		return environmentList(stdout, stderr, engine)
	case "info":
		if len(rest) != 2 {
			fmt.Fprintln(stderr, "usage: veil environment info <name>")
			return 2
		}
		return environmentInfo(rest[1], stdout, stderr, engine)
	case "install":
		if len(rest) != 2 {
			fmt.Fprintln(stderr, "usage: veil environment install <name>")
			return 2
		}
		return environmentInstall(rest[1], stdout, stderr, engine)
	case "remove":
		if len(rest) != 2 {
			fmt.Fprintln(stderr, "usage: veil environment remove <name>")
			return 2
		}
		return environmentRemove(rest[1], stdout, stderr, engine)
	case "provision":
		if len(rest) != 2 {
			fmt.Fprintln(stderr, "usage: veil environment provision <name>")
			return 2
		}
		return environmentProvision(rest[1], stdout, stderr, engine)
	default:
		fmt.Fprintf(stderr, "veil environment: unknown command %q\n", rest[0])
		return 2
	}
}

func environmentOverview(stdout, stderr io.Writer, engine *environment.Engine) int {
	if code := environmentList(stdout, stderr, engine); code != 0 {
		return code
	}
	sess := engine.DetectSession()
	fmt.Fprintf(stdout, "\nSession: %s\n", sess.Message)
	if sess.DisplayManager != "" {
		fmt.Fprintf(stdout, "Display manager active: %s\n", sess.DisplayManager)
	}
	return 0
}

func environmentList(stdout, stderr io.Writer, engine *environment.Engine) int {
	entries, err := engine.List()
	if err != nil {
		fmt.Fprintf(stderr, "veil: %v\n", err)
		return 1
	}
	if len(entries) == 0 {
		fmt.Fprintln(stdout, "No environment experiences known.")
		return 0
	}
	fmt.Fprintln(stdout, "ENVIRONMENT      DISPLAY            STATUS      COMPOSITOR")
	fmt.Fprintln(stdout, "--------------------------------------------------------------")
	for _, en := range entries {
		compositor := en.Components[experience.CompCompositor]
		if compositor == "" {
			compositor = "-"
		}
		display := en.DisplayName
		if display == "" {
			display = en.Name
		}
		fmt.Fprintf(stdout, "%-16s %-18s %-11s %s\n", en.Name, display, en.Status, compositor)
	}
	return 0
}

func environmentInfo(name string, stdout, stderr io.Writer, engine *environment.Engine) int {
	en, err := engine.Info(name)
	if err != nil {
		fmt.Fprintf(stderr, "veil: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Environment: %s\n", en.Name)
	if en.DisplayName != "" {
		fmt.Fprintf(stdout, "Display name: %s\n", en.DisplayName)
	}
	fmt.Fprintf(stdout, "Status: %s\n", en.Status)
	if en.RPM != "" {
		fmt.Fprintf(stdout, "Package: %s\n", en.RPM)
	}
	fmt.Fprintf(stdout, "Description: %s\n", en.Description)
	if len(en.ComponentList()) > 0 {
		fmt.Fprintln(stdout, "Components:")
		for _, c := range en.ComponentList() {
			val := c.Value
			if c.Builtin {
				val += " (provided by shell)"
			}
			fmt.Fprintf(stdout, "  %-16s %s\n", c.Key, val)
		}
	}
	fmt.Fprintf(stdout, "Session: %s\n", en.Session.Message)
	return 0
}

func environmentInstall(name string, stdout, stderr io.Writer, engine *environment.Engine) int {
	plan, err := engine.PlanInstall(name)
	if err != nil {
		fmt.Fprintf(stderr, "veil: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Installing environment experience %q...\n", plan.Name)
	if len(plan.CreatedConfig) > 0 {
		fmt.Fprintln(stdout, "User configuration to create (first-touch):")
		for _, f := range plan.CreatedConfig {
			fmt.Fprintf(stdout, "  %s\n", f)
		}
	}
	if len(plan.ExistingConfig) > 0 {
		fmt.Fprintln(stdout, "Existing user configuration (preserved, not overwritten):")
		for _, f := range plan.ExistingConfig {
			fmt.Fprintf(stdout, "  %s\n", f)
		}
	}
	if plan.DeclaredDM != "" {
		switch {
		case !plan.WillEnableDM && plan.EnabledDM != "" && plan.EnabledDM != plan.DeclaredDM:
			fmt.Fprintf(stdout, "Display manager %s already enabled — left as-is.\n", plan.EnabledDM)
		case !plan.WillEnableDM:
			fmt.Fprintf(stdout, "Display manager %s already enabled.\n", plan.DeclaredDM)
		default:
			fmt.Fprintf(stdout, "Display manager to enable: %s\n", plan.DeclaredDM)
		}
	}
	if plan.WillSetTarget {
		fmt.Fprintf(stdout, "Default target will switch to graphical.target (rollback: systemctl set-default %s).\n",
			plan.RollbackTarget)
	}

	res, err := engine.Install(name)
	if err != nil {
		fmt.Fprintf(stderr, "veil: %v\n", err)
		return 1
	}
	for _, s := range res.Steps {
		fmt.Fprintf(stdout, "  %s\n", s)
	}
	fmt.Fprintf(stdout, "Environment %q activated. Reboot to log in.\n", plan.Name)
	return 0
}

func environmentRemove(name string, stdout, stderr io.Writer, engine *environment.Engine) int {
	plan, err := engine.PlanRemove(name)
	if err != nil {
		fmt.Fprintf(stderr, "veil: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Removing environment experience %q...\n", plan.Name)
	plan, err = engine.Remove(name)
	if err != nil {
		fmt.Fprintf(stderr, "veil: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Removed %s (DNF).\n", plan.RPM)
	fmt.Fprintln(stdout, "Remaining user configuration (preserved):")
	fmt.Fprintf(stdout, "  %s\n", environment.JoinFiles(plan.PreservedFiles))
	fmt.Fprintln(stdout, "Remaining Veilbox-managed configuration (not destroyed):")
	fmt.Fprintf(stdout, "  %s\n", environment.JoinFiles(plan.VeilboxFiles))
	if plan.DeactivateHint != "" {
		fmt.Fprintln(stdout, plan.DeactivateHint)
	}
	return 0
}

func environmentProvision(name string, stdout, stderr io.Writer, engine *environment.Engine) int {
	res, err := engine.Provision(name)
	if err != nil {
		fmt.Fprintf(stderr, "veil: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Provisioning environment %q (Veilbox-owned configuration only):\n", name)
	for _, f := range res.Created {
		fmt.Fprintf(stdout, "  created %s\n", f)
	}
	for _, f := range res.Preserved {
		fmt.Fprintf(stdout, "  preserved %s (user-owned, not overwritten)\n", f)
	}
	for _, f := range res.Regenerated {
		fmt.Fprintf(stdout, "  regenerated %s\n", f)
	}
	return 0
}
