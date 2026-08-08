package main

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/Shreyas0047/veilbox/v3/core/internal/capability"
)

// cmdCapability implements 'veil capability list' and
// 'veil capability info <name>'.
func cmdCapability(args []string, stdout, stderr io.Writer, d deps) int {
	fs := flag.NewFlagSet("capability", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	reg := d.capabilities()
	res, rerr := capability.NewResolver(reg, d.catalog())
	if rerr != nil {
		fmt.Fprintf(stderr, "veil: %v\n", rerr)
		return 1
	}
	switch rest[0] {
	case "list":
		return capabilityList(d, reg, res, stdout, stderr)
	case "info":
		if len(rest) != 2 {
			fmt.Fprintln(stderr, "usage: veil capability info <name>")
			return 2
		}
		return capabilityInfo(reg, res, rest[1], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "veil capability: unknown command %q\n", rest[0])
		return 2
	}
}

func capabilityList(d deps, reg *capability.Registry, res *capability.Resolver, stdout, stderr io.Writer) int {
	names, err := reg.List()
	if err != nil {
		fmt.Fprintf(stderr, "veil: %v\n", err)
		return 1
	}
	if len(names) == 0 {
		fmt.Fprintln(stdout, "No capabilities known.")
		return 0
	}
	statuses := map[string]string{}
	if entries, err := d.catalog().List(); err == nil {
		for _, e := range entries {
			statuses[e.Name] = string(e.Status)
		}
	}
	fmt.Fprintln(stdout, "CAPABILITY                 DOMAIN               EXPERIENCES")
	fmt.Fprintln(stdout, "----------------------------------------------------------------")
	for _, n := range names {
		m, lerr := reg.Load(n)
		if lerr != nil {
			continue
		}
		exps, _ := res.ExperiencesFor([]string{n})
		summary := strings.Join(exps, ", ")
		if summary == "" {
			summary = "(planned)"
		}
		fmt.Fprintf(stdout, "%-25s %-20s %s\n", n, m.Domain, summary)
	}
	return 0
}

func capabilityInfo(reg *capability.Registry, res *capability.Resolver, name string, stdout, stderr io.Writer) int {
	m, err := reg.Load(name)
	if err != nil {
		fmt.Fprintf(stderr, "veil: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Capability: %s\n", m.Name)
	fmt.Fprintf(stdout, "Domain: %s\n", m.Domain)
	fmt.Fprintf(stdout, "Tier: %s\n", m.Tier)
	fmt.Fprintf(stdout, "Description: %s\n", m.Description)
	exps, _ := res.ExperiencesFor([]string{name})
	if len(exps) == 0 {
		fmt.Fprintln(stdout, "Implementing experiences: (none yet — planned)")
	} else {
		fmt.Fprintln(stdout, "Implementing experiences:")
		for _, e := range exps {
			fmt.Fprintf(stdout, "  - %s\n", e)
		}
	}
	return 0
}
