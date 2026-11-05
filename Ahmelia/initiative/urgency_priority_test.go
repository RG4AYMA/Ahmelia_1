package initiative

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func b963710BaseOpportunity(now time.Time) InitiativeOpportunity {
	s := InitiativeSignal{ID: "s1", Type: "opportunity", Source: SourceConversationalContext, Reason: ReasonConversationContinue, Relevant: true, Confidence: .9, Priority: PriorityMedium, Urgency: UrgencyLow, ObservedAt: now}
	return InitiativeOpportunity{Exists: true, Source: s.Source, Reason: s.Reason, Relevance: .9, Priority: s.Priority, Urgency: s.Urgency, UserAvailability: AvailabilityAvailable, Privacy: InformationAllowed, Interruptibility: InterruptSafe, ConversationLoad: LoadNormal, Signal: s, Signals: []InitiativeSignal{s}}
}

func TestB963710_10_1PriorityModel(t *testing.T) {
	for _, p := range []Priority{PriorityLow, PriorityMedium, PriorityHigh, PriorityCritical} {
		if NormalizePriority(p) != p {
			t.Fatalf("priority %q did not normalize canonically", p)
		}
	}
	if NormalizePriority(Priority("NORMAL")) != PriorityMedium {
		t.Fatal("legacy NORMAL must migrate to MEDIUM")
	}
}

func TestB963710_10_2UrgencyModel(t *testing.T) {
	for _, u := range []Urgency{UrgencyNone, UrgencyLow, UrgencyMedium, UrgencyHigh, UrgencyCritical} {
		if NormalizeUrgency(u) != u {
			t.Fatalf("urgency %q did not normalize canonically", u)
		}
	}
	if NormalizeUrgency(Urgency("URGENT")) != UrgencyCritical {
		t.Fatal("legacy URGENT must migrate to CRITICAL")
	}
}

func TestB963710_10_3PrioritySignals(t *testing.T) {
	now := time.Now()
	op := b963710BaseOpportunity(now)
	op.Signals = append(op.Signals, InitiativeSignal{ID: "critical", Type: "deadline_approaching", Source: SourceOperationalWork, Reason: ReasonWarning, Relevant: true, Confidence: .8, Priority: PriorityCritical, ObservedAt: now})
	p := NewPriorityUrgencyPolicy()
	got, evidence := p.AggregatePriority(op, SignalInput{Now: now})
	if got != PriorityCritical {
		t.Fatalf("priority = %s, want CRITICAL", got)
	}
	if len(evidence) < 2 {
		t.Fatalf("expected priority evidence, got %d", len(evidence))
	}
}

func TestB963710_10_4UrgencySignals(t *testing.T) {
	now := time.Now()
	op := b963710BaseOpportunity(now)
	got, evidence := NewPriorityUrgencyPolicy().AggregateUrgency(op, SignalInput{Now: now, UserUrgency: .91})
	if got != UrgencyCritical {
		t.Fatalf("urgency = %s, want CRITICAL", got)
	}
	if len(evidence) == 0 {
		t.Fatal("missing urgency evidence")
	}
}

func TestB963710_10_5PriorityAggregation(t *testing.T) {
	now := time.Now()
	op := b963710BaseOpportunity(now)
	op.Signals = []InitiativeSignal{
		{ID: "a", Type: "a", Source: SourceConversationalContext, Reason: ReasonConversationContinue, Confidence: .99, Priority: PriorityHigh, ObservedAt: now},
		{ID: "b", Type: "b", Source: SourceOperationalWork, Reason: ReasonNextStepAvailable, Confidence: .20, Priority: PriorityCritical, ObservedAt: now.Add(time.Second)},
	}
	got, _ := NewPriorityUrgencyPolicy().AggregatePriority(op, SignalInput{Now: now})
	if got != PriorityCritical {
		t.Fatalf("aggregation lost highest explicit priority: %s", got)
	}
}

