// Command veil is the Veilbox v3 command-line interface.
//
// Veilbox is an Operations Platform for DevOps, SRE, Platform, Cloud,
// and Kubernetes engineers. Fedora manages the operating system;
// Veilbox manages the engineer.
//
// Usage is intent-oriented, not package-oriented:
//
//	veil status
//	veil doctor
//	veil profile
//	veil profile apply <name> [--capability ...]
//	veil experience list
//	veil experience install <name>
//	veil experience remove <name>
//	veil workspace
//	veil update
package main

import (
	"fmt"
	"os"
)

// version is overridden at build time via -ldflags.
var version = "0.0.1-dev"

const usage = `Veilbox v3 — Operations Platform for engineers

Usage:
  veil <command> [options]

Commands:
  profile      Manage engineer profiles (intent)
  experience   Manage installable experiences (capabilities)
  workspace    Manage the personalized operations workspace
  status       Show current Veilbox state
  doctor       Diagnose Veilbox and system health
  update       Update Veilbox components and experiences
  version      Print version information
  help         Show this help

Run 'veil <command> --help' for command details.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	switch os.Args[1] {
	case "help", "-h", "--help":
		fmt.Print(usage)
	case "version", "--version":
		fmt.Println("veil", version)
	case "status", "doctor", "profile", "experience", "workspace", "update":
		// Commands land in Day 2 as their engines land.
		fmt.Fprintf(os.Stderr, "veil %s: not implemented yet\n", os.Args[1])
		os.Exit(1)
	default:
		fmt.Fprintf(os.Stderr, "veil: unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
}
