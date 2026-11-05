package autonomy

import (
	"testing"
	"time"
)

func TestSupervisorReplansThenEscalatesOnStagnation(t *testing.T) {
	s := NewSupervisor(Budget{MaxSteps: 50, MaxReplans: 1, Stagnation: 3}, time.Now())
	var last Decision
	for i := 0; i < 3; i++ {
		last = s.Observe(Step{Delta: 0.5, Verified: false}) // no verificado: no cuenta
	}
	if last.Verdict != VerdictReplan {
		t.Fatalf("want replan, got %+v", last)
	}
	for i := 0; i < 3; i++ {
		last = s.Observe(Step{Verified: true, Delta: 0})
	}
	if last.Verdict != VerdictEscalate {
		t.Fatalf("want escalate, got %+v", last)
	}
	if s.Progress() != 0 {
		t.Fatal("unverified progress counted")
	}
}

func TestSupervisorSameFailureClassForcesStrategyChange(t *testing.T) {
	s := NewSupervisor(Budget{MaxReplans: 2, SameFailure: 2, Stagnation: 99}, time.Now())
	s.Observe(Step{FailureClass: "build"})
	if d := s.Observe(Step{FailureClass: "build"}); d.Verdict != VerdictReplan {
		t.Fatalf("%+v", d)
	}
}

func TestSupervisorStopsEndogenouslyOnVerifiedCompletionAndBudget(t *testing.T) {
	s := NewSupervisor(Budget{MaxSteps: 3}, time.Now())
	s.Observe(Step{Verified: true, Delta: 0.5})
	if d := s.Observe(Step{Verified: true, Delta: 0.6}); d.Verdict != VerdictStop || d.Reason != "goal_progress_verified_complete" {
		t.Fatalf("%+v", d)
	}
	b := NewSupervisor(Budget{MaxSteps: 2, Stagnation: 99}, time.Now())
	b.Observe(Step{Verified: true, Delta: 0.1})
	if d := b.Observe(Step{Verified: true, Delta: 0.1}); d.Verdict != VerdictStop {
		t.Fatalf("%+v", d)
	}
}
