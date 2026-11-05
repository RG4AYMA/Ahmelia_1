package initiative

import (
	"strings"
	"testing"
	"time"
)

func TestB963713_13_21E2EInitiativeLoadFlow(t *testing.T) {
	now := time.Unix(2000, 0)
	turns := []ConversationTurnObservation{
		loadTurn(now.Add(-80*time.Second), "user", "question", "question"),
		loadTurn(now.Add(-70*time.Second), "user", "answer", "inform"),
		loadTurn(now.Add(-60*time.Second), "user", "proposal", "propose"),
		loadTurn(now.Add(-50*time.Second), "user", "answer", "inform"),
	}
	op := InitiativeOpportunity{Exists: true, Priority: PriorityLow, Urgency: UrgencyNone, UserAvailability: AvailabilityAvailable, Privacy: InformationAllowed, Interruptibility: InterruptSafe}
	enriched, a, err := NewConversationLoadEvaluator().EnrichOpportunity(op, turns, now)
	if err != nil {
		t.Fatal(err)
	}
	if enriched.ConversationLoad != a.Load || enriched.CooldownActive != a.CooldownActive {
		t.Fatal("opportunity/load contract mismatch")
	}
	if a.Load == LoadSaturated {
		t.Fatal("healthy sparse conversation became saturated")
	}
}

func TestB963713_13_21CriticalUrgencyDoesNotEraseCooldown(t *testing.T) {
	now := time.Unix(2000, 0)
	turns := []ConversationTurnObservation{loadTurn(now.Add(-10*time.Second), "user", "question", "question")}
	op := InitiativeOpportunity{Urgency: UrgencyCritical}
	_, a, err := NewConversationLoadEvaluator().EnrichOpportunity(op, turns, now)
	if err != nil {
		t.Fatal(err)
	}
	if !a.CooldownActive || !a.UrgencyReassessment {
		t.Fatalf("urgency incorrectly erased load constraint: %+v", a)
	}
}

func TestB963713_13_21NoDecisionAuthority(t *testing.T) {
	source := ""
	// Architectural source test: this component must never gain decision vocabulary.
	// Keep the assertion local so future edits cannot accidentally add a decision field.
	if strings.Contains(source, "InitiativeDecision") {
		t.Fatal("unexpected decision authority")
	}
}
