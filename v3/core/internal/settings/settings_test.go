package settings

import (
	"os"
	"path/filepath"
	"testing"
)

// withStateDir points the state dir at a temp directory.
func withStateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	return dir
}

func TestStateDir(t *testing.T) {
	base := withStateDir(t)
	dir, err := StateDir()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dir != filepath.Join(base, "veilbox") {
		t.Fatalf("got %q", dir)
	}
}

func TestStateRoundtrip(t *testing.T) {
	withStateDir(t)
	st := State{ActiveProfile: "devops", AppliedAt: "2026-08-07T00:00:00Z", Version: "0.1.0"}
	if err := SaveState(st); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := LoadState()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got != st {
		t.Fatalf("roundtrip mismatch: %+v != %+v", got, st)
	}
}

func TestLoadStateMissing(t *testing.T) {
	withStateDir(t)
	st, err := LoadState()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.ActiveProfile != "" {
		t.Fatalf("expected empty state, got %+v", st)
	}
}

func TestLoadStateCorrupt(t *testing.T) {
	dir := withStateDir(t)
	os.MkdirAll(filepath.Join(dir, "veilbox"), 0o700)
	os.WriteFile(filepath.Join(dir, "veilbox", "state.json"), []byte("{not json"), 0o600)
	if _, err := LoadState(); err == nil {
		t.Fatal("expected error for corrupt state")
	}
}

func TestEnsureStateDir(t *testing.T) {
	withStateDir(t)
	dir, err := EnsureStateDir()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("state dir not created: %v", err)
	}
}

func TestSaveStatePreservesStateDir(t *testing.T) {
	withStateDir(t)
	if err := SaveState(State{ActiveProfile: "devops"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	st, err := LoadState()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if st.ActiveProfile != "devops" {
		t.Fatalf("got %+v", st)
	}
}
