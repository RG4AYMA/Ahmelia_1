package integration

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"ahmelia/programming"
)

// observedEvidenceFact is intentionally test-local. It is a semantic
// experiment, not a proposed production contract. The experiment asks what
// factual payload must survive independently of EffectEvaluation.
type observedEvidenceFact struct {
	WorkID        string                   `json:"work_id"`
	AttemptNumber int                      `json:"attempt_number"`
	Evidence      programming.ObservedFile `json:"evidence"`
}

func TestObservedEvidenceContentDistinguishesB1FromB2AfterReconstruction(t *testing.T) {
	at := time.Date(2026, 9, 27, 17, 10, 0, 0, time.UTC)

	b1 := observedEvidenceFact{
		WorkID:        "w1",
		AttemptNumber: 1,
		Evidence: programming.ObservedFile{
			Source: "filesystem", Path: "main.go", Exists: true,
			SHA256: "sha-a", ObservedAt: at,
		},
	}
	b2 := observedEvidenceFact{
		WorkID:        "w1",
		AttemptNumber: 1,
		Evidence: programming.ObservedFile{
			Source: "filesystem", Path: "main.go", Exists: true,
			SHA256: "sha-b", ObservedAt: at,
		},
	}

	b1r := roundTripObservedEvidence(t, b1)
	b2r := roundTripObservedEvidence(t, b2)

	if reflect.DeepEqual(b1r, b2r) {
		t.Fatal("different observed facts must not collapse after reconstruction")
	}
	if b1r.Evidence.SHA256 != "sha-a" || b2r.Evidence.SHA256 != "sha-b" {
		t.Fatalf("observed content was not preserved: b1=%+v b2=%+v", b1r, b2r)
	}
}

func TestObservedEvidenceIsDistinctFromEffectEvaluation(t *testing.T) {
	at := time.Date(2026, 9, 27, 17, 11, 0, 0, time.UTC)
	observation := observedEvidenceFact{
		WorkID: "w1", AttemptNumber: 1,
		Evidence: programming.ObservedFile{
			Source: "filesystem", Path: "main.go", Exists: true,
			SHA256: "sha-a", ObservedAt: at,
		},
	}

	// B: observation existed, but runtime crashed before evaluation persisted.
	observationOnly := struct {
		Observation observedEvidenceFact           `json:"observation"`
		Evaluation  []programming.EffectEvaluation `json:"evaluation"`
	}{Observation: observation}

	// D: the same observed evidence was subsequently evaluated.
	evaluation := programming.EffectEvaluation{
		ID: "eval-1", WorkID: "w1", AttemptNumber: 1,
		ExpectedChangeID: "change-a", Observed: observation.Evidence,
		Judgment:    programming.CompatibilityJudgment{Compatible: true},
		EvaluatedAt: at.Add(time.Second),
	}
	withEvaluation := struct {
		Observation observedEvidenceFact           `json:"observation"`
		Evaluation  []programming.EffectEvaluation `json:"evaluation"`
	}{Observation: observation, Evaluation: []programming.EffectEvaluation{evaluation}}

	if len(observationOnly.Evaluation) != 0 {
		t.Fatal("B must contain observation without evaluation")
	}
	if len(withEvaluation.Evaluation) != 1 {
		t.Fatal("D must contain an evaluation")
	}
	if withEvaluation.Evaluation[0].Observed.SHA256 != observation.Evidence.SHA256 {
		t.Fatal("evaluation must not erase the observed factual content")
	}
}

func TestObservedEvidenceDoesNotNeedCompatibilityToExist(t *testing.T) {
	at := time.Date(2026, 9, 27, 17, 12, 0, 0, time.UTC)
	fact := observedEvidenceFact{
		WorkID: "w1", AttemptNumber: 1,
		Evidence: programming.ObservedFile{
			Source: "filesystem", Path: "main.go", Exists: false,
			SHA256: "", ObservedAt: at,
		},
	}

	reconstructed := roundTripObservedEvidence(t, fact)
	if reconstructed.Evidence.Exists {
		t.Fatal("absence is itself an observed filesystem fact and must remain absence")
	}
	if reconstructed.Evidence.ObservedAt.IsZero() {
		t.Fatal("observation timestamp must survive reconstruction")
	}
}

func roundTripObservedEvidence(t *testing.T, fact observedEvidenceFact) observedEvidenceFact {
	t.Helper()
	encoded, err := json.Marshal(fact)
	if err != nil {
		t.Fatalf("marshal observed evidence: %v", err)
	}
	var reconstructed observedEvidenceFact
	if err := json.Unmarshal(encoded, &reconstructed); err != nil {
		t.Fatalf("unmarshal observed evidence: %v", err)
	}
	return reconstructed
}
