package initiative

import (
	"reflect"
	"testing"
	"time"
)

func loadTurn(at time.Time, user, assistant, act string) ConversationTurnObservation {
	return ConversationTurnObservation{At: at, UserText: user, Assistant: assistant, SpeechAct: act}
}

func TestB963713_13_1ConversationLoadModel(t *testing.T) {
	now := time.Unix(1000, 0)
	turns := []ConversationTurnObservation{
		loadTurn(now.Add(-20*time.Second), "a", "question?", "question"),
		loadTurn(now.Add(-10*time.Second), "b", "proposal", "propose"),
		loadTurn(now.Add(-5*time.Second), "c", "answer", "inform"),
	}
	a := NewConversationLoadEvaluator().Evaluate(turns, InitiativeOpportunity{}, now)
	if a.Load != LoadNormal || a.RecentInitiatives != 2 {
		t.Fatalf("unexpected load: %+v", a)
	}
}

func TestB963713_13_2InteractionDensity(t *testing.T) {
	now := time.Unix(1000, 0)
	turns := []ConversationTurnObservation{
		loadTurn(now.Add(-50*time.Second), "u", "q", "question"),
		loadTurn(now.Add(-40*time.Second), "u", "q", "question"),
		loadTurn(now.Add(-30*time.Second), "u", "answer", "inform"),
		loadTurn(now.Add(-20*time.Second), "u", "answer", "inform"),
	}
	a := NewConversationLoadEvaluator().Evaluate(turns, InitiativeOpportunity{}, now)
	if a.InteractionDensity != .5 {
		t.Fatalf("density=%v", a.InteractionDensity)
	}
}

func TestB963713_13_3RecentInitiativeTracking(t *testing.T) {
	now := time.Unix(1000, 0)
	turns := []ConversationTurnObservation{loadTurn(now.Add(-5*time.Second), "u", "q", "question")}
	a := NewConversationLoadEvaluator().Evaluate(turns, InitiativeOpportunity{}, now)
	if a.RecentInitiatives != 1 || a.TimeSinceLastInitiative != 5*time.Second {
		t.Fatalf("tracking=%+v", a)
	}
}

func TestB963713_13_4InitiativeFrequency(t *testing.T) {
	now := time.Unix(1000, 0)
	turns := []ConversationTurnObservation{
		loadTurn(now.Add(-20*time.Second), "u", "q", "question"),
		loadTurn(now.Add(-15*time.Second), "u", "q", "question"),
		loadTurn(now.Add(-10*time.Second), "u", "q", "question"),
	}
	a := NewConversationLoadEvaluator().Evaluate(turns, InitiativeOpportunity{}, now)
	if a.Load != LoadHigh || !a.RepeatedIntervention {
		t.Fatalf("frequency=%+v", a)
	}
}

func TestB963713_13_5CooldownModel(t *testing.T) {
	now := time.Unix(1000, 0)
	turns := []ConversationTurnObservation{loadTurn(now.Add(-10*time.Second), "u", "q", "question")}
	a := NewConversationLoadEvaluator().Evaluate(turns, InitiativeOpportunity{}, now)
	if a.Cooldown != CooldownActive || !a.CooldownActive {
		t.Fatalf("cooldown=%+v", a)
	}
}

func TestB963713_13_6CooldownPolicies(t *testing.T) {
	p := DefaultConversationLoadPolicy()
	p.ActiveCooldown = 5 * time.Second
	e := &ConversationLoadEvaluator{Policy: p}
	now := time.Unix(1000, 0)
	turns := []ConversationTurnObservation{loadTurn(now.Add(-10*time.Second), "u", "q", "question")}
	a := e.Evaluate(turns, InitiativeOpportunity{}, now)
	if a.CooldownActive {
		t.Fatalf("policy ignored: %+v", a)
	}
}

func TestB963713_13_7RepeatedInterventionDetection(t *testing.T) {
	now := time.Unix(1000, 0)
	turns := []ConversationTurnObservation{
		loadTurn(now.Add(-20*time.Second), "u", "q", "question"),
		loadTurn(now.Add(-10*time.Second), "u", "q", "question"),
	}
	a := NewConversationLoadEvaluator().Evaluate(turns, InitiativeOpportunity{}, now)
	if !a.RepeatedIntervention || a.ConsecutiveInitiatives != 2 {
		t.Fatalf("repeat=%+v", a)
	}
}

func TestB963713_13_8UserResponseFeedback(t *testing.T) {
	now := time.Unix(1000, 0)
	turns := []ConversationTurnObservation{
		loadTurn(now.Add(-40*time.Second), "u", "q", "question"),
		loadTurn(now.Add(-20*time.Second), "u", "answer", "inform"),
	}
	a := NewConversationLoadEvaluator().Evaluate(turns, InitiativeOpportunity{}, now)
	if a.RecentUserResponses != 2 || a.UserResponseRate != 1 {
		t.Fatalf("feedback=%+v", a)
	}
}

