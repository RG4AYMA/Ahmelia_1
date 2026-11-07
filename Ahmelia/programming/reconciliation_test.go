package programming

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func testReconciliationProject(t *testing.T, mainContent string) (string, MultiFilePlan) {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"go.mod":  "module reconcile\n\ngo 1.20\n",
		"main.go": mainContent,
		"main_test.go": `package main

import "testing"

func TestAdd(t *testing.T) {
	if Add(2, 3) != 5 {
		t.Fatal("bad sum")
	}
}
`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	project := CodeProject{
		ID: "reconcile", Name: "reconcile", Root: root, Language: "Go",
		BuildCommand: "go build ./...", TestCommand: "go test ./...",
	}
	plan := MultiFilePlan{
		ID: "plan-reconcile", Project: project,
		Changes: []MultiFileChange{
			{ID: "main", Operation: SourceCreate, File: SourceFile{Path: "main.go", Language: "Go", Role: RoleSource, ProjectID: project.ID}, Content: mainContent},
			{ID: "test", Operation: SourceCreate, File: SourceFile{Path: "main_test.go", Language: "Go", Role: RoleTest, ProjectID: project.ID}, Content: files["main_test.go"]},
		},
	}
	return root, plan
}

func TestReconcileMultiFilePlanSucceedsFromObservedFilesAndTests(t *testing.T) {
	mainContent := `package main

func Add(a, b int) int { return a + b }

func main() {}
`
	_, plan := testReconciliationProject(t, mainContent)
	result, err := ReconcileMultiFilePlan(context.Background(), plan)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if !result.Success || !result.Verified || !result.Observed || result.OutcomeStatus != "succeeded" {
		t.Fatalf("unexpected reconciled result: %+v", result)
	}
}

func TestReconcileMultiFilePlanFailsWhenFileDoesNotMatch(t *testing.T) {
	mainContent := `package main

func Add(a, b int) int { return a + b }

func main() {}
`
	_, plan := testReconciliationProject(t, mainContent)
	plan.Changes[0].Content = `package main

func Add(a, b int) int { return a - b }

func main() {}
`
	result, err := ReconcileMultiFilePlan(context.Background(), plan)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if result.Success || !result.Observed || result.OutcomeStatus != "failed" {
		t.Fatalf("expected observed failure: %+v", result)
	}
}

func TestReconcileMultiFilePlanFailsWhenFileIsMissing(t *testing.T) {
	mainContent := `package main

func Add(a, b int) int { return a + b }

func main() {}
`
	root, plan := testReconciliationProject(t, mainContent)
	if err := os.Remove(filepath.Join(root, "main.go")); err != nil {
		t.Fatalf("remove main: %v", err)
	}
	result, err := ReconcileMultiFilePlan(context.Background(), plan)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if result.Success || !result.Observed || result.OutcomeStatus != "failed" {
		t.Fatalf("expected observed failure: %+v", result)
	}
}
