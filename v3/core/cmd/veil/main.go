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
//	veil experience list
//	veil experience info <name>
//	veil experience install <name>
//	veil experience remove <name>
//	veil status
//	veil doctor
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Shreyas0047/veilbox/v3/core/internal/dnfops"
	"github.com/Shreyas0047/veilbox/v3/core/internal/experience"
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
  experience list         List known experiences and their status
  experience info <name>  Show details about an experience
  experience install <name>  Install an experience through DNF
  experience remove <name>   Remove an experience through DNF
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
	newCatalog func() *experience.Catalog
	newDNF     func() *dnfops.System
}

func (d deps) catalog() *experience.Catalog {
	if d.newCatalog != nil {
		return d.newCatalog()
	}
	return experience.NewCatalog()
}

func (d deps) dnf() *dnfops.System {
	if d.newDNF != nil {
		return d.newDNF()
	}
	return dnfops.New()
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
	case "experience":
		return cmdExperience(args[1:], stdout, stderr, d)
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
	statuses := map[string]string{}
	if entries, err := d.catalog().List(); err == nil {
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
	if len(m.Workspace) > 0 {
		fmt.Fprintln(stdout, "Workspace preferences:")
		for _, k := range sortedKeys(m.Workspace) {
			fmt.Fprintf(stdout, "  %s: %s\n", k, m.Workspace[k])
		}
	}

	fmt.Fprintln(stdout, "Recommended experiences:")
	for _, e := range m.Recommended {
		fmt.Fprintf(stdout, "  - %-20s %s\n", e, statusText(statuses, e))
	}
	fmt.Fprintln(stdout, "Optional experiences:")
	for _, e := range m.Optional {
		fmt.Fprintf(stdout, "  - %-20s %s\n", e, statusText(statuses, e))
	}
	return 0
}

func statusText(statuses map[string]string, name string) string {
	if s, ok := statuses[name]; ok {
		return s
	}
	return "unknown"
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	// profiles are small; a simple sort keeps output deterministic
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func profileApply(name string, stdout, stderr io.Writer, d deps) int {
	st, err := profile.Apply(name)
	if err != nil {
		fmt.Fprintf(stderr, "veil: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Profile %q applied\n", st.ActiveProfile)

	m, err := profile.NewRegistry().Load(name)
	if err != nil {
		return 0
	}
	statuses := map[string]string{}
	if entries, err := d.catalog().List(); err == nil {
		for _, e := range entries {
			statuses[e.Name] = string(e.Status)
		}
	} else {
		fmt.Fprintf(stderr, "veil: warning: statuses unavailable: %v\n", err)
	}
	fmt.Fprintln(stdout, "This profile recommends the following experiences:")
	for _, e := range m.Recommended {
		fmt.Fprintf(stdout, "  - %-20s %s\n", e, statusText(statuses, e))
	}
	if len(m.Optional) > 0 {
		fmt.Fprintln(stdout, "Optional:")
		for _, e := range m.Optional {
			fmt.Fprintf(stdout, "  - %-20s %s\n", e, statusText(statuses, e))
		}
	}
	fmt.Fprintln(stdout, "Nothing was installed. Run 'veil profile sync' to install the recommended experiences.")
	return 0
}

func profileDiff(name string, stdout, stderr io.Writer, d deps) int {
	reg := profile.NewRegistry()
	p, err := profile.Diff(reg, d.catalog(), name)
	if err != nil {
		fmt.Fprintf(stderr, "veil: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Profile %s — intent vs machine state\n", p.Profile)

	section := func(title string, items []string) {
		if len(items) == 0 {
			return
		}
		fmt.Fprintln(stdout, title)
		for _, i := range items {
			fmt.Fprintf(stdout, "  - %s\n", i)
		}
	}
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
	p, err := profile.Diff(reg, d.catalog(), st.ActiveProfile)
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

	// Reverse lookup: profiles that recommend this experience.
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
				if ref == m.Name {
					recommending = append(recommending, pn)
					break
				}
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
		p, derr := profile.Diff(reg, d.catalog(), st.ActiveProfile)
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
		missing, rerr := profile.CheckReferences(reg, catalog, st.ActiveProfile)
		check("Profile references known experiences", true,
			rerr == nil && len(missing) == 0,
			fmt.Sprintf("%d unknown reference(s)", len(missing)))
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

	_ = workspace.Provision()

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