func TestB963713_13_9ConversationSaturation(t *testing.T) {
	now := time.Unix(1000, 0)
	turns := make([]ConversationTurnObservation, 5)
	for i := range turns {
		turns[i] = loadTurn(now.Add(time.Duration(-50+i)*time.Second), "u", "q", "question")
	}
	a := NewConversationLoadEvaluator().Evaluate(turns, InitiativeOpportunity{}, now)
	if a.Load != LoadSaturated || !a.Saturated || a.Cooldown != CooldownSaturated {
		t.Fatalf("saturation=%+v", a)
	}
}

func TestB963713_13_10LoadEscalation(t *testing.T) {
	now := time.Unix(1000, 0)
	turns := []ConversationTurnObservation{loadTurn(now.Add(-30*time.Second), "u", "q", "question"), loadTurn(now.Add(-20*time.Second), "u", "q", "question"), loadTurn(now.Add(-10*time.Second), "u", "q", "question")}
	a := NewConversationLoadEvaluator().Evaluate(turns, InitiativeOpportunity{}, now)
	if a.Load != LoadHigh {
		t.Fatalf("load=%s", a.Load)
	}
}

func TestB963713_13_11LoadRecovery(t *testing.T) {
	now := time.Unix(1000, 0)
	turns := []ConversationTurnObservation{loadTurn(now.Add(-2*time.Minute), "u", "q", "question")}
	a := NewConversationLoadEvaluator().Evaluate(turns, InitiativeOpportunity{}, now)
	if !a.RecoveryEligible || a.CooldownActive {
		t.Fatalf("recovery=%+v", a)
	}
}

func TestB963713_13_12UrgencyInteraction(t *testing.T) {
	now := time.Unix(1000, 0)
	op := InitiativeOpportunity{Urgency: UrgencyCritical}
	a := NewConversationLoadEvaluator().Evaluate(nil, op, now)
	if !a.UrgencyReassessment {
		t.Fatal("critical urgency did not trigger reassessment")
	}
}

func TestB963713_13_13PriorityDoesNotAuthorize(t *testing.T) {
	now := time.Unix(1000, 0)
	op := InitiativeOpportunity{Priority: PriorityCritical}
	a := NewConversationLoadEvaluator().Evaluate(nil, op, now)
	if a.Load != LoadLow {
		t.Fatalf("priority changed load unexpectedly: %+v", a)
	}
}

func TestB963713_13_14InterruptionInteraction(t *testing.T) {
	now := time.Unix(1000, 0)
	op := InitiativeOpportunity{Interruptibility: InterruptForbidden}
	a := NewConversationLoadEvaluator().Evaluate(nil, op, now)
	if a.CooldownActive {
		t.Fatalf("load layer changed interruption state: %+v", a)
	}
}

func TestB963713_13_15AvailabilityInteraction(t *testing.T) {
	now := time.Unix(1000, 0)
	op := InitiativeOpportunity{UserAvailability: AvailabilityExplicitlyUnavailable}
	a := NewConversationLoadEvaluator().Evaluate(nil, op, now)
	if a.Load != LoadLow {
		t.Fatalf("availability changed load: %+v", a)
	}
}

func TestB963713_13_16TimingAssessmentIsNotDecision(t *testing.T) {
	typ := reflect.TypeOf(ConversationLoadAssessment{})
	for _, name := range []string{"Decision", "Action"} {
		if _, ok := typ.FieldByName(name); ok {
			t.Fatalf("load assessment owns %s", name)
		}
	}
}

func TestB963713_13_17OpportunityEnrichment(t *testing.T) {
	now := time.Unix(1000, 0)
	turns := []ConversationTurnObservation{loadTurn(now.Add(-10*time.Second), "u", "q", "question")}
	op, a, err := NewConversationLoadEvaluator().EnrichOpportunity(InitiativeOpportunity{}, turns, now)
	if err != nil {
		t.Fatal(err)
	}
	if op.ConversationLoad != a.Load || op.CooldownActive != a.CooldownActive {
		t.Fatalf("enrichment mismatch: op=%+v assessment=%+v", op, a)
	}
}

func TestB963713_13_20UnitContract(t *testing.T) {
	now := time.Unix(1000, 0)
	a := NewConversationLoadEvaluator().Evaluate(nil, InitiativeOpportunity{}, now)
	if err := a.Validate(); err != nil {
		t.Fatal(err)
	}
}
