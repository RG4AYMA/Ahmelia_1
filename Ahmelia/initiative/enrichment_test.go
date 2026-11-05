package initiative

import (
	"testing"
	"time"
)

func TestB96375EnrichmentProducesEvidenceWithoutDecision(t *testing.T) {
	now := time.Date(2026, 9, 7, 14, 0, 0, 0, time.UTC)
	in := SignalInput{Now: now, ActiveTopic: "project", Enrichment: BuildEnrichment(now,
		ContextEnrichment{HasContext: true, Topic: "project", OpenThreads: 1, ObservedAt: now},
		CognitionEnrichment{Valid: true, Intent: "discuss", IntentConfidence: .9, Relevance: .8, ObservedAt: now},
		UserModelEnrichment{Available: true, ReliableGoals: 2, ReliablePreferences: 1, ObservedAt: now},
		HistoricalMemoryEnrichment{Available: true, ObservedAt: now},
		WorkEnrichment{Exists: true, ID: "w1", Status: "running", PendingSteps: 2, UpdatedAt: now},
		CommitmentEnrichment{OpenCount: 1, LatestAt: now},
		AutonomyEnrichment{ExecutionState: "Idle", StaleGoals: 1, ObservedAt: now},
		AvailabilityEnrichment{Availability: AvailabilityAvailable, Interruptibility: InterruptSafe, ConversationLoad: LoadNormal, ObservedAt: now},
		PrivacySafetyEnrichment{Policy: InformationAllowed, PolicyKnown: true, Interruptibility: InterruptSafe, ObservedAt: now},
		ChannelEnrichment{Channel: "text", Supported: true, ObservedAt: now},
	)}
	got := EnrichAndCorrelate(in, nil, now)
	if len(got) < 8 {
		t.Fatalf("expected enriched evidence, got %d", len(got))
	}
	for _, s := range got {
		if s.Provenance.Subsystem != "B96.37.5" && s.Provenance.Subsystem != "B96.37.3" {
			t.Fatalf("unexpected provenance subsystem: %q", s.Provenance.Subsystem)
		}
	}
}

func TestB96375CorrelationIsDerivedOnly(t *testing.T) {
	now := time.Now()
	in := SignalInput{Now: now, ActiveTopic: "topic", Enrichment: InitiativeSourceEnrichment{
		ConversationalContext: ContextEnrichment{HasContext: true, Topic: "topic", ObservedAt: now},
		Work:                  WorkEnrichment{Exists: true, ID: "w1", Status: "running", UpdatedAt: now},
	}}
	got := EnrichAndCorrelate(in, nil, now)
	found := false
	for _, s := range got {
		if s.Type == "cross_source_work_context" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected cross-source correlation signal")
	}
}

func TestB96375FailureIsolation(t *testing.T) {
	bad := sourceAdapter{name: "bad", collect: func(RuntimeSnapshot) []InitiativeSignal { panic("boom") }}
	good := testSnapshotSource{name: "good", signals: []InitiativeSignal{makeSignal(SignalInput{Now: time.Now()}, SourceOperationalWork, ReasonNextStepAvailable, "good", "x", "", true, .8)}}
	p := NewRuntimePipeline(bad, good)
	got, failures := p.SignalsWithReport(SignalInput{Now: time.Now()}, time.Now())
	if len(got) == 0 {
		t.Fatal("failure of one source isolated incorrectly")
	}
	if len(failures) != 1 || failures[0].Source != "bad" {
		t.Fatalf("expected auditable bad-source failure, got %+v", failures)
	}
}

func TestB96375RuntimeToPolicyE2E(t *testing.T) {
	now := time.Date(2026, 9, 7, 14, 0, 0, 0, time.UTC)
	in := SignalInput{
		Now: now, ActiveTopic: "project", CandidateExists: true, CandidateText: "continue project", CandidateConfidence: .9,
		Enrichment: BuildEnrichment(now,
			ContextEnrichment{HasContext: true, Topic: "project", ObservedAt: now},
			CognitionEnrichment{Valid: true, Intent: "discuss", IntentConfidence: .9, Relevance: .9, ObservedAt: now},
			UserModelEnrichment{Available: true, ReliableGoals: 1, ObservedAt: now},
			HistoricalMemoryEnrichment{Available: true, ObservedAt: now},
			WorkEnrichment{Exists: true, ID: "w1", Status: "running", PendingSteps: 1, UpdatedAt: now},
			CommitmentEnrichment{OpenCount: 1, LatestAt: now},
			AutonomyEnrichment{ExecutionState: "Idle", ObservedAt: now},
			AvailabilityEnrichment{Availability: AvailabilityAvailable, Interruptibility: InterruptSafe, ConversationLoad: LoadLow, ObservedAt: now},
			PrivacySafetyEnrichment{Policy: InformationAllowed, PolicyKnown: true, Interruptibility: InterruptSafe, ObservedAt: now},
			ChannelEnrichment{Channel: "text", Supported: true, ObservedAt: now},
		),
	}
	r := NewRuntime()
	op := r.Opportunity(in)
	decision := DefaultPolicy().Evaluate(op, now)
	if !op.Exists || len(op.Signals) == 0 {
		t.Fatal("expected runtime opportunity")
	}
	if !decision.Decision.Valid() {
		t.Fatalf("invalid policy decision: %q", decision.Decision)
	}
	if decision.Opportunity.Signals == nil {
		t.Fatal("decision lost enriched evidence")
	}
}
