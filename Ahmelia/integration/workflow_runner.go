package integration

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"ahmelia/autonomy"
	"ahmelia/execution"
	"ahmelia/planning"
	"ahmelia/production"
)

// WorkflowWorkPort is the minimal WorkSession state boundary needed by the
// workflow runner. The canonical WorkSession remains owned by production;
// this bridge only projects execution progress and terminal outcome into it.
type WorkflowWorkPort interface {
	SetProgress(string, string, []string, []string) error
	Complete(string, bool, string) error
	Block(string, string) error
}

// WorkflowRunner connects a canonical WorkSession to the existing TaskGraph
// and Execution Fabric. It is an orchestration bridge, not a second workflow
// authority: planning.Task owns task identity, TaskGraphExecutor owns graph
// scheduling, and Execution Fabric owns physical worker dispatch.
// WorkAttemptReconciler is the optional bridge to the canonical WorkAttempt
// lifecycle. TaskGraph checkpoints are operational recovery state, not a second
// epistemic authority; when a Goal/Workflow reconciliation confirms an effect,
// the same observation must also close the corresponding canonical WorkAttempt.
type WorkAttemptReconciler interface {
	LastAttempt(string) (production.WorkAttempt, bool)
	ReconcileUnknownAttempt(string, int, production.ActionResult) error
}

type WorkflowRunner struct {
	graph    *execution.TaskGraphExecutor
	store    WorkflowWorkPort
	checks   execution.TaskGraphCheckpointStore
	attempts WorkAttemptReconciler
}

func NewWorkflowRunner(graph *execution.TaskGraphExecutor, store WorkflowWorkPort, checkpoints execution.TaskGraphCheckpointStore) (*WorkflowRunner, error) {
	if graph == nil {
		return nil, fmt.Errorf("integration: workflow graph nil")
	}
	if store == nil {
		return nil, fmt.Errorf("integration: workflow work store nil")
	}
	if checkpoints == nil {
		return nil, fmt.Errorf("integration: workflow checkpoint store nil")
	}
	runner := &WorkflowRunner{graph: graph, store: store, checks: checkpoints}
	if reconciler, ok := store.(WorkAttemptReconciler); ok {
		runner.attempts = reconciler
	}
	return runner, nil
}

