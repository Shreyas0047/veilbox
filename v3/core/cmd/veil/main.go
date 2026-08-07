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
//	veil profile apply <name>
//	veil experience list
//	veil experience install <name>
//	veil experience remove <name>
//	veil status
//	veil doctor
package main

import (
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
  profile apply <name>    Apply an engineer profile (intent, no packages)
  experience list         List known experiences and their status
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
		return cmdProfile(args[1:], stdout, stderr)
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

func cmdProfile(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("profile", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rest := fs.Args()
	if len(rest) > 0 && rest[0] == "apply" {
		if len(rest) != 2 {
			fmt.Fprintln(stderr, "usage: veil profile apply <name>")
			return 2
		}
		st, err := profile.Apply(rest[1])
		if err != nil {
			fmt.Fprintf(stderr, "veil: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "Profile %q applied\n", st.ActiveProfile)
		return 0
	}
	if len(rest) > 0 {
		fmt.Fprintf(stderr, "veil profile: unknown argument %q\n", rest[0])
		return 2
	}
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

	catalog := experience.NewCatalog()
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
