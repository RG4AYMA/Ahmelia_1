package core

import (
	conversationalinitiative "ahmelia/initiative"
	"strings"
	"testing"
	"time"
)

func TestB963718ArchitectureAudit(t *testing.T) {
	isolateTestWorkspace(t)
	e := NewEngine(nil, nil, nil, nil, nil)
	if e.initiativeScheduler == nil {
		t.Fatal("B96.37.18 scheduler missing from Engine")
	}
	if e.conversationalInitiative == nil {
		t.Fatal("B96.37.2 policy missing")
	}
	if !conversationalinitiative.Defer.Valid() || !conversationalinitiative.Intervene.Valid() || !conversationalinitiative.Wait.Valid() {
		t.Fatal("decision contract invalid")
	}
	// Scheduler package contains no decision-authority API: its reevaluation
	// lifecycle accepts an already-produced decision and never creates one.
	got, _, err := e.initiativeScheduler.ScheduleDecision(conversationalinitiative.InitiativeDecision{Decision: conversationalinitiative.Intervene}, nowForB963718())
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "" {
		t.Fatal("non-DEFER decision must not create a schedule")
	}
	if strings.Contains(strings.ToLower(string(got.State)), "intervene") {
		t.Fatal("schedule state must never encode intervention")
	}
}

func nowForB963718() time.Time { return time.Date(2026, 9, 7, 16, 0, 0, 0, time.UTC) }
