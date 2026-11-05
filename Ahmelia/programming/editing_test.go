package programming

import (
	memoryintelligence "ahmelia/memory_intelligence"
	"ahmelia/production"
	"os"
	"path/filepath"
	"testing"
)

func TestPreciseCodeEditorCreatesUniquePatch(t *testing.T) {
	file := SourceFile{Path: "main.go", Language: "go", Role: RoleEntrypoint, ProjectID: "p1"}
	spec := CodeEditSpec{ID: "patch-1", ProjectID: "p1", File: file, Expected: "return 1", Replace: "return 2"}
	got, err := NewPreciseCodeEditor().CreatePatch(spec, "func main() { return 1 }")
	if err != nil {
		t.Fatal(err)
	}
	if got.Patch.Before == got.Patch.After || got.Patch.After != "func main() { return 2 }" {
		t.Fatalf("unexpected patch: %#v", got.Patch)
	}
}

func TestPreciseCodeEditorRejectsAmbiguousTarget(t *testing.T) {
	file := SourceFile{Path: "main.go", Language: "go", Role: RoleSource, ProjectID: "p1"}
	spec := CodeEditSpec{ID: "patch-2", ProjectID: "p1", File: file, Expected: "x := 1", Replace: "x := 2"}
	if _, err := NewPreciseCodeEditor().CreatePatch(spec, "x := 1\nx := 1\n"); err == nil {
		t.Fatal("expected ambiguity error")
	}
}

type editingWorkspaceSource struct{}

func (editingWorkspaceSource) Snapshot() (memoryintelligence.WorkspaceSnapshot, error) {
	return memoryintelligence.WorkspaceSnapshot{}, nil
}

func TestApplyPatchRejectsStalePatch(t *testing.T) {
	root := t.TempDir()
	pw, err := production.NewWorkspace("w", editingWorkspaceSource{})
	if err != nil {
		t.Fatal(err)
	}
	ws, err := NewProgrammingWorkspace(pw, CodeProject{ID: "p1", Name: "p", Root: root})
	if err != nil {
		t.Fatal(err)
	}
	file := SourceFile{Path: "main.go", Language: "go", Role: RoleSource, ProjectID: "p1"}
	ops, err := NewOSSourceFileOperations(ws)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ops.Create(file, "package main\n"); err != nil {
		t.Fatal(err)
	}
	patch := Patch{ID: "stale", ProjectID: "p1", File: "main.go", Before: "package old\n", After: "package new\n"}
	if _, err = ApplyPatch(ops, patch); err == nil {
		t.Fatal("expected stale patch error")
	}
	data, _ := os.ReadFile(filepath.Join(root, "main.go"))
	if string(data) != "package main\n" {
		t.Fatal("file was unexpectedly modified")
	}
}
