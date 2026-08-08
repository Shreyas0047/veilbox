package profile

import (
	"os"
	"path/filepath"
	"testing"
)

const devopsYAML = `name: devops
display_name: DevOps Engineer
description: Builds and operates delivery pipelines.
role: devops
recommended_capabilities:
  - base-operations
  - networking
  - terminal-operations
optional_capabilities:
  - observability
tags: [ci, delivery, operations]
workspace_preferences:
  shell: bash
  editor: vim
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
	if m.Name != "devops" || m.DisplayName != "DevOps Engineer" || m.Description == "" {
		t.Fatalf("bad manifest: %+v", m)
	}
	if len(m.Recommended) != 3 || m.Recommended[0] != "base-operations" {
		t.Fatalf("bad recommended: %+v", m.Recommended)
	}
	if len(m.Optional) != 1 || m.Optional[0] != "observability" {
		t.Fatalf("bad optional: %+v", m.Optional)
	}
	if len(m.Tags) != 3 || m.Workspace.Shell != "bash" || m.Workspace.Editor != "vim" {
		t.Fatalf("bad metadata: %+v %+v", m.Tags, m.Workspace)
	}
}

func TestLoadDefaults(t *testing.T) {
	dir := t.TempDir()
	writeProfile(t, dir, "sre", "name: sre\ndescription: keeps systems reliable.\n")
	m, err := NewRegistryDir(dir).Load("sre")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if m.DisplayName != "sre" || m.Role != "sre" {
		t.Fatalf("defaults not applied: %+v", m)
	}
}

func TestAllReferences(t *testing.T) {
	dir := t.TempDir()
	writeProfile(t, dir, "devops", devopsYAML)
	m, err := NewRegistryDir(dir).Load("devops")
	if err != nil {
		t.Fatal(err)
	}
	got := m.AllReferences()
	if len(got) != 4 || got[0] != "base-operations" || got[3] != "terminal-operations" {
		t.Fatalf("references: %v", got)
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
	writeProfile(t, dir, "devops", "name: other\ndescription: x\n")
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

func TestLoadValidation(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{"missing description", "name: devops\n"},
		{"bad name chars", "name: Dev Ops\ndescription: x\n"},
		{"duplicate recommended", "name: devops\ndescription: x\nrecommended_capabilities: [a, a]\n"},
		{"bad reference", "name: devops\ndescription: x\nrecommended_capabilities: [a b]\n"},
		{"invalid filename", "name: ../evil\ndescription: x\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeProfile(t, dir, "devops", tc.content)
			if _, err := NewRegistryDir(dir).Load("devops"); err == nil {
				t.Fatalf("expected validation error for %q", tc.content)
			}
		})
	}
}

func TestList(t *testing.T) {
	dir := t.TempDir()
	writeProfile(t, dir, "devops", devopsYAML)
	writeProfile(t, dir, "sre", "name: sre\ndescription: x\n")
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
