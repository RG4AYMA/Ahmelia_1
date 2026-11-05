package initiative

import (
	"testing"
	"time"
)

func TestBeliefIdleWithinSessionIsAvailableAndLongIdleIsAway(t *testing.T) {
	b := NewPresenceBelief()
	now := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)
	a, ok := b.Assess(AvailabilityEvidence{LastUserActivity: now.Add(-10 * time.Minute), Now: now})
	if !ok || a.Availability != AvailabilityAvailable {
		t.Fatalf("%+v", a)
	}
	a, _ = b.Assess(AvailabilityEvidence{LastUserActivity: now.Add(-2 * time.Hour), Now: now})
	if a.Availability != AvailabilityAway {
		t.Fatalf("%+v", a)
	}
}

func TestBeliefWithoutEvidenceIsUnknownAndLearnsCircadianPrior(t *testing.T) {
	b := NewPresenceBelief()
	now := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)
	if _, ok := b.Assess(AvailabilityEvidence{Now: now}); ok {
		t.Fatal("no evidence must stay unknown")
	}
	for i := 0; i < 30; i++ {
		b.Observe(now, true)
	}
	a, _ := b.Assess(AvailabilityEvidence{LastUserActivity: now.Add(-2 * time.Hour), Now: now})
	if a.Availability != AvailabilityUnknown {
		t.Fatalf("a learned active hour must soften AWAY to UNKNOWN: %+v", a)
	}
}
