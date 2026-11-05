package initiative

import (
	"testing"
	"time"
)

func lifecycleOp(now time.Time) InitiativeOpportunity {
	return InitiativeOpportunity{Exists: true, Source: SourceConversationalContext, Reason: ReasonPendingTask, Context: "Blender pendiente", Target: "Blender", Signal: InitiativeSignal{ID: "sig-1"}, ExpiresAt: now.Add(time.Hour)}
}
func TestB963719_SubblocksLifecycleCreationIdentityAndDedup(t *testing.T) {
	now := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	m := NewLifecycleManager(nil)
	a, e := m.Create(lifecycleOp(now), now)
	if e != nil {
		t.Fatal(e)
	}
	b, e := m.Create(lifecycleOp(now), now.Add(time.Second))
	if e != nil {
		t.Fatal(e)
	}
	if a.ID != b.ID {
		t.Fatalf("duplicate lifecycle created: %s != %s", a.ID, b.ID)
	}
}
func TestB963719_StateTransitionValidation(t *testing.T) {
	now := time.Now()
	m := NewLifecycleManager(nil)
	x, _ := m.Create(lifecycleOp(now), now)
	if _, e := m.Transition(x.ID, LifecycleResolved, now, "test", "bad shortcut"); e == nil {
		t.Fatal("invalid shortcut accepted")
	}
	x, _ = m.Transition(x.ID, LifecycleEvaluating, now, "test", "evaluate")
	x, _ = m.Transition(x.ID, LifecycleCandidate, now, "test", "candidate")
	x, _ = m.Transition(x.ID, LifecycleSelected, now, "test", "selected")
	x, _ = m.Transition(x.ID, LifecycleDecided, now, "B96.37.2", "decision recorded")
	if x.State != LifecycleDecided {
		t.Fatal(x.State)
	}
}
func TestB963719_DecisionIsRecordedNotCreated(t *testing.T) {
	now := time.Now()
	m := NewLifecycleManager(nil)
	x, _ := m.Create(lifecycleOp(now), now)
	d := InitiativeDecision{ID: "d1", Decision: Defer, Opportunity: x.Opportunity, Reason: ReasonPendingTask, Timing: TimingDefer, CreatedAt: now, PolicyVersion: "B96.37.2"}
	x, e := m.RecordDecision(x.ID, d, now)
	if e != nil || x.Decision == nil || x.Decision.Decision != Defer {
		t.Fatalf("decision recording failed: %v", e)
	}
}
func TestB963719_OrphanAndExpiration(t *testing.T) {
	now := time.Now()
	op := lifecycleOp(now)
	op.ExpiresAt = time.Time{}
	m := NewLifecycleManager(nil)
	x, _ := m.Create(op, now)
	o := m.Orphans(now)
	if len(o) != 0 {
		t.Fatal("new DETECTED initiative incorrectly orphaned")
	}
	m.Transition(x.ID, LifecycleEvaluating, now, "test", "x")
	m.Transition(x.ID, LifecycleDeferred, now, "B96.37.2", "deferred")
	if len(m.Orphans(now)) != 1 {
		t.Fatal("deferred orphan not detected")
	}
}
func TestB963719_TerminalStatesAndProvenance(t *testing.T) {
	now := time.Now()
	op := lifecycleOp(now)
	op.ExpiresAt = now.Add(-time.Second)
	m := NewLifecycleManager(nil)
	_, _ = m.Create(op, now.Add(-time.Hour))
	got := m.Expire(now)
	if len(got) != 1 || got[0].State != LifecycleExpired {
		t.Fatal("expiration failed")
	}
	if got[0].Provenance.Subsystem != "B96.37.19" {
		t.Fatal("missing provenance")
	}
}
func TestB963719_AuthorityBoundary(t *testing.T) {
	if CanTransitionLifecycle(LifecycleSelected, LifecycleIntervening) {
		t.Fatal("lifecycle cannot authorize intervention directly")
	}
}

func TestB963719_FindByDecisionID(t *testing.T) {
	now := time.Now()
	m := NewLifecycleManager(nil)
	x, err := m.Create(lifecycleOp(now), now)
	if err != nil {
		t.Fatal(err)
	}
	d := InitiativeDecision{ID: "decision-find", Decision: Intervene, Opportunity: x.Opportunity, Action: ActionComment, CreatedAt: now, PolicyVersion: "B96.37.2"}
	if _, err = m.RecordDecision(x.ID, d, now); err != nil {
		t.Fatal(err)
	}
	got, ok := m.FindByDecisionID(d.ID)
	if !ok || got.ID != x.ID {
		t.Fatalf("decision correlation failed: %+v %v", got, ok)
	}
}
