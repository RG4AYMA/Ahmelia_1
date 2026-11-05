package initiative

import "testing"

func TestB963716_RuntimeIntegrationIsEvidenceOnly(t *testing.T) {
	r := NewRuntime()
	out := learningOutcome("runtime-1", OutcomeAccepted, ActionSuggest, .95, r.Now())
	if _, err := r.ObserveInitiativeOutcome(out); err != nil {
		t.Fatal(err)
	}
	if got := r.InitiativeAdaptationSignals(r.Now()); len(got) != 0 {
		t.Fatalf("one outcome must not expose adaptation: %+v", got)
	}
	if r.Learner == nil {
		t.Fatal("runtime learner missing")
	}
}
