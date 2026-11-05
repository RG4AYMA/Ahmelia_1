package programming

import (
	"os"
	"path/filepath"
	"testing"

	"ahmelia/memory_intelligence"
	"ahmelia/production"
)

type sourceTestSnapshot struct{}

func newSourceWorkspace(t *testing.T) (*ProgrammingWorkspace, string) {
	t.Helper()
	root := t.TempDir()
	src := &sourceTestSource{}
	pw, err := production.NewWorkspace("ws-source", src)
	if err != nil {
		t.Fatal(err)
	}
	project := CodeProject{ID: "proj-source", Name: "demo", Root: root, Language: "go"}
	workspace, err := NewProgrammingWorkspace(pw, project)
	if err != nil {
		t.Fatal(err)
	}
	return workspace, root
}

type sourceTestSource struct{}

func (sourceTestSource) Snapshot() (memory_intelligence.WorkspaceSnapshot, error) {
	return memory_intelligence.WorkspaceSnapshot{}, nil
}

func sourceFile(path string) SourceFile {
	return SourceFile{Path: path, Language: "go", Role: RoleSource, ProjectID: "proj-source"}
}

func TestSourceFileOperations_CreateReadWriteEdit(t *testing.T) {
	workspace, _ := newSourceWorkspace(t)
	ops, err := NewOSSourceFileOperations(workspace)
	if err != nil {
		t.Fatal(err)
	}
	file := sourceFile("internal/main.go")
	if _, err := ops.Create(file, "package main\n"); err != nil {
		t.Fatal(err)
	}
	got, err := ops.Read(file)
	if err != nil || got.Content != "package main\n" {
		t.Fatalf("read: %v %#v", err, got)
	}
	if _, err := ops.Write(file, "package main\nfunc main() {}\n"); err != nil {
		t.Fatal(err)
	}
	edited, err := ops.Edit(file, "func main() {}", "func main() { println(1) }")
	if err != nil {
		t.Fatal(err)
	}
	if edited.Before == edited.After || edited.Content != edited.After {
		t.Fatal("edit no preservó before/after")
	}
}

func TestSourceFileOperations_MoveRenameDelete(t *testing.T) {
	workspace, root := newSourceWorkspace(t)
	ops, _ := NewOSSourceFileOperations(workspace)
	file := sourceFile("main.go")
	if _, err := ops.Create(file, "package main\n"); err != nil {
		t.Fatal(err)
	}
	moved, err := ops.Move(file, "cmd/main.go")
	if err != nil {
		t.Fatal(err)
	}
	renamed, err := ops.Rename(moved.File, "entry.go")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "cmd", "entry.go")); err != nil {
		t.Fatal(err)
	}
	if _, err := ops.Delete(renamed.File); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "cmd", "entry.go")); !os.IsNotExist(err) {
		t.Fatal("archivo no fue eliminado")
	}
}

func TestSourceFileOperations_RejectsProjectMismatchAndEscape(t *testing.T) {
	workspace, _ := newSourceWorkspace(t)
	ops, _ := NewOSSourceFileOperations(workspace)
	wrong := SourceFile{Path: "x.go", Language: "go", ProjectID: "other"}
	if _, err := ops.Create(wrong, ""); err == nil {
		t.Fatal("project mismatch aceptado")
	}
	escape := sourceFile(filepath.Join("..", "outside.go"))
	if _, err := ops.Create(escape, ""); err == nil {
		t.Fatal("path traversal aceptado")
	}
}

func TestSourceFileOperations_RejectsOverwriteAndMissingEditTarget(t *testing.T) {
	workspace, _ := newSourceWorkspace(t)
	ops, _ := NewOSSourceFileOperations(workspace)
	file := sourceFile("main.go")
	if _, err := ops.Create(file, "one\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := ops.Create(file, "two\n"); err == nil {
		t.Fatal("create sobrescribió archivo")
	}
	if _, err := ops.Edit(file, "missing", "x"); err == nil {
		t.Fatal("edit aceptó target inexistente")
	}
}
