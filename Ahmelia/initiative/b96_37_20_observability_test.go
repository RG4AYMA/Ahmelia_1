package initiative

import (
	"testing"
	"time"
)

func TestB963720_TraceBuildsFullEvidenceChain(t *testing.T) {
	now := time.Date(2026, 9, 7, 16, 0, 0, 0, time.UTC)
	o := NewInitiativeObservability(nil)
	trace, err := o.Create("initiative-184", SourceOpenCommitment, Provenance{Component: "test", Subsystem: "B96.37.20", CreatedAt: now}, now)
	if err != nil {
		t.Fatal(err)
	}
	s := InitiativeSignal{ID: "sig-184", Type: "unfinished_commitment", Value: "pending", Relevant: true, Confidence: .9, Source: SourceOpenCommitment, Reason: ReasonUnfinishedCommitment, ObservedAt: now, Provenance: Provenance{Component: "test", Subsystem: "B96.37.3", OriginID: "legacy-1", CreatedAt: now}}
	if _, err = o.RecordSignal(trace.ID, s, now); err != nil {
		t.Fatal(err)
	}
	op := InitiativeOpportunity{Exists: true, Source: s.Source, Reason: s.Reason, Relevance: .9, Priority: PriorityHigh, Urgency: UrgencyMedium, UserAvailability: AvailabilityBusy, Privacy: InformationAllowed, Interruptibility: InterruptLimited, ConversationLoad: LoadNormal, Signal: s, Signals: []InitiativeSignal{s}, Context: "pending work"}
	if _, err = o.RecordOpportunity(trace.ID, op, now); err != nil {
		t.Fatal(err)
	}
	if _, err = o.RecordEvaluation(trace.ID, "B96.37.9", map[string]string{"availability": "BUSY"}, now); err != nil {
		t.Fatal(err)
	}
	arb := ArbitrationResult{ComparedAt: now, Selected: InitiativeCandidateRef{Valid: true, ID: "arb-184", Opportunity: op, Rank: 1}, Provenance: Provenance{Component: "initiative.InitiativeArbiter", Subsystem: "B96.37.17", CreatedAt: now}}
	if _, err = o.RecordArbitration(trace.ID, arb, now); err != nil {
		t.Fatal(err)
	}
	d := InitiativeDecision{ID: "decision-184", Decision: Defer, Opportunity: op, Source: op.Source, Reason: op.Reason, Timing: TimingDefer, Priority: op.Priority, Urgency: op.Urgency, CreatedAt: now, PolicyVersion: "B96.37.2"}
	if _, err = o.RecordDecision(trace.ID, d, now); err != nil {
		t.Fatal(err)
	}
	sched := Schedule{ID: "schedule-184", Mode: ScheduleDeferred, Trigger: ScheduleTriggerAvailability, State: SchedulePending, CreatedAt: now, Revision: 1, SourceDecisionID: d.ID}
	if _, err = o.RecordScheduling(trace.ID, sched, now); err != nil {
		t.Fatal(err)
	}
	out := InitiativeOutcomeRecord{ID: "outcome-184", InitiativeID: d.ID, Outcome: OutcomeEngaged, ObservedAt: now, Provenance: Provenance{Component: "test", Subsystem: "B96.37.15", CreatedAt: now}}
	if _, err = o.RecordOutcome(trace.ID, out, now); err != nil {
		t.Fatal(err)
	}
	got, ok := o.Find(trace.ID)
	if !ok {
		t.Fatal("trace missing")
	}
	if got.DecisionID != d.ID || got.ScheduleID != sched.ID || got.OutcomeID != out.ID {
		t.Fatalf("correlation incomplete: %+v", got)
	}
	if len(got.SignalIDs) != 1 || got.SignalIDs[0] != s.ID {
		t.Fatalf("signal correlation missing: %+v", got.SignalIDs)
	}
	if len(got.Events) < 7 {
		t.Fatalf("expected full trace, got %d events", len(got.Events))
	}
}

func TestB963720_ObservabilityCannotAuthorize(t *testing.T) {
	o := NewInitiativeObservability(nil)
	now := time.Now()
	tr, err := o.Create("initiative-x", SourceConversationalContext, Provenance{Subsystem: "B96.37.20", CreatedAt: now}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = o.RecordDecision(tr.ID, InitiativeDecision{ID: "d-x", Decision: Intervene, CreatedAt: now, PolicyVersion: "B96.37.2"}, now); err != nil {
		t.Fatal(err)
	}
	// Recording a canonical decision is permitted; no API here can produce a decision.
	pending := o.Pending()
	if len(pending) != 1 || pending[0].DecisionID != "d-x" {
		t.Fatal("trace query altered authority or failed to record evidence")
	}
}

func TestB963720_RetentionAndReadOnlyQueries(t *testing.T) {
	now := time.Date(2026, 9, 7, 16, 0, 0, 0, time.UTC)
	o := NewInitiativeObservability(nil)
	o.Policy.TTL = time.Hour
	tr, err := o.Create("initiative-r", SourceOperationalWork, Provenance{CreatedAt: now}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(o.ByInitiative(tr.InitiativeID)) != 1 {
		t.Fatal("ByInitiative failed")
	}
	if n := o.Expire(now.Add(2 * time.Hour)); n != 1 {
		t.Fatalf("expected one expired trace, got %d", n)
	}
	if got, ok := o.Find(tr.ID); !ok || got.Status != TraceExpired {
		t.Fatal("trace did not expire")
	}
}
