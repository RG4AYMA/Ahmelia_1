package integration

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

// observationFact is intentionally test-local. This experiment asks whether
// the minimum factual marker needed to distinguish "no observation occurred"
// from "an observation occurred but no evaluation was persisted" can be
// represented without introducing an operational interpretation.
//
// It is NOT a proposed production contract.
type observationFact struct {
	WorkID        string    `json:"work_id"`
	AttemptNumber int       `json:"attempt_number"`
	Source        string    `json:"source"`
	ObservedAt    time.Time `json:"observed_at"`
}

func TestEpistemicBoundaryMinimumFactDistinguishesAFromB(t *testing.T) {
	at := time.Date(2026, 9, 27, 17, 0, 0, 0, time.UTC)

	// A: execution happened, then runtime crashed before observation.
	var worldA []observationFact

	// B: execution happened, observation was acquired, then runtime crashed
	// before EffectEvaluation was persisted.
	worldB := []observationFact{{
		WorkID: "w1", AttemptNumber: 1, Source: "filesystem", ObservedAt: at,
	}}

	if reflect.DeepEqual(worldA, worldB) {
		t.Fatal("minimum observation fact failed to distinguish A from B")
	}
	if len(worldA) != 0 {
		t.Fatal("A must contain no observation fact")
	}
	if len(worldB) != 1 || worldB[0].WorkID != "w1" || worldB[0].AttemptNumber != 1 || worldB[0].Source != "filesystem" || worldB[0].ObservedAt.IsZero() {
		t.Fatalf("B lost the minimum factual identity: %+v", worldB)
	}
}

func TestEpistemicBoundaryMinimumFactDistinguishesBFromC(t *testing.T) {
	at := time.Date(2026, 9, 27, 17, 1, 0, 0, time.UTC)

	// B: observation exists; evaluation does not.
	observationOnly := struct {
		Observation []observationFact `json:"observation"`
		Evaluation  []struct{}        `json:"evaluation"`
	}{
		Observation: []observationFact{{WorkID: "w1", AttemptNumber: 1, Source: "filesystem", ObservedAt: at}},
	}

	// C: the same observation exists and an evaluation exists.
	withEvaluation := struct {
		Observation []observationFact `json:"observation"`
		Evaluation  []struct{}        `json:"evaluation"`
	}{
		Observation: []observationFact{{WorkID: "w1", AttemptNumber: 1, Source: "filesystem", ObservedAt: at}},
		Evaluation:  []struct{}{{}},
	}

	if reflect.DeepEqual(observationOnly, withEvaluation) {
		t.Fatal("observation and evaluation must remain distinguishable")
	}
	if len(observationOnly.Observation) != 1 || len(observationOnly.Evaluation) != 0 {
		t.Fatal("B must mean observation without evaluation")
	}
	if len(withEvaluation.Observation) != 1 || len(withEvaluation.Evaluation) != 1 {
		t.Fatal("C must contain both observation and evaluation")
	}
}

func TestEpistemicBoundaryObservationIsNotEvaluation(t *testing.T) {
	at := time.Date(2026, 9, 27, 17, 2, 0, 0, time.UTC)
	observation := observationFact{WorkID: "w1", AttemptNumber: 1, Source: "filesystem", ObservedAt: at}

	encoded, err := json.Marshal(observation)
	if err != nil {
		t.Fatalf("observation fact must be serializable: %v", err)
	}
	var reconstructed observationFact
	if err := json.Unmarshal(encoded, &reconstructed); err != nil {
		t.Fatalf("observation fact must survive reconstruction: %v", err)
	}
	if !reflect.DeepEqual(observation, reconstructed) {
		t.Fatalf("observation fact changed across reconstruction: before=%+v after=%+v", observation, reconstructed)
	}

	// Deliberately no compatibility field, judgment, completion, recovery,
	// or causal statement is present. The fact says only that observation
	// occurred, where/for which attempt, and when.
}
