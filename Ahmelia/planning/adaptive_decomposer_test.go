package planning

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fixedGoalTaskDecomposer struct {
	blueprints []GoalTaskBlueprint
	err        error
	baseline   string
}

func (d fixedGoalTaskDecomposer) DecomposeGoal(ctx context.Context, _ Goal) ([]GoalTaskBlueprint, error) {
	d.baseline = EnvironmentBaseline(ctx)
	return d.blueprints, d.err
}

func TestGeneratePlanUsesGoalSpecificDecompositionForUnseenGoal(t *testing.T) {
	planner := NewPlanner()
	planner.SetGoalTaskDecomposer(fixedGoalTaskDecomposer{blueprints: []GoalTaskBlueprint{
		{Title: "Inspect existing project", Description: "Inventory scenes, scripts, and assets", TimeEstimate: time.Minute},
		{Title: "Create missing environment", Description: "Build only the missing scene elements", Dependencies: []int{0}, TimeEstimate: 2 * time.Minute},
		{Title: "Integrate and verify", Description: "Connect assets and scripts, then run a behavior check", Dependencies: []int{0, 1}, TimeEstimate: 3 * time.Minute},
	}})
	goal := NewGoal("Build a small survival scene", "Create a Unity scene with assets and integrated gameplay code", PriorityMedium, "test")
	plan := planner.GeneratePlan(goal)
	if !plan.Feasible {
		t.Fatalf("adaptive plan should be feasible: %v", plan.Issues)
	}
	if len(plan.Tasks) != 3 {
		t.Fatalf("expected model-derived 3 tasks, got %d", len(plan.Tasks))
	}
	if plan.Tasks[0].Title != "Inspect existing project" || plan.Tasks[1].Title != "Create missing environment" {
		t.Fatalf("planner did not materialize goal-specific tasks: %#v", plan.Tasks)
	}
	if len(plan.Tasks[2].Dependencies) != 2 || plan.Tasks[2].Dependencies[0] != plan.Tasks[0].ID || plan.Tasks[2].Dependencies[1] != plan.Tasks[1].ID {
		t.Fatalf("dependency graph was not preserved: %#v", plan.Tasks[2].Dependencies)
	}
}

func TestGeneratePlanRejectsInvalidAdaptiveGraphAndRecordsFallback(t *testing.T) {
	planner := NewPlanner()
	planner.SetGoalTaskDecomposer(fixedGoalTaskDecomposer{blueprints: []GoalTaskBlueprint{
		{Title: "Invalid dependency", Description: "Depends on a future task", Dependencies: []int{1}},
	}})
	goal := NewGoal("Unseen project", "A goal without a predefined template", PriorityMedium, "test")
	plan := planner.GeneratePlan(goal)
	if !plan.Feasible {
		t.Fatalf("deterministic fallback should preserve existing degraded behavior: %v", plan.Issues)
	}
	found := false
	for _, issue := range plan.Issues {
		if strings.Contains(issue, "adaptive decomposition unavailable") {
			found = true
		}
	}
	if !found {
		t.Fatalf("fallback must be explicit in plan issues: %#v", plan.Issues)
	}
	if len(plan.Tasks) != 4 || plan.Tasks[0].Title != "Análisis" {
		t.Fatalf("expected legacy fallback graph, got %#v", plan.Tasks)
	}
}

func TestGeneratePlanFallsBackWhenAdaptiveModelFails(t *testing.T) {
	planner := NewPlanner()
	planner.SetGoalTaskDecomposer(fixedGoalTaskDecomposer{err: errors.New("model unavailable")})
	goal := NewGoal("Novel goal", "Do something not covered by a named template", PriorityMedium, "test")
	plan := planner.GeneratePlan(goal)
	if len(plan.Tasks) == 0 || len(plan.Issues) == 0 {
		t.Fatalf("expected a usable but explicitly degraded fallback plan, got %+v", plan)
	}
}

func TestGeneratePlanContextPropagatesObservedBaseline(t *testing.T) {
	planner := NewPlanner()
	decomposer := fixedGoalTaskDecomposer{blueprints: []GoalTaskBlueprint{{Title: "Inspect", Description: "Inspect current state"}}}
	planner.SetGoalTaskDecomposer(decomposer)
	goal := NewGoal("Unseen goal", "Use observed state before acting", PriorityMedium, "test")
	ctx := WithEnvironmentBaseline(context.Background(), `{"focused_app":"Unity","open_apps":["Unity","Blender"]}`)
	plan := planner.GeneratePlanContext(ctx, goal)
	if !plan.Feasible || len(plan.Tasks) != 1 {
		t.Fatalf("expected feasible plan: %+v", plan)
	}
	if got := EnvironmentBaseline(ctx); !strings.Contains(got, "Unity") || !strings.Contains(got, "Blender") {
		t.Fatalf("observed baseline was not available to planning context: %q", got)
	}
}

func TestGeneratePlanContextHonorsCancellationBeforeDecomposition(t *testing.T) {
	planner := NewPlanner()
	planner.SetGoalTaskDecomposer(fixedGoalTaskDecomposer{err: context.Canceled})
	goal := NewGoal("Cancelled goal", "Do not plan after cancellation", PriorityMedium, "test")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	plan := planner.GeneratePlanContext(ctx, goal)
	if plan.Feasible || len(plan.Tasks) != 0 || len(plan.Issues) == 0 || !strings.Contains(plan.Issues[0], "planning cancelled") {
		t.Fatalf("cancelled planning must not silently fall back to an executable plan: %+v", plan)
	}
}

type cancelDuringDecomposition struct{ cancel context.CancelFunc }

func (d cancelDuringDecomposition) DecomposeGoal(context.Context, Goal) ([]GoalTaskBlueprint, error) {
	d.cancel()
	return nil, context.Canceled
}

func TestGeneratePlanContextDoesNotFallbackAfterMidPlanningCancellation(t *testing.T) {
	planner := NewPlanner()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	planner.SetGoalTaskDecomposer(cancelDuringDecomposition{cancel: cancel})
	goal := NewGoal("Cancelled mid-plan", "Do not create a fallback executable plan", PriorityMedium, "test")
	plan := planner.GeneratePlanContext(ctx, goal)
	if plan.Feasible || len(plan.Tasks) != 0 || len(plan.Issues) == 0 || !strings.Contains(plan.Issues[0], "planning cancelled during decomposition") {
		t.Fatalf("mid-decomposition cancellation must not create a fallback plan: %+v", plan)
	}
}
