// Package dnfops wraps Fedora-native package operations.
//
// Veilbox never bypasses DNF/RPM. Installed-state queries go through
// rpm (user-level, no privilege required); transactions go through
// dnf with the calling user's normal privileges (sudo prompting where
// configured). This package shells out to the real system tools and
// nothing else.
package dnfops

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Runner executes commands. The default Runner uses os/exec; tests
// substitute a fake to assert command behavior without touching the
// system.
type Runner interface {
	Run(name string, args ...string) (string, error)
	// RunInteractive executes a command connected to the calling
	// terminal (used for sudo-prompting transactions).
	RunInteractive(name string, args ...string) error
}

// ExecRunner is the production Runner backed by os/exec.
type ExecRunner struct{}

func (ExecRunner) Run(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return out.String(), err
	}
	return out.String(), nil
}

func (ExecRunner) RunInteractive(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// System is the DNF/RPM facade used by Veilbox engines.
type System struct {
	runner Runner
}

// New returns a System backed by real rpm/dnf/sudo binaries.
func New() *System {
	return &System{runner: ExecRunner{}}
}

// NewWithRunner returns a System using the given Runner (tests).
func NewWithRunner(r Runner) *System {
	return &System{runner: r}
}

// Binaries used by Veilbox. Indirection keeps test fakes honest and
// lets doctor report availability.
const (
	RPMBinary = "rpm"
	DNFBinary = "dnf"
	SudoBinary = "sudo"
)

// Available reports whether the given binary exists on PATH.
func Available(binary string) bool {
	_, err := exec.LookPath(binary)
	return err == nil
}

// IsInstalled reports whether a package is present in the RPM database.
// rpm -q exits non-zero for a missing package; the "is not installed"
// diagnostic is the authoritative signal, so fakes stay honest.
func (s *System) IsInstalled(pkg string) (bool, error) {
	out, err := s.runner.Run(RPMBinary, "-q", pkg)
	if err == nil {
		return true, nil
	}
	if strings.Contains(out, "is not installed") {
		return false, nil
	}
	return false, fmt.Errorf("rpm -q %s: %w", pkg, err)
}

// ListInstalledByPrefix lists installed packages whose name starts
// with the given prefix (e.g. "veilbox-experience-").
func (s *System) ListInstalledByPrefix(prefix string) ([]string, error) {
	out, err := s.runner.Run(RPMBinary, "-qa", "--queryformat", "%{NAME}\n")
	if err != nil {
		return nil, fmt.Errorf("rpm -qa: %w", err)
	}
	var names []string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, prefix) {
			names = append(names, line)
		}
	}
	return names, nil
}

// Repos returns the currently configured repositories (dnf repolist).
func (s *System) Repos() (string, error) {
	out, err := s.runner.Run(DNFBinary, "repolist")
	if err != nil {
		return "", fmt.Errorf("dnf repolist: %w", err)
	}
	return out, nil
}

// Transaction runs a dnf transaction with elevated privileges. The
// privileged prefix is appended to the calling user's sudo
// configuration; Veilbox never assumes passwordless sudo exists.
func (s *System) Transaction(args ...string) error {
	full := append([]string{DNFBinary}, args...)
	if err := s.runner.RunInteractive(SudoBinary, full...); err != nil {
		return fmt.Errorf("dnf %s: %w", strings.Join(args, " "), err)
	}
	return nil
}
