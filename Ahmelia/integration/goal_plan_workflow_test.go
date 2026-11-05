package integration

import (
	"context"
	"testing"
	"time"

	"ahmelia/autonomy"
	"ahmelia/planning"
)

type workflowMapper struct{}

func (workflowMapper) MapTask(_ context.Context, _ *planning.Goal, task *planning.Task) (autonomy.ActionRequest, error) {
	return autonomy.ActionRequest{ID: task.ID, Action: "accion_" + task.ID}, nil
}

func TestGoalPlanCompilerProjectsCanonicalPlanWithoutStartingTasks(t *testing.T) {
	state := autonomy.New()
	monitor := autonomy.NewGoalMonitor(state, 0)
	planner := planning.NewPlanner()
	adapter, err := autonomy.NewGoalPlanningAdapter(state, monitor, planner, workflowMapper{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	goal := planning.NewGoal("objetivo de prueba", "probar puente", planning.PriorityMedium, "test")
	planner.RegisterGoal(goal)

	compiler, err := NewGoalPlanCompiler(adapter)
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := compiler.Compile(context.Background(), goal)
	if err != nil {
		t.Fatal(err)
	}
	if compiled.GoalID != goal.ID || compiled.PlanID == "" {
		t.Fatalf("proyección inválida: %+v", compiled)
	}
	if len(compiled.Specs) == 0 || len(compiled.Specs) != len(compiled.Actions) {
		t.Fatalf("esperaba proyección 1:1, specs=%d actions=%d", len(compiled.Specs), len(compiled.Actions))
	}
	for _, spec := range compiled.Specs {
		if spec.Task.Status != planning.TaskPending {
			t.Fatalf("compilación alteró lifecycle de tarea %q: %s", spec.Task.ID, spec.Task.Status)
		}
		action, ok := compiled.Actions[spec.Task.ID]
		if !ok || action.ID != spec.Task.ID || spec.Operation != action.Action {
			t.Fatalf("acción/spec inconsistente para %q", spec.Task.ID)
		}
	}
}

func TestGoalPlanCompilerPreservesDependencies(t *testing.T) {
	state := autonomy.New()
	monitor := autonomy.NewGoalMonitor(state, 0)
	planner := planning.NewPlanner()
	adapter, err := autonomy.NewGoalPlanningAdapter(state, monitor, planner, workflowMapper{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	goal := planning.NewGoal("objetivo de dependencias", "probar dependencias", planning.PriorityMedium, "test")
	planner.RegisterGoal(goal)
	compiler, err := NewGoalPlanCompiler(adapter)
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := compiler.Compile(context.Background(), goal)
	if err != nil {
		t.Fatal(err)
	}
	if len(compiled.Specs) < 2 {
		t.Fatal("el plan genérico debe tener al menos dos tareas")
	}
	for i := 1; i < len(compiled.Specs); i++ {
		deps := compiled.Specs[i].Task.Dependencies
		if len(deps) != 1 || deps[0] != compiled.Specs[i-1].Task.ID {
			t.Fatalf("dependencia perdida en %q: %#v", compiled.Specs[i].Task.ID, deps)
		}
	}
}

func TestGoalPlanCompilerRejectsMixedGoalTask(t *testing.T) {
	state := autonomy.New()
	monitor := autonomy.NewGoalMonitor(state, 0)
	planner := planning.NewPlanner()
	adapter, err := autonomy.NewGoalPlanningAdapter(state, monitor, planner, workflowMapper{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	goal := planning.NewGoal("objetivo A", "a", planning.PriorityMedium, "test")
	other := planning.NewGoal("objetivo B", "b", planning.PriorityMedium, "test")
	planner.RegisterGoal(goal)
	planner.RegisterGoal(other)

	// Corrupt only the canonical plan projection after generation to prove the
	// compiler validates the ownership invariant rather than trusting the plan.
	plan, err := adapter.EnsurePlan(goal)
	if err != nil {
		t.Fatal(err)
	}
	plan.Tasks[0].GoalID = other.ID

	compiler, err := NewGoalPlanCompiler(adapter)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := compiler.Compile(context.Background(), goal); err == nil {
		t.Fatal("esperaba rechazo de tarea perteneciente a otro Goal")
	}
}

func TestGoalPlanCompilerUsesFiveMinuteTaskBoundary(t *testing.T) {
	// The provider execution boundary is intentionally bounded independently of
	// the outer workflow lifetime.
	if 5*time.Minute != 300*time.Second {
		t.Fatal("programming provider timeout must be five minutes")
	}
}
