package profile

import (
	"os"
	"path/filepath"
	"testing"
)

const devopsYAML = `name: devops
description: DevOps engineer — builds and operates delivery pipelines.
capabilities:
  containers:
    - podman
  infrastructure:
    - ansible
    - terraform
`

func writeProfile(t *testing.T, dir, name, content string) {
	t.Helper()
	os.MkdirAll(dir, 0o755)
	if err := os.WriteFile(filepath.Join(dir, name+".yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	writeProfile(t, dir, "devops", devopsYAML)
	m, err := NewRegistryDir(dir).Load("devops")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if m.Name != "devops" || m.Description == "" {
		t.Fatalf("bad manifest: %+v", m)
	}
	if len(m.Capabilities["containers"]) != 1 || m.Capabilities["containers"][0] != "podman" {
		t.Fatalf("bad capabilities: %+v", m.Capabilities)
	}
	if got := m.CapabilityNames(); len(got) != 3 || got[0] != "ansible" {
		t.Fatalf("capability names: %v", got)
	}
}

func TestLoadUnknown(t *testing.T) {
	dir := t.TempDir()
	if _, err := NewRegistryDir(dir).Load("nope"); err == nil {
		t.Fatal("expected error for unknown profile")
	}
}

func TestLoadNameMismatch(t *testing.T) {
	dir := t.TempDir()
	writeProfile(t, dir, "devops", "name: other\n")
	if _, err := NewRegistryDir(dir).Load("devops"); err == nil {
		t.Fatal("expected error for name mismatch")
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	writeProfile(t, dir, "devops", "name: [unclosed\n")
	if _, err := NewRegistryDir(dir).Load("devops"); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestList(t *testing.T) {
	dir := t.TempDir()
	writeProfile(t, dir, "devops", devopsYAML)
	writeProfile(t, dir, "sre", "name: sre\n")
	names, err := NewRegistryDir(dir).List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(names) != 2 || names[0] != "devops" || names[1] != "sre" {
		t.Fatalf("got %v", names)
	}
}

// TestApplyWritesState verifies profile apply persists intent state
// and installs nothing.
func TestApplyWritesState(t *testing.T) {
	dir := t.TempDir()
	writeProfile(t, dir, "devops", devopsYAML)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	st, err := ApplyWith(NewRegistryDir(dir), "devops")
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if st.ActiveProfile != "devops" || st.AppliedAt == "" {
		t.Fatalf("bad state: %+v", st)
	}
	got, err := Active()
	if err != nil {
		t.Fatalf("active: %v", err)
	}
	if got.ActiveProfile != "devops" {
		t.Fatalf("persisted state: %+v", got)
	}
}

func TestApplyUnknownProfile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if _, err := Apply("nope"); err == nil {
		t.Fatal("expected error for unknown profile")
	}
}
