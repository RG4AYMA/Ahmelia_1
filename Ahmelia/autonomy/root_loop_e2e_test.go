package autonomy

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ahmelia/planning"
)

type rootLoopFilesystemExecutor struct{}

func (rootLoopFilesystemExecutor) ExecuteAction(_ context.Context, req ActionRequest) ActionResponse {
	path := req.Target
	if req.Parameters["ruta"] != "" {
		path = req.Parameters["ruta"]
	}
	err := os.MkdirAll(path, 0o755)
	return ActionResponse{RequestID: req.ID, Executed: err == nil, Error: err}
}

type rootLoopFilesystemInspector struct{}

func (rootLoopFilesystemInspector) InspectAction(_ context.Context, req ActionRequest) (EnvironmentObservation, error) {
	path := req.Target
	if req.Parameters["ruta"] != "" {
		path = req.Parameters["ruta"]
	}
	info, err := os.Stat(path)
	if err != nil {
		return EnvironmentObservation{Details: err.Error()}, nil
	}
	if !info.IsDir() {
		return EnvironmentObservation{Details: "target is not a directory"}, nil
	}
	return EnvironmentObservation{Confirmed: true, Details: "directory exists", Status: ObservationConfirmed, ActionID: req.ID, Operation: req.Action, Target: req.Target, Source: "test.root_loop_filesystem", ObservedAt: time.Now().UTC(), Facts: []ObservationFact{{Subject: path, Predicate: "filesystem.object_type", Expected: "directory", Observed: "directory"}}}, nil
}

func TestRootAutonomousLoop_GoalToTaskToActionToObservedCompletion(t *testing.T) {
	state := New()
	monitor := NewGoalMonitor(state, 0)
	planner := planning.NewPlanner()
	adapter, err := NewGoalPlanningAdapter(state, monitor, planner, NewCapabilityResolver(nil), nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(t.TempDir(), "autonomous-root")
	goal := planning.NewGoal("crea carpeta llamada "+target, "", planning.PriorityHigh, "ahmelia")
	planner.RegisterGoal(goal)
	state.SetCurrentGoal(goal)

	cycle := NewCycle(state, nil, nil, adapter, rootLoopFilesystemExecutor{}, nil, nil, nil, adapter)
	cycle.SetEnvironmentVerifier(NewEnvironmentVerifier(rootLoopFilesystemInspector{}))

	if err := cycle.RunStep(context.Background()); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(target); err != nil {
		t.Fatalf("filesystem effect missing: %v", err)
	}
	if goal.Status != planning.GoalCompleted {
		t.Fatalf("goal status=%s, want completed", goal.Status)
	}
	if goal.TasksCompleted != 1 || goal.TasksTotal != 1 {
		t.Fatalf("progress=%d/%d, want 1/1", goal.TasksCompleted, goal.TasksTotal)
	}

	if _, ok, err := adapter.GetNextAction(context.Background()); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatal("completed goal produced another action")
	}
}
