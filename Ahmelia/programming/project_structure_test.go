package programming

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProjectStructureAnalyzer_MapsProject(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "src"))
	mustMkdir(t, filepath.Join(root, "tests"))
	mustMkdir(t, filepath.Join(root, "assets"))
	mustWrite(t, filepath.Join(root, "src", "main.go"), "package main\n")
	mustWrite(t, filepath.Join(root, "tests", "main_test.go"), "package main\n")
	mustWrite(t, filepath.Join(root, "README.md"), "readme\n")

	project := CodeProject{ID: "p1", Root: root, SourceRoots: []string{"src"}, TestRoots: []string{"tests"}}
	got, err := NewOSProjectStructureAnalyzer().AnalyzeProjectStructure(project)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Directories) != 3 {
		t.Fatalf("directories=%d, want 3", len(got.Directories))
	}
	if len(got.Files) != 2 {
		t.Fatalf("source files=%d, want 2", len(got.Files))
	}
	if got.Files[0].ProjectID != project.ID {
		t.Fatal("file project identity lost")
	}
}

func TestProjectStructureAnalyzer_EmptyRootIsValid(t *testing.T) {
	root := t.TempDir()
	got, err := NewOSProjectStructureAnalyzer().AnalyzeProjectStructure(CodeProject{ID: "p1", Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Files) != 0 {
		t.Fatal("expected no source files")
	}
}

func TestProjectStructureAnalyzer_RejectsMissingRoot(t *testing.T) {
	_, err := NewOSProjectStructureAnalyzer().AnalyzeProjectStructure(CodeProject{ID: "p1", Root: filepath.Join(t.TempDir(), "missing")})
	if err == nil {
		t.Fatal("expected missing root error")
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
}
func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
