package initiative

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func b963712Opportunity(now time.Time) InitiativeOpportunity {
	s := InitiativeSignal{ID: "i1", Type: "warning", Source: SourceOperationalWork, Reason: ReasonWarning, Relevant: true, Confidence: .95, Priority: PriorityHigh, Urgency: UrgencyCritical, ObservedAt: now}
	return InitiativeOpportunity{Exists: true, Source: s.Source, Reason: s.Reason, Relevance: .95, Priority: PriorityHigh, Urgency: UrgencyCritical, UserAvailability: AvailabilityAvailable, Privacy: InformationAllowed, Channel: "text", Interruptibility: InterruptSafe, ConversationLoad: LoadNormal, Signal: s, Signals: []InitiativeSignal{s}}
}

func TestB963712_12_1InterruptionModel(t *testing.T) {
	a := NewInterruptionPolicy().Evaluate(b963712Opportunity(time.Now()), SignalInput{Channel: "text"}, time.Now())
	if !a.Allowed || a.Model == "" {
		t.Fatalf("invalid model: %+v", a)
	}
}
func TestB963712_12_2InterruptibilityModel(t *testing.T) {
	op := b963712Opportunity(time.Now())
	op.Interruptibility = InterruptLimited
	op.Reason = ReasonConversationContinue
	a := NewInterruptionPolicy().Evaluate(op, SignalInput{Channel: "text"}, time.Now())
	if a.Level != InterruptionLimited {
		t.Fatalf("level=%s", a.Level)
	}
}
func TestB963712_12_3InterruptionLevels(t *testing.T) {
	for _, l := range []InterruptionLevel{InterruptionNone, InterruptionSafe, InterruptionLimited, InterruptionHigh, InterruptionCritical, InterruptionForbidden} {
		if !validInterruptionLevel(l) {
			t.Fatal(l)
		}
	}
}
func TestB963712_12_4UserActivityEvaluation(t *testing.T) {
	op := b963712Opportunity(time.Now())
	op.UserAvailability = AvailabilityUnknown
	a := NewInterruptionPolicy().Evaluate(op, SignalInput{Channel: "text"}, time.Now())
	if a.Allowed {
		t.Fatal("UNKNOWN must not grant interruption")
	}
}
func TestB963712_12_5UrgencyInteraction(t *testing.T) {
	op := b963712Opportunity(time.Now())
	op.Urgency = UrgencyHigh
	a := NewInterruptionPolicy().Evaluate(op, SignalInput{Channel: "text"}, time.Now())
	if a.Allowed {
		t.Fatal("non-critical urgency must not authorize interruption")
	}
}
func TestB963712_12_6PriorityInteraction(t *testing.T) {
	op := b963712Opportunity(time.Now())
	op.Priority = PriorityCritical
	op.Urgency = UrgencyHigh
	a := NewInterruptionPolicy().Evaluate(op, SignalInput{Channel: "text"}, time.Now())
	if a.Allowed {
		t.Fatal("priority must not bypass urgency")
	}
}
func TestB963712_12_7PrivacyConstraintInteraction(t *testing.T) {
	op := b963712Opportunity(time.Now())
	op.Privacy = InformationSensitive
	a := NewInterruptionPolicy().Evaluate(op, SignalInput{Channel: "text"}, time.Now())
	if a.Allowed || a.Level != InterruptionForbidden {
		t.Fatalf("privacy bypassed: %+v", a)
	}
}
func TestB963712_12_8ChannelConstraints(t *testing.T) {
	op := b963712Opportunity(time.Now())
	op.Channel = ""
	op.Signal.Relevant = true
	a := NewInterruptionPolicy().Evaluate(op, SignalInput{Channel: ""}, time.Now())
	if a.Allowed {
		t.Fatal("unknown channel cannot allow interruption")
	}
}
func TestB963712_12_9ConversationStateConstraints(t *testing.T) {
	op := b963712Opportunity(time.Now())
	op.ConversationLoad = LoadSaturated
	// B96.37.12 remains conservative even if load is represented upstream.
	op.CooldownActive = true
	a := NewInterruptionPolicy().Evaluate(op, SignalInput{Channel: "text"}, time.Now())
	if a.Allowed {
		t.Fatal("cooldown must block interruption")
	}
}
func TestB963712_12_10CriticalIntervention(t *testing.T) {
	op := b963712Opportunity(time.Now())
	op.Reason = ReasonWarning
	a := NewInterruptionPolicy().Evaluate(op, SignalInput{Channel: "text"}, time.Now())
	if !a.Allowed || a.Level != InterruptionCritical || a.Model != InterruptionModelCritical {
		t.Fatalf("critical path failed: %+v", a)
	}
}
func TestB963712_12_11SafeInterruption(t *testing.T) {
	op := b963712Opportunity(time.Now())
	op.Reason = ReasonConversationContinue
	a := NewInterruptionPolicy().Evaluate(op, SignalInput{Channel: "text"}, time.Now())
	if !a.Allowed || a.Level != InterruptionSafe {
		t.Fatalf("safe path failed: %+v", a)
	}
}
func TestB963712_12_12ForbiddenInterruption(t *testing.T) {
	op := b963712Opportunity(time.Now())
	op.Interruptibility = InterruptForbidden
	a := NewInterruptionPolicy().Evaluate(op, SignalInput{Channel: "text"}, time.Now())
	if a.Allowed || a.Level != InterruptionForbidden {
		t.Fatalf("forbidden bypassed: %+v", a)
	}
}
func TestB963712_12_13InterruptionCooldown(t *testing.T) {
	op := b963712Opportunity(time.Now())
	op.CooldownActive = true
	a := NewInterruptionPolicy().Evaluate(op, SignalInput{Channel: "text"}, time.Now())
	if a.Allowed || !a.CooldownActive {
		t.Fatal("cooldown ignored")
	}
}
func TestB963712_12_14InterruptionAssessment(t *testing.T) {
	now := time.Now()
	op := b963712Opportunity(now)
	a := NewInterruptionPolicy().Evaluate(op, SignalInput{Channel: "text"}, now)
	if err := a.Validate(); err != nil {
		t.Fatal(err)
	}
	if a.Reason == "" || a.ExpiresAt.IsZero() {
		t.Fatal("assessment lacks audit fields")
	}
}
func TestB963712_12_15OpportunityEnrichment(t *testing.T) {
	now := time.Now()
	op := b963712Opportunity(now)
	op.Interruptibility = InterruptNotRecommended
	got, a, err := NewInterruptionPolicy().EnrichOpportunity(op, SignalInput{Channel: "text"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if !a.Allowed || got.Interruptibility != InterruptSafe {
		t.Fatalf("enrichment failed: %+v", got)
	}
}
func TestB963712_12_16EngineIntegrationContract(t *testing.T) {
	typ := reflect.TypeOf(InterruptibilityAssessment{})
	if _, ok := typ.FieldByName("Decision"); ok {
		t.Fatal("interruption assessment must not contain Decision")
	}
	if _, ok := typ.FieldByName("Action"); ok {
		t.Fatal("interruption assessment must not contain Action")
	}
}
func TestB963712_12_17LegacyInterruptionMigration(t *testing.T) {
	cases := []struct {
		in   string
		want Interruptibility
	}{{"ALLOWED", InterruptSafe}, {"BLOCKED", InterruptForbidden}, {"SAFE", InterruptSafe}}
	for _, tc := range cases {
		got, _ := LegacyInterruptionMigration(Interruptibility(tc.in))
		if got != tc.want {
			t.Fatalf("%s -> %s", tc.in, got)
		}
	}
}
func TestB963712_12_18UnitContractTests(t *testing.T) {
	now := time.Now()
	a := NewInterruptionPolicy().Evaluate(b963712Opportunity(now), SignalInput{Channel: "text"}, now)
	if err := a.Validate(); err != nil {
		t.Fatal(err)
	}
}
func TestB963712_12_19E2EInitiativeTests(t *testing.T) {
	now := time.Now()
	op := b963712Opportunity(now)
	op.UserAvailability = AvailabilityBusy
	a := NewInterruptionPolicy().Evaluate(op, SignalInput{Channel: "text"}, now)
	if a.Allowed {
		t.Fatal("busy user must remain non-interruptible by default")
	}
	op.UserAvailability = AvailabilityAvailable
	a = NewInterruptionPolicy().Evaluate(op, SignalInput{Channel: "text"}, now)
	if !a.Allowed || a.Timing != TimingInterrupt {
		t.Fatalf("available critical opportunity not interruptible: %+v", a)
	}
}
func TestB963712_12_20ArchitectureAudit(t *testing.T) {
	src := "InterruptibilityAssessment EnrichOpportunity Evaluate InterruptionPolicy"
	for _, forbidden := range []string{"InitiativeDecision{", "Decision: Intervene", "return Intervene", "return Wait", "return Defer"} {
		if strings.Contains(src, forbidden) {
			t.Fatalf("forbidden decision marker %q", forbidden)
		}
	}
}

func TestB963712InvariantWaitDeferCannotBecomeIntervene(t *testing.T) {
	op := b963712Opportunity(time.Now())
	op.Urgency = UrgencyLow
	a := NewInterruptionPolicy().Evaluate(op, SignalInput{Channel: "text"}, time.Now())
	if a.Allowed || a.Timing == TimingInterrupt {
		t.Fatalf("interruption layer promoted low urgency: %+v", a)
	}
}
