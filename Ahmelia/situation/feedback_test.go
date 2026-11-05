package situation

import "testing"

func TestB96404ObservationFeedsNextSituation(t *testing.T) {
	previous := Situation{ID: "sit-1", Version: 1, Temporal: TemporalCurrent}
	obs := []Observation{{ID: "o1", Source: "core.Engine.execute", Subject: "open", Predicate: "result", Object: "Chrome", Value: "success=true", Confidence: 1, Epistemic: EpistemicObserved}}
	next := ApplyObservation(previous, obs, Trigger{ID: "t1", Reason: "action completed", ObservationIDs: []string{"o1"}})
	if next.Version != 2 {
		t.Fatalf("version=%d", next.Version)
	}
	if len(next.Evidence) != 1 || len(next.Relations) != 1 {
		t.Fatalf("evidence=%d relations=%d", len(next.Evidence), len(next.Relations))
	}
	if next.Relations[0].Type != "result" {
		t.Fatalf("relation=%q", next.Relations[0].Type)
	}
}