// ReconcileConfirmedAttempt closes the canonical WorkAttempt only after the
// external observer has produced affirmative evidence. The TaskGraph checkpoint
// is deliberately not sufficient to close Work: both representations must
// converge on the same WorkID/attempt identity before the graph can proceed.
func (r *WorkflowRunner) ReconcileConfirmedAttempt(workID, jobID string, observation autonomy.EnvironmentObservation) error {
	if r == nil {
		return fmt.Errorf("integration: workflow runner unavailable during reconciliation")
	}
	if r.attempts == nil {
		return fmt.Errorf("integration: canonical WorkAttempt reconciliation is unavailable for work %q", workID)
	}
	if !observation.HasSufficientConfirmedEvidence() {
		return fmt.Errorf("integration: reconciliation requires explicit structured evidence whose observed values satisfy every expected postcondition; confirmed=%t status=%q facts=%d", observation.Confirmed, observation.Status, len(observation.Facts))
	}
	if observation.Source == "" || observation.ActionID == "" || observation.Operation == "" || !observation.IsFresh(time.Now().UTC(), 30*time.Second) {
		return fmt.Errorf("integration: reconciliation observation lacks complete, fresh action provenance")
	}
	attempt, ok := r.attempts.LastAttempt(workID)
	if !ok {
		return fmt.Errorf("integration: canonical WorkAttempt missing for reconciliation work %q", workID)
	}
	if attempt.TaskID != "" && observation.ActionID != attempt.TaskID {
		return fmt.Errorf("integration: reconciliation task identity mismatch: observed_action=%q stored_task=%q", observation.ActionID, attempt.TaskID)
	}
	if strings.TrimSpace(jobID) != "" && strings.TrimSpace(attempt.JobID) != "" && strings.TrimSpace(attempt.JobID) != strings.TrimSpace(jobID) {
		return fmt.Errorf("integration: reconciliation execution identity mismatch: work=%q attempt=%d stored_job=%q checkpoint_job=%q", workID, attempt.Number, attempt.JobID, jobID)
	}
	if attempt.Status != "unverified" {
		return fmt.Errorf("integration: canonical attempt %d for work %q is not awaiting reconciliation: status=%q", attempt.Number, workID, attempt.Status)
	}
	facts := make([]production.ObservedPostconditionFact, 0, len(observation.Facts))
	for _, fact := range observation.Facts {
		facts = append(facts, production.ObservedPostconditionFact{Subject: fact.Subject, Predicate: fact.Predicate, Expected: fact.Expected, Observed: fact.Observed})
	}
	result := production.ActionResult{
		Executed: true, Success: true, OutcomeStatus: production.OutcomeSucceeded,
		Verified: true, Observed: true, WorkID: workID, AttemptNumber: attempt.Number,
		ObservationDetails:  strings.TrimSpace(observation.Details),
		ObservationActionID: observation.ActionID, ObservationOperation: observation.Operation,
		ObservationTarget: observation.Target, ObservationSource: observation.Source,
		ObservationCapturedAt: observation.ObservedAt, ObservationFacts: facts,
		VerificationReason: "external observation confirmed the expected effect",
		VerificationSource: observation.Source,
		OutcomeCode:        "reconciled_confirmed",
	}
	if err := r.attempts.ReconcileUnknownAttempt(workID, attempt.Number, result); err != nil {
		return fmt.Errorf("integration: canonical work reconciliation failed: %w", err)
	}
	return nil
}

// Run executes one already-planned workflow under an existing WorkID.
// A task callback must return nil only after the task's required behavioral
// verification has succeeded. The runner never invents verification itself.
func (r *WorkflowRunner) Run(ctx context.Context, workID string, specs []execution.TaskExecutionSpec, run func(context.Context, execution.TaskExecutionSpec) error) (execution.TaskGraphResult, error) {
	return r.run(ctx, workID, specs, run, nil)
}

// RunWithReconciler injects observation for this workflow invocation without
// changing the shared TaskGraphExecutor configuration.
func (r *WorkflowRunner) RunWithReconciler(ctx context.Context, workID string, specs []execution.TaskExecutionSpec, run func(context.Context, execution.TaskExecutionSpec) error, reconciler execution.ExecutionReconciler) (execution.TaskGraphResult, error) {
	if reconciler == nil {
		return execution.TaskGraphResult{}, fmt.Errorf("integration: workflow reconciler nil")
	}
	return r.run(ctx, workID, specs, run, reconciler)
}

