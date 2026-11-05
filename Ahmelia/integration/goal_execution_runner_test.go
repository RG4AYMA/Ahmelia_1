package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	"ahmelia/autonomy"
	"ahmelia/execution"
	"ahmelia/planning"
	"ahmelia/production"
)

type goalExecStub struct {
	calls int
	err   error
}

func (s *goalExecStub) ExecuteAction(_ context.Context, req autonomy.ActionRequest) autonomy.ActionResponse {
	s.calls++
	return autonomy.ActionResponse{RequestID: req.ID, Executed: s.err == nil, Error: s.err}
}

type goalVerifyStub struct {
	success bool
	calls   int
}

func (s *goalVerifyStub) Verify(_ context.Context, _ autonomy.ActionRequest, _ autonomy.ActionResponse) autonomy.VerificationResult {
	s.calls++
	if s.success {
		return autonomy.VerificationResult{Success: true}
	}
	return autonomy.VerificationResult{Success: false, ErrorReason: "insufficient evidence"}
}

func newGoalExecutionRunnerForTest(t *testing.T, goal *planning.Goal, exec autonomy.ExecutorAdapter, verifier ActionVerification) *GoalExecutionRunner {
	t.Helper()
	d, err := execution.NewDispatcher(execution.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	w := &execution.LocalWorker{WorkerID: "local", Caps: execution.WorkerCapabilities{Domains: map[string]bool{"test": true}, Capabilities: map[string]bool{"action": true}, CPU: 4, MemoryMB: 4096}}
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
	workflow, err := NewWorkflowRunner(graph, &workflowStoreStub{}, checkpoints)
	if err != nil {
		t.Fatal(err)
	}
	goals := &goalWorkflowStoreStub{goal: goal}
	goalWorkflow, err := NewGoalWorkflowRunner(workflow, goals)
	if err != nil {
		t.Fatal(err)
	}
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
	runner, err := NewGoalExecutionRunner(compiler, goalWorkflow, exec, verifier)
	if err != nil {
		t.Fatal(err)
	}
	return runner
}

func newGoalExecutionRunnerWithMapper(t *testing.T, goal *planning.Goal, exec autonomy.ExecutorAdapter, verifier ActionVerification, mapper autonomy.GoalActionMapper) *GoalExecutionRunner {
	t.Helper()
	d, err := execution.NewDispatcher(execution.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	w := &execution.LocalWorker{WorkerID: "local", Caps: execution.WorkerCapabilities{Domains: map[string]bool{"test": true}, Capabilities: map[string]bool{"action": true, "produce": true}, CPU: 4, MemoryMB: 4096}}
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
	workflow, err := NewWorkflowRunner(graph, &workflowStoreStub{}, checkpoints)
	if err != nil {
		t.Fatal(err)
	}
	goals := &goalWorkflowStoreStub{goal: goal}
	goalWorkflow, err := NewGoalWorkflowRunner(workflow, goals)
	if err != nil {
		t.Fatal(err)
	}
	state := autonomy.New()
	monitor := autonomy.NewGoalMonitor(state, 0)
	planner := planning.NewPlanner()
	adapter, err := autonomy.NewGoalPlanningAdapter(state, monitor, planner, mapper, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	planner.RegisterGoal(goal)
	compiler, err := NewGoalPlanCompiler(adapter)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := NewGoalExecutionRunner(compiler, goalWorkflow, exec, verifier)
	if err != nil {
		t.Fatal(err)
	}
	return runner
}

// This test uses the existing planner/mapper stack to prove the new runner is
// not a special-case task callback. The exact mapping setup is provided by the
// shared test helper already used by the GoalPlan compiler tests.
func newGoalExecutionRunnerWithReconciliationStore(t *testing.T, goal *planning.Goal, exec autonomy.ExecutorAdapter, verifier ActionVerification, mapper autonomy.GoalActionMapper, store *workflowReconciliationStore) *GoalExecutionRunner {
	t.Helper()
	d, err := execution.NewDispatcher(execution.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	w := &execution.LocalWorker{WorkerID: "local", Caps: execution.WorkerCapabilities{Domains: map[string]bool{"test": true}, Capabilities: map[string]bool{"action": true, "produce": true}, CPU: 4, MemoryMB: 4096}}
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
	workflow, err := NewWorkflowRunner(graph, store, checkpoints)
	if err != nil {
		t.Fatal(err)
	}
	goals := &goalWorkflowStoreStub{goal: goal}
	goalWorkflow, err := NewGoalWorkflowRunner(workflow, goals)
	if err != nil {
		t.Fatal(err)
	}
	state := autonomy.New()
	monitor := autonomy.NewGoalMonitor(state, 0)
	planner := planning.NewPlanner()
	adapter, err := autonomy.NewGoalPlanningAdapter(state, monitor, planner, mapper, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	planner.RegisterGoal(goal)
	compiler, err := NewGoalPlanCompiler(adapter)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := NewGoalExecutionRunner(compiler, goalWorkflow, exec, verifier)
	if err != nil {
		t.Fatal(err)
	}
	return runner
}

func TestGoalExecutionRunnerRejectsUnverifiedExecution(t *testing.T) {
	goal := planning.NewGoal("crear carpeta", "crear_carpeta", planning.PriorityMedium, "test")
	exec := &goalExecStub{}
	verifier := &goalVerifyStub{success: false}
	runner := newGoalExecutionRunnerWithMapper(t, goal, exec, verifier, workflowMapper{})

	result, err := runner.Run(context.Background(), goal, "work-unverified")
	if err != nil {
		t.Fatalf("unexpected runner error: %v", err)
	}
	if exec.calls == 0 || verifier.calls == 0 {
		t.Fatal("expected execution and verification")
	}
	if goal.Status != planning.GoalPaused {
		t.Fatalf("goal status = %v, want paused pending reconciliation", goal.Status)
	}
	if len(result.Completed) != 0 {
		t.Fatalf("unverified workflow completed tasks: %v", result.Completed)
	}
	if len(result.NeedsReconciliation) != 1 {
		t.Fatalf("reconciliation-required tasks=%v, want one uncertain task", result.NeedsReconciliation)
	}
	priorCalls := exec.calls
	resumed, err := runner.Run(context.Background(), goal, "work-unverified")
	if err != nil {
		t.Fatalf("resume without reconciler should preserve pending state, got error: %v", err)
	}
	if exec.calls != priorCalls {
		t.Fatalf("uncertain operation replayed while reconciliation was unavailable: before=%d after=%d", priorCalls, exec.calls)
	}
	if goal.Status != planning.GoalPaused || len(resumed.NeedsReconciliation) == 0 {
		t.Fatalf("pending reconciliation was not preserved: goal=%q pending=%v", goal.Status, resumed.NeedsReconciliation)
	}

}

func TestGoalExecutionRunnerConfirmedObservationAlsoReconcilesCanonicalWorkAttempt(t *testing.T) {
	goal := planning.NewGoal("crear carpeta", "crear_carpeta", planning.PriorityMedium, "test")
	exec := &goalExecStub{}
	verifier := &goalVerifyFirstFailure{}
	store := &workflowReconciliationStore{attempt: production.WorkAttempt{Number: 1, Status: "unverified"}}
	runner := newGoalExecutionRunnerWithReconciliationStore(t, goal, exec, verifier, workflowMapper{}, store)
	observer := &goalActionReconcilerStub{observation: autonomy.EnvironmentObservation{Confirmed: true, Status: autonomy.ObservationConfirmed, Details: "observed expected folder"}}
	runner.SetActionReconciler(observer)

	first, err := runner.Run(context.Background(), goal, "work-cross-state")
	if err != nil {
		t.Fatal(err)
	}
	if len(first.NeedsReconciliation) != 1 || exec.calls != 1 {
		t.Fatalf("first run should preserve uncertainty: %+v calls=%d", first, exec.calls)
	}

	second, err := runner.Run(context.Background(), goal, "work-cross-state")
	if err != nil {
		t.Fatal(err)
	}
	if !store.reconciled {
		t.Fatal("confirmed observation closed TaskGraph but not canonical WorkAttempt")
	}
	if store.lastResult.WorkID != "work-cross-state" || store.lastResult.AttemptNumber != 1 || !store.lastResult.Observed || !store.lastResult.Verified || store.lastResult.OutcomeStatus != production.OutcomeSucceeded {
		t.Fatalf("canonical WorkAttempt did not receive same epistemic identity: %+v", store.lastResult)
	}
	if exec.calls != 4 || verifier.calls != 4 || goal.Status != planning.GoalCompleted || len(second.Completed) == 0 {
		t.Fatalf("cross-state reconciliation did not converge Goal/TaskGraph: goal=%q completed=%v exec=%d verify=%d", goal.Status, second.Completed, exec.calls, verifier.calls)
	}
}

func TestGoalExecutionRunnerRequiresVerificationBoundary(t *testing.T) {
	if _, err := NewGoalExecutionRunner(nil, nil, &goalExecStub{}, &goalVerifyStub{success: true}); err == nil {
		t.Fatal("expected constructor rejection")
	}
}

type goalVerifyFirstFailure struct{ calls int }

func (s *goalVerifyFirstFailure) Verify(context.Context, autonomy.ActionRequest, autonomy.ActionResponse) autonomy.VerificationResult {
	s.calls++
	if s.calls == 1 {
		return autonomy.VerificationResult{Success: false, ErrorReason: "uncertain first effect"}
	}
	return autonomy.VerificationResult{Success: true}
}

type goalActionReconcilerStub struct {
	observation autonomy.EnvironmentObservation
	err         error
	calls       int
	lastRequest autonomy.ActionRequest
}

func (s *goalActionReconcilerStub) InspectForReconciliation(_ context.Context, req autonomy.ActionRequest) (autonomy.EnvironmentObservation, error) {
	s.calls++
	s.lastRequest = req
	observation := s.observation
	// The stub models a concrete inspector: it binds the observation to the
	// request it actually inspected. Tests that set non-empty conflicting
	// provenance retain it so mismatch cases can be exercised.
	if observation.ActionID == "" {
		observation.ActionID = req.ID
	}
	if observation.Operation == "" {
		observation.Operation = req.Action
	}
	if observation.Target == "" {
		observation.Target = req.Target
	}
	if observation.Source == "" {
		observation.Source = "test.goal_action_reconciler"
	}
	if observation.ObservedAt.IsZero() {
		observation.ObservedAt = time.Now().UTC()
	}
	// Positive test observations model an inspector that reports a concrete
	// expected/observed fact, not merely a boolean and narrative.
	if observation.Confirmed && observation.Status == autonomy.ObservationConfirmed && len(observation.Facts) == 0 {
		subject := strings.TrimSpace(req.Target)
		if subject == "" {
			subject = "test-resource:" + req.Action
		}
		observation.Facts = []autonomy.ObservationFact{{Subject: subject, Predicate: "postcondition.test", Expected: "satisfied", Observed: "satisfied"}}
	}
	return observation, s.err
}

func TestGoalExecutionRunnerReconcilesUncertainEffectFromObserver(t *testing.T) {
	goal := planning.NewGoal("crear carpeta", "crear_carpeta", planning.PriorityMedium, "test")
	exec := &goalExecStub{}
	verifier := &goalVerifyFirstFailure{}
	store := &workflowReconciliationStore{attempt: production.WorkAttempt{Number: 1, Status: "unverified"}}
	runner := newGoalExecutionRunnerWithReconciliationStore(t, goal, exec, verifier, workflowMapper{}, store)
	observer := &goalActionReconcilerStub{observation: autonomy.EnvironmentObservation{
		Confirmed: true, Status: autonomy.ObservationConfirmed, Details: "observed expected folder",
	}}
	runner.SetActionReconciler(observer)

	first, err := runner.Run(context.Background(), goal, "work-observer-reconcile")
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if len(first.NeedsReconciliation) != 1 || exec.calls != 1 {
		t.Fatalf("first run should preserve one uncertain dispatch: pending=%v calls=%d", first.NeedsReconciliation, exec.calls)
	}

	second, err := runner.Run(context.Background(), goal, "work-observer-reconcile")
	if err != nil {
		t.Fatalf("reconciliation run: %v", err)
	}
	if observer.calls != 1 {
		t.Fatalf("observer calls=%d, want 1", observer.calls)
	}
	if !store.reconciled {
		t.Fatal("confirmed observation closed TaskGraph/Goal without reconciling canonical WorkAttempt")
	}
	if exec.calls != verifier.calls || exec.calls <= 1 {
		t.Fatalf("expected subsequent tasks to proceed without replaying the uncertain first task: executions=%d verifications=%d", exec.calls, verifier.calls)
	}
	if goal.Status != planning.GoalCompleted || len(second.Completed) == 0 {
		t.Fatalf("confirmed observation did not close canonical task/goal: goal=%q completed=%v", goal.Status, second.Completed)
	}
}

func TestGoalExecutionRunnerKeepsContradictedObservationPending(t *testing.T) {
	goal := planning.NewGoal("crear carpeta", "crear_carpeta", planning.PriorityMedium, "test")
	exec := &goalExecStub{}
	runner := newGoalExecutionRunnerWithMapper(t, goal, exec, &goalVerifyStub{success: false}, workflowMapper{})
	runner.SetActionReconciler(&goalActionReconcilerStub{observation: autonomy.EnvironmentObservation{
		Confirmed: false, Status: autonomy.ObservationContradicted, Details: "expected effect not observed",
	}})

	first, err := runner.Run(context.Background(), goal, "work-contradicted-reconcile")
	if err != nil || len(first.NeedsReconciliation) != 1 {
		t.Fatalf("first run should be pending, result=%+v err=%v", first, err)
	}
	second, err := runner.Run(context.Background(), goal, "work-contradicted-reconcile")
	if err != nil {
		t.Fatalf("reconciliation run should preserve pending state, got %v", err)
	}
	if exec.calls != 1 {
		t.Fatalf("contradicted observation triggered unsafe replay: calls=%d", exec.calls)
	}
	if goal.Status != planning.GoalPaused || len(second.NeedsReconciliation) != 1 {
		t.Fatalf("contradicted observation should remain pending: goal=%q pending=%v", goal.Status, second.NeedsReconciliation)
	}
}

func TestGoalExecutionRunnerDoesNotCloseGoalWhenCanonicalAttemptMissing(t *testing.T) {
	goal := planning.NewGoal("crear carpeta", "crear_carpeta", planning.PriorityMedium, "test")
	exec := &goalExecStub{}
	verifier := &goalVerifyFirstFailure{}
	runner := newGoalExecutionRunnerWithMapper(t, goal, exec, verifier, workflowMapper{})
	runner.SetActionReconciler(&goalActionReconcilerStub{observation: autonomy.EnvironmentObservation{
		Confirmed: true, Status: autonomy.ObservationConfirmed, Details: "observer reports effect present",
	}})

	first, err := runner.Run(context.Background(), goal, "work-missing-canonical-attempt")
	if err != nil {
		t.Fatalf("first uncertain run: %v", err)
	}
	if len(first.NeedsReconciliation) != 1 || goal.Status != planning.GoalPaused {
		t.Fatalf("first run must remain pending, goal=%q pending=%v", goal.Status, first.NeedsReconciliation)
	}
	priorCalls := exec.calls

	second, err := runner.Run(context.Background(), goal, "work-missing-canonical-attempt")
	if err != nil {
		t.Fatalf("missing canonical attempt should remain pending, not crash workflow: %v", err)
	}
	if exec.calls != priorCalls {
		t.Fatalf("missing canonical attempt caused operation replay: before=%d after=%d", priorCalls, exec.calls)
	}
	if goal.Status != planning.GoalPaused || len(second.NeedsReconciliation) == 0 || len(second.Completed) != 0 {
		t.Fatalf("Goal/TaskGraph closed without canonical WorkAttempt reconciliation: goal=%q pending=%v completed=%v", goal.Status, second.NeedsReconciliation, second.Completed)
	}
}

func TestGoalExecutionRunnerRejectsObservationFromDifferentActionIdentity(t *testing.T) {
	goal := planning.NewGoal("crear carpeta", "crear_carpeta", planning.PriorityMedium, "test")
	exec := &goalExecStub{}
	verifier := &goalVerifyStub{success: false}
	store := &workflowReconciliationStore{attempt: production.WorkAttempt{Number: 1, Status: "unverified"}}
	runner := newGoalExecutionRunnerWithReconciliationStore(t, goal, exec, verifier, workflowMapper{}, store)
	observer := &goalActionReconcilerStub{observation: autonomy.EnvironmentObservation{
		Confirmed: true, Status: autonomy.ObservationConfirmed, Details: "stale confirmation",
		ActionID: "previous-action", Source: "test.stale", Operation: "crear_carpeta",
		ObservedAt: time.Now().UTC(),
	}}
	runner.SetActionReconciler(observer)
	if _, err := runner.Run(context.Background(), goal, "work-stale-observation"); err != nil {
		t.Fatalf("first uncertain run: %v", err)
	}
	priorCalls := exec.calls
	result, err := runner.Run(context.Background(), goal, "work-stale-observation")
	if err != nil {
		t.Logf("mismatch correctly surfaced as runner error: %v", err)
	}
	if observer.calls == 0 {
		t.Fatal("expected observer to be queried for reconciliation")
	}
	if store.reconciled || goal.Status == planning.GoalCompleted || exec.calls != priorCalls || len(result.NeedsReconciliation) == 0 {
		t.Fatalf("stale observation must preserve uncertainty without replay or closure: reconciled=%t goal=%q exec_calls=%d prior=%d pending=%v", store.reconciled, goal.Status, exec.calls, priorCalls, result.NeedsReconciliation)
	}
}

func TestGoalExecutionRunnerRejectsStaleObservationWithoutReplayOrClosure(t *testing.T) {
	goal := planning.NewGoal("crear carpeta", "crear_carpeta", planning.PriorityMedium, "test")
	exec := &goalExecStub{}
	verifier := &goalVerifyFirstFailure{}
	store := &workflowReconciliationStore{attempt: production.WorkAttempt{Number: 1, Status: "unverified"}}
	runner := newGoalExecutionRunnerWithReconciliationStore(t, goal, exec, verifier, workflowMapper{}, store)
	observer := &goalActionReconcilerStub{observation: autonomy.EnvironmentObservation{Confirmed: true, Status: autonomy.ObservationConfirmed, Details: "stale evidence", ObservedAt: time.Now().UTC().Add(-time.Minute)}}
	runner.SetActionReconciler(observer)
	if _, err := runner.Run(context.Background(), goal, "work-stale-evidence"); err != nil {
		t.Fatalf("initial run: %v", err)
	}
	priorCalls := exec.calls
	result, err := runner.Run(context.Background(), goal, "work-stale-evidence")
	if err != nil {
		t.Logf("stale evidence rejected: %v", err)
	}
	if store.reconciled || exec.calls != priorCalls || goal.Status == planning.GoalCompleted || len(result.NeedsReconciliation) == 0 {
		t.Fatalf("stale evidence must preserve uncertainty without replay/closure: reconciled=%t calls=%d/%d goal=%q pending=%v", store.reconciled, exec.calls, priorCalls, goal.Status, result.NeedsReconciliation)
	}
}
