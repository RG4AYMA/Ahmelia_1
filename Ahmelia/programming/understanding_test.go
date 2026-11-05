package programming

import (
	memoryintelligence "ahmelia/memory_intelligence"
	"ahmelia/production"
	"os"
	"path/filepath"
	"testing"
)

func TestGoAnalyzerFindsStructureAndDependencies(t *testing.T) {
	file := SourceFile{Path: "main.go", Language: "go", Role: RoleEntrypoint, ProjectID: "p"}
	src := `package main
import "fmt"
type User struct { Name string }
type Store interface { Get() }
const Version = "1"
var global = 1
func main() { fmt.Println(global) }
func (u *User) Get() {}
`
	a, err := (GoAnalyzer{}).AnalyzeFile(file, src)
	if err != nil {
		t.Fatal(err)
	}
	if a.Package != "main" {
		t.Fatalf("package = %q", a.Package)
	}
	if len(a.Imports) != 1 || a.Imports[0].Path != "fmt" {
		t.Fatalf("imports = %#v", a.Imports)
	}
	kinds := map[SymbolKind]bool{}
	for _, s := range a.Symbols {
		kinds[s.Kind] = true
	}
	for _, k := range []SymbolKind{SymbolPackage, SymbolImport, SymbolStruct, SymbolInterface, SymbolConstant, SymbolVariable, SymbolFunction, SymbolMethod} {
		if !kinds[k] {
			t.Errorf("missing symbol kind %q", k)
		}
	}
}

func TestUnderstandingRejectsUnknownLanguage(t *testing.T) {
	u := NewCodeUnderstanding()
	_, err := u.AnalyzeFile(SourceFile{Path: "x.py", Language: "python", Role: RoleSource, ProjectID: "p"}, "print(1)")
	if err == nil {
		t.Fatal("expected unknown language error")
	}
}

func TestAnalyzeProjectDeterministicDependencies(t *testing.T) {
	u := NewCodeUnderstanding()
	if err := u.RegisterAnalyzer(GoAnalyzer{}); err != nil {
		t.Fatal(err)
	}
	p := CodeProject{ID: "p", Root: "/tmp/p"}
	files := []SourceFile{{Path: "b.go", Language: "go", Role: RoleSource, ProjectID: "p"}, {Path: "a.go", Language: "go", Role: RoleSource, ProjectID: "p"}}
	contents := map[string]string{"a.go": `package p
import "z"`, "b.go": `package p
import "a"`}
	got, err := u.AnalyzeProject(p, files, contents)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Files) != 2 || got.Files[0].File.Path != "a.go" {
		t.Fatalf("files not sorted: %#v", got.Files)
	}
	if len(got.Dependencies) != 2 || got.Dependencies[0] != "a" || got.Dependencies[1] != "z" {
		t.Fatalf("deps = %#v", got.Dependencies)
	}
}

func TestAnalyzeProjectFromWorkspace(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc main(){}"), 0644); err != nil {
		t.Fatal(err)
	}
	p := CodeProject{ID: "p", Root: root}
	prodWS, err := production.NewWorkspace("w", testWorkspaceSource{})
	if err != nil {
		t.Fatal(err)
	}
	ws, err := NewProgrammingWorkspace(prodWS, p)
	if err != nil {
		t.Fatal(err)
	}
	u := NewCodeUnderstanding()
	if err := u.RegisterAnalyzer(GoAnalyzer{}); err != nil {
		t.Fatal(err)
	}
	got, err := u.AnalyzeProjectFromWorkspace(p, ws)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Files) != 1 || got.Files[0].Package != "main" {
		t.Fatalf("analysis = %#v", got)
	}
}

type testWorkspaceSource struct{}

func (testWorkspaceSource) Snapshot() (memoryintelligence.WorkspaceSnapshot, error) {
	return memoryintelligence.WorkspaceSnapshot{}, nil
}
