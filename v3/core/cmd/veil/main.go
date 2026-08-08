// Command veil is the Veilbox v3 command-line interface.
//
// Veilbox is an Operations Platform for DevOps, SRE, Platform, Cloud,
// and Kubernetes engineers. Fedora manages the operating system;
// Veilbox manages the engineer.
//
// Usage is intent-oriented, not package-oriented:
//
//	veil version
//	veil profile
//	veil profile list
//	veil profile show <name>
//	veil profile apply <name>
//	veil profile diff <name>
//	veil profile sync [--yes]
//	veil capability list
//	veil capability info <name>
//	veil experience list
//	veil experience info <name>
//	veil experience install <name>
//	veil experience remove <name>
//	veil environment list
//	veil environment info <name>
//	veil environment install <name>
//	veil environment remove <name>
//	veil environment provision <name>
//	veil desktop ... (alias, accepted for one release)
//	veil workspace [plan|apply|status|reset]
//	veil onboard [--yes]
//	veil status
//	veil doctor
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Shreyas0047/veilbox/v3/core/internal/capability"
	"github.com/Shreyas0047/veilbox/v3/core/internal/dnfops"
	"github.com/Shreyas0047/veilbox/v3/core/internal/environment"
	"github.com/Shreyas0047/veilbox/v3/core/internal/experience"
	"github.com/Shreyas0047/veilbox/v3/core/internal/onboarding"
	"github.com/Shreyas0047/veilbox/v3/core/internal/profile"
	"github.com/Shreyas0047/veilbox/v3/core/internal/settings"
	"github.com/Shreyas0047/veilbox/v3/core/internal/workspace"
)

// version is overridden at build time via -ldflags.
var version = "0.1.0"

