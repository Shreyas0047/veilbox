// Package safetoken validates simple shell-safe tokens used across
// Veilbox declarative configuration (workspace preferences, desktop
// experience components, aliases, environment values).
//
// A safe token can only contain letters, digits and a restricted set
// of punctuation. It can never introduce shell metacharacters, so
// declarative manifests can never smuggle in arbitrary shell.
package safetoken

import "regexp"

// Token matches a single shell-safe token.
var Token = regexp.MustCompile(`^[A-Za-z0-9_./:+\-=~]+$`)

// Valid reports whether s is a shell-safe token.
func Valid(s string) bool {
	return Token.MatchString(s)
}

// Command matches a simple command: one or more safe tokens joined by
// single spaces (e.g. "kubectl get pods").
var Command = regexp.MustCompile(`^[A-Za-z0-9_./:+\-=~]+(?: [A-Za-z0-9_./:+\-=~]+)*$`)

// ValidCommand reports whether s is a simple safe command.
func ValidCommand(s string) bool {
	return Command.MatchString(s)
}
