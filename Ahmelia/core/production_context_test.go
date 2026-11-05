package core

import (
	"path/filepath"
	"testing"
)

func TestProductionContextResolvesProjectAndArtifacts(t *testing.T) {
	root := filepath.Join(`C:\Users\User\Desktop`, "AhmeliaProduced", "hello_ahmelia")
	ctx := &ProductionContext{}
	ctx.Set("production-hello", "programming", ProductionProject{ID: "hello_ahmelia", Name: "hello_ahmelia", Path: root}, []ProductionArtifact{
		{Name: "go.mod", Path: filepath.Join(root, "go.mod"), Type: "source"},
		{Name: "main.go", Path: filepath.Join(root, "main.go"), Type: "source"},
		{Name: "main_test.go", Path: filepath.Join(root, "main_test.go"), Type: "test"},
		{Name: "hello_ahmelia.exe", Path: filepath.Join(root, "hello_ahmelia.exe"), Type: "executable"},
	})

	a, ok := ctx.ResolveArtifact("hello_ahmelia.exe")
	if !ok {
		t.Fatal("expected executable to resolve")
	}
	if a.Path != filepath.Join(root, "hello_ahmelia.exe") {
		t.Fatalf("unexpected artifact path: %q", a.Path)
	}

	project, ok := ctx.DescribeProject()
	if !ok {
		t.Fatal("expected project context")
	}
	if project == "" {
		t.Fatal("expected project description")
	}

	files, ok := ctx.DescribeArtifacts()
	if !ok {
		t.Fatal("expected artifact description")
	}
	for _, name := range []string{"go.mod", "main.go", "main_test.go", "hello_ahmelia.exe"} {
		if !contains(files, name) {
			t.Fatalf("artifact %q missing from %q", name, files)
		}
	}
}

func contains(s, needle string) bool {
	for i := 0; i+len(needle) <= len(s); i++ {
		if s[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
