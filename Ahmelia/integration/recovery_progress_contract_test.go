package integration

import (
	"os"
	"path/filepath"
	"testing"

	"ahmelia/programming"
)

func TestRecoveryProgressRequiresChangeToDiagnosedFile(t *testing.T) {
	root := t.TempDir()
	failing := "package main\nfunc Broken() int { return Missing }\n"
	unrelated := "package main\nfunc Unrelated() {}\n"
	if err := os.WriteFile(filepath.Join(root, "failing.go"), []byte(failing), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "unrelated.go"), []byte(unrelated), 0o600); err != nil {
		t.Fatal(err)
	}
	objective := "tests failed: ./failing.go:2: undefined: Missing"
	plan := &programming.MultiFilePlan{Changes: []programming.MultiFileChange{{
		Operation: programming.SourceWrite,
		File:      programming.SourceFile{Path: "unrelated.go"},
		Content:   unrelated + "// unrelated edit\n",
	}}}
	if planHasEffectiveRepairChanges(plan, root, objective) {
		t.Fatal("editing an unrelated file must not count as repair progress")
	}

	plan.Changes = append(plan.Changes, programming.MultiFileChange{
		Operation: programming.SourceWrite,
		File:      programming.SourceFile{Path: "failing.go"},
		Content:   failing,
	})
	if planHasEffectiveRepairChanges(plan, root, objective) {
		t.Fatal("re-emitting unchanged diagnosed file must not count as repair progress")
	}

	plan.Changes[1].Content = "package main\nfunc Broken() int { return 42 }\n"
	if !planHasEffectiveRepairChanges(plan, root, objective) {
		t.Fatal("a material change to the diagnosed file should count as progress")
	}
}
