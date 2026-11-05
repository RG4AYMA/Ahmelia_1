package core

import (
	"testing"
	"time"

	conversationalinitiative "ahmelia/initiative"
)

func TestB963710EngineWiresPriorityUrgencyPolicy(t *testing.T) {
	isolateTestWorkspace(t)
	e := NewEngine(nil, nil, nil, nil, nil)
	if e.initiativePriorityUrgencyPolicy == nil {
		t.Fatal("B96.37.10 policy is not initialized")
	}
	if e.conversationalInitiative == nil || e.initiativeTimingEvaluator == nil {
		t.Fatal("initiative decision/timing chain is incomplete")
	}
}

func TestB963710EngineChainPreservesDecisionAuthority(t *testing.T) {
	isolateTestWorkspace(t)
	e := NewEngine(nil, nil, nil, nil, nil)
	now := time.Date(2026, 9, 7, 15, 0, 0, 0, time.UTC)
	op := conversationalinitiative.InitiativeOpportunity{
		Exists: true, Source: conversationalinitiative.SourceConversationalContext,
		Reason: conversationalinitiative.ReasonWarning, Relevance: .95,
		Priority: conversationalinitiative.PriorityMedium, Urgency: conversationalinitiative.UrgencyCritical,
		UserAvailability: conversationalinitiative.AvailabilityBusy,
		Privacy:          conversationalinitiative.InformationAllowed,
		Interruptibility: conversationalinitiative.InterruptSafe,
		ConversationLoad: conversationalinitiative.LoadNormal,
		Signal:           conversationalinitiative.InitiativeSignal{Relevant: true, Source: conversationalinitiative.SourceConversationalContext, Reason: conversationalinitiative.ReasonWarning, Confidence: .95, Priority: conversationalinitiative.PriorityMedium, Urgency: conversationalinitiative.UrgencyCritical, ObservedAt: now},
	}
	op.Signals = []conversationalinitiative.InitiativeSignal{op.Signal}
	input := conversationalinitiative.SignalInput{Now: now, UserUrgency: .95, UserAvailability: conversationalinitiative.AvailabilityBusy, Interruptibility: conversationalinitiative.InterruptSafe, ConversationLoad: conversationalinitiative.LoadNormal}
	enriched, _, err := e.initiativePriorityUrgencyPolicy.Enrich(op, input, now)
	if err != nil {
		t.Fatal(err)
	}
	decision := e.conversationalInitiative.Evaluate(enriched, now)
	if decision.Decision != conversationalinitiative.Intervene || decision.Timing != conversationalinitiative.TimingInterrupt {
		t.Fatalf("expected B96.37.2 to make the final interrupt decision, got %+v", decision)
	}
	if decision.Urgency != conversationalinitiative.UrgencyCritical {
		t.Fatalf("decision did not consume B96.37.10 urgency: %+v", decision)
	}
}

func TestB963710HighPriorityLowUrgencyDoesNotForceIntervention(t *testing.T) {
	isolateTestWorkspace(t)
	e := NewEngine(nil, nil, nil, nil, nil)
	now := time.Now()
	op := conversationalinitiative.InitiativeOpportunity{
		Exists: true, Source: conversationalinitiative.SourceConversationalContext, Reason: conversationalinitiative.ReasonConversationContinue,
		Relevance: .9, Priority: conversationalinitiative.PriorityHigh, Urgency: conversationalinitiative.UrgencyLow,
		UserAvailability: conversationalinitiative.AvailabilityBusy, Privacy: conversationalinitiative.InformationAllowed,
		Interruptibility: conversationalinitiative.InterruptSafe, ConversationLoad: conversationalinitiative.LoadNormal,
		Signal: conversationalinitiative.InitiativeSignal{Relevant: true, Source: conversationalinitiative.SourceConversationalContext, Reason: conversationalinitiative.ReasonConversationContinue, Confidence: .9, Priority: conversationalinitiative.PriorityHigh, Urgency: conversationalinitiative.UrgencyLow, ObservedAt: now},
	}
	op.Signals = []conversationalinitiative.InitiativeSignal{op.Signal}
	enriched, assessment, err := e.initiativePriorityUrgencyPolicy.Enrich(op, conversationalinitiative.SignalInput{Now: now}, now)
	if err != nil {
		t.Fatal(err)
	}
	if assessment.Priority != conversationalinitiative.PriorityHigh || assessment.Urgency != conversationalinitiative.UrgencyLow {
		t.Fatalf("bad enrichment: %+v", assessment)
	}
	d := e.conversationalInitiative.Evaluate(enriched, now)
	if d.Decision != conversationalinitiative.Defer {
		t.Fatalf("high priority must not override busy timing with low urgency: %+v", d)
	}
}
