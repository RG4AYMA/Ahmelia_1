package initiative

import (
	"testing"
	"time"
)

func TestAvailabilityExplicitWins(t *testing.T) {
	now := time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)
	a := NewAvailabilityPolicy().Evaluate(AvailabilityEvidence{Explicit: AvailabilityBusy, CurrentTurnActive: true, Now: now})
	if a.Availability != AvailabilityBusy || a.Confidence != 1 || a.Inferred {
		t.Fatalf("unexpected: %+v", a)
	}
	if err := ValidateAvailabilityAssessment(a); err != nil {
		t.Fatal(err)
	}
}

func TestAvailabilityCurrentTurn(t *testing.T) {
	now := time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)
	a := NewAvailabilityPolicy().Evaluate(AvailabilityEvidence{Explicit: AvailabilityUnknown, CurrentTurnActive: true, Now: now})
	if a.Availability != AvailabilityAvailable || !a.Inferred {
		t.Fatalf("unexpected: %+v", a)
	}
}

func TestAvailabilityRecentActivity(t *testing.T) {
	now := time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)
	a := NewAvailabilityPolicy().Evaluate(AvailabilityEvidence{LastUserActivity: now.Add(-90 * time.Second), Now: now})
	if a.Availability != AvailabilityAvailable {
		t.Fatalf("unexpected: %+v", a)
	}
}

// B96.45-R: la inactividad ya no es "desconocida": se estima con PresenceBelief.
func TestAvailabilityStaleActivityIsEstimatedNotUnknown(t *testing.T) {
	now := time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)
	a := NewAvailabilityPolicy().Evaluate(AvailabilityEvidence{LastUserActivity: now.Add(-3 * time.Minute), Now: now})
	if a.Availability != AvailabilityAvailable || a.Reason == "availability_not_observed" {
		t.Fatalf("unexpected: %+v", a)
	}
	none := NewAvailabilityPolicy().Evaluate(AvailabilityEvidence{Now: now})
	if none.Availability != AvailabilityUnknown {
		t.Fatalf("no evidence must stay unknown: %+v", none)
	}
}

func TestAvailabilityWaitingForUser(t *testing.T) {
	now := time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)
	a := NewAvailabilityPolicy().Evaluate(AvailabilityEvidence{WaitingForUser: true, Now: now})
	if a.Availability != AvailabilityAvailable {
		t.Fatalf("unexpected: %+v", a)
	}
}

func TestAvailabilityDoesNotAuthorizeInitiative(t *testing.T) {
	now := time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)
	a := NewAvailabilityPolicy().Evaluate(AvailabilityEvidence{CurrentTurnActive: true, Now: now})
	if a.Availability != AvailabilityAvailable {
		t.Fatalf("unexpected: %+v", a)
	}
	op := InitiativeOpportunity{Exists: true, Relevance: .9, Signal: InitiativeSignal{Relevant: true}, UserAvailability: a.Availability, Privacy: InformationAllowed, Interruptibility: InterruptSafe, ConversationLoad: LoadNormal}
	d := DefaultPolicy().Evaluate(op, now)
	if d.Decision != Intervene {
		t.Fatalf("availability must not itself authorize; policy result here is expected from complete opportunity: %+v", d)
	}
}
