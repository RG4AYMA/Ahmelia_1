package integration

import (
	"context"
	"testing"

	"ahmelia/autonomy"
	"ahmelia/execution"
	"ahmelia/planning"
)

func TestProgrammingGoalActionMapperPreservesCanonicalProductionOperation(t *testing.T) {
	goal := planning.NewGoal("programming", "", planning.PriorityHigh, "user")
	task := planning.NewTask(goal.ID, "implement", "implementa el cambio y ejecuta las pruebas", 1, 0)
	ctx := context.WithValue(context.Background(), programmingProjectIDKey{}, "demo")
	ctx = context.WithValue(ctx, ProgrammingWorkIDContextKey{}, "work-canonical")

	req, err := (ProgrammingGoalActionMapper{}).MapTask(ctx, goal, task)
	if err != nil {
		t.Fatal(err)
	}
	if req.Action != "produce" || req.Parameters["raw"] != task.Description || req.Parameters["project_id"] != "demo" || req.Parameters["work_id"] != "work-canonical" {
		t.Fatalf("unexpected action: %+v", req)
	}
}

func TestProgrammingWorkSessionRunsThroughCanonicalGoalExecutionRunner(t *testing.T) {
	goal := planning.NewGoal("programming", "crear un proyecto Go", planning.PriorityHigh, "user")
	exec := &goalExecStub{}
	verifier := &goalVerifyStub{success: true}

	runner := newGoalExecutionRunnerWithMapper(t, goal, exec, verifier, ProgrammingGoalActionMapper{})
	session, err := NewProgrammingWorkSession(runner)
	if err != nil {
		t.Fatal(err)
	}
	result, err := session.Run(context.Background(), goal, "work-programming", "demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Completed) == 0 || goal.Status != planning.GoalCompleted {
		t.Fatalf("programming goal did not complete: result=%+v status=%v", result, goal.Status)
	}
	if exec.calls == 0 || verifier.calls == 0 {
		t.Fatal("expected execution and verification")
	}
}

func TestProgrammingWorkSessionRejectsMissingProjectIdentity(t *testing.T) {
	goal := planning.NewGoal("programming", "crear proyecto", planning.PriorityHigh, "user")
	runner := newGoalExecutionRunnerWithMapper(t, goal, &goalExecStub{}, &goalVerifyStub{success: true}, ProgrammingGoalActionMapper{})
	session, err := NewProgrammingWorkSession(runner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.Run(context.Background(), goal, "work-programming", ""); err == nil {
		t.Fatal("expected project identity validation")
	}
}

// Compile-time assertion: the mapper remains a GoalActionMapper, not an
// execution authority.
var _ autonomy.GoalActionMapper = ProgrammingGoalActionMapper{}
var _ = execution.TaskGraphResult{}
