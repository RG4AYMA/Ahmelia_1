package programming

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func testObservedFile(path, sha string, exists bool, observedAt time.Time) ObservedFile {
	return ObservedFile{
		Source:     "filesystem",
		Path:       path,
		Exists:     exists,
		SHA256:     sha,
		ObservedAt: observedAt,
	}
}

func TestEffectEvaluationValidatesIdentityAndTemporalContext(t *testing.T) {
	now := time.Date(2026, 9, 27, 3, 0, 0, 0, time.UTC)
	evaluation := EffectEvaluation{
		ID:               "eval-1",
		WorkID:           "work-1",
		AttemptNumber:    2,
		ExpectedChangeID: "change-a",
		Observed:         testObservedFile("a.txt", "sha-a", true, now.Add(-time.Second)),
		Judgment:         CompatibilityJudgment{Compatible: true},
		EvaluatedAt:      now,
	}
	if err := evaluation.Validate(); err != nil {
		t.Fatalf("valid evaluation rejected: %v", err)
	}

	invalid := []EffectEvaluation{
		{},
		{ID: "eval-1", WorkID: "", AttemptNumber: 1, ExpectedChangeID: "change-a", Observed: testObservedFile("a.txt", "", false, now), EvaluatedAt: now},
		{ID: "eval-1", WorkID: "work-1", AttemptNumber: 0, ExpectedChangeID: "change-a", Observed: testObservedFile("a.txt", "", false, now), EvaluatedAt: now},
		{ID: "eval-1", WorkID: "work-1", AttemptNumber: 1, ExpectedChangeID: "", Observed: testObservedFile("a.txt", "", false, now), EvaluatedAt: now},
		{ID: "eval-1", WorkID: "work-1", AttemptNumber: 1, ExpectedChangeID: "change-a", Observed: ObservedFile{Path: "a.txt", ObservedAt: now}, EvaluatedAt: now},
		{ID: "eval-1", WorkID: "work-1", AttemptNumber: 1, ExpectedChangeID: "change-a", Observed: testObservedFile("a.txt", "", false, time.Time{}), EvaluatedAt: now},
		{ID: "eval-1", WorkID: "work-1", AttemptNumber: 1, ExpectedChangeID: "change-a", Observed: testObservedFile("a.txt", "", false, now), EvaluatedAt: time.Time{}},
	}
	for i, tc := range invalid {
		if err := tc.Validate(); err == nil {
			t.Fatalf("invalid evaluation %d unexpectedly validated", i)
		}
	}
}

func TestEffectEvaluationPreservesMultipleObservationsForOneExpectedChange(t *testing.T) {
	t1 := time.Date(2026, 9, 27, 3, 0, 1, 0, time.UTC)
	t2 := t1.Add(time.Second)
	t3 := t2.Add(time.Second)

	evaluations := []EffectEvaluation{
		{
			ID:               "eval-1",
			WorkID:           "work-1",
			AttemptNumber:    1,
			ExpectedChangeID: "change-a",
			Observed:         testObservedFile("a.txt", "", false, t1),
			Judgment:         CompatibilityJudgment{Compatible: false},
			EvaluatedAt:      t1.Add(100 * time.Millisecond),
		},
		{
			ID:               "eval-2",
			WorkID:           "work-1",
			AttemptNumber:    1,
			ExpectedChangeID: "change-a",
			Observed:         testObservedFile("a.txt", "sha-a", true, t2),
			Judgment:         CompatibilityJudgment{Compatible: true},
			EvaluatedAt:      t2.Add(100 * time.Millisecond),
		},
		{
			ID:               "eval-3",
			WorkID:           "work-1",
			AttemptNumber:    1,
			ExpectedChangeID: "change-a",
			Observed:         testObservedFile("a.txt", "sha-other", true, t3),
			Judgment:         CompatibilityJudgment{Compatible: false},
			EvaluatedAt:      t3.Add(100 * time.Millisecond),
		},
	}

	for _, evaluation := range evaluations {
		if err := evaluation.Validate(); err != nil {
			t.Fatalf("evaluation %q invalid: %v", evaluation.ID, err)
		}
	}
	if evaluations[0].Observed == evaluations[1].Observed {
		t.Fatal("distinct observations were collapsed")
	}
	if evaluations[0].ID == evaluations[1].ID || evaluations[1].ID == evaluations[2].ID {
		t.Fatal("repeated evaluations must retain distinct identities")
	}
}

