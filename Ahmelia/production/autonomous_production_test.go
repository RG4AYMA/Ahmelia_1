package production

import (
	"context"
	"errors"
	"testing"

	"ahmelia/planning"
)

type testGoalSelector struct{ goal *planning.Goal }

func (s testGoalSelector) SelectGoal() *planning.Goal { return s.goal }

type testPlanProvider struct {
	plan  *planning.Plan
	err   error
	calls int
}

func (p *testPlanProvider) EnsurePlan(goal *planning.Goal) (*planning.Plan, error) {
	p.calls++
	if p.err != nil {
		return nil, p.err
	}
	return p.plan, nil
}

type testProductionFactory struct {
	multi *MultiStepProduction
	err   error
	calls int
}

func (f *testProductionFactory) Build(context.Context, *planning.Goal, *planning.Plan) (*MultiStepProduction, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.multi, nil
}

func TestAutonomousProductionRequiresCollaborators(t *testing.T) {
	goal := planning.NewGoal("g", "d", planning.PriorityMedium, "user")
	if _, err := NewAutonomousProduction(nil, &testPlanProvider{}, &testProductionFactory{}); err == nil {
		t.Fatal("expected selector error")
	}
	if _, err := NewAutonomousProduction(testGoalSelector{goal}, nil, &testProductionFactory{}); err == nil {
		t.Fatal("expected plan provider error")
	}
	if _, err := NewAutonomousProduction(testGoalSelector{goal}, &testPlanProvider{}, nil); err == nil {
		t.Fatal("expected factory error")
	}
}

func TestAutonomousProductionNoGoalIsNoOp(t *testing.T) {
	p, err := NewAutonomousProduction(testGoalSelector{}, &testPlanProvider{}, &testProductionFactory{})
	if err != nil {
		t.Fatal(err)
	}
	_, ran, err := p.RunNext(context.Background())
	if err != nil || ran {
		t.Fatalf("expected no-op, ran=%v err=%v", ran, err)
	}
}

func TestAutonomousProductionPropagatesPlanningFailure(t *testing.T) {
	goal := planning.NewGoal("g", "d", planning.PriorityMedium, "user")
	want := errors.New("planning failed")
	plans := &testPlanProvider{err: want}
	p, err := NewAutonomousProduction(testGoalSelector{goal}, plans, &testProductionFactory{})
	if err != nil {
		t.Fatal(err)
	}
	_, ran, err := p.RunNext(context.Background())
	if !errors.Is(err, want) || ran {
		t.Fatalf("expected planning failure, ran=%v err=%v", ran, err)
	}
}

func TestAutonomousProductionRejectsWrongPlanGoal(t *testing.T) {
	goal := planning.NewGoal("g", "d", planning.PriorityMedium, "user")
	other := planning.NewGoal("other", "d", planning.PriorityMedium, "user")
	plan := &planning.Plan{ID: "p1", GoalID: other.ID}
	plans := &testPlanProvider{plan: plan}
	factory := &testProductionFactory{}
	p, err := NewAutonomousProduction(testGoalSelector{goal}, plans, factory)
	if err != nil {
		t.Fatal(err)
	}
	_, ran, err := p.RunNext(context.Background())
	if err == nil || ran {
		t.Fatalf("expected plan validation error, ran=%v err=%v", ran, err)
	}
	if factory.calls != 0 {
		t.Fatalf("factory should not be called, got %d", factory.calls)
	}
}

func TestAutonomousProductionContextCancellation(t *testing.T) {
	goal := planning.NewGoal("g", "d", planning.PriorityMedium, "user")
	p, err := NewAutonomousProduction(testGoalSelector{goal}, &testPlanProvider{}, &testProductionFactory{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, ran, err := p.RunNext(ctx)
	if err == nil || ran {
		t.Fatalf("expected cancellation, ran=%v err=%v", ran, err)
	}
}