func (r *WorkflowRunner) run(ctx context.Context, workID string, specs []execution.TaskExecutionSpec, run func(context.Context, execution.TaskExecutionSpec) error, reconciler execution.ExecutionReconciler) (execution.TaskGraphResult, error) {
	if r == nil || r.graph == nil || r.store == nil || r.checks == nil {
		return execution.TaskGraphResult{}, fmt.Errorf("integration: workflow runner unavailable")
	}
	workID = strings.TrimSpace(workID)
	if workID == "" {
		return execution.TaskGraphResult{}, fmt.Errorf("integration: workflow workID vacío")
	}

	completedBefore, pendingBefore := progressSets(specs, nil)
	if err := r.store.SetProgress(workID, "starting workflow", completedBefore, pendingBefore); err != nil {
		return execution.TaskGraphResult{}, fmt.Errorf("integration: set workflow start progress: %w", err)
	}

	graphRun := r.graph.RunResumable
	if reconciler != nil {
		graphRun = func(ctx context.Context, workID string, specs []execution.TaskExecutionSpec, run func(context.Context, execution.TaskExecutionSpec) error, store execution.TaskGraphCheckpointStore) (execution.TaskGraphResult, error) {
			return r.graph.RunResumableWithReconciler(ctx, workID, specs, run, store, reconciler)
		}
	}
	result, err := graphRun(ctx, workID, specs, func(execCtx context.Context, spec execution.TaskExecutionSpec) error {
		completed, pending := progressSets(specs, []string{spec.Task.ID})
		current := spec.Task.ID
		if err := r.store.SetProgress(workID, current, completed, pending); err != nil {
			return fmt.Errorf("workflow progress before %q: %w", spec.Task.ID, err)
		}
		taskErr := run(execCtx, spec)
		if taskErr != nil {
			return taskErr
		}
		completed, pending = progressSets(specs, []string{spec.Task.ID})
		if err := r.store.SetProgress(workID, current, completed, pending); err != nil {
			return fmt.Errorf("workflow progress after %q: %w", spec.Task.ID, err)
		}
		return nil
	}, r.checks)
	if err != nil {
		var pending *execution.ReconciliationPendingError
		if errors.As(err, &pending) {
			result.NeedsReconciliation = appendUniqueTaskID(result.NeedsReconciliation, pending.TaskID)
			completed, pendingTasks := progressSets(specs, nil)
			message := "awaiting execution reconciliation for tasks: " + strings.Join(result.NeedsReconciliation, ", ")
			if progressErr := r.store.SetProgress(workID, message, completed, pendingTasks); progressErr != nil {
				return result, fmt.Errorf("integration: persist reconciliation-required progress: %w", progressErr)
			}
			return result, nil
		}
		_ = r.store.Block(workID, err.Error())
		return result, err
	}
	if result.Cancelled {
		_ = r.store.Block(workID, "workflow cancelled")
		return result, context.Canceled
	}
	if len(result.NeedsReconciliation) > 0 {
		// This is not an ordinary terminal task failure: the physical effect
		// may exist, but verification did not establish its state. Keep the
		// WorkSession resumable and expose the pending reconciliation rather
		// than converting uncertainty into a terminal Blocked state.
		completed, pending := progressSets(specs, nil)
		message := "awaiting execution reconciliation for tasks: " + strings.Join(result.NeedsReconciliation, ", ")
		if err := r.store.SetProgress(workID, message, completed, pending); err != nil {
			return result, fmt.Errorf("integration: persist reconciliation-required progress: %w", err)
		}
		return result, nil
	}
	if len(result.Failed) > 0 {
		_ = r.store.Block(workID, summarizeFailures(result.Failed))
		return result, nil
	}
	if len(result.Completed) == len(specs) {
		if err := r.store.Complete(workID, true, "workflow completed with task-level success and verification contract"); err != nil {
			return result, fmt.Errorf("integration: complete workflow: %w", err)
		}
	}
	return result, nil
}

func appendUniqueTaskID(values []string, id string) []string {
	for _, value := range values {
		if value == id {
			return values
		}
	}
	return append(values, id)
}

func progressSets(specs []execution.TaskExecutionSpec, active []string) ([]string, []string) {
	activeSet := make(map[string]bool, len(active))
	for _, id := range active {
		activeSet[id] = true
	}
	completed := make([]string, 0, len(specs))
	pending := make([]string, 0, len(specs))
	for _, spec := range specs {
		if spec.Task == nil {
			continue
		}
		switch spec.Task.Status {
		case planning.TaskCompleted:
			completed = append(completed, spec.Task.ID)
		default:
			pending = append(pending, spec.Task.ID)
		}
		if activeSet[spec.Task.ID] && spec.Task.Status == planning.TaskCompleted {
			// Completion is authoritative even when the callback is returning
			// from the same task that just finished.
		}
	}
	return completed, pending
}

func summarizeFailures(failures map[string]error) string {
	for id, err := range failures {
		return fmt.Sprintf("workflow task %s failed: %v", id, err)
	}
	return "workflow task failed"
}
