package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"ahmelia/environment"
	"ahmelia/production"
	"ahmelia/programming"
)

type workspaceAuthoritySynth struct{}

func (workspaceAuthoritySynth) Synthesize(_ context.Context, o programming.CodeObjective) (*programming.MultiFilePlan, error) {
	return &programming.MultiFilePlan{ID: "workspace-authority", Project: programming.CodeProject{ID: o.ProjectID, Name: o.ProjectID, Root: o.ProjectRoot, Language: "Go", SourceRoots: []string{"."}, TestRoots: []string{"."}, BuildCommand: "go build ./...", TestCommand: "go test ./...", RuntimeCommand: "go run ."}, Changes: []programming.MultiFileChange{
		{ID: "source", Operation: programming.SourceCreate, File: programming.SourceFile{Path: "feature.go", Language: "Go", Role: programming.RoleSource, ProjectID: o.ProjectID}, Content: "package main\n\nfunc Feature() int { return 42 }\n"},
		{ID: "test", Operation: programming.SourceCreate, File: programming.SourceFile{Path: "feature_test.go", Language: "Go", Role: programming.RoleTest, ProjectID: o.ProjectID}, Content: "package main\n\nimport \"testing\"\n\nfunc TestFeature(t *testing.T) { if Feature() != 42 { t.Fatal(\"bad\") } }\n"},
	}}, nil
}

func TestProgrammingProviderUsesExplicitWorkspaceRootForNewWork(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example\n\ngo 1.23.2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	world := environment.NewManager()
	provider, err := NewProgrammingProviderWithSynthesizer(world, workspaceAuthoritySynth{})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("AHMELIA_WORKSPACE_ROOT", root)
	result := provider.Execute(context.Background(), production.ProviderRequest{Input: "Implement a Go feature and test it", Project: production.ProductionProject{ID: "workspace"}})
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if result.Project.Path != root {
		t.Fatalf("project path = %q, want %q", result.Project.Path, root)
	}
	if _, err := os.Stat(filepath.Join(root, "feature.go")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "feature_test.go")); err != nil {
		t.Fatal(err)
	}
}
