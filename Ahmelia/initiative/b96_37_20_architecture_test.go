package initiative

import (
	"testing"
	"time"
)

func TestB963720_ArchitectureBoundary(t *testing.T) {
	// Compile-time/API-level guard: observability exposes recording/querying
	// primitives only. Authorization remains on Policy.Evaluate.
	var _ interface {
		Evaluate(InitiativeOpportunity, time.Time) InitiativeDecision
	} = Policy{}
	_ = t
}
