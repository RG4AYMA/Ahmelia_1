package core

import (
	conversationalinitiative "ahmelia/initiative"
	"testing"
)

func TestB96379EngineUsesAvailabilityPolicy(t *testing.T) {
	isolateTestWorkspace(t)
	e := NewEngine(nil, nil, nil, nil, nil)
	if e.initiativeAvailabilityPolicy == nil {
		t.Fatal("B96.37.9 availability policy not initialized")
	}
	a := e.initiativeAvailabilityPolicy.Evaluate(conversationalinitiative.AvailabilityEvidence{CurrentTurnActive: true})
	if a.Availability != conversationalinitiative.AvailabilityAvailable {
		t.Fatalf("unexpected availability: %+v", a)
	}
}

func TestB96379AvailabilityIsNotDecisionAuthority(t *testing.T) {
	isolateTestWorkspace(t)
	e := NewEngine(nil, nil, nil, nil, nil)
	if e.initiativeAvailabilityPolicy == nil {
		t.Fatal("B96.37.9 availability policy not initialized")
	}
	a := e.initiativeAvailabilityPolicy.Evaluate(conversationalinitiative.AvailabilityEvidence{CurrentTurnActive: true})
	if a.Availability != conversationalinitiative.AvailabilityAvailable {
		t.Fatalf("unexpected availability: %+v", a)
	}
	op := conversationalinitiative.InitiativeOpportunity{Exists: false, UserAvailability: a.Availability}
	d := e.conversationalInitiative.Evaluate(op, a.ObservedAt)
	if d.Decision != conversationalinitiative.Wait {
		t.Fatalf("availability policy must not bypass initiative policy: %+v", d)
	}
}
