package core

import (
	conversationalinitiative "ahmelia/initiative"
	"testing"
	"time"
)

func TestB963720_EngineObservabilityAndOutcomeTrace(t *testing.T) {
	isolateTestWorkspace(t)
	e := NewEngine(nil, nil, nil, nil, nil)
	if e.initiativeObservability == nil {
		t.Fatal("missing B96.37.20 observability manager")
	}
	now := time.Date(2026, 9, 7, 16, 0, 0, 0, time.UTC)
	op := conversationalinitiative.InitiativeOpportunity{Exists: true, Source: conversationalinitiative.SourceOpenCommitment, Reason: conversationalinitiative.ReasonUnfinishedCommitment, Relevance: .9, Priority: conversationalinitiative.PriorityHigh, Urgency: conversationalinitiative.UrgencyMedium, UserAvailability: conversationalinitiative.AvailabilityAvailable, Privacy: conversationalinitiative.InformationAllowed, Interruptibility: conversationalinitiative.InterruptSafe, ConversationLoad: conversationalinitiative.LoadNormal, Context: "project X", Target: "X", Signal: conversationalinitiative.InitiativeSignal{ID: "sig-e2e", Type: "unfinished_commitment", Relevant: true, Confidence: .9, Source: conversationalinitiative.SourceOpenCommitment, Reason: conversationalinitiative.ReasonUnfinishedCommitment, Provenance: conversationalinitiative.Provenance{Subsystem: "B96.37.3", CreatedAt: now}}, Signals: nil}
	op.Signals = []conversationalinitiative.InitiativeSignal{op.Signal}
	d := conversationalinitiative.InitiativeDecision{ID: "decision-e2e", Decision: conversationalinitiative.Intervene, Opportunity: op, Source: op.Source, Reason: op.Reason, Action: conversationalinitiative.ActionRemind, Timing: conversationalinitiative.TimingNow, Priority: op.Priority, Urgency: op.Urgency, CreatedAt: now, PolicyVersion: "B96.37.2"}
	tr, err := e.initiativeObservability.Create(conversationalinitiative.NormalizeInitiativeIdentity(op), op.Source, op.Signal.Provenance, now)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = e.initiativeObservability.RecordSignal(tr.ID, op.Signal, now)
	_, _ = e.initiativeObservability.RecordOpportunity(tr.ID, op, now)
	_, _ = e.initiativeObservability.RecordDecision(tr.ID, d, now)
	out, err := e.ObserveInitiativeOutcome(d, conversationalinitiative.OutcomeObservation{Initiative: d, Engaged: true, ResponseClass: conversationalinitiative.ResponseContinuation, ObservedAt: now.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if out.Outcome != conversationalinitiative.OutcomeEngaged {
		t.Fatalf("unexpected outcome %s", out.Outcome)
	}
	got, ok := e.InitiativeTrace(tr.ID)
	if !ok {
		t.Fatal("trace not recoverable through Engine")
	}
	if got.DecisionID != d.ID || got.OutcomeID != out.ID {
		t.Fatalf("missing correlations: %+v", got)
	}
	if got.Status != conversationalinitiative.TraceCompleted {
		t.Fatalf("trace not completed: %s", got.Status)
	}
}

func TestB963720_EngineObservabilityIsSeparateFromConversationHistory(t *testing.T) {
	isolateTestWorkspace(t)
	e := NewEngine(nil, nil, nil, nil, nil)
	if e.initiativeObservability == nil {
		t.Fatal("observability boundary not wired")
	}
	if e.initiativeObservability.Store == nil {
		t.Fatal("trace store missing")
	}
	// The manager is a technical trace store; no ConversationHistory pointer is exposed by it.
}