func TestB963710_10_6UrgencyAggregation(t *testing.T) {
	now := time.Now()
	op := b963710BaseOpportunity(now)
	op.Signals = []InitiativeSignal{
		{ID: "a", Type: "a", Source: SourceConversationalContext, Reason: ReasonConversationContinue, Confidence: .99, Urgency: UrgencyMedium, ObservedAt: now},
		{ID: "b", Type: "b", Source: SourceOperationalWork, Reason: ReasonWarning, Confidence: .20, Urgency: UrgencyHigh, ObservedAt: now.Add(time.Second)},
	}
	got, _ := NewPriorityUrgencyPolicy().AggregateUrgency(op, SignalInput{Now: now})
	if got != UrgencyHigh {
		t.Fatalf("aggregation lost highest explicit urgency: %s", got)
	}
}

func TestB963710_10_7PriorityUrgencyInteractionIndependent(t *testing.T) {
	now := time.Now()
	op := b963710BaseOpportunity(now)
	op.Signal.Priority, op.Signal.Urgency = PriorityHigh, UrgencyLow
	op.Signals[0] = op.Signal
	got, assessment, err := NewPriorityUrgencyPolicy().Enrich(op, SignalInput{Now: now}, now)
	if err != nil {
		t.Fatal(err)
	}
	if got.Priority != PriorityHigh || got.Urgency != UrgencyLow {
		t.Fatalf("unexpected profile: %+v", got)
	}
	if assessment.Interaction != InteractionHighPriorityLowUrgency {
		t.Fatalf("interaction = %s", assessment.Interaction)
	}
}

func TestB963710_10_8TemporalEscalationRequiresExplicitRule(t *testing.T) {
	now := time.Now()
	op := b963710BaseOpportunity(now.Add(-2 * time.Hour))
	op.Signal.Urgency = UrgencyLow
	op.Signals[0] = op.Signal
	p := NewPriorityUrgencyPolicy()
	got, assessment, err := p.Enrich(op, SignalInput{Now: now}, now)
	if err != nil {
		t.Fatal(err)
	}
	if got.Urgency != UrgencyLow || assessment.TemporalEscalated {
		t.Fatal("age alone must not escalate urgency")
	}
	p.TemporalRules = []TemporalEscalationRule{{Name: "deadline_window", SignalType: "deadline_approaching", After: time.Hour, From: UrgencyLow, To: UrgencyMedium, MinConfidence: .7}}
	op.Signal.Type = "deadline_approaching"
	op.Signal.Confidence = .9
	op.Signals[0] = op.Signal
	got, assessment, err = p.Enrich(op, SignalInput{Now: now}, now)
	if err != nil {
		t.Fatal(err)
	}
	if got.Urgency != UrgencyMedium || !assessment.TemporalEscalated {
		t.Fatalf("explicit escalation failed: %+v", assessment)
	}
}

func TestB963710_10_9StalenessExpiration(t *testing.T) {
	now := time.Now()
	op := b963710BaseOpportunity(now.Add(-time.Hour))
	op.Signal.ExpiresAt = now.Add(-time.Minute)
	op.Signals[0] = op.Signal
	got, assessment, err := NewPriorityUrgencyPolicy().Enrich(op, SignalInput{Now: now}, now)
	if err != nil {
		t.Fatal(err)
	}
	if !assessment.Stale || got.Urgency != UrgencyNone {
		t.Fatalf("stale opportunity not de-urgented: %+v", assessment)
	}
}

func TestB963710_10_10ConflictResolution(t *testing.T) {
	now := time.Now()
	op := b963710BaseOpportunity(now)
	op.Signals = []InitiativeSignal{
		{ID: "old", Type: "same", Source: SourceConversationalContext, Reason: ReasonConversationContinue, Priority: PriorityHigh, Confidence: .9, ObservedAt: now.Add(-time.Minute)},
		{ID: "new", Type: "same", Source: SourceConversationalContext, Reason: ReasonConversationContinue, Priority: PriorityMedium, Confidence: .9, ObservedAt: now},
	}
	resolved := ResolveSignalConflicts(op.Signals)
	if len(resolved) != 1 || resolved[0].ID != "new" {
		t.Fatalf("fresh conflict did not win: %+v", resolved)
	}
}

