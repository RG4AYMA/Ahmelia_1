package integration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ahmelia/programming"
)

type fakeStructuredGenerator struct {
	response string
	err      error
	prompt   string
}

func (f *fakeStructuredGenerator) GenerateStructuredContext(_ context.Context, prompt string, _ json.RawMessage, _ int) (string, error) {
	f.prompt = prompt
	return f.response, f.err
}

func TestCodeObjectiveSynthesisRequiresCompleteGoProject(t *testing.T) {
	fake := &fakeStructuredGenerator{response: `{"files":[{"path":"go.mod","content":"module example\n\ngo 1.20\n","role":"config"},{"path":"main.go","content":"package main\n\nimport \"fmt\"\nfunc Average(a,b,c int) int { return (a+b+c)/3 }\nfunc main(){ fmt.Println(Average(3,6,9)) }\n","role":"entrypoint"},{"path":"main_test.go","content":"package main\n\nimport \"testing\"\nfunc TestAverage(t *testing.T){ if got:=Average(3,6,9); got!=6 { t.Fatalf(\"got %d\",got) } }\n","role":"test"}]}`}
	s := &OllamaCodeObjectiveSynthesizer{Client: fake}
	plan, err := s.Synthesize(context.Background(), programming.CodeObjective{Raw: "Crea un programa Go que calcule el promedio de tres números", Language: "Go", ProjectID: "demo", ProjectRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Changes) != 3 {
		t.Fatalf("changes=%d, want 3", len(plan.Changes))
	}
	if plan.Changes[0].Operation != programming.SourceCreate {
		t.Fatalf("first operation=%q, want create", plan.Changes[0].Operation)
	}
	if !strings.Contains(fake.prompt, "Every test must exercise the actual production code") {
		t.Fatal("synthesis prompt lost the test-quality contract")
	}
	if !strings.Contains(fake.prompt, "Every import must be used") || !strings.Contains(fake.prompt, "Do not introduce external dependencies") {
		t.Fatal("synthesis prompt lost source correctness and dependency minimization contracts")
	}
	if !strings.Contains(fake.prompt, "Never import the project's own module") || !strings.Contains(fake.prompt, "tests must call production functions directly") {
		t.Fatal("synthesis prompt lost Go root-package test contract")
	}
}

func TestCodeObjectiveSynthesisExistingProjectUsesWrites(t *testing.T) {
	fake := &fakeStructuredGenerator{response: `{"files":[{"path":"go.mod","content":"module example\n\ngo 1.20\n","role":"config"},{"path":"main.go","content":"package main\nfunc Average(a,b,c int) int { return (a+b+c)/3 }\n","role":"source"},{"path":"main_test.go","content":"package main\nimport \"testing\"\nfunc TestAverage(t *testing.T){ if Average(3,6,9)!=6 { t.Fatal(\"bad\") } }\n","role":"test"}]}`}
	s := &OllamaCodeObjectiveSynthesizer{Client: fake}
	plan, err := s.Synthesize(context.Background(), programming.CodeObjective{Raw: "Modifica el programa para mantener el promedio y actualiza go.mod si es necesario", Language: "Go", ProjectID: "demo", ProjectRoot: t.TempDir(), Existing: true, Files: []programming.CodeObjectiveFile{{Path: "go.mod", Content: "module example\n\ngo 1.20\n"}, {Path: "main.go", Content: "package main\n"}, {Path: "main_test.go", Content: "package main\n"}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range plan.Changes {
		if c.Operation != programming.SourceWrite {
			t.Fatalf("existing project operation=%q for %s, want write", c.Operation, c.File.Path)
		}
	}
	if !strings.Contains(fake.prompt, "CURRENT PROJECT FILES") {
		t.Fatal("existing project state was not included in synthesis prompt")
	}
}

func TestCodeObjectiveSynthesisRepairsMissingGoModuleButRejectsMissingTests(t *testing.T) {
	t.Run("new project gets deterministic module metadata", func(t *testing.T) {
		fake := &fakeStructuredGenerator{response: `{"files":[{"path":"main.go","content":"package main\nfunc main(){}","role":"entrypoint"},{"path":"main_test.go","content":"package main\n","role":"test"}]}`}
		s := &OllamaCodeObjectiveSynthesizer{Client: fake}
		plan, err := s.Synthesize(context.Background(), programming.CodeObjective{Raw: "crea un programa Go", Language: "Go", ProjectID: "f10_repair", ProjectRoot: t.TempDir()})
		if err != nil {
			t.Fatal(err)
		}
		for _, change := range plan.Changes {
			if change.File.Path == "go.mod" {
				if change.Operation != programming.SourceCreate || !strings.Contains(change.Content, "module ahmelia.local/project-f10_repair") {
					t.Fatalf("unexpected generated module change: %+v", change)
				}
				return
			}
		}
		t.Fatal("missing deterministic go.mod change")
	})
	t.Run("missing test remains a synthesis contract failure", func(t *testing.T) {
		fake := &fakeStructuredGenerator{response: `{"files":[{"path":"go.mod","content":"module example\n","role":"config"},{"path":"main.go","content":"package main\nfunc main(){}","role":"entrypoint"}]}`}
		s := &OllamaCodeObjectiveSynthesizer{Client: fake}
		_, err := s.Synthesize(context.Background(), programming.CodeObjective{Raw: "crea un programa Go", Language: "Go", ProjectID: "x", ProjectRoot: t.TempDir()})
		if err == nil || !strings.Contains(err.Error(), "at least one _test.go") {
			t.Fatalf("expected missing-test contract failure, got %v", err)
		}
	})
}

func TestCodeObjectiveSynthesisDoesNotInventModuleForExistingProject(t *testing.T) {
	fake := &fakeStructuredGenerator{response: `{"files":[{"path":"main.go","content":"package main\nfunc main(){}","role":"entrypoint"},{"path":"main_test.go","content":"package main\n","role":"test"}]}`}
	s := &OllamaCodeObjectiveSynthesizer{Client: fake}
	_, err := s.Synthesize(context.Background(), programming.CodeObjective{Raw: "modifica el programa Go", Language: "Go", ProjectID: "existing", ProjectRoot: t.TempDir(), Existing: true})
	if err == nil || !strings.Contains(err.Error(), "existing Go project has no go.mod") {
		t.Fatalf("expected existing-project module identity failure, got %v", err)
	}
}

func TestCodeObjectiveSynthesisNormalizesProjectPrefixedPaths(t *testing.T) {
	fake := &fakeStructuredGenerator{response: `{"files":[{"path":"f10_repair/go.mod","content":"module f10_repair\n\ngo 1.20\n","role":"config"},{"path":"f10_repair/main.go","content":"package main\nfunc main() {}\n","role":"entrypoint"},{"path":"f10_repair/main_test.go","content":"package main\nimport \"testing\"\nfunc TestMain(t *testing.T) {}\n","role":"test"}]}`}
	plan, err := NewOllamaCodeObjectiveSynthesizerWithClient(fake).Synthesize(context.Background(), programming.CodeObjective{
		Raw: "Quiero crear un programa Go llamado f10_repair", Language: "Go", ProjectID: "f10_repair", ProjectRoot: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"go.mod": false, "main.go": false, "main_test.go": false}
	for _, change := range plan.Changes {
		if _, ok := want[change.File.Path]; !ok {
			t.Fatalf("unexpected path %q; expected project-root-relative paths", change.File.Path)
		}
		want[change.File.Path] = true
	}
	for path, found := range want {
		if !found {
			t.Errorf("missing normalized path %q", path)
		}
	}
}

func TestCodeObjectiveSynthesisRecoveryCanRepairSourceWithoutRewritingProtectedManifest(t *testing.T) {
	module := "module f10_repair\n\ngo 1.20\n"
	fake := &fakeStructuredGenerator{response: `{"files":[{"path":"f10_repair/go.mod","content":"module f10_repair\n\ngo 1.20\n","role":"config"},{"path":"f10_repair/main.go","content":"package main\nfunc main() { println(2) }\n","role":"entrypoint"},{"path":"f10_repair/main_test.go","content":"package main\nimport \"testing\"\nfunc TestMain(t *testing.T) { if 1+1 != 2 { t.Fatal(\"bad\") } }\n","role":"test"}]}`}
	files := []programming.CodeObjectiveFile{
		{Path: "go.mod", Content: module},
		{Path: "main.go", Content: "package main\nfunc main() { println(1) }\n"},
		{Path: "main_test.go", Content: "package main\nimport \"testing\"\nfunc TestMain(t *testing.T) { if 1+1 != 2 { t.Fatal(\"bad\") } }\n"},
	}
	plan, err := NewOllamaCodeObjectiveSynthesizerWithClient(fake).Synthesize(context.Background(), programming.CodeObjective{
		Raw:      "Quiero crear un programa Go llamado f10_repair\n\n[Recovery evidence]\ntests failed: exit status 1",
		Language: "Go", ProjectID: "f10_repair", ProjectRoot: t.TempDir(), Existing: true, Files: files,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Changes) != 2 {
		t.Fatalf("changes=%d, want only two source/test repairs with unchanged go.mod omitted: %+v", len(plan.Changes), plan.Changes)
	}
	for _, required := range []string{"concrete failed verification contract", "smallest source-level correction", "imported and not used", "CURRENT PROJECT FILES"} {
		if !strings.Contains(fake.prompt, required) {
			t.Errorf("recovery prompt missing targeted repair contract %q", required)
		}
	}
	for _, change := range plan.Changes {
		if change.File.Path == "go.mod" || strings.HasPrefix(change.File.Path, "f10_repair/") {
			t.Fatalf("recovery plan should preserve manifest and use root-relative paths, got %+v", change)
		}
		if change.Operation != programming.SourceWrite {
			t.Fatalf("recovery change %s operation=%s, want write", change.File.Path, change.Operation)
		}
	}
}

func TestCodeObjectiveSynthesisRecoveryIgnoresProtectedManifestMutationAndKeepsSourceRepair(t *testing.T) {
	module := "module ahmelia.local/project-f10_repair\n\ngo 1.20\n"
	fake := &fakeStructuredGenerator{response: `{"files":[{"path":"go.mod","content":"module f10_repair\n\ngo 1.20\n","role":"config"},{"path":"f10_repair.go","content":"package main\nfunc Repair() error { return nil }\n","role":"source"},{"path":"f10_repair_test.go","content":"package main\nimport \"testing\"\nfunc TestRepair(t *testing.T) { if err := Repair(); err != nil { t.Fatal(err) } }\n","role":"test"}]}`}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(module), 0o600); err != nil {
		t.Fatal(err)
	}
	files := []programming.CodeObjectiveFile{
		{Path: "go.mod", Content: module},
		{Path: "f10_repair.go", Content: "package main\nfunc Repair() error { return nil }\n"},
		{Path: "f10_repair_test.go", Content: "package main\nimport (\"testing\"; \"f10_repair\")\nfunc TestRepair(t *testing.T) { _ = f10_repair.Repair() }\n"},
	}
	plan, err := NewOllamaCodeObjectiveSynthesizerWithClient(fake).Synthesize(context.Background(), programming.CodeObjective{
		Raw: "Crea un programa Go llamado f10_repair", RecoveryEvidence: "stage=build; failure=f10_repair_test.go:6: package f10_repair is not in std", Language: "Go", ProjectID: "f10_repair", ProjectRoot: root, Existing: true, Files: files,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Changes) != 2 {
		t.Fatalf("changes=%d, want only source/test repair after protected manifest proposal is ignored: %+v", len(plan.Changes), plan.Changes)
	}
	if !strings.Contains(fake.prompt, "stage=build; failure=f10_repair_test.go:6: package f10_repair is not in std") {
		t.Fatalf("typed recovery evidence was not passed to synthesis prompt: %s", fake.prompt)
	}
	for _, change := range plan.Changes {
		if change.File.Path == "go.mod" {
			t.Fatal("recovery plan must never mutate protected go.mod implicitly")
		}
	}
	got, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != module {
		t.Fatalf("protected module changed: got %q want %q", got, module)
	}
}

func TestCodeObjectiveSynthesisCreatesNewFilesInExistingProject(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example\n\ngo 1.23.2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	fake := &fakeStructuredGenerator{response: `{"files":[{"path":"feature.go","content":"package main\nfunc Feature() int { return 42 }\n","role":"source"},{"path":"feature_test.go","content":"package main\nimport \"testing\"\nfunc TestFeature(t *testing.T){ if Feature()!=42 { t.Fatal(\"bad\") } }\n","role":"test"}]}`}
	s := NewOllamaCodeObjectiveSynthesizerWithClient(fake)
	plan, err := s.Synthesize(context.Background(), programming.CodeObjective{Raw: "Crea una nueva función Feature y añade tests", Language: "Go", ProjectID: "demo", ProjectRoot: root, Existing: true, Files: []programming.CodeObjectiveFile{{Path: "go.mod", Content: "module example\n\ngo 1.23.2\n"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Changes) != 2 {
		t.Fatalf("changes=%d", len(plan.Changes))
	}
	for _, c := range plan.Changes {
		if c.Operation != programming.SourceCreate {
			t.Fatalf("%s operation=%s, want create", c.File.Path, c.Operation)
		}
	}
}

func TestCodeObjectiveSynthesisRejectsUnrelatedExistingWriteForCreation(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example\n\ngo 1.23.2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	fake := &fakeStructuredGenerator{response: `{"files":[{"path":"main.go","content":"package main\nfunc main() { println(1) }\n","role":"entrypoint"},{"path":"feature_test.go","content":"package main\nimport \"testing\"\nfunc TestFeature(t *testing.T) {}\n","role":"test"}]}`}
	s := NewOllamaCodeObjectiveSynthesizerWithClient(fake)
	_, err := s.Synthesize(context.Background(), programming.CodeObjective{Raw: "Crea una nueva función Feature y añade tests", Language: "Go", ProjectID: "demo", ProjectRoot: root, Existing: true, Files: []programming.CodeObjectiveFile{{Path: "go.mod", Content: "module example\n\ngo 1.23.2\n"}, {Path: "main.go", Content: "package main\nfunc main() {}\n"}}})
	if err == nil || !strings.Contains(err.Error(), "cannot overwrite unrelated existing file") {
		t.Fatalf("expected unrelated write rejection, got %v", err)
	}
}