func TestEffectEvaluationSupportsMultipleExpectedChangesInOneAttempt(t *testing.T) {
	now := time.Date(2026, 9, 27, 3, 1, 0, 0, time.UTC)
	evaluations := []EffectEvaluation{
		{
			ID: "eval-a", WorkID: "work-1", AttemptNumber: 1, ExpectedChangeID: "change-a",
			Observed: testObservedFile("a.txt", "sha-a", true, now), Judgment: CompatibilityJudgment{Compatible: true}, EvaluatedAt: now,
		},
		{
			ID: "eval-b", WorkID: "work-1", AttemptNumber: 1, ExpectedChangeID: "change-b",
			Observed: testObservedFile("b.txt", "", false, now.Add(time.Second)), Judgment: CompatibilityJudgment{Compatible: false}, EvaluatedAt: now.Add(time.Second),
		},
	}
	for _, evaluation := range evaluations {
		if err := evaluation.Validate(); err != nil {
			t.Fatalf("evaluation %q invalid: %v", evaluation.ID, err)
		}
	}
	if evaluations[0].WorkID != evaluations[1].WorkID || evaluations[0].AttemptNumber != evaluations[1].AttemptNumber {
		t.Fatal("expected both evaluations to belong to the same attempt")
	}
	if evaluations[0].ExpectedChangeID == evaluations[1].ExpectedChangeID {
		t.Fatal("expected distinct change identities")
	}
}

func TestEffectEvaluationDoesNotImplyCausality(t *testing.T) {
	now := time.Date(2026, 9, 27, 3, 2, 0, 0, time.UTC)
	observation := testObservedFile("a.txt", "sha-a", true, now)

	w1 := EffectEvaluation{
		ID: "eval-w1", WorkID: "work-1", AttemptNumber: 1, ExpectedChangeID: "change-a",
		Observed: observation, Judgment: CompatibilityJudgment{Compatible: true}, EvaluatedAt: now.Add(time.Millisecond),
	}
	w2 := EffectEvaluation{
		ID: "eval-w2", WorkID: "work-2", AttemptNumber: 1, ExpectedChangeID: "change-a",
		Observed: observation, Judgment: CompatibilityJudgment{Compatible: true}, EvaluatedAt: now.Add(2 * time.Millisecond),
	}

	if err := w1.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := w2.Validate(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(w1.Observed, w2.Observed) {
		t.Fatal("the same primary evidence should be representable in both evaluations")
	}
	if w1.WorkID == w2.WorkID {
		t.Fatal("causality test requires distinct operational contexts")
	}
}

func TestEffectEvaluationSurvivesJSONReconstruction(t *testing.T) {
	now := time.Date(2026, 9, 27, 3, 3, 0, 0, time.UTC)
	original := EffectEvaluation{
		ID:               "eval-crash-1",
		WorkID:           "work-crash-1",
		AttemptNumber:    7,
		ExpectedChangeID: "change-a",
		Observed:         testObservedFile("a.txt", "sha-a", true, now),
		Judgment:         CompatibilityJudgment{Compatible: true},
		EvaluatedAt:      now.Add(250 * time.Millisecond),
	}
	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}

	var reconstructed EffectEvaluation
	if err := json.Unmarshal(encoded, &reconstructed); err != nil {
		t.Fatal(err)
	}
	if err := reconstructed.Validate(); err != nil {
		t.Fatalf("reconstructed evaluation invalid: %v", err)
	}
	if !reflect.DeepEqual(reconstructed, original) {
		t.Fatalf("reconstructed evaluation=%+v, want %+v", reconstructed, original)
	}
}