func TestB963710_10_11OpportunityEnrichment(t *testing.T) {
	now := time.Now()
	op := b963710BaseOpportunity(now)
	got, assessment, err := NewPriorityUrgencyPolicy().Enrich(op, SignalInput{Now: now, UserUrgency: .8}, now)
	if err != nil {
		t.Fatal(err)
	}
	if got.Priority == "" || got.Urgency != UrgencyHigh || assessment.Interaction == "" {
		t.Fatalf("incomplete enrichment: %+v", assessment)
	}
}

func TestB963710_10_12EngineIntegrationContractShape(t *testing.T) {
	if _, ok := reflect.TypeOf(PriorityUrgencyAssessment{}).FieldByName("Decision"); ok {
		t.Fatal("B96.37.10 assessment must not contain a Decision field")
	}
}

func TestB963710_10_13LegacyPriorityMigration(t *testing.T) {
	for _, in := range []Priority{Priority("NORMAL"), Priority("MEDIUM"), Priority("urgent")} {
		if NormalizePriority(in) == "" {
			t.Fatalf("legacy priority %q rejected", in)
		}
	}
}

func TestB963710_10_14LegacyUrgencyMigration(t *testing.T) {
	for _, in := range []Urgency{Urgency("URGENT"), Urgency("critical"), Urgency("medium")} {
		if NormalizeUrgency(in) == "" {
			t.Fatalf("legacy urgency %q rejected", in)
		}
	}
}

func TestB963710_10_15UnitContractValidation(t *testing.T) {
	now := time.Now()
	a := PriorityUrgencyAssessment{Priority: PriorityHigh, Urgency: UrgencyCritical, Interaction: InteractionHighPriorityCriticalUrgency, EvaluatedAt: now}
	if err := a.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (TemporalEscalationRule{Name: "x", SignalType: "deadline", After: time.Hour, From: UrgencyLow, To: UrgencyMedium, MinConfidence: 1}).Valid(); !err {
		t.Fatal("valid temporal rule rejected")
	}
}

func TestB963710_10_16E2EInitiativeEnrichment(t *testing.T) {
	now := time.Date(2026, 9, 7, 15, 0, 0, 0, time.UTC)
	op := b963710BaseOpportunity(now)
	op.UserAvailability = AvailabilityBusy
	op.Signal.Priority = PriorityMedium
	op.Signal.Urgency = UrgencyCritical
	op.Signals[0] = op.Signal
	input := SignalInput{Now: now, UserUrgency: .95, UserAvailability: AvailabilityBusy, Interruptibility: InterruptSafe, ConversationLoad: LoadNormal}
	enriched, assessment, err := NewPriorityUrgencyPolicy().Enrich(op, input, now)
	if err != nil {
		t.Fatal(err)
	}
	if enriched.Urgency != UrgencyCritical || assessment.Interaction != InteractionMediumPriorityCriticalUrgency {
		t.Fatalf("unexpected E2E enrichment: %+v", assessment)
	}
	// B96.37.10 stops here. Timing/decision are deliberately not produced.
}

func TestB963710_10_17ArchitectureAudit(t *testing.T) {
	typeNames := []string{"Decision", "Timing", "Action"}
	rt := reflect.TypeOf(PriorityUrgencyAssessment{})
	for _, name := range typeNames {
		if _, ok := rt.FieldByName(name); ok {
			t.Fatalf("forbidden authority field %q", name)
		}
	}
	src := "urgency_priority.go"
	data, err := osReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, forbidden := range []string{"InitiativeDecision{", "Intervene", "Defer", "TimingInterrupt"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("B96.37.10 implementation contains forbidden decision authority token %q", forbidden)
		}
	}
}

// osReadFile is a tiny indirection so the architecture test remains explicit.
var osReadFile = func(name string) ([]byte, error) { return readFile(name) }

func readFile(name string) ([]byte, error) {
	return os.ReadFile(name)
}
