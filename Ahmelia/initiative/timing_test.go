package initiative

import (
	"testing"
	"time"
)

func TestTimingEvaluatorNow(t *testing.T) {
	now := time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)
	got := NewTimingEvaluator().Evaluate(InitiativeOpportunity{
		Exists: true, UserAvailability: AvailabilityAvailable,
		Interruptibility: InterruptSafe, ConversationLoad: LoadNormal,
	}, now)
	if err := ValidateTimingAssessment(got); err != nil {
		t.Fatal(err)
	}
	if got.Timing != TimingNow || !got.Admissible {
		t.Fatalf("unexpected timing: %+v", got)
	}
}

func TestTimingEvaluatorDefersBusyNonCritical(t *testing.T) {
	now := time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)
	got := NewTimingEvaluator().Evaluate(InitiativeOpportunity{
		UserAvailability: AvailabilityBusy, Urgency: UrgencyHigh,
		Interruptibility: InterruptSafe, ConversationLoad: LoadNormal,
	}, now)
	if got.Timing != TimingDefer || got.Admissible || !got.RequiresDeferral {
		t.Fatalf("unexpected timing: %+v", got)
	}
	if err := ValidateTimingAssessment(got); err != nil {
		t.Fatal(err)
	}
}

func TestTimingEvaluatorCriticalInterrupt(t *testing.T) {
	now := time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)
	got := NewTimingEvaluator().Evaluate(InitiativeOpportunity{
		UserAvailability: AvailabilityDistracted, Urgency: UrgencyCritical,
		Interruptibility: InterruptSafe, ConversationLoad: LoadNormal,
	}, now)
	if got.Timing != TimingInterrupt || !got.Admissible || !got.Interruptible {
		t.Fatalf("unexpected timing: %+v", got)
	}
	if err := ValidateTimingAssessment(got); err != nil {
		t.Fatal(err)
	}
}

func TestTimingEvaluatorHardBlocksInterrupt(t *testing.T) {
	now := time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)
	got := NewTimingEvaluator().Evaluate(InitiativeOpportunity{
		UserAvailability: AvailabilityBusy, Urgency: UrgencyCritical,
		Interruptibility: InterruptForbidden, ConversationLoad: LoadNormal,
	}, now)
	if got.Timing != TimingDefer || got.Admissible {
		t.Fatalf("unexpected timing: %+v", got)
	}
}

func TestTimingEvaluatorCooldownWins(t *testing.T) {
	now := time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)
	got := NewTimingEvaluator().Evaluate(InitiativeOpportunity{
		UserAvailability: AvailabilityAvailable, Interruptibility: InterruptSafe,
		ConversationLoad: LoadNormal, CooldownActive: true,
	}, now)
	if got.Timing != TimingDefer || got.Reason != "cooldown_active" {
		t.Fatalf("unexpected timing: %+v", got)
	}
}
