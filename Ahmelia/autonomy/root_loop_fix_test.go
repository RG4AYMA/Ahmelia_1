package autonomy

import (
	"context"
	"strings"
	"testing"

	"ahmelia/planning"
)

func TestGoalPlanningAdapter_PreservesExecutableGoalIntent(t *testing.T) {
	state := New()
	monitor := NewGoalMonitor(state, 0)
	planner := planning.NewPlanner()
	adapter, err := NewGoalPlanningAdapter(state, monitor, planner, NewCapabilityResolver(nil), nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	goal := planning.NewGoal("crea carpeta llamada prueba_root_loop", "", planning.PriorityHigh, "ahmelia")
	planner.RegisterGoal(goal)

	plan, err := adapter.EnsurePlan(goal)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Tasks) != 1 {
		t.Fatalf("tasks=%d, want 1", len(plan.Tasks))
	}
	if plan.Tasks[0].Description != goal.Name {
		t.Fatalf("description=%q, want %q", plan.Tasks[0].Description, goal.Name)
	}

	req, err := adapter.mapper.MapTask(context.Background(), goal, plan.Tasks[0])
	if err != nil {
		t.Fatal(err)
	}
	if req.Action != "crear_carpeta" {
		t.Fatalf("action=%q", req.Action)
	}
	if !strings.Contains(req.Target, "prueba_root_loop") {
		t.Fatalf("target=%q", req.Target)
	}
}