const usage = `Veilbox v3 — Operations Platform for engineers

Usage:
  veil <command> [options]

Commands:
  profile                 Show the active engineer profile
  profile list            List known profiles
  profile show <name>     Show a profile and its recommendations
  profile apply <name>    Apply an engineer profile (intent, no packages)
  profile diff <name>     Compare machine state with a profile's intent
  profile sync [--yes]    Install missing recommended experiences
  capability list         List known capabilities (intent concepts)
  capability info <name>  Show a capability and its implementing experiences
  experience list         List known experiences and their status
  experience info <name>  Show details about an experience
  experience install <name>  Install an experience through DNF
  experience remove <name>   Remove an experience through DNF
  environment             Show environment overview and session state
  environment list        List environment experiences and their status
  environment info <name> Show an environment experience's stack
  environment install <name>  Install + activate an environment experience
  environment remove <name>   Remove an environment experience (conservative)
  environment provision <name>  Regenerate Veilbox-owned environment config
  desktop                 Alias for 'environment' (one release)
  workspace               Show the workspace overview
  workspace plan          Show what apply would do (no changes)
  workspace apply [--yes] [--force]
                          Apply the active profile's workspace
  workspace status        Report applied state, drift, conflicts
  workspace reset [--yes] Remove only Veilbox-managed workspace config
  onboard [--yes]         Run the onboarding wizard (role, environment,
                          capabilities, workspace) and apply the plan
  status                  Show current Veilbox and system state
  doctor                  Diagnose Veilbox and system health
  version                 Print version information
  help                    Show this help

Run 'veil <command> --help' for command details.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, deps{}))
}

// deps allows tests to substitute engines without touching the system.
type deps struct {
	newCatalog      func() *experience.Catalog
	newDNF          func() *dnfops.System
	newEnvironment  func(c *experience.Catalog) *environment.Engine
	newCapabilities func() *capability.Registry
}

func (d deps) catalog() *experience.Catalog {
	if d.newCatalog != nil {
		return d.newCatalog()
	}
	return experience.NewCatalog()
}

func (d deps) capabilities() *capability.Registry {
	if d.newCapabilities != nil {
		return d.newCapabilities()
	}
	return capability.NewRegistry()
}

func (d deps) dnf() *dnfops.System {
	if d.newDNF != nil {
		return d.newDNF()
	}
	return dnfops.New()
}

func (d deps) environmentEngine() *environment.Engine {
	if d.newEnvironment != nil {
		return d.newEnvironment(d.catalog())
	}
	return environment.New(d.catalog())
}

func run(args []string, stdout, stderr io.Writer, d deps) int {
	if len(args) < 1 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	case "version", "--version":
		fmt.Fprintf(stdout, "veil %s\n", version)
		return 0
	case "profile":
		return cmdProfile(args[1:], stdout, stderr, d)
	case "capability":
		return cmdCapability(args[1:], stdout, stderr, d)
	case "experience":
		return cmdExperience(args[1:], stdout, stderr, d)
	case "environment":
		return cmdEnvironment(args[1:], stdout, stderr, d)
	case "desktop":
		// Legacy alias (ADR-0012): accepted for one release.
		return cmdEnvironment(args[1:], stdout, stderr, d)
	case "workspace":
		return cmdWorkspace(args[1:], stdout, stderr)
	case "onboard":
		return cmdOnboard(args[1:], stdout, stderr, d)
	case "status":
		return cmdStatus(stdout, stderr, d)
	case "doctor":
		return cmdDoctor(stdout, stderr, d)
	default:
		fmt.Fprintf(stderr, "veil: unknown command %q\n\n%s", args[0], usage)
		return 2
	}
}

func cmdProfile(args []string, stdout, stderr io.Writer, d deps) int {
	fs := flag.NewFlagSet("profile", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rest := fs.Args()
	if len(rest) == 0 {
		return showActiveProfile(stdout, stderr)
	}
	switch rest[0] {
	case "list":
		return profileList(stdout, stderr)
	case "show":
		if len(rest) != 2 {
			fmt.Fprintln(stderr, "usage: veil profile show <name>")
			return 2
		}
		return profileShow(rest[1], stdout, stderr, d)
	case "apply":
		if len(rest) != 2 {
			fmt.Fprintln(stderr, "usage: veil profile apply <name>")
			return 2
		}
		return profileApply(rest[1], stdout, stderr, d)
	case "diff":
		if len(rest) != 2 {
			fmt.Fprintln(stderr, "usage: veil profile diff <name>")
			return 2
		}
		return profileDiff(rest[1], stdout, stderr, d)
	case "sync":
		return profileSync(rest[1:], stdout, stderr, d)
	default:
		fmt.Fprintf(stderr, "veil profile: unknown command %q\n", rest[0])
		return 2
	}
}

func showActiveProfile(stdout, stderr io.Writer) int {
	st, err := profile.Active()
	if err != nil {
		fmt.Fprintf(stderr, "veil: %v\n", err)
		return 1
	}
	if st.ActiveProfile == "" {
		fmt.Fprintln(stdout, "No profile configured.")
		fmt.Fprintln(stdout, "Use 'veil profile apply <name>' to set one.")
		return 0
	}
	fmt.Fprintf(stdout, "Profile: %s\n", st.ActiveProfile)
	if st.AppliedAt != "" {
		fmt.Fprintf(stdout, "Applied: %s\n", st.AppliedAt)
	}
	return 0
}

func profileList(stdout, stderr io.Writer) int {
	reg := profile.NewRegistry()
	names, err := reg.List()
	if err != nil {
		fmt.Fprintf(stderr, "veil: %v\n", err)
		return 1
	}
	st, err := settings.LoadState()
	if err != nil {
		fmt.Fprintf(stderr, "veil: %v\n", err)
		return 1
	}
	active := st.ActiveProfile
	for _, n := range names {
		if n == active {
			fmt.Fprintf(stdout, "%s (active)\n", n)
		} else {
			fmt.Fprintln(stdout, n)
		}
	}
	return 0
}

func profileShow(name string, stdout, stderr io.Writer, d deps) int {
	reg := profile.NewRegistry()
	m, err := reg.Load(name)
	if err != nil {
		fmt.Fprintf(stderr, "veil: %v\n", err)
		return 1
	}
	catalog := d.catalog()
	res, rerr := capability.NewResolver(d.capabilities(), catalog)
	if rerr != nil {
		fmt.Fprintf(stderr, "veil: %v\n", rerr)
		return 1
	}
	statuses := map[string]string{}
	if entries, err := catalog.List(); err == nil {
		for _, e := range entries {
			statuses[e.Name] = string(e.Status)
		}
	} else {
		fmt.Fprintf(stderr, "veil: warning: statuses unavailable: %v\n", err)
	}

	fmt.Fprintf(stdout, "Profile: %s (%s)\n", m.Name, m.DisplayName)
	fmt.Fprintf(stdout, "Role: %s\n", m.Role)
	fmt.Fprintf(stdout, "Description: %s\n", m.Description)
	if len(m.Tags) > 0 {
		fmt.Fprintf(stdout, "Tags: %s\n", strings.Join(m.Tags, ", "))
	}
	prefLines := renderPrefs(m.Workspace)
	if len(prefLines) > 0 {
		fmt.Fprintln(stdout, "Workspace preferences:")
		for _, l := range prefLines {
			fmt.Fprintf(stdout, "  %s\n", l)
		}
	}

	fmt.Fprintln(stdout, "Recommended capabilities:")
	for _, c := range m.Recommended {
		fmt.Fprintf(stdout, "  - %-24s %s\n", c, capabilitySummary(res, statuses, c))
	}
	fmt.Fprintln(stdout, "Optional capabilities:")
	for _, c := range m.Optional {
		fmt.Fprintf(stdout, "  - %-24s %s\n", c, capabilitySummary(res, statuses, c))
	}
	base, berr := res.Base()
	if berr == nil {
		fmt.Fprintf(stdout, "Always included: %s (%s)\n", base.Name, capabilitySummary(res, statuses, base.Name))
	}
	return 0
}

// capabilitySummary renders a capability's implementing experiences
// with install status, e.g. "containers -> containers-tools [available]".
func capabilitySummary(res *capability.Resolver, statuses map[string]string, name string) string {
	exps, err := res.ExperiencesFor([]string{name})
	if err != nil {
		return "unknown capability"
	}
	if len(exps) == 0 {
		return "no installable experience (planned)"
	}
	var parts []string
	for _, e := range exps {
		s := statuses[e]
		if s == "" {
			s = "unknown"
		}
		parts = append(parts, fmt.Sprintf("%s [%s]", e, s))
	}
	return "-> " + strings.Join(parts, ", ")
}

func statusText(statuses map[string]string, name string) string {
	if s, ok := statuses[name]; ok {
		return s
	}
	return "unknown"
}

func profileApply(name string, stdout, stderr io.Writer, d deps) int {
	st, err := profile.Apply(name)
	if err != nil {
		fmt.Fprintf(stderr, "veil: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Profile %q applied\n", st.ActiveProfile)

	// The recommendation summary is best-effort: on a minimal install
	// without a catalog it is skipped, never fatal.
	m, err := profile.NewRegistry().Load(name)
	if err != nil {
		return 0
	}
	catalog := d.catalog()
	res, rerr := capability.NewResolver(d.capabilities(), catalog)
	if rerr != nil {
		return 0
	}
	statuses := map[string]string{}
	if entries, err := catalog.List(); err == nil {
		for _, e := range entries {
			statuses[e.Name] = string(e.Status)
		}
	}
	fmt.Fprintln(stdout, "This profile recommends the following capabilities:")
	for _, c := range m.Recommended {
		fmt.Fprintf(stdout, "  - %-24s %s\n", c, capabilitySummary(res, statuses, c))
	}
	if len(m.Optional) > 0 {
		fmt.Fprintln(stdout, "Optional:")
		for _, c := range m.Optional {
			fmt.Fprintf(stdout, "  - %-24s %s\n", c, capabilitySummary(res, statuses, c))
		}
	}
	fmt.Fprintln(stdout, "Nothing was installed. Run 'veil profile sync' to install the recommended capabilities' experiences.")
	return 0
}

func profileDiff(name string, stdout, stderr io.Writer, d deps) int {
	reg := profile.NewRegistry()
	p, err := profile.Diff(reg, d.capabilities(), d.catalog(), name)
	if err != nil {
		fmt.Fprintf(stderr, "veil: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Profile %s — intent vs machine state\n", p.Profile)
	fmt.Fprintf(stdout, "Recommended capabilities: %s\n", strings.Join(p.RecommendedCaps, ", "))

	section := func(title string, items []string) {
		if len(items) == 0 {
			return
		}
		fmt.Fprintln(stdout, title)
		for _, i := range items {
			fmt.Fprintf(stdout, "  - %s\n", i)
		}
	}
	section("Unknown capabilities (cannot be derived):", p.UnknownCapabilities)
	section("Missing recommended experiences:", p.MissingRecommended)
	section("Not yet installable (planned):", p.NotInstallable)
	section("Unknown to the catalog:", p.UnknownRecommended)
	section("Already satisfied:", p.Satisfied)
	if len(p.OptionalInstalled)+len(p.OptionalMissing) > 0 {
		fmt.Fprintln(stdout, "Optional experiences:")
		for _, e := range p.OptionalInstalled {
			fmt.Fprintf(stdout, "  - %s (installed)\n", e)
		}
		for _, e := range p.OptionalMissing {
			fmt.Fprintf(stdout, "  - %s (not installed)\n", e)
		}
	}
	section("Extra experiences (kept, not in profile):", p.Extras)

	if p.Synced() {
		fmt.Fprintln(stdout, "Profile is synced.")
	} else {
		fmt.Fprintf(stdout, "%d recommended experience(s) missing. Run 'veil profile sync' to install.\n",
			len(p.MissingRecommended))
	}
	return 0
}

func profileSync(args []string, stdout, stderr io.Writer, d deps) int {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	fs.SetOutput(stderr)
	yes := fs.Bool("yes", false, "skip confirmation")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if len(fs.Args()) > 0 {
		fmt.Fprintf(stderr, "veil profile sync: unknown argument %q\n", fs.Args()[0])
		return 2
	}

	st, err := profile.Active()
	if err != nil {
		fmt.Fprintf(stderr, "veil: %v\n", err)
		return 1
	}
	if st.ActiveProfile == "" {
		fmt.Fprintln(stderr, "veil: no active profile — apply one first with 'veil profile apply <name>'")
		return 1
	}
	reg := profile.NewRegistry()
	p, err := profile.Diff(reg, d.capabilities(), d.catalog(), st.ActiveProfile)
	if err != nil {
		fmt.Fprintf(stderr, "veil: %v\n", err)
		return 1
	}

	if len(p.MissingRecommended) == 0 {
		fmt.Fprintf(stdout, "Active profile %q is already synced.\n", p.Profile)
		if len(p.NotInstallable) > 0 {
			fmt.Fprintf(stdout, "  deferred (planned): %s\n", strings.Join(p.NotInstallable, ", "))
		}
		if len(p.UnknownRecommended) > 0 {
			fmt.Fprintf(stdout, "  unknown to catalog: %s\n", strings.Join(p.UnknownRecommended, ", "))
		}
		return 0
	}

	fmt.Fprintf(stdout, "Active profile: %s\n", p.Profile)
	fmt.Fprintln(stdout, "Recommended experiences to install:")
	for _, e := range p.MissingRecommended {
		fmt.Fprintf(stdout, "  - %s\n", e)
	}
	if len(p.NotInstallable) > 0 {
		fmt.Fprintf(stdout, "Skipping (planned, not yet installable): %s\n", strings.Join(p.NotInstallable, ", "))
	}
	if len(p.UnknownRecommended) > 0 {
		fmt.Fprintf(stdout, "Skipping (unknown to catalog): %s\n", strings.Join(p.UnknownRecommended, ", "))
	}

	if !*yes {
		fmt.Fprint(stdout, "Proceed? [y/N]: ")
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if !strings.EqualFold(strings.TrimSpace(line), "y") {
			fmt.Fprintln(stdout, "Aborted.")
			return 1
		}
	}

	installed := 0
	catalog := d.catalog()
	for _, e := range p.MissingRecommended {
		if err := catalog.Install(e); err != nil {
			fmt.Fprintf(stderr, "veil: install experience %s: %v\n", e, err)
			return 1
		}
		fmt.Fprintf(stdout, "Installed %s\n", e)
		installed++
	}
	fmt.Fprintf(stdout, "Profile sync complete: %d installed, %d deferred.\n",
		installed, len(p.NotInstallable)+len(p.UnknownRecommended))
	return 0
}

func cmdExperience(args []string, stdout, stderr io.Writer, d deps) int {
	fs := flag.NewFlagSet("experience", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	catalog := d.catalog()
	switch rest[0] {
	case "list":
		entries, err := catalog.List()
		if err != nil {
			fmt.Fprintf(stderr, "veil: %v\n", err)
			return 1
		}
		if len(entries) == 0 {
			fmt.Fprintln(stdout, "No experiences known.")
			return 0
		}
		fmt.Fprintln(stdout, "EXPERIENCE                 STATUS      PACKAGE")
		fmt.Fprintln(stdout, "-------------------------------------------------------")
		for _, e := range entries {
			fmt.Fprintf(stdout, "%-25s %-11s %s\n", e.Name, e.Status, e.RPM)
		}
		return 0
	case "info":
		if len(rest) != 2 {
			fmt.Fprintln(stderr, "usage: veil experience info <name>")
			return 2
		}
		return experienceInfo(rest[1], stdout, stderr, catalog)
	case "install":
		if len(rest) != 2 {
			fmt.Fprintln(stderr, "usage: veil experience install <name>")
			return 2
		}
		if err := catalog.Install(rest[1]); err != nil {
			fmt.Fprintf(stderr, "veil: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "Experience %q installed\n", rest[1])
		return 0
	case "remove":
		if len(rest) != 2 {
			fmt.Fprintln(stderr, "usage: veil experience remove <name>")
			return 2
		}
		if err := catalog.Remove(rest[1]); err != nil {
			fmt.Fprintf(stderr, "veil: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "Experience %q removed\n", rest[1])
		return 0
	default:
		fmt.Fprintf(stderr, "veil experience: unknown command %q\n", rest[0])
		return 2
	}
}

func experienceInfo(name string, stdout, stderr io.Writer, catalog *experience.Catalog) int {
	entries, err := catalog.List()
	if err != nil {
		fmt.Fprintf(stderr, "veil: %v\n", err)
		return 1
	}
	var entry *experience.Entry
	for i := range entries {
		if entries[i].Name == name {
			entry = &entries[i]
			break
		}
	}
	if entry == nil {
		fmt.Fprintf(stderr, "veil: experience %q not found\n", name)
		return 1
	}
	m := entry.Manifest
	fmt.Fprintf(stdout, "Experience: %s\n", m.Name)
	fmt.Fprintf(stdout, "Status: %s\n", entry.Status)
	if m.RPM != "" {
		fmt.Fprintf(stdout, "Package: %s\n", m.RPM)
	}
	fmt.Fprintf(stdout, "Description: %s\n", m.Description)
	if len(m.Packages) > 0 {
		fmt.Fprintln(stdout, "Packages:")
		for _, p := range m.Packages {
			fmt.Fprintf(stdout, "  - %s\n", p)
		}
	}

	// Reverse lookup: profiles that recommend a capability this
	// experience implements.
	reg := profile.NewRegistry()
	pnames, err := reg.List()
	if err == nil {
		var recommending []string
		for _, pn := range pnames {
			pm, lerr := reg.Load(pn)
			if lerr != nil {
				continue
			}
			for _, ref := range pm.AllReferences() {
				for _, c := range m.Capabilities {
					if ref == c {
						recommending = append(recommending, pn)
						break
					}
				}
			}
		}
		if len(m.Capabilities) > 0 {
			fmt.Fprintln(stdout, "Implements capabilities:")
			for _, c := range m.Capabilities {
				fmt.Fprintf(stdout, "  - %s\n", c)
			}
		}
		if len(recommending) > 0 {
			fmt.Fprintln(stdout, "Recommended by profiles:")
			for _, pn := range recommending {
				fmt.Fprintf(stdout, "  - %s\n", pn)
			}
		}
	}
	return 0
}

func cmdStatus(stdout, stderr io.Writer, d deps) int {
	fmt.Fprintln(stdout, "Veilbox status")
	fmt.Fprintln(stdout, "--------------")

	coreRPM := installedCoreRPM(d)
	fmt.Fprintf(stdout, "Core version:   %s\n", coreRPM)

	// The composition is the applied product record (ADR-0010): the
	// source of truth for what was agreed. Status reads it plus the
	// RPM database; it never guesses.
	comp, cerr := onboarding.LoadComposition()
	if cerr != nil {
		fmt.Fprintf(stdout, "Composition:    (unreadable: %v)\n", cerr)
	} else if comp.SchemaVersion == 0 {
		fmt.Fprintln(stdout, "Composition:    (none — run 'veil onboard --yes' to record the applied product)")
	} else {
		fmt.Fprintf(stdout, "Composition:    applied %s (schema %d)\n", comp.AppliedAt, comp.SchemaVersion)
	}

	st, err := profile.Active()
	if err != nil {
		fmt.Fprintf(stderr, "veil: %v\n", err)
		return 1
	}
	if st.ActiveProfile == "" {
		fmt.Fprintln(stdout, "Profile:        (none configured)")
	} else {
		fmt.Fprintf(stdout, "Profile:        %s\n", st.ActiveProfile)
		reg := profile.NewRegistry()
		p, derr := profile.Diff(reg, d.capabilities(), d.catalog(), st.ActiveProfile)
		switch {
		case derr != nil:
			fmt.Fprintf(stdout, "Profile sync:   (unavailable: %v)\n", derr)
		case p.Synced():
			fmt.Fprintln(stdout, "Profile sync:   synced")
		default:
			msg := fmt.Sprintf("missing %d recommended experience(s)", len(p.MissingRecommended))
			if len(p.NotInstallable) > 0 {
				msg += fmt.Sprintf(", %d not yet installable", len(p.NotInstallable))
			}
			if len(p.UnknownRecommended) > 0 {
				msg += fmt.Sprintf(", %d unknown to catalog", len(p.UnknownRecommended))
			}
			fmt.Fprintf(stdout, "Profile sync:   %s\n", msg)
		}
	}

	dnf := d.dnf()
	installed, err := dnf.ListInstalledByPrefix(experience.PackagePrefix)
	if err != nil {
		fmt.Fprintf(stderr, "veil: %v\n", err)
		return 1
	}
	if len(installed) == 0 {
		fmt.Fprintln(stdout, "Experiences:    (none installed)")
	} else {
		fmt.Fprintf(stdout, "Experiences:    %s\n", strings.Join(installed, ", "))
	}
	if comp.SchemaVersion > 0 {
		entries, lerr := d.catalog().List()
		var missing []string
		if lerr == nil {
			installedNames := make(map[string]bool)
			for _, e := range entries {
				if e.Status == experience.StatusInstalled {
					installedNames[e.Name] = true
				}
			}
			for _, name := range comp.Experiences {
				if !installedNames[name] {
					missing = append(missing, name)
				}
			}
		}
		if len(missing) > 0 {
			fmt.Fprintf(stdout, "Composition drift: %s recorded but no longer installed\n", strings.Join(missing, ", "))
		}
	}

	envs, derr := d.environmentEngine().List()
	if derr != nil {
		fmt.Fprintf(stderr, "veil: %v\n", derr)
		return 1
	}
	statusBy := make(map[string]experience.Status, len(envs))
	activeCompositor := ""
	for _, en := range envs {
		statusBy[en.Name] = en.Status
		if en.Session.Compositor != "" {
			activeCompositor = en.Session.Compositor
		}
	}
	if comp.SchemaVersion > 0 && comp.Environment != nil {
		rec := comp.Environment
		switch statusBy[rec.Name] {
		case experience.StatusInstalled:
			fmt.Fprintf(stdout, "Environment:    %s (%s)\n", rec.Name, rec.RPM)
		case experience.StatusAvailable:
			fmt.Fprintf(stdout, "Environment:    %s — recorded but package %s not installed (drift)\n", rec.Name, rec.RPM)
		default:
			fmt.Fprintf(stdout, "Environment:    %s — recorded but unknown to the current catalog (drift)\n", rec.Name)
		}
	} else {
		var installedEnvs []string
		for _, en := range envs {
			if en.Status == experience.StatusInstalled {
				installedEnvs = append(installedEnvs, en.Name)
			}
		}
		if len(installedEnvs) == 0 {
			fmt.Fprintln(stdout, "Environment:    (none installed)")
		} else {
			fmt.Fprintf(stdout, "Environment:    %s (no composition record)\n", strings.Join(installedEnvs, ", "))
		}
	}
	if activeCompositor == "" {
		fmt.Fprintln(stdout, "Session:        no graphical Veilbox environment session detected")
	} else {
		fmt.Fprintf(stdout, "Session:        %s active\n", activeCompositor)
	}

	rel := fedoraRelease()
	if rel != "" {
		fmt.Fprintf(stdout, "Fedora:         %s\n", rel)
	} else {
		fmt.Fprintln(stdout, "Fedora:         (not detected)")
	}

	pkgCount := rpmPackageCount()
	if pkgCount > 0 {
		fmt.Fprintf(stdout, "RPM packages:   %d installed\n", pkgCount)
	}

	repos, err := dnf.Repos()
	if err != nil {
		fmt.Fprintf(stdout, "Repositories:   (dnf repolist failed: %v)\n", err)
	} else {
		fmt.Fprintln(stdout, "Repositories:")
		for _, line := range strings.Split(strings.TrimSpace(repos), "\n") {
			if strings.TrimSpace(line) != "" {
				fmt.Fprintf(stdout, "  %s\n", line)
			}
		}
	}
	return 0
}

func cmdDoctor(stdout, stderr io.Writer, d deps) int {
	fail := 0
	check := func(name string, critical bool, ok bool, detail string) {
		status := "OK"
		if !ok {
			if critical {
				status = "FAIL"
			} else {
				status = "WARN"
			}
		}
		if detail != "" {
			fmt.Fprintf(stdout, "  [%s] %s — %s\n", status, name, detail)
		} else {
			fmt.Fprintf(stdout, "  [%s] %s\n", status, name)
		}
		if !ok && critical {
			fail++
		}
	}

	fmt.Fprintln(stdout, "Veilbox doctor")
	fmt.Fprintln(stdout, "--------------")

	check("DNF available", true, dnfops.Available(dnfops.DNFBinary), "")
	check("RPM available", true, dnfops.Available(dnfops.RPMBinary), "")
	check("Fedora base detected", true, fedoraRelease() != "", fedoraRelease())

	dir, err := settings.StateDir()
	if err == nil {
		writable := true
		if _, err := settings.EnsureStateDir(); err != nil {
			writable = false
		}
		check("State dir writable", true, writable, dir)
	} else {
		check("State dir writable", true, false, err.Error())
	}

	catalog := d.catalog()
	entries, err := catalog.List()
	if err != nil {
		check("Experience catalog loads", true, false, err.Error())
	} else {
		check("Experience catalog loads", true, true,
			fmt.Sprintf("%d known experiences", len(entries)))
		inconsistent := 0
		dnf := d.dnf()
		for _, e := range entries {
			if e.RPM == "" {
				continue
			}
			ok, err := dnf.IsInstalled(e.RPM)
			if err == nil && ok != (e.Status == experience.StatusInstalled) {
				inconsistent++
			}
		}
		check("Experience package state consistent", true, inconsistent == 0,
			fmt.Sprintf("%d inconsistencies", inconsistent))
	}

	// Capability catalog consistency.
	capReg := d.capabilities()
	cnames, cerr := capReg.List()
	if cerr != nil {
		check("Capability catalog loads", true, false, cerr.Error())
	} else {
		check("Capability catalog loads", true, true, fmt.Sprintf("%d known capabilities", len(cnames)))
	}
	capRes, rerr := capability.NewResolver(capReg, catalog)
	if rerr != nil {
		check("Capability mapping resolves", true, false, rerr.Error())
	} else {
		planned, unknownRefs := capRes.CheckMapping()
		if len(unknownRefs) > 0 {
			check("Capability mapping consistent", true, false,
				fmt.Sprintf("unknown experience→capability reference(s): %s", strings.Join(unknownRefs, ", ")))
		} else {
			check("Capability mapping consistent", true, true,
				fmt.Sprintf("%d planned capability(ies)", len(planned)))
		}
	}

	// Profile consistency.
	reg := profile.NewRegistry()
	st, err := settings.LoadState()
	check("Profile state parses", true, err == nil, "state.json")
	if err == nil && st.ActiveProfile != "" {
		_, lerr := reg.Load(st.ActiveProfile)
		if lerr != nil {
			check("Active profile exists", true, false, lerr.Error())
		} else {
			check("Active profile exists", true, true, st.ActiveProfile)
		}
	} else if err == nil {
		check("Active profile exists", true, true, "(none configured)")
	}
	pnames, _ := reg.List()
	var invalid []string
	for _, n := range pnames {
		if _, lerr := reg.Load(n); lerr != nil {
			invalid = append(invalid, n)
		}
	}
	if len(invalid) > 0 {
		check("Profile manifests valid", false, false,
			fmt.Sprintf("invalid: %s", strings.Join(invalid, ", ")))
	} else {
		check("Profile manifests valid", false, true, fmt.Sprintf("%d profiles", len(pnames)))
	}
	if err == nil && st.ActiveProfile != "" {
		missing, rerr := profile.CheckCapabilities(reg, capReg, catalog, st.ActiveProfile)
		check("Profile capabilities resolve to experiences", true,
			rerr == nil && len(missing) == 0,
			fmt.Sprintf("%d unknown capability reference(s)", len(missing)))
	}

	// The composition record (ADR-0010): what was applied, and whether
	// it still holds against the live catalogs and RPM database.
	comp, cerr := onboarding.LoadComposition()
	check("Composition record parses", false, cerr == nil, "composition.json")
	if cerr == nil && comp.SchemaVersion > 0 {
		var drift []string
		if comp.Profile != "" {
			if _, lerr := reg.Load(comp.Profile); lerr != nil {
				drift = append(drift, "profile "+comp.Profile+" unknown to the registry")
			}
		}
		installedSet := make(map[string]bool)
		for _, e := range entries {
			if e.Status == experience.StatusInstalled {
				installedSet[e.Name] = true
			}
		}
		for _, name := range comp.Experiences {
			if !installedSet[name] {
				drift = append(drift, "experience "+name+" no longer installed")
			}
		}
		if comp.Environment != nil {
			ok, rerr := d.dnf().IsInstalled(comp.Environment.RPM)
			switch {
			case rerr != nil:
				drift = append(drift, "environment "+comp.Environment.Name+": "+rerr.Error())
			case !ok:
				drift = append(drift, "environment "+comp.Environment.Name+" package not installed")
			}
		}
		status := "consistent with live state"
		if len(drift) > 0 {
			status = "drift: " + strings.Join(drift, "; ")
		}
		check("Composition consistent with live state", false, len(drift) == 0, status)
	}

	// Repository reachability.
	dnf := d.dnf()
	repos, rerr := dnf.Repos()
	if rerr != nil {
		check("DNF repositories reachable", true, false, rerr.Error())
	} else {
		check("DNF repositories reachable", true, true, "")
		if strings.Contains(repos, "veilbox") {
			check("Veilbox repository configured", false, true, "(dnf repolist)")
		} else {
			check("Veilbox repository configured", false, false,
				"no veilbox repo in dnf repolist — experiences may not resolve")
		}
	}

	// Workspace health (warn-level: a drifted workspace is recoverable
	// and is the user's decision to reconcile).
	wsSt, werr := workspace.LoadState()
	check("Workspace state parses", false, werr == nil, fmt.Sprintf("generation %d", wsSt.Generation))
	if werr == nil && st.ActiveProfile != "" {
		if m, err := profile.NewRegistry().Load(st.ActiveProfile); err == nil {
			verr := m.Workspace.Validate()
			if verr != nil {
				check("Workspace preferences valid", false, false, verr.Error())
			} else {
				rep, serr := newWorkspaceEngine().Status(m.Workspace, st.ActiveProfile)
				if serr != nil {
					check("Workspace status readable", false, false, serr.Error())
				} else {
					conflicts := 0
					for _, it := range rep.Items {
						if it.Verdict == workspace.VerdictConflict || it.Verdict == workspace.VerdictDrifted {
							conflicts++
						}
					}
					check("Workspace preferences valid", false, true, "")
					check("Workspace has no conflicts", false, conflicts == 0,
						fmt.Sprintf("%d drifted/conflicted item(s)", conflicts))
				}
			}
		}
	}

	// Environment health (warn-level: an environment is optional
	// functionality, and graphical runtime checks are skipped from a
	// TTY session).
	var environmentEntries []experience.Entry
	for _, en := range entries {
		if en.Type == experience.TypeEnvironment {
			environmentEntries = append(environmentEntries, en)
		}
	}
	if len(environmentEntries) > 0 {
		invalid := 0
		var installedEnvironment *experience.Entry
		for i := range environmentEntries {
			if verr := environmentEntries[i].Manifest.Validate(); verr != nil {
				invalid++
			}
			if environmentEntries[i].Status == experience.StatusInstalled && installedEnvironment == nil {
				installedEnvironment = &environmentEntries[i]
			}
		}
		check("Environment manifest valid", false, invalid == 0,
			fmt.Sprintf("%d environment experience(s)", len(environmentEntries)))

		var missingPkgs []string
		for _, en := range environmentEntries {
			if en.Status != experience.StatusInstalled {
				continue
			}
			for key, val := range en.Components {
				if val == "builtin" {
					continue
				}
				ok, err := d.dnf().IsInstalled(val)
				if err != nil || !ok {
					missingPkgs = append(missingPkgs, fmt.Sprintf("%s:%s", en.Name, key))
				}
			}
		}
		check("Environment packages installed", false, len(missingPkgs) == 0,
			fmt.Sprintf("%d missing", len(missingPkgs)))

		var missingSession []string
		var templateErr []string
		if installedEnvironment != nil {
			sf := filepath.Join(d.environmentEngine().SessionDir(),
				installedEnvironment.Components[experience.CompCompositor]+".desktop")
			if _, err := os.Stat(sf); err != nil {
				missingSession = append(missingSession, sf)
			}
			td := d.environmentEngine().TemplateDir(installedEnvironment.Manifest)
			if _, err := os.Stat(td); err != nil {
				templateErr = append(templateErr, err.Error())
			}
		}
		check("Environment session file present", false, len(missingSession) == 0,
			strings.Join(missingSession, ", "))
		check("Environment templates readable", false, len(templateErr) == 0,
			strings.Join(templateErr, ", "))

		// Validation hooks (ADR-0015): the environment declares what
		// doctor checks, never the other way around.
		var cfgFileErr []string
		var hookErr []string
		if installedEnvironment != nil && installedEnvironment.Manifest.Environment != nil {
			cfg, cerr := environment.ConfigDir()
			if cerr != nil {
				cfgFileErr = append(cfgFileErr, cerr.Error())
			} else {
				for _, f := range installedEnvironment.Manifest.Environment.Validate.Files {
					p := filepath.Join(cfg, f)
					if _, err := os.Stat(p); err != nil {
						cfgFileErr = append(cfgFileErr, p)
					}
				}
			}
			runner := dnfops.ExecRunner{}
			for _, cmd := range installedEnvironment.Manifest.Environment.Validate.Commands {
				if len(cmd) == 0 {
					continue
				}
				if _, err := runner.Run(cmd[0], cmd[1:]...); err != nil {
					hookErr = append(hookErr, strings.Join(cmd, " "))
				}
			}
		}
		check("Environment config files present", false, len(cfgFileErr) == 0,
			strings.Join(cfgFileErr, ", "))
		check("Environment config hooks pass", false, len(hookErr) == 0,
			strings.Join(hookErr, ", "))

		sess := d.environmentEngine().DetectSession()
		if sess.Graphical {
			check("Environment graphical session", false, sess.Compositor != "", sess.Message)
		} else {
			check("Environment graphical session", false, true,
				"skipped (no graphical session detected from this terminal)")
		}
	}

	if fail > 0 {
		fmt.Fprintf(stdout, "%d critical check(s) failed.\n", fail)
		return 1
	}
	fmt.Fprintln(stdout, "All critical checks passed.")
	return 0
}

// installedCoreRPM returns "x.y.z (rpm)" if veilbox-core is installed.
func installedCoreRPM(d deps) string {
	dnf := d.dnf()
	ok, _ := dnf.IsInstalled("veilbox-core")
	if !ok {
		return version
	}
	out, err := dnfops.ExecRunner{}.Run("rpm", "-q", "veilbox-core")
	if err == nil {
		return version + " (" + strings.TrimSpace(out) + ")"
	}
	return version
}

// fedoraRelease returns the Fedora release string or "" when not Fedora.
func fedoraRelease() string {
	data, err := os.ReadFile("/etc/fedora-release")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// rpmPackageCount returns the number of installed RPM packages.
func rpmPackageCount() int {
	out, err := dnfops.ExecRunner{}.Run("rpm", "-qa", "--queryformat", "\n")
	if err != nil {
		return 0
	}
	return strings.Count(out, "\n")
}

// containsString reports membership in a string list.
func containsString(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
