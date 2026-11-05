package core

import (
	conversationalinitiative "ahmelia/initiative"
	"testing"
	"time"
)

func TestB963719_EngineLifecycleRecordsCanonicalDefer(t *testing.T) {
	isolateTestWorkspace(t)
	e := NewEngine(nil, nil, nil, nil, nil)
	now := time.Date(2026, 9, 7, 16, 0, 0, 0, time.UTC)
	op := conversationalinitiative.InitiativeOpportunity{Exists: true, Source: conversationalinitiative.SourceConversationalContext, Reason: conversationalinitiative.ReasonPendingTask, Context: "Blender pendiente", Target: "Blender", ExpiresAt: now.Add(time.Hour), UserAvailability: conversationalinitiative.AvailabilityBusy}
	d := conversationalinitiative.InitiativeDecision{ID: "d19", Decision: conversationalinitiative.Defer, Opportunity: op, Reason: conversationalinitiative.ReasonPendingTask, Timing: conversationalinitiative.TimingDefer, CreatedAt: now, PolicyVersion: "B96.37.2"}
	x, err := e.initiativeLifecycleManager.Create(op, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.initiativeLifecycleManager.Transition(x.ID, conversationalinitiative.LifecycleEvaluating, now, "core.Engine", "evaluate"); err != nil {
		t.Fatal(err)
	}
	if _, err = e.initiativeLifecycleManager.Transition(x.ID, conversationalinitiative.LifecycleCandidate, now, "B96.37.3", "candidate"); err != nil {
		t.Fatal(err)
	}
	if _, err = e.initiativeLifecycleManager.Transition(x.ID, conversationalinitiative.LifecycleSelected, now, "B96.37.17", "selected"); err != nil {
		t.Fatal(err)
	}
	if _, err = e.initiativeLifecycleManager.RecordDecision(x.ID, d, now); err != nil {
		t.Fatal(err)
	}
	x, err = e.initiativeLifecycleManager.Transition(x.ID, conversationalinitiative.LifecycleDecided, now, "B96.37.2", "DEFER")
	if err != nil {
		t.Fatal(err)
	}
	x, err = e.initiativeLifecycleManager.Transition(x.ID, conversationalinitiative.LifecycleDeferred, now, "B96.37.2", "deferred")
	if err != nil {
		t.Fatal(err)
	}
	if x.Decision == nil || x.Decision.Decision != conversationalinitiative.Defer {
		t.Fatal("decision not recorded")
	}
}

func TestB963719_EngineOwnsLifecycleBoundary(t *testing.T) {
	isolateTestWorkspace(t)
	e := NewEngine(nil, nil, nil, nil, nil)
	if e.initiativeLifecycleManager == nil {
		t.Fatal("missing lifecycle manager")
	}
}
