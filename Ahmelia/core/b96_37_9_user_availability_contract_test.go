package core

import (
	conversationalinitiative "ahmelia/initiative"
	"testing"
)

func TestB96379EngineAvailabilityBoundary(t *testing.T) {
	isolateTestWorkspace(t)
	e := NewEngine(nil, nil, nil, nil, nil)
	if e.initiativeAvailabilityPolicy == nil {
		t.Fatal("missing B96.37.9 policy")
	}
	a := e.initiativeAvailabilityPolicy.Evaluate(conversationalinitiative.AvailabilityEvidence{CurrentTurnActive: true})
	if err := conversationalinitiative.ValidateAvailabilityAssessment(a); err != nil {
		t.Fatal(err)
	}
	if a.Availability != conversationalinitiative.AvailabilityAvailable {
		t.Fatalf("unexpected assessment: %+v", a)
	}
}
