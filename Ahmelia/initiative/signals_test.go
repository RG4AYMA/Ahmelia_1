package initiative

import (
	"testing"
	"time"
)

func TestSignalSourcesDoNotDecide(t *testing.T) {
	now := time.Date(2026, 9, 6, 19, 0, 0, 0, time.UTC)
	e := NewSignalEngine()
	in := SignalInput{Now: now, ActiveTopic: "Ahmelia", ActiveObjective: "continuar arquitectura", CandidateExists: true, CandidateTopic: "Ahmelia", CandidateText: "¿Qué parte seguimos?", CandidateConfidence: .9, OpenThreadCount: 1, CommitmentCount: 1}
	signals := e.Collect(in)
	if len(signals) < 3 {
		t.Fatalf("expected multiple signals, got %d", len(signals))
	}
	for _, s := range signals {
		if s.Type == "" || s.Provenance.Subsystem != "B96.37.3" && s.Provenance.Subsystem != "B96.22.21" {
			t.Fatalf("bad provenance: %+v", s)
		}
	}
}

func TestSignalNormalizationAndConflictResolutionKeepsFreshObservation(t *testing.T) {
	old := InitiativeSignal{Type: "availability", Source: SourceInteractionContinuity, Reason: ReasonRelevantComment, Relevant: true, Confidence: .4, ObservedAt: time.Unix(10, 0)}
	fresh := InitiativeSignal{Type: "availability", Source: SourceInteractionContinuity, Reason: ReasonRelevantComment, Relevant: false, Confidence: .9, ObservedAt: time.Unix(20, 0)}
	got := ResolveSignalConflicts(NormalizeSignals([]InitiativeSignal{old, fresh}, time.Unix(30, 0)))
	if len(got) != 1 || got[0].ObservedAt.Unix() != 20 || got[0].Relevant {
		t.Fatalf("unexpected conflict result: %+v", got)
	}
}

func TestBuildOpportunityRetainsAllSignals(t *testing.T) {
	now := time.Now()
	signals := []InitiativeSignal{
		makeSignal(SignalInput{Now: now}, SourceConversationalContext, ReasonConversationContinue, "context", "x", "topic", true, .6),
		makeSignal(SignalInput{Now: now}, SourceOpenCommitment, ReasonUnfinishedCommitment, "commitment", "x", "topic", true, .9),
	}
	op := BuildOpportunity(signals, SignalInput{Now: now, ActiveTopic: "topic"})
	if !op.Exists || len(op.Signals) != 2 || op.Relevance != .9 {
		t.Fatalf("unexpected opportunity: %+v", op)
	}
	if op.Signal.Confidence != .9 {
		t.Fatalf("primary signal was not aggregated: %+v", op.Signal)
	}
}

func TestSignalEngineIsNotPolicy(t *testing.T) {
	e := NewSignalEngine()
	op := e.Opportunity(SignalInput{Now: time.Now(), CandidateExists: true, CandidateConfidence: .9, CandidateText: "continue"})
	if !op.Exists {
		t.Fatal("expected opportunity")
	}
	// The signal layer exposes evidence; it has no Decision field or policy call.
	if op.Source == "" || op.Signal.Provenance.Subsystem == "" {
		t.Fatalf("incomplete evidence: %+v", op)
	}
}
