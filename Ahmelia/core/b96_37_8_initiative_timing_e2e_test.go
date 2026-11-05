package core

import (
	"testing"
	"time"

	conversationalinitiative "ahmelia/initiative"
)

func TestB96378EngineUsesTimingEvaluator(t *testing.T) {
	isolateTestWorkspace(t)
	e := NewEngine(nil, nil, nil, nil, nil)
	if e.initiativeTimingEvaluator == nil {
		t.Fatal("B96.37.8 timing evaluator is not initialized")
	}
	if e.conversationalInitiative == nil || e.conversationalInitiative.Timing == nil {
		t.Fatal("B96.37.8 timing evaluator is not wired into initiative policy")
	}
	if e.conversationalInitiative.Timing != e.initiativeTimingEvaluator {
		t.Fatal("engine timing evaluator is not the canonical policy evaluator")
	}

	now := time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)
	op := conversationalinitiative.InitiativeOpportunity{
		Exists: true, Signal: conversationalinitiative.InitiativeSignal{Relevant: true}, Relevance: 0.9,
		UserAvailability: conversationalinitiative.AvailabilityBusy,
		Urgency:          conversationalinitiative.UrgencyHigh,
		Interruptibility: conversationalinitiative.InterruptSafe,
		ConversationLoad: conversationalinitiative.LoadNormal,
	}
	d := e.conversationalInitiative.Evaluate(op, now)
	if d.Decision != conversationalinitiative.Defer || d.Timing != conversationalinitiative.TimingDefer {
		t.Fatalf("expected B96.37.8 to defer busy non-critical opportunity, got %+v", d)
	}
}
