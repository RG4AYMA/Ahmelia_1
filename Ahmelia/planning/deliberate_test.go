package planning

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeD struct {
	bp  []GoalTaskBlueprint
	err error
}

func (f fakeD) DecomposeGoal(context.Context, Goal) ([]GoalTaskBlueprint, error) { return f.bp, f.err }

func h(n int) time.Duration { return time.Duration(n) * time.Hour }

func TestDeliberatePicksParallelPlanAndToleratesFailingStrategy(t *testing.T) {
	g := NewGoal("cualquier objetivo", "x", PriorityMedium, "ahmelia")
	serial := []GoalTaskBlueprint{{Title: "a", Description: "d", TimeEstimate: h(2)}, {Title: "b", Description: "d", Dependencies: []int{0}, TimeEstimate: h(2)}, {Title: "c", Description: "d", Dependencies: []int{1}, TimeEstimate: h(2)}, {Title: "d", Description: "d", Dependencies: []int{2}, TimeEstimate: h(2)}}
	par := []GoalTaskBlueprint{{Title: "a", Description: "d", TimeEstimate: h(2)}, {Title: "b", Description: "d", TimeEstimate: h(2)}, {Title: "c", Description: "d", TimeEstimate: h(2)}, {Title: "d", Description: "d", Dependencies: []int{0, 1, 2}, TimeEstimate: h(1)}}
	pl := NewPlanner()
	plan, ranked, err := pl.Deliberate(context.Background(), g, []Proposer{
		{"serial", fakeD{bp: serial}}, {"parallel", fakeD{bp: par}}, {"broken", fakeD{err: errors.New("llm down")}},
	}, StructuralCritic{})
	if err != nil || len(ranked) != 2 || ranked[0].Strategy != "parallel" || len(plan.Tasks) != 4 {
		t.Fatalf("err=%v ranked=%+v", err, ranked)
	}
	found := false
	for _, is := range plan.Issues {
		found = found || len(is) > 0
	}
	if !found {
		t.Fatal("discarded strategy must be reported in plan.Issues")
	}
}

func TestDeliberateFailsLoudlyWithoutAnyValidStrategy(t *testing.T) {
	g := NewGoal("x", "x", PriorityMedium, "user")
	_, _, err := NewPlanner().Deliberate(context.Background(), g, []Proposer{{"b", fakeD{err: errors.New("down")}}}, StructuralCritic{})
	if err == nil {
		t.Fatal("must not fall back silently")
	}
}

func TestDeliberateRejectsForwardDependencies(t *testing.T) {
	g := NewGoal("x", "x", PriorityMedium, "user")
	bad := []GoalTaskBlueprint{{Title: "a", Description: "d", Dependencies: []int{1}, TimeEstimate: h(1)}, {Title: "b", Description: "d", TimeEstimate: h(1)}}
	if _, _, err := NewPlanner().Deliberate(context.Background(), g, []Proposer{{"bad", fakeD{bp: bad}}}, StructuralCritic{}); err == nil {
		t.Fatal("cyclic/forward blueprint accepted")
	}
}
