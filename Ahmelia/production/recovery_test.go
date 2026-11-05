package production

import (
	"context"
	"testing"

	"ahmelia/autonomy"
	"ahmelia/planning"
	"ahmelia/tools"
)

func TestRecoveryServiceReusesReplannerAndRetries(t *testing.T) {
	goal := planning.NewGoal("Crear proyecto", "", planning.PriorityMedium, "user")
	plan := &planning.Plan{ID: "plan-1", GoalID: goal.ID}
	task := planning.NewTask(goal.ID, "crear archivo", "", 1, 0)
	plan.Tasks = []*planning.Task{task}
	task.Start()
	task.Fail("verification failed")

	replanner := autonomy.NewReplanner(autonomy.ReplanLimits{MaxTaskRetries: 2, MaxPlanRegenerations: 1})
	service, err := NewRecoveryService(replanner)
	if err != nil {
		t.Fatal(err)
	}

	result := tools.VerifiedResult{ActionID: "action-1", Success: false, Reason: "file missing", Attempts: []tools.Attempt{{Number: 1}}}
	decision, err := service.Recover(context.Background(), goal, plan, task, result)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != tools.RecoveryRetry {
		t.Fatalf("expected retry, got %q", decision.Action)
	}
}

func TestRecoveryServiceDoesNotRecoverSuccess(t *testing.T) {
	goal := planning.NewGoal("goal", "", planning.PriorityMedium, "user")
	plan := &planning.Plan{ID: "plan-1", GoalID: goal.ID}
	task := planning.NewTask(goal.ID, "task", "", 1, 0)
	plan.Tasks = []*planning.Task{task}
	service, _ := NewRecoveryService(autonomy.NewReplanner(autonomy.ReplanLimits{}))
	_, err := service.Recover(context.Background(), goal, plan, task, tools.VerifiedResult{Success: true})
	if err == nil {
		t.Fatal("expected success recovery rejection")
	}
}

func TestApplyRecoveryDecisionRetryUsesPlanningTask(t *testing.T) {
	task := planning.NewTask("goal-1", "task", "", 1, 0)
	task.Start()
	task.Fail("failed")
	if err := ApplyRecoveryDecision(task, tools.RecoveryDecision{Action: tools.RecoveryRetry}); err != nil {
		t.Fatal(err)
	}
	if task.Status != planning.TaskPending {
		t.Fatalf("expected pending, got %q", task.Status)
	}
}

func TestApplyRecoveryDecisionDoesNotPretendToReplan(t *testing.T) {
	task := planning.NewTask("goal-1", "task", "", 1, 0)
	task.Fail("failed")
	if err := ApplyRecoveryDecision(task, tools.RecoveryDecision{Action: tools.RecoveryReplan}); err != nil {
		t.Fatal(err)
	}
	if task.Status != planning.TaskBlocked {
		t.Fatalf("replan should not mutate task, got %q", task.Status)
	}
}

func TestRecoveryServiceRejectsMismatchedPlanAndTask(t *testing.T) {
	goal := planning.NewGoal("goal", "", planning.PriorityMedium, "user")
	other := planning.NewGoal("other", "", planning.PriorityMedium, "user")
	plan := &planning.Plan{ID: "plan-2", GoalID: other.ID}
	task := planning.NewTask(goal.ID, "task", "", 1, 0)
	service, _ := NewRecoveryService(autonomy.NewReplanner(autonomy.ReplanLimits{}))
	_, err := service.Recover(context.Background(), goal, plan, task, tools.VerifiedResult{ActionID: "a"})
	if err == nil {
		t.Fatal("expected mismatched plan error")
	}
}
