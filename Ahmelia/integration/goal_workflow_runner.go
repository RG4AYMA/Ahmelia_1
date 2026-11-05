package integration

import (
	"context"
	"fmt"
	"strings"

	"ahmelia/autonomy"
	"ahmelia/execution"
	"ahmelia/planning"
)

// GoalWorkflowPort exposes the canonical Goal authority without creating a
// second Goal model. Implementations are expected to return the same
// *planning.Goal identity already owned by Core/Planner.
type GoalWorkflowPort interface {
	Get(string) (*planning.Goal, bool)
	Persist() error
}

// GoalWorkflowRunner binds one canonical Goal to the existing Work/TaskGraph
// workflow. It does not plan or execute tasks itself; WorkflowRunner remains
// the execution authority and planning.Goal remains the goal authority.
type GoalWorkflowRunner struct {
	workflow *WorkflowRunner
	goals    GoalWorkflowPort
}

func NewGoalWorkflowRunner(workflow *WorkflowRunner, goals GoalWorkflowPort) (*GoalWorkflowRunner, error) {
	if workflow == nil {
		return nil, fmt.Errorf("integration: goal workflow runner nil workflow")
	}
	if goals == nil {
		return nil, fmt.Errorf("integration: goal workflow runner nil goal authority")
	}
	return &GoalWorkflowRunner{workflow: workflow, goals: goals}, nil
}

// ReconcileConfirmedAttempt propagates one confirmed environmental observation
// to the canonical WorkAttempt before Goal/TaskGraph closure is allowed.
func (r *GoalWorkflowRunner) ReconcileConfirmedAttempt(workID, jobID string, observation autonomy.EnvironmentObservation) error {
	if r == nil || r.workflow == nil {
		return fmt.Errorf("integration: goal workflow reconciliation unavailable")
	}
	return r.workflow.ReconcileConfirmedAttempt(workID, jobID, observation)
}

// Run executes a workflow that belongs to exactly one Goal and reconciles
// task progress back into that same Goal. A workflow may only complete the
// Goal when every task completed; a failed workflow blocks the Goal with the
// observed failure reason. No success is inferred from the workflow runner:
// task callbacks must already honor their behavioral-verification contract.
func (r *GoalWorkflowRunner) Run(ctx context.Context, goalID, workID string, specs []execution.TaskExecutionSpec, run func(context.Context, execution.TaskExecutionSpec) error) (execution.TaskGraphResult, error) {
	return r.run(ctx, goalID, workID, specs, run, nil)
}

// RunWithReconciler supplies an observer-backed reconciliation contract for one Goal run.
func (r *GoalWorkflowRunner) RunWithReconciler(ctx context.Context, goalID, workID string, specs []execution.TaskExecutionSpec, run func(context.Context, execution.TaskExecutionSpec) error, reconciler execution.ExecutionReconciler) (execution.TaskGraphResult, error) {
	if reconciler == nil {
		return execution.TaskGraphResult{}, fmt.Errorf("integration: goal workflow reconciler nil")
	}
	return r.run(ctx, goalID, workID, specs, run, reconciler)
}

func (r *GoalWorkflowRunner) run(ctx context.Context, goalID, workID string, specs []execution.TaskExecutionSpec, run func(context.Context, execution.TaskExecutionSpec) error, reconciler execution.ExecutionReconciler) (execution.TaskGraphResult, error) {
	if r == nil || r.workflow == nil || r.goals == nil {
		return execution.TaskGraphResult{}, fmt.Errorf("integration: goal workflow runner unavailable")
	}
	goalID = normalizeID(goalID)
	if goalID == "" {
		return execution.TaskGraphResult{}, fmt.Errorf("integration: goal workflow goalID vacío")
	}
	goal, ok := r.goals.Get(goalID)
	if !ok || goal == nil {
		return execution.TaskGraphResult{}, fmt.Errorf("integration: goal %q no encontrado", goalID)
	}
	if err := validateGoalWorkflow(goalID, specs); err != nil {
		return execution.TaskGraphResult{}, err
	}

	syncGoalProgress(goal, specs)
	if err := r.goals.Persist(); err != nil {
		return execution.TaskGraphResult{}, fmt.Errorf("integration: persist goal start: %w", err)
	}

	var result execution.TaskGraphResult
	var err error
	if reconciler != nil {
		result, err = r.workflow.RunWithReconciler(ctx, workID, specs, run, reconciler)
	} else {
		result, err = r.workflow.Run(ctx, workID, specs, run)
	}
	syncGoalResult(goal, specs, result, err)
	if persistErr := r.goals.Persist(); persistErr != nil && err == nil {
		err = fmt.Errorf("integration: persist goal result: %w", persistErr)
	}
	return result, err
}

func validateGoalWorkflow(goalID string, specs []execution.TaskExecutionSpec) error {
	if len(specs) == 0 {
		return fmt.Errorf("integration: goal workflow no tiene tareas")
	}
	seen := make(map[string]struct{}, len(specs))
	for _, spec := range specs {
		if spec.Task == nil {
			return fmt.Errorf("integration: goal workflow contiene tarea nil")
		}
		if spec.Task.ID == "" {
			return fmt.Errorf("integration: goal workflow contiene tarea sin ID")
		}
		if _, exists := seen[spec.Task.ID]; exists {
			return fmt.Errorf("integration: goal workflow tarea duplicada %q", spec.Task.ID)
		}
		seen[spec.Task.ID] = struct{}{}
		if spec.Task.GoalID != goalID {
			return fmt.Errorf("integration: tarea %q pertenece al Goal %q, no al Goal %q", spec.Task.ID, spec.Task.GoalID, goalID)
		}
	}
	return nil
}

func syncGoalProgress(goal *planning.Goal, specs []execution.TaskExecutionSpec) {
	completed := 0
	for _, spec := range specs {
		if spec.Task != nil && spec.Task.Status == planning.TaskCompleted {
			completed++
		}
	}
	goal.UpdateProgress(completed, len(specs))
}

func syncGoalResult(goal *planning.Goal, specs []execution.TaskExecutionSpec, result execution.TaskGraphResult, runErr error) {
	syncGoalProgress(goal, specs)
	if runErr != nil {
		goal.SetBlocked(runErr.Error())
		return
	}
	if result.Cancelled {
		goal.SetBlocked("workflow cancelled")
		return
	}
	if len(result.NeedsReconciliation) > 0 {
		// The goal is not complete and the effect is not known to have failed.
		// Pause its lifecycle while the existing execution reconciler observes
		// the same task/job identity. Do not route this state through the normal
		// terminal failure path or generate a replacement plan yet.
		goal.Status = planning.GoalPaused
		goal.AddNote("reconciliation required before next decision for tasks: " + strings.Join(result.NeedsReconciliation, ", "))
		return
	}
	if len(result.Failed) > 0 {
		goal.SetBlocked(summarizeFailures(result.Failed))
		return
	}
	if len(result.Completed) == len(specs) && len(specs) > 0 {
		goal.Complete()
	}
}

func normalizeID(id string) string {
	if id == "" {
		return ""
	}
	// Keep this deliberately dependency-free; IDs are opaque identifiers.
	start := 0
	end := len(id)
	for start < end && id[start] <= ' ' {
		start++
	}
	for end > start && id[end-1] <= ' ' {
		end--
	}
	return id[start:end]
}
