package initiative

import "testing"

func TestB963719_ArchitectureAudit(t *testing.T) {
	if CanTransitionLifecycle(LifecycleSelected, LifecycleIntervening) {
		t.Fatal("selected cannot authorize intervention")
	}
	if CanTransitionLifecycle(LifecycleSelected, LifecycleExecuting) {
		t.Fatal("selected cannot execute")
	}
	if !CanTransitionLifecycle(LifecycleDecided, LifecycleDeferred) {
		t.Fatal("decided must record defer")
	}
	if !CanTransitionLifecycle(LifecycleDecided, LifecycleIntervening) {
		t.Fatal("decided must record canonical intervention")
	}
}
