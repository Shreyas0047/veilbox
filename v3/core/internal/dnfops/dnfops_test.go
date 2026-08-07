package dnfops

import (
	"errors"
	"strings"
	"testing"
)

func TestIsInstalled(t *testing.T) {
	f := newFakeRunner()
	f.responses["rpm -q veilbox-core"] = "veilbox-core-0.1.0-1.fc44.x86_64"
	s := NewWithRunner(f)

	ok, err := s.IsInstalled("veilbox-core")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected installed")
	}
}

func TestIsInstalledMissing(t *testing.T) {
	f := newFakeRunner()
	f.notInstalled("nope")
	s := NewWithRunner(f)

	ok, err := s.IsInstalled("nope")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected not installed")
	}
}

func TestIsInstalledError(t *testing.T) {
	f := newFakeRunner()
	f.errByCmd["rpm -q bad"] = errors.New("rpm db corrupted")
	s := NewWithRunner(f)

	if _, err := s.IsInstalled("bad"); err == nil {
		t.Fatal("expected error for non-missing-package failure")
	}
}

func TestListInstalledByPrefix(t *testing.T) {
	f := newFakeRunner()
	f.responses["rpm -qa --queryformat %{NAME}\n"] = "veilbox-core\nveilbox-experience-networking-tools\nbash\n"
	s := NewWithRunner(f)

	got, err := s.ListInstalledByPrefix("veilbox-experience-")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0] != "veilbox-experience-networking-tools" {
		t.Fatalf("got %v", got)
	}
}

func TestTransactionUsesSudoDnfByName(t *testing.T) {
	f := newFakeRunner()
	s := NewWithRunner(f)

	if err := s.Transaction("install", "-y", "veilbox-experience-networking-tools"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := f.joined()
	want := "interactive:sudo dnf install -y veilbox-experience-networking-tools"
	if len(got) != 1 || got[0] != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRepos(t *testing.T) {
	f := newFakeRunner()
	f.responses["dnf repolist"] = "repo id       repo name\nveilbox-dev   Veilbox Development Repository\n"
	s := NewWithRunner(f)

	out, err := s.Repos()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "veilbox-dev") {
		t.Fatalf("repo not listed: %q", out)
	}
}
