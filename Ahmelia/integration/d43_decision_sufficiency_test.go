package integration

import (
	"reflect"
	"testing"
	"time"

	"ahmelia/programming"
)

// This file is deliberately an experiment, not a new production decision
// contract. It asks whether the durable facts already produced by D43 can
// distinguish the factual situations a later authority would need to know.
// No recovery/retry/resume/reconciliation decision is implemented here.

func d43Evaluation(workID, changeID string, attempt int, exists bool, sha string, at time.Time, compatible bool) programming.EffectEvaluation {
	return programming.EffectEvaluation{
		ID:               workID + "-" + changeID + "-" + at.Format("150405.000000000"),
		WorkID:           workID,
		AttemptNumber:    attempt,
		ExpectedChangeID: changeID,
		Observed: programming.ObservedFile{
			Source: "filesystem", Path: "a.txt", Exists: exists, SHA256: sha, ObservedAt: at,
		},
		Judgment:    programming.CompatibilityJudgment{Compatible: compatible},
		EvaluatedAt: at.Add(time.Millisecond),
	}
}

func TestD43DecisionSufficiencyDistinguishesRepresentableFacts(t *testing.T) {
	now := time.Date(2026, 9, 27, 16, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		data []programming.EffectEvaluation
		want func([]programming.EffectEvaluation) bool
	}{
		{
			name: "expected and observed compatible",
			data: []programming.EffectEvaluation{d43Evaluation("w1", "change-a", 1, true, "sha-a", now, true)},
			want: func(es []programming.EffectEvaluation) bool {
				return len(es) == 1 && es[0].Judgment.Compatible && es[0].Observed.Exists
			},
		},
		{
			name: "expected absent",
			data: []programming.EffectEvaluation{d43Evaluation("w1", "change-a", 1, false, "", now, false)},
			want: func(es []programming.EffectEvaluation) bool {
				return len(es) == 1 && !es[0].Judgment.Compatible && !es[0].Observed.Exists
			},
		},
		{
			name: "expected present but incompatible content",
			data: []programming.EffectEvaluation{d43Evaluation("w1", "change-a", 1, true, "sha-other", now, false)},
			want: func(es []programming.EffectEvaluation) bool {
				return len(es) == 1 && !es[0].Judgment.Compatible && es[0].Observed.Exists && es[0].Observed.SHA256 == "sha-other"
			},
		},
		{
			name: "multiple historical evaluations",
			data: []programming.EffectEvaluation{
				d43Evaluation("w1", "change-a", 1, false, "", now, false),
				d43Evaluation("w1", "change-a", 1, true, "sha-a", now.Add(time.Second), true),
			},
			want: func(es []programming.EffectEvaluation) bool {
				return len(es) == 2 && es[0].EvaluatedAt.Before(es[1].EvaluatedAt) && !es[0].Judgment.Compatible && es[1].Judgment.Compatible
			},
		},
		{
			name: "later observation differs",
			data: []programming.EffectEvaluation{
				d43Evaluation("w1", "change-a", 1, true, "sha-a", now, true),
				d43Evaluation("w1", "change-a", 1, true, "sha-b", now.Add(time.Second), false),
			},
			want: func(es []programming.EffectEvaluation) bool {
				return len(es) == 2 && es[0].Observed.SHA256 != es[1].Observed.SHA256
			},
		},
		{
			name: "same expected evidence across two works",
			data: []programming.EffectEvaluation{
				d43Evaluation("w1", "change-a", 1, true, "sha-a", now, true),
				d43Evaluation("w2", "change-a", 1, true, "sha-a", now, true),
			},
			want: func(es []programming.EffectEvaluation) bool {
				return len(es) == 2 && es[0].WorkID != es[1].WorkID && reflect.DeepEqual(es[0].Observed, es[1].Observed)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, e := range tc.data {
				if err := e.Validate(); err != nil {
					t.Fatalf("D43 evaluation invalid: %v", err)
				}
			}
			if !tc.want(tc.data) {
				t.Fatalf("D43 did not preserve the facts required by this case: %+v", tc.data)
			}
		})
	}
}

func TestD43DecisionSufficiencyEvidenceInsufficientHasNoDistinctRepresentation(t *testing.T) {
	now := time.Date(2026, 9, 27, 16, 1, 0, 0, time.UTC)

	// World A: observation was never obtained.
	// World B: observation was obtained, but evaluation was never persisted.
	// Both worlds produce exactly the same D43 durable knowledge: no evaluation.
	var worldA []programming.EffectEvaluation
	var worldB []programming.EffectEvaluation

	if !reflect.DeepEqual(worldA, worldB) {
		t.Fatal("control condition failed: the two indistinguishable D43 states must be equal")
	}

	// D43 can represent a negative observation (file absent), but that is not
	// the same fact as insufficient evidence. A zero/invalid observation is
	// rejected by the D43 contract rather than classified as insufficient.
	negativeObservation := d43Evaluation("w1", "change-a", 1, false, "", now, false)
	if err := negativeObservation.Validate(); err != nil {
		t.Fatalf("negative filesystem observation should remain representable: %v", err)
	}
	if negativeObservation.Observed.Source == "" || negativeObservation.Observed.Path == "" || negativeObservation.Observed.ObservedAt.IsZero() {
		t.Fatal("negative observation unexpectedly lost its identity")
	}

	insufficient := programming.EffectEvaluation{
		ID:               "insufficient",
		WorkID:           "w1",
		AttemptNumber:    1,
		ExpectedChangeID: "change-a",
		Observed:         programming.ObservedFile{},
		Judgment:         programming.CompatibilityJudgment{Compatible: false},
		EvaluatedAt:      now,
	}
	if err := insufficient.Validate(); err == nil {
		t.Fatal("D43 must not silently reinterpret an invalid/empty observation as 'evidence insufficient'")
	}
}

func TestD43DecisionSufficiencyCrashPhaseIsNotDistinguishable(t *testing.T) {
	// The persisted D43 state after a crash before observation is identical to
	// the persisted D43 state after observation succeeded but before evaluation
	// was persisted: in both cases there is no EffectEvaluation.
	beforeObservation := []programming.EffectEvaluation(nil)
	afterObservationBeforeEvaluation := []programming.EffectEvaluation(nil)

	if !reflect.DeepEqual(beforeObservation, afterObservationBeforeEvaluation) {
		t.Fatal("crash-phase control condition failed")
	}

	// Once an evaluation exists, D43 can identify its observation/evaluation
	// times. The missing phase distinction exists specifically in the gap before
	// an EffectEvaluation record exists.
	e := d43Evaluation("w1", "change-a", 1, true, "sha-a", time.Date(2026, 9, 27, 16, 2, 0, 0, time.UTC), true)
	if err := e.Validate(); err != nil {
		t.Fatal(err)
	}
	if e.Observed.ObservedAt.IsZero() || e.EvaluatedAt.IsZero() {
		t.Fatal("existing D43 evaluation lost temporal evidence")
	}
}
