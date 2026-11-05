package integration

import (
	"context"
	"testing"

	"ahmelia/autonomy"
	"ahmelia/planning"
)

func TestGoalPlanCompilerProgrammingMapperProjectsCanonicalPlan(t *testing.T) {
	goal := planning.NewGoal("crear programa demo", "produce", planning.PriorityMedium, "programming")
	state := autonomy.New()
	monitor := autonomy.NewGoalMonitor(state, 0)
	planner := planning.NewPlanner()
	adapter, err := autonomy.NewGoalPlanningAdapter(state, monitor, planner, workflowMapper{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	planner.RegisterGoal(goal)
	compiler, err := NewGoalPlanCompiler(adapter)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.WithValue(context.Background(), ProgrammingProjectContextKey{}, "demo")
	compiled, err := compiler.CompileWithMapper(ctx, goal, ProgrammingGoalActionMapper{})
	if err != nil {
		t.Fatal(err)
	}
	if len(compiled.Specs) == 0 {
		t.Fatal("expected at least one planned task")
	}
	for _, spec := range compiled.Specs {
		if spec.ExecutionFingerprint == "" {
			t.Fatalf("task %q lacks execution fingerprint for safe checkpoint recovery", spec.Task.ID)
		}
		if compiled.Actions[spec.Task.ID].Action != "produce" {
			t.Fatalf("task %q action=%q, want produce", spec.Task.ID, compiled.Actions[spec.Task.ID].Action)
		}
		if compiled.Actions[spec.Task.ID].Parameters["project_id"] != "demo" {
			t.Fatalf("task %q lost project_id", spec.Task.ID)
		}
	}
}
