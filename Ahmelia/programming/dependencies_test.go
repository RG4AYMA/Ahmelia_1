package programming

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDependencyRequestValidation(t *testing.T) {
	project := CodeProject{ID: "p", Root: t.TempDir()}
	req := DependencyRequest{ID: "d", Project: project, Operation: DependencyInstall, Dependency: Dependency{Name: "example.com/x", Manager: "go", ProjectID: "p"}}
	if err := req.Validate(); err != nil {
		t.Fatal(err)
	}
	req.Operation = "bad"
	if err := req.Validate(); err == nil {
		t.Fatal("expected invalid operation")
	}
}

func TestGoDependencyManagerResolve(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/app\n\ngo 1.23.2\n\nrequire (\n\tgithub.com/a/b v1.2.3\n\tgolang.org/x/x v0.1.0 // indirect\n)\n"), 0644); err != nil {
		t.Fatal(err)
	}
	p := CodeProject{ID: "p", Root: root}
	got := NewGoDependencyManager().Resolve(context.Background(), DependencyRequest{ID: "r", Project: p, Operation: DependencyResolve})
	if !got.Success || len(got.Dependencies) != 2 {
		t.Fatalf("unexpected result: %+v", got)
	}
	if got.Dependencies[0].Manager != "go" || got.Dependencies[0].ProjectID != "p" {
		t.Fatalf("bad dependency metadata: %+v", got.Dependencies[0])
	}
}

func TestGoDependencyManagerRejectsMissingGoMod(t *testing.T) {
	p := CodeProject{ID: "p", Root: t.TempDir()}
	got := NewGoDependencyManager().Resolve(context.Background(), DependencyRequest{ID: "r", Project: p, Operation: DependencyResolve})
	if got.Success || len(got.Errors) == 0 {
		t.Fatalf("expected failure: %+v", got)
	}
}

func TestDependencyOperationsRequireDependency(t *testing.T) {
	p := CodeProject{ID: "p", Root: t.TempDir()}
	req := DependencyRequest{ID: "x", Project: p, Operation: DependencyInstall}
	if err := req.Validate(); err == nil {
		t.Fatal("expected dependency validation failure")
	}
}
