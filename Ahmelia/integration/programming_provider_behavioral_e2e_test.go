package integration

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"ahmelia/environment"
	"ahmelia/interaction"
	"ahmelia/production"
	"ahmelia/programming"
)

type behavioralSynthesizer struct {
	mode string
	seen *programming.CodeObjective
}

func (s behavioralSynthesizer) Synthesize(_ context.Context, objective programming.CodeObjective) (*programming.MultiFilePlan, error) {
	if s.seen != nil {
		objective.Contract = objective.Contract.Clone()
		*s.seen = objective
	}
	files := []programming.MultiFileChange{
		{
			ID:        "main",
			Operation: programming.SourceCreate,
			File:      programming.SourceFile{Path: "main.go", Language: "Go", Role: programming.RoleEntrypoint, ProjectID: objective.ProjectID},
			Content:   "package main\n\nfunc Add(a, b int) int { return a + b }\n\nfunc main() {}\n",
		},
		{
			ID:        "test",
			Operation: programming.SourceCreate,
			File:      programming.SourceFile{Path: "main_test.go", Language: "Go", Role: programming.RoleTest, ProjectID: objective.ProjectID},
			Content:   "package main\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(2, 3) != 5 { t.Fatalf(\"unexpected result\") }\n}\n",
		},
		{
			ID:        "mod",
			Operation: programming.SourceCreate,
			File:      programming.SourceFile{Path: "go.mod", Language: "Go", Role: programming.RoleConfig, ProjectID: objective.ProjectID},
			Content:   "module behavioraltest\n\ngo 1.20\n",
		},
	}
	if s.mode == "bad" {
		files[0].Content = "package main\n\nfunc Add(a, b int) int { return a + }\n\nfunc main() {}\n"
	}
	return &programming.MultiFilePlan{ID: "llm-fixture-" + s.mode, Project: programming.CodeProject{ID: objective.ProjectID, Name: objective.ProjectID, Root: objective.ProjectRoot, Language: "Go", SourceRoots: []string{"."}, TestRoots: []string{"."}, BuildCommand: "go build ./...", TestCommand: "go test ./...", RuntimeCommand: "go run ."}, Changes: files}, nil
}

func TestProgrammingProviderPreservesSemanticObjectiveAndLeavesAcceptanceUnassessed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	seen := programming.CodeObjective{}
	provider, err := NewProgrammingProviderWithSynthesizer(environment.NewManager(), behavioralSynthesizer{mode: "good", seen: &seen})
	if err != nil {
		t.Fatal(err)
	}
	frame := interaction.ObjectiveFrame{
		PrimaryGoal: "crear calculadora", PrimaryOperation: "create", Subject: "programa", Workspace: "local",
		Constraints: []string{"sin dependencias externas"}, RequiredActions: []string{"sumar", "probar"},
		Verification: []string{"behavioral_result"}, Recovery: []string{"diagnosticar"}, Confidence: 0.9, Provenance: "semantic_request",
	}
	result := provider.Execute(context.Background(), production.ProviderRequest{Input: "Create and test a Go calculator", Project: production.ProductionProject{ID: "semantic-objective", Path: filepath.Join(home, "semantic-objective")}, ObjectiveFrame: frame})
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if seen.Contract.PrimaryGoal != frame.PrimaryGoal || seen.Contract.PrimaryOperation != frame.PrimaryOperation || seen.Contract.Provenance != frame.Provenance {
		t.Fatalf("structured objective did not reach synthesizer: %+v", seen.Contract)
	}
	if len(seen.Contract.Constraints) != 1 || seen.Contract.Constraints[0] != frame.Constraints[0] || len(seen.Contract.Verification) != 1 {
		t.Fatalf("structured objective lists were lost: %+v", seen.Contract)
	}
	if result.ObjectiveStatus != "unassessed" {
		t.Fatalf("build/test result was misrepresented as objective evaluation: status=%q", result.ObjectiveStatus)
	}
	var persisted programming.ObjectiveContract
	if err := json.Unmarshal([]byte(result.ObjectiveContractJSON), &persisted); err != nil {
		t.Fatalf("objective contract was not serialized into result evidence: %v", err)
	}
	if persisted.PrimaryGoal != frame.PrimaryGoal || persisted.Provenance != frame.Provenance {
		t.Fatalf("objective contract was not retained in result evidence: %+v", persisted)
	}
}

func TestProgrammingProviderBehavioralE2E_GoodLLMOutputCompletes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	world := environment.NewManager()
	provider, err := NewProgrammingProviderWithSynthesizer(world, behavioralSynthesizer{mode: "good"})
	if err != nil {
		t.Fatal(err)
	}

	result := provider.Execute(context.Background(), production.ProviderRequest{Input: "Create a Go program that adds two integers and test it", Project: production.ProductionProject{ID: "good-output", Path: filepath.Join(home, "good-output")}})
	if result.Err != nil {
		t.Fatalf("unexpected provider error: %v", result.Err)
	}
	if !result.Success || !result.Verified || !result.Observed {
		t.Fatalf("good output did not complete successfully: %+v", result)
	}
	if result.Project.ID != "good-output" {
		t.Fatalf("project id=%q", result.Project.ID)
	}
	if _, err := os.Stat(filepath.Join(result.Project.Path, "main.go")); err != nil {
		t.Fatalf("expected real source artifact: %v", err)
	}
	if len(result.Artifacts) == 0 {
		t.Fatal("expected production artifacts")
	}
}

