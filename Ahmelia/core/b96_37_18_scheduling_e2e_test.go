package core

import (
	conversationalinitiative "ahmelia/initiative"
	"testing"
	"time"
)

func TestB9637_18_EngineOwnsSchedulerWithoutDecisionAuthority(t *testing.T) {
	isolateTestWorkspace(t)
	e := NewEngine(nil, nil, nil, nil, nil)
	if e.initiativeScheduler == nil {
		t.Fatal("engine must own B96.37.18 scheduler")
	}
	now := time.Now()
	d := conversationalinitiative.InitiativeDecision{ID: "test-defer", Decision: conversationalinitiative.Defer, Opportunity: conversationalinitiative.InitiativeOpportunity{Exists: true, Source: conversationalinitiative.SourceConversationalContext, Reason: conversationalinitiative.ReasonPendingTask, UserAvailability: conversationalinitiative.AvailabilityBusy, Context: "x"}, Timing: conversationalinitiative.TimingDefer, Priority: conversationalinitiative.PriorityHigh, Reason: conversationalinitiative.ReasonPendingTask, CreatedAt: now, PolicyVersion: "B96.37.2"}
	s, created, err := e.initiativeScheduler.ScheduleDecision(d, now)
	if err != nil || !created {
		t.Fatalf("schedule: %v %v", err, created)
	}
	if s.State != conversationalinitiative.SchedulePending {
		t.Fatalf("unexpected state %s", s.State)
	}
	if len(e.initiativeScheduler.Trigger(conversationalinitiative.ScheduleTriggerAvailability, "availability", "available", now.Add(time.Minute))) != 1 {
		t.Fatal("expected trigger")
	}
	x, err := e.initiativeScheduler.BeginReevaluation(s.ID, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if x.State != conversationalinitiative.ScheduleReevaluating {
		t.Fatal("scheduler did not enter reevaluation")
	}
}

func TestB963718_EngineReevaluatesScheduledInitiativeThroughPolicy(t *testing.T) {
	isolateTestWorkspace(t)
	e := NewEngine(nil, nil, nil, nil, nil)
	now := time.Now()
	d := conversationalinitiative.InitiativeDecision{ID: "sched-reeval", Decision: conversationalinitiative.Defer, Opportunity: conversationalinitiative.InitiativeOpportunity{Exists: true, Source: conversationalinitiative.SourceConversationalContext, Reason: conversationalinitiative.ReasonPendingTask, UserAvailability: conversationalinitiative.AvailabilityAvailable, Context: "x"}, Timing: conversationalinitiative.TimingDefer, Priority: conversationalinitiative.PriorityHigh, Reason: conversationalinitiative.ReasonPendingTask, CreatedAt: now, PolicyVersion: "B96.37.2"}
	s, _, err := e.initiativeScheduler.ScheduleDecision(d, now)
	if err != nil {
		t.Fatal(err)
	}
	// Make the temporal trigger due without bypassing policy.
	if _, err = e.initiativeScheduler.BeginReevaluation(s.ID, now); err == nil {
		t.Fatal("BeginReevaluation must require a triggered schedule")
	}
	decisions, err := e.ReevaluateScheduledInitiatives(now.Add(5 * time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(decisions) != 1 {
		t.Fatalf("expected one reevaluation, got %d", len(decisions))
	}
	if decisions[0].Decision != conversationalinitiative.Intervene && decisions[0].Decision != conversationalinitiative.Defer && decisions[0].Decision != conversationalinitiative.Wait {
		t.Fatalf("invalid canonical decision %q", decisions[0].Decision)
	}
}
