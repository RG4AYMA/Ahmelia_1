package initiative

import (
	"strings"
	"testing"
	"time"
)

func schedDecision(now time.Time, availability UserAvailability, cooldown bool) InitiativeDecision {
	return InitiativeDecision{ID: "d1", Decision: Defer, Opportunity: InitiativeOpportunity{Exists: true, Source: SourceConversationalContext, Reason: ReasonPendingTask, Priority: PriorityHigh, Urgency: UrgencyMedium, UserAvailability: availability, CooldownActive: cooldown, Context: "topic-x", Target: "topic-x"}, Reason: ReasonPendingTask, Priority: PriorityHigh, Timing: TimingDefer, CreatedAt: now, ExpiresAt: now.Add(time.Hour), Provenance: Provenance{Component: "test", CreatedAt: now}, PolicyVersion: "B96.37.2"}
}

func TestB9637_18_ScheduleModelAndValidation(t *testing.T) {
	now := time.Now()
	s := Schedule{ID: "s", Mode: ScheduleDeferred, Trigger: ScheduleTriggerAvailability, CreatedAt: now, State: SchedulePending, Revision: 1}
	if err := s.Valid(); err != nil {
		t.Fatal(err)
	}
}
func TestB9637_18_ImmediateSchedulingNeverAuthorizes(t *testing.T) {
	now := time.Now()
	sch := NewInitiativeScheduler(nil)
	d := schedDecision(now, AvailabilityAvailable, false)
	d.Timing = TimingNow
	d.Opportunity.UserAvailability = AvailabilityAvailable
	d.Opportunity.CooldownActive = false
	got, created, err := sch.ScheduleDecision(d, now)
	if err != nil || !created {
		t.Fatalf("schedule: %v %v", err, created)
	}
	if got.State != SchedulePending {
		t.Fatal("must remain pending until trigger/reevaluation")
	}
}
func TestB9637_18_DeferredAvailabilityScheduling(t *testing.T) {
	now := time.Now()
	sch := NewInitiativeScheduler(nil)
	got, _, err := sch.ScheduleDecision(schedDecision(now, AvailabilityBusy, false), now)
	if err != nil {
		t.Fatal(err)
	}
	if got.Trigger != ScheduleTriggerAvailability || got.Mode != ScheduleDeferred {
		t.Fatalf("got %+v", got)
	}
	tr := sch.Trigger(ScheduleTriggerAvailability, "availability", "available", now.Add(time.Minute))
	if len(tr) != 1 || tr[0].State != ScheduleTriggered {
		t.Fatalf("trigger=%+v", tr)
	}
}
func TestB9637_18_CooldownTrigger(t *testing.T) {
	now := time.Now()
	sch := NewInitiativeScheduler(nil)
	got, _, err := sch.ScheduleDecision(schedDecision(now, AvailabilityAvailable, true), now)
	if err != nil {
		t.Fatal(err)
	}
	if got.Trigger != ScheduleTriggerCooldown {
		t.Fatalf("got %+v", got)
	}
	if len(sch.Trigger(ScheduleTriggerCooldown, "cooldown", "recovery", now.Add(time.Minute))) != 1 {
		t.Fatal("expected cooldown trigger")
	}
}
func TestB9637_18_TimeDue(t *testing.T) {
	now := time.Now()
	sch := NewInitiativeScheduler(nil)
	d := schedDecision(now, AvailabilityAvailable, false)
	d.Timing = TimingDefer
	got, _, _ := sch.ScheduleDecision(d, now)
	x, _ := sch.Store.Get(got.ID)
	x.Mode = ScheduleTime
	x.Trigger = ScheduleTriggerTime
	x.NotBefore = now.Add(2 * time.Minute)
	x.Revision++
	if err := sch.Store.Put(x); err != nil {
		t.Fatal(err)
	}
	if len(sch.Due(now.Add(time.Minute))) != 0 {
		t.Fatal("fired early")
	}
	if len(sch.Due(now.Add(3*time.Minute))) != 1 {
		t.Fatal("did not fire")
	}
}
func TestB9637_18_Expiration(t *testing.T) {
	now := time.Now()
	sch := NewInitiativeScheduler(nil)
	sch.Policy.DefaultTTL = time.Minute
	d := schedDecision(now, AvailabilityBusy, false)
	d.ExpiresAt = time.Time{}
	got, _, _ := sch.ScheduleDecision(d, now)
	expired := sch.Expire(now.Add(2 * time.Minute))
	if len(expired) != 1 || expired[0].State != ScheduleExpired {
		t.Fatalf("%+v", expired)
	}
	x, _ := sch.Store.Get(got.ID)
	if x.State != ScheduleExpired {
		t.Fatal("not persisted")
	}
}
func TestB9637_18_ReevaluationReturnsAuthorityToPolicy(t *testing.T) {
	now := time.Now()
	sch := NewInitiativeScheduler(nil)
	got, _, _ := sch.ScheduleDecision(schedDecision(now, AvailabilityBusy, false), now)
	_, _ = sch.Trigger(ScheduleTriggerAvailability, "availability", "available", now.Add(time.Minute)), 0
	_, err := sch.BeginReevaluation(got.ID, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	policy := DefaultPolicy()
	d := policy.Evaluate(got.Opportunity, now.Add(time.Minute))
	if !d.Decision.Valid() {
		t.Fatal("invalid policy decision")
	}
	completed, err := sch.CompleteReevaluation(got.ID, d, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if completed.State != ScheduleCompleted {
		t.Fatalf("%+v", completed)
	}
	if strings.Contains(strings.ToLower(string(d.Decision)), "speak") {
		t.Fatal("scheduler must not authorize speech")
	}
}
func TestB9637_18_DeduplicationAndCancellation(t *testing.T) {
	now := time.Now()
	sch := NewInitiativeScheduler(nil)
	d := schedDecision(now, AvailabilityBusy, false)
	a, created, _ := sch.ScheduleDecision(d, now)
	b, created2, _ := sch.ScheduleDecision(d, now.Add(time.Second))
	if !created || created2 || a.ID != b.ID {
		t.Fatalf("dedup failed: %v %v %s %s", created, created2, a.ID, b.ID)
	}
	c, err := sch.Cancel(a.ID, now.Add(time.Second))
	if err != nil || c.State != ScheduleCancelled {
		t.Fatalf("%+v %v", c, err)
	}
}
func TestB9637_18_TriggerMapping(t *testing.T) {
	pairs := map[RecheckTrigger]ScheduleTriggerType{TriggerAvailability: ScheduleTriggerAvailability, TriggerContext: ScheduleTriggerContext, TriggerTopic: ScheduleTriggerContext, TriggerPriority: ScheduleTriggerCondition, TriggerUrgency: ScheduleTriggerCondition, TriggerCooldown: ScheduleTriggerCooldown, TriggerUserResponse: ScheduleTriggerEvent, TriggerExplicit: ScheduleTriggerExplicit}
	for a, b := range pairs {
		if TriggerForRecheck(a) != b {
			t.Fatalf("%s", a)
		}
	}
}
