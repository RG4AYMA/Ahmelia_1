package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"ahmelia/interaction"
	"ahmelia/production"
	"ahmelia/programming"
)

func TestEffectEvaluationPreservesOperationalAssociationAcrossReconstruction(t *testing.T) {
	home := t.TempDir()
	storePath := filepath.Join(home, "work.json")
	store := production.NewWorkStore(storePath)

	w1, err := store.Begin(interaction.SemanticRequest{Objective: interaction.Objective{Text: "work one"}})
	if err != nil {
		t.Fatal(err)
	}
	attempt1, err := store.BeginAttempt(w1.ID, "programming.create_file", "create_file", "programming")
	if err != nil {
		t.Fatal(err)
	}

	w2, err := store.Begin(interaction.SemanticRequest{Objective: interaction.Objective{Text: "work two"}})
	if err != nil {
		t.Fatal(err)
	}
	attempt2, err := store.BeginAttempt(w2.ID, "programming.create_file", "create_file", "programming")
	if err != nil {
		t.Fatal(err)
	}

	observedAt := time.Date(2026, 9, 27, 3, 10, 0, 0, time.UTC)
	observation := programming.ObservedFile{
		Source:     "filesystem",
		Path:       "main.go",
		Exists:     true,
		SHA256:     "sha-a",
		ObservedAt: observedAt,
	}

	w1Evaluation := programming.EffectEvaluation{
		ID:               "eval-w1",
		WorkID:           w1.ID,
		AttemptNumber:    attempt1,
		ExpectedChangeID: "change-a",
		Observed:         observation,
		Judgment:         programming.CompatibilityJudgment{Compatible: true},
		EvaluatedAt:      observedAt.Add(100 * time.Millisecond),
	}
	w2Evaluation := programming.EffectEvaluation{
		ID:               "eval-w2",
		WorkID:           w2.ID,
		AttemptNumber:    attempt2,
		ExpectedChangeID: "change-a",
		Observed:         observation,
		Judgment:         programming.CompatibilityJudgment{Compatible: true},
		EvaluatedAt:      observedAt.Add(200 * time.Millisecond),
	}
	for _, evaluation := range []programming.EffectEvaluation{w1Evaluation, w2Evaluation} {
		if err := evaluation.Validate(); err != nil {
			t.Fatal(err)
		}
	}

	// This file is only the test's serialized reconstruction boundary. It is
	// deliberately not introduced as a second production persistence owner.
	evaluationPath := filepath.Join(home, "evaluations.json")
	encoded, err := json.Marshal([]programming.EffectEvaluation{w1Evaluation, w2Evaluation})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(evaluationPath, encoded, 0644); err != nil {
		t.Fatal(err)
	}

	store = nil
	data, err := os.ReadFile(evaluationPath)
	if err != nil {
		t.Fatal(err)
	}
	var reconstructed []programming.EffectEvaluation
	if err := json.Unmarshal(data, &reconstructed); err != nil {
		t.Fatal(err)
	}
	if len(reconstructed) != 2 {
		t.Fatalf("reconstructed evaluations=%d, want 2", len(reconstructed))
	}
	for _, evaluation := range reconstructed {
		if err := evaluation.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(reconstructed[0], w1Evaluation) || !reflect.DeepEqual(reconstructed[1], w2Evaluation) {
		t.Fatalf("reconstructed evaluations lost identity/evidence: %+v", reconstructed)
	}
	if reconstructed[0].WorkID == reconstructed[1].WorkID {
		t.Fatal("evaluations from concurrent work attempts must remain distinct")
	}
}

func TestEffectEvaluationPreservesRepeatedObservationsWithoutOverwritingHistory(t *testing.T) {
	now := time.Date(2026, 9, 27, 3, 11, 0, 0, time.UTC)
	evaluations := []programming.EffectEvaluation{
		{
			ID: "eval-1", WorkID: "work-1", AttemptNumber: 1, ExpectedChangeID: "change-a",
			Observed: programming.ObservedFile{Source: "filesystem", Path: "main.go", Exists: false, ObservedAt: now},
			Judgment: programming.CompatibilityJudgment{Compatible: false}, EvaluatedAt: now.Add(time.Millisecond),
		},
		{
			ID: "eval-2", WorkID: "work-1", AttemptNumber: 1, ExpectedChangeID: "change-a",
			Observed: programming.ObservedFile{Source: "filesystem", Path: "main.go", Exists: true, SHA256: "sha-a", ObservedAt: now.Add(time.Second)},
			Judgment: programming.CompatibilityJudgment{Compatible: true}, EvaluatedAt: now.Add(time.Second + time.Millisecond),
		},
	}

	for _, evaluation := range evaluations {
		if err := evaluation.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	if evaluations[0].Observed.ObservedAt.Equal(evaluations[1].Observed.ObservedAt) {
		t.Fatal("repeated observations must retain distinct observation times")
	}
	if evaluations[0].ID == evaluations[1].ID {
		t.Fatal("repeated evaluations must retain distinct identities")
	}
}

func TestEffectEvaluationPersistsInsideExistingWorkAttemptAcrossRuntimeReconstruction(t *testing.T) {
	home := t.TempDir()
	storePath := filepath.Join(home, "work.json")
	store := production.NewWorkStore(storePath)
	work, err := store.Begin(interaction.SemanticRequest{Objective: interaction.Objective{Text: "persist evaluation"}})
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := store.BeginAttempt(work.ID, "programming.create_file", "create_file", "programming")
	if err != nil {
		t.Fatal(err)
	}

	evaluation := programming.EffectEvaluation{
		ID:               "eval-persisted",
		WorkID:           work.ID,
		AttemptNumber:    attempt,
		ExpectedChangeID: "change-a",
		Observed: programming.ObservedFile{
			Source: "filesystem", Path: "a.txt", Exists: false,
			ObservedAt: time.Date(2026, 9, 27, 3, 12, 0, 0, time.UTC),
		},
		Judgment:    programming.CompatibilityJudgment{Compatible: false},
		EvaluatedAt: time.Date(2026, 9, 27, 3, 12, 1, 0, time.UTC),
	}
	if err := evaluation.Validate(); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(evaluation)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordAttemptEffectEvaluation(work.ID, attempt, encoded); err != nil {
		t.Fatal(err)
	}

	store = nil
	restarted := production.NewWorkStore(storePath)
	recovered, ok := restarted.Get(work.ID)
	if !ok {
		t.Fatal("work was not recovered")
	}
	if len(recovered.Attempts) != 1 || len(recovered.Attempts[0].EffectEvaluations) != 1 {
		t.Fatalf("expected one persisted evaluation, got attempts=%+v", recovered.Attempts)
	}

	var reconstructed programming.EffectEvaluation
	if err := json.Unmarshal(recovered.Attempts[0].EffectEvaluations[0], &reconstructed); err != nil {
		t.Fatal(err)
	}
	if err := reconstructed.Validate(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reconstructed, evaluation) {
		t.Fatalf("reconstructed evaluation=%+v, want %+v", reconstructed, evaluation)
	}
	if reconstructed.WorkID != work.ID || reconstructed.AttemptNumber != attempt {
		t.Fatalf("evaluation lost WorkAttempt identity: %+v", reconstructed)
	}
}
