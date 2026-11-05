package integration

import (
	"context"
	"testing"
	"time"

	"ahmelia/execution"
	"ahmelia/planning"
)

type goalWorkflowStoreStub struct {
	goal     *planning.Goal
	persists int
}

func (s *goalWorkflowStoreStub) Get(id string) (*planning.Goal, bool) {
	if s.goal == nil || s.goal.ID != id {
		return nil, false
	}
	return s.goal, true
}
func (s *goalWorkflowStoreStub) Persist() error { s.persists++; return nil }

func newGoalWorkflowRunnerForTest(t *testing.T, store GoalWorkflowPort) *GoalWorkflowRunner {
	t.Helper()
	d, err := execution.NewDispatcher(execution.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	w := &execution.LocalWorker{WorkerID: "local", Caps: execution.WorkerCapabilities{Domains: map[string]bool{"test": true}, Capabilities: map[string]bool{"test": true}, CPU: 4, MemoryMB: 4096}}
	if err := d.RegisterWorker(w); err != nil {
		t.Fatal(err)
	}
	graph, err := execution.NewTaskGraphExecutor(d, 0)
	if err != nil {
		t.Fatal(err)
	}
	checkpoints, err := execution.NewFileTaskGraphCheckpointStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workStore := &workflowStoreStub{}
	workflow, err := NewWorkflowRunner(graph, workStore, checkpoints)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := NewGoalWorkflowRunner(workflow, store)
	if err != nil {
		t.Fatal(err)
	}
	return runner
}

func TestGoalWorkflowRunnerCompletesSameCanonicalGoal(t *testing.T) {
	goal := planning.NewGoal("g", "desc", planning.PriorityMedium, "user")
	store := &goalWorkflowStoreStub{goal: goal}
	runner := newGoalWorkflowRunnerForTest(t, store)
	a := planning.NewTask(goal.ID, "a", "", 1, time.Second)
	b := planning.NewTask(goal.ID, "b", "", 2, time.Second)
	b.Dependencies = []string{a.ID}
	specs := []execution.TaskExecutionSpec{{Task: a, CapabilityID: "test", Operation: "a"}, {Task: b, CapabilityID: "test", Operation: "b"}}
	result, err := runner.Run(context.Background(), goal.ID, "work-goal-1", specs, func(_ context.Context, s execution.TaskExecutionSpec) error { s.Task.Complete(); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Completed) != 2 || goal.Status != planning.GoalCompleted || goal.Progress != 1 {
		t.Fatalf("unexpected goal/result: %+v status=%s progress=%v", result, goal.Status, goal.Progress)
	}
	if store.persists < 2 {
		t.Fatalf("expected start and terminal persistence, got %d", store.persists)
	}
}

func TestGoalWorkflowRunnerBlocksGoalOnWorkflowFailure(t *testing.T) {
	goal := planning.NewGoal("g", "desc", planning.PriorityMedium, "user")
	store := &goalWorkflowStoreStub{goal: goal}
	runner := newGoalWorkflowRunnerForTest(t, store)
	task := planning.NewTask(goal.ID, "fail", "", 1, time.Second)
	specs := []execution.TaskExecutionSpec{{Task: task, CapabilityID: "test", Operation: "fail"}}
	result, err := runner.Run(context.Background(), goal.ID, "work-goal-2", specs, func(context.Context, execution.TaskExecutionSpec) error { return context.Canceled })
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Failed) == 0 || goal.Status != planning.GoalBlocked {
		t.Fatalf("expected blocked goal: result=%+v status=%s blockers=%v", result, goal.Status, goal.Blockers)
	}
}

func TestGoalWorkflowRunnerRejectsMixedGoalTasks(t *testing.T) {
	goal := planning.NewGoal("g", "desc", planning.PriorityMedium, "user")
	other := planning.NewGoal("other", "desc", planning.PriorityMedium, "user")
	runner := newGoalWorkflowRunnerForTest(t, &goalWorkflowStoreStub{goal: goal})
	a := planning.NewTask(goal.ID, "a", "", 1, time.Second)
	b := planning.NewTask(other.ID, "b", "", 2, time.Second)
	_, err := runner.Run(context.Background(), goal.ID, "work-goal-3", []execution.TaskExecutionSpec{{Task: a}, {Task: b}}, func(context.Context, execution.TaskExecutionSpec) error { return nil })
	if err == nil {
		t.Fatal("expected mixed-goal workflow rejection")
	}
}
