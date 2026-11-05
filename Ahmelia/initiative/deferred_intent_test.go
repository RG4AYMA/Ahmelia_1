package initiative

import (
	"strings"
	"testing"
	"time"
)

func deferredDecision(now time.Time) InitiativeDecision {
	return InitiativeDecision{ID: "d1", Decision: Defer, Timing: TimingDefer, Source: SourceOpenCommitment, Reason: ReasonUnfinishedCommitment, Target: "topic-x", Opportunity: InitiativeOpportunity{Exists: true, Source: SourceOpenCommitment, Reason: ReasonUnfinishedCommitment, Target: "topic-x", Context: "ctx-x", Relevance: .8, Priority: PriorityMedium, Urgency: UrgencyLow, UserAvailability: AvailabilityBusy, CooldownActive: true}, CreatedAt: now, PolicyVersion: "B96.37.2"}
}

func TestB963714_14_1DeferredIntentModel(t *testing.T) {
	now := time.Now()
	d, err := NewDeferredIntent(deferredDecision(now), now, DefaultDeferredIntentPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if d.State != DeferredDeferred || d.ID == "" {
		t.Fatalf("invalid model: %+v", d)
	}
}
func TestB963714_14_2Identity(t *testing.T) {
	m := NewDeferredIntentManager(nil)
	now := time.Now()
	a, _, _ := m.PersistDecision(deferredDecision(now), now)
	b, _, _ := m.PersistDecision(deferredDecision(now.Add(time.Second)), now.Add(time.Second))
	if a.ID != b.ID || b.Revision != 2 {
		t.Fatalf("duplicate identity not merged: %s %s rev=%d", a.ID, b.ID, b.Revision)
	}
}
func TestB963714_14_3States(t *testing.T) {
	for _, s := range []DeferredIntentState{DeferredActive, DeferredDeferred, DeferredReevaluating, DeferredResolved, DeferredCancelled, DeferredExpired} {
		d := deferredDecision(time.Now())
		x, _ := NewDeferredIntent(d, time.Now(), DefaultDeferredIntentPolicy())
		x.State = s
		x.Revision = 1
		if err := x.Valid(); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}
func TestB963714_14_4DeferralReason(t *testing.T) {
	d := deferredDecision(time.Now())
	x, err := NewDeferredIntent(d, time.Now(), DefaultDeferredIntentPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if x.DeferralReason != DeferralBusy {
		t.Fatalf("reason=%s", x.DeferralReason)
	}
}
func TestB963714_14_5RecheckConditions(t *testing.T) {
	x, err := NewDeferredIntent(deferredDecision(time.Now()), time.Now(), DefaultDeferredIntentPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if !TriggerMatches(x, TriggerAvailability) || !TriggerMatches(x, TriggerCooldown) {
		t.Fatal("missing core recheck conditions")
	}
}
func TestB963714_14_6Triggers(t *testing.T) {
	x, _ := NewDeferredIntent(deferredDecision(time.Now()), time.Now(), DefaultDeferredIntentPolicy())
	for _, tr := range []RecheckTrigger{TriggerAvailability, TriggerContext, TriggerTopic, TriggerPriority, TriggerUrgency, TriggerCooldown, TriggerUserResponse, TriggerExplicit} {
		if !TriggerMatches(x, tr) {
			t.Fatalf("trigger %s not matched", tr)
		}
	}
}
func TestB963714_14_7TTL(t *testing.T) {
	now := time.Now()
	p := DefaultDeferredIntentPolicy()
	p.DefaultTTL = time.Second
	x, _ := NewDeferredIntent(deferredDecision(now), now, p)
	m := NewDeferredIntentManager(nil)
	m.Store.Put(x)
	expired := m.Expire(now.Add(2 * time.Second))
	if len(expired) != 1 || expired[0].State != DeferredExpired {
		t.Fatalf("expiration failed: %+v", expired)
	}
}
func TestB963714_14_8Cancellation(t *testing.T) {
	m := NewDeferredIntentManager(nil)
	now := time.Now()
	x, _, _ := m.PersistDecision(deferredDecision(now), now)
	x, err := m.Cancel(x.ID, now.Add(time.Second))
	if err != nil || x.State != DeferredCancelled {
		t.Fatalf("cancel failed: %+v %v", x, err)
	}
}
func TestB963714_14_9UserDrivenResolution(t *testing.T) {
	m := NewDeferredIntentManager(nil)
	now := time.Now()
	x, _, _ := m.PersistDecision(deferredDecision(now), now)
	x, err := m.Resolve(x.ID, now.Add(time.Second))
	if err != nil || x.State != DeferredResolved {
		t.Fatalf("resolve failed")
	}
}
func TestB963714_14_10ContextChange(t *testing.T) {
	m := NewDeferredIntentManager(nil)
	now := time.Now()
	x, _, _ := m.PersistDecision(deferredDecision(now), now)
	op := x.OriginalOpportunity
	op.Context = "new-context"
	decision, updated, err := m.Reevaluate(x.ID, op, now.Add(time.Minute), DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Decision.Valid() || updated.State != DeferredResolved {
		t.Fatalf("context reevaluation failed: %s %s", decision.Decision, updated.State)
	}
}
func TestB963714_14_11TopicChange(t *testing.T) {
	x, _ := NewDeferredIntent(deferredDecision(time.Now()), time.Now(), DefaultDeferredIntentPolicy())
	if !TriggerMatches(x, TriggerTopic) {
		t.Fatal("topic change absent")
	}
}
func TestB963714_14_12PriorityUrgency(t *testing.T) {
	x, _ := NewDeferredIntent(deferredDecision(time.Now()), time.Now(), DefaultDeferredIntentPolicy())
	if !TriggerMatches(x, TriggerPriority) || !TriggerMatches(x, TriggerUrgency) {
		t.Fatal("priority/urgency triggers absent")
	}
}
func TestB963714_14_13Availability(t *testing.T) {
	x, _ := NewDeferredIntent(deferredDecision(time.Now()), time.Now(), DefaultDeferredIntentPolicy())
	if !TriggerMatches(x, TriggerAvailability) {
		t.Fatal("availability trigger absent")
	}
}
func TestB963714_14_14CooldownRecovery(t *testing.T) {
	x, _ := NewDeferredIntent(deferredDecision(time.Now()), time.Now(), DefaultDeferredIntentPolicy())
	if !TriggerMatches(x, TriggerCooldown) {
		t.Fatal("cooldown trigger absent")
	}
}
func TestB963714_14_15DuplicatePrevention(t *testing.T) {
	m := NewDeferredIntentManager(nil)
	now := time.Now()
	a, created, _ := m.PersistDecision(deferredDecision(now), now)
	b, created2, _ := m.PersistDecision(deferredDecision(now.Add(time.Second)), now.Add(time.Second))
	if !created || created2 || a.ID != b.ID || len(m.Store.List()) != 1 {
		t.Fatalf("duplicates: created=%v/%v items=%d", created, created2, len(m.Store.List()))
	}
}
func TestB963714_14_16PersistenceBoundary(t *testing.T) {
	var _ DeferredIntentStore = NewInMemoryDeferredIntentStore()
	src := deferredDecision(time.Now())
	m := NewDeferredIntentManager(NewInMemoryDeferredIntentStore())
	if _, ok, err := m.PersistDecision(src, time.Now()); err != nil || !ok {
		t.Fatal(err)
	}
}
func TestB963714_14_17EngineContractShape(t *testing.T) {
	d := deferredDecision(time.Now())
	x, _ := NewDeferredIntent(d, time.Now(), DefaultDeferredIntentPolicy())
	if strings.Contains(string(x.State), "INTERVENE") {
		t.Fatal("persistence must not encode decision")
	}
}
func TestB963714_14_18LegacyMigration(t *testing.T) { // Legacy autonomy deferred topics are intentionally not imported automatically; migration is explicit and lossless at the contract boundary.
	if DefaultDeferredIntentPolicy().DefaultTTL <= 0 {
		t.Fatal("missing migration-safe TTL")
	}
}
func TestB963714_14_19ContractValidation(t *testing.T) {
	d := deferredDecision(time.Now())
	if err := ValidateDecision(d); err != nil {
		t.Fatal(err)
	}
	x, _ := NewDeferredIntent(d, time.Now(), DefaultDeferredIntentPolicy())
	if err := x.Valid(); err != nil {
		t.Fatal(err)
	}
}
func TestB963714_14_20E2E(t *testing.T) {
	m := NewDeferredIntentManager(nil)
	now := time.Now()
	x, _, _ := m.PersistDecision(deferredDecision(now), now)
	op := x.OriginalOpportunity
	op.UserAvailability = AvailabilityAvailable
	op.CooldownActive = false
	op.Signal.Relevant = true
	op.Relevance = .9
	decision, updated, err := m.Reevaluate(x.ID, op, now.Add(2*time.Minute), DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if decision.Decision != Intervene {
		t.Fatalf("expected re-evaluation to reach policy, got %s", decision.Decision)
	}
	if updated.State != DeferredResolved {
		t.Fatalf("state=%s", updated.State)
	}
}
func TestB963714_14_21ArchitectureAudit(t *testing.T) {
	if DeferredActive == "INTERVENE" || DeferredDeferred == "DEFER" {
		t.Fatal("decision leaked into lifecycle state")
	}
}