func TestProgrammingProviderBehavioralE2E_BadLLMOutputDoesNotComplete(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	world := environment.NewManager()
	provider, err := NewProgrammingProviderWithSynthesizer(world, behavioralSynthesizer{mode: "bad"})
	if err != nil {
		t.Fatal(err)
	}

	result := provider.Execute(context.Background(), production.ProviderRequest{Input: "Create a Go program that adds two integers and test it", Project: production.ProductionProject{ID: "bad-output", Path: filepath.Join(home, "bad-output")}})
	if result.Err != nil {
		t.Fatalf("expected structured production failure, got provider error: %v", result.Err)
	}
	if result.Success || result.Verified {
		t.Fatalf("bad output was incorrectly accepted: %+v", result)
	}
	if result.Observed {
		t.Fatalf("failed production must not claim successful observation: %+v", result)
	}
	if result.OutcomeCode != "production_result" {
		t.Fatalf("unexpected outcome code=%q", result.OutcomeCode)
	}
	if result.Project.ID != "bad-output" {
		t.Fatalf("project id=%q", result.Project.ID)
	}
}

func TestProgrammingAttemptPersistsEffectBeforeWorldMutationAndSurvivesRuntimeLoss(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	world := environment.NewManager()
	storePath := filepath.Join(home, "work.json")
	store := production.NewWorkStore(storePath)

	// Establish the same durable identity the real ProductionRuntime creates.
	req := interaction.SemanticRequest{Objective: interaction.Objective{Text: "Create a Go program that adds two integers and test it"}}
	work, err := store.Begin(req)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AttachProduction(work.ID, "", "", "programming"); err != nil {
		t.Fatal(err)
	}
	attempt, err := store.BeginAttempt(work.ID, "programming.create_file", "create_file", "programming")
	if err != nil {
		t.Fatal(err)
	}
	jobID := work.ID + "-attempt-1"
	executionID := jobID + "-execution"
	if err := store.SetAttemptExecutionIdentity(work.ID, attempt, jobID, executionID); err != nil {
		t.Fatal(err)
	}

	provider, err := NewProgrammingProviderWithSynthesizer(world, behavioralSynthesizer{mode: "good"})
	if err != nil {
		t.Fatal(err)
	}
	result := provider.Execute(context.Background(), production.ProviderRequest{
		Input:          req.Objective.Text,
		WorkID:         work.ID,
		AttemptNumber:  attempt,
		Project:        production.ProductionProject{ID: "crash-recovery", Path: filepath.Join(home, "crash-recovery")},
		EffectRecorder: func(spec []byte) error { return store.SetAttemptEffectSpecification(work.ID, attempt, spec) },
	})
	if result.Err != nil || !result.Success || !result.Verified || !result.Observed {
		t.Fatalf("programming effect did not materialize and verify: %+v", result)
	}

	// Simulated crash: the process loses ProviderResult/dispatcher memory before
	// CompleteAttempt. We deliberately do not call CompleteAttempt.
	restarted := production.NewWorkStore(storePath)
	recovered, ok := restarted.Get(work.ID)
	if !ok {
		t.Fatal("work disappeared after simulated runtime loss")
	}
	if len(recovered.Attempts) != 1 {
		t.Fatalf("attempt count=%d, want 1", len(recovered.Attempts))
	}
	a := recovered.Attempts[0]
	if a.Status != "unverified" {
		t.Fatalf("attempt status=%q, want unverified after runtime loss/restart", a.Status)
	}
	if len(recovered.Events) == 0 || recovered.Events[len(recovered.Events)-1].Type != "attempt_interrupted_on_restart" {
		t.Fatalf("restart must record interruption and require reconciliation: %+v", recovered.Events)
	}
	if a.JobID != jobID || a.ExecutionAttemptID != executionID {
		t.Fatalf("execution identity was not durable: %+v", a)
	}
	if len(a.EffectSpecification) == 0 {
		t.Fatal("effect specification was lost before crash recovery")
	}
	var recoveredPlan programming.MultiFilePlan
	if err := json.Unmarshal(a.EffectSpecification, &recoveredPlan); err != nil {
		t.Fatalf("persisted effect specification is not a MultiFilePlan: %v", err)
	}
	if len(recoveredPlan.Changes) != 3 {
		t.Fatalf("recovered change count=%d, want 3", len(recoveredPlan.Changes))
	}
	for _, name := range []string{"go.mod", "main.go", "main_test.go"} {
		if _, err := os.Stat(filepath.Join(result.Project.Path, name)); err != nil {
			t.Fatalf("world effect %q missing after simulated crash: %v", name, err)
		}
	}
}

func TestProgrammingEffectSpecificationFailurePreventsWorldMutation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	world := environment.NewManager()
	provider, err := NewProgrammingProviderWithSynthesizer(world, behavioralSynthesizer{mode: "good"})
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(home, "blocked-before-effect")
	result := provider.Execute(context.Background(), production.ProviderRequest{
		Input:   "Create a Go program that adds two integers and test it",
		Project: production.ProductionProject{ID: "blocked-before-effect", Path: root},
		EffectRecorder: func([]byte) error {
			return errors.New("simulated durable persistence failure")
		},
	})
	if result.Err == nil {
		t.Fatal("expected effect persistence failure")
	}
	if _, err := os.Stat(filepath.Join(root, "main.go")); !os.IsNotExist(err) {
		t.Fatalf("world mutated despite failed pre-effect persistence: err=%v", err)
	}
}
