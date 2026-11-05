package integration

import (
	"context"
	"fmt"
	"strings"
	"time"

	"ahmelia/autonomy"
	"ahmelia/execution"
	"ahmelia/planning"
)

// ActionVerification is the minimum verification boundary required to close a
// goal task. A successful executor response is not enough: the verifier must
// explicitly accept the observed execution result.
type ActionVerification interface {
	Verify(context.Context, autonomy.ActionRequest, autonomy.ActionResponse) autonomy.VerificationResult
}

// ActionReconciler observes the current external state for a previously dispatched
// action. It must not execute or repeat the action.
type ActionReconciler interface {
	InspectForReconciliation(context.Context, autonomy.ActionRequest) (autonomy.EnvironmentObservation, error)
}

// verificationFailureError marks a completed callback whose physical effect
// was not proven. The graph must not blindly repeat that operation: recovery
// needs observation/reconciliation first, even when the verifier only reports
// insufficient evidence.
type verificationFailureError struct {
	reason string
}

func (e verificationFailureError) Error() string              { return e.reason }
func (verificationFailureError) Retryable() bool              { return false }
func (verificationFailureError) RequiresReconciliation() bool { return true }

// GoalExecutionRunner closes the canonical Goal -> Plan -> Workflow ->
// Execution -> Verification path. It is an adapter only: planning owns the
// Goal/Plan/Task lifecycle, WorkflowRunner owns graph orchestration, and the
// supplied ExecutorAdapter remains the physical execution authority.
type GoalExecutionRunner struct {
	compiler   *GoalPlanCompiler
	workflow   *GoalWorkflowRunner
	executor   autonomy.ExecutorAdapter
	verifier   ActionVerification
	reconciler ActionReconciler
}

func NewGoalExecutionRunner(compiler *GoalPlanCompiler, workflow *GoalWorkflowRunner, executor autonomy.ExecutorAdapter, verifier ActionVerification) (*GoalExecutionRunner, error) {
	if compiler == nil {
		return nil, fmt.Errorf("integration: goal execution compiler nil")
	}
	if workflow == nil {
		return nil, fmt.Errorf("integration: goal execution workflow nil")
	}
	if executor == nil {
		return nil, fmt.Errorf("integration: goal execution executor nil")
	}
	if verifier == nil {
		return nil, fmt.Errorf("integration: goal execution verifier nil")
	}
	return &GoalExecutionRunner{compiler: compiler, workflow: workflow, executor: executor, verifier: verifier}, nil
}

// SetActionReconciler connects a read-only domain observer to the durable task
// reconciliation boundary. The observer is scoped per run and never dispatches.
func (r *GoalExecutionRunner) SetActionReconciler(reconciler ActionReconciler) {
	if r != nil {
		r.reconciler = reconciler
	}
}

// Run compiles the canonical plan, executes every task through the existing
// workflow, and only returns nil for a task callback after verification has
// succeeded. TaskGraph then performs the canonical task completion transition.
func (r *GoalExecutionRunner) Run(ctx context.Context, goal *planning.Goal, workID string) (execution.TaskGraphResult, error) {
	if r == nil || r.compiler == nil || r.workflow == nil || r.executor == nil || r.verifier == nil {
		return execution.TaskGraphResult{}, fmt.Errorf("integration: goal execution runner unavailable")
	}
	if goal == nil {
		return execution.TaskGraphResult{}, fmt.Errorf("integration: goal execution nil goal")
	}

	compiled, err := r.compiler.Compile(ctx, goal)
	if err != nil {
		return execution.TaskGraphResult{}, err
	}
	return r.runCompiled(ctx, goal, workID, compiled)
}

// RunCompiled executes an already compiled canonical workflow. The compilation
// step remains separate so domain mappers can project the same canonical Plan.
func (r *GoalExecutionRunner) RunCompiled(ctx context.Context, goal *planning.Goal, workID string, compiled CompiledGoalWorkflow) (execution.TaskGraphResult, error) {
	return r.runCompiled(ctx, goal, workID, compiled)
}

func (r *GoalExecutionRunner) runCompiled(ctx context.Context, goal *planning.Goal, workID string, compiled CompiledGoalWorkflow) (execution.TaskGraphResult, error) {
	if goal == nil {
		return execution.TaskGraphResult{}, fmt.Errorf("integration: goal execution nil goal")
	}

	runAction := func(execCtx context.Context, spec execution.TaskExecutionSpec) error {
		action, ok := compiled.Actions[spec.Task.ID]
		if !ok {
			return fmt.Errorf("integration: no canonical action for task %q", spec.Task.ID)
		}
		response := r.executor.ExecuteAction(execCtx, action)
		verification := r.verifier.Verify(execCtx, action, response)
		if !verification.Success {
			reason := verification.ErrorReason
			if reason == "" {
				reason = "execution result was not verified"
			}
			return verificationFailureError{reason: fmt.Sprintf("task %q verification failed; retry suppressed until effect is reconciled: %s", spec.Task.ID, reason)}
		}
		return nil
	}
	if r.reconciler == nil {
		return r.workflow.Run(ctx, goal.ID, workID, compiled.Specs, runAction)
	}
	reconcile := func(reconcileCtx context.Context, spec execution.TaskExecutionSpec, jobID string) (execution.ReconcileDecision, error) {
		action, ok := compiled.Actions[spec.Task.ID]
		if !ok {
			return "", fmt.Errorf("integration: no canonical action for reconciliation task %q", spec.Task.ID)
		}
		observation, err := r.reconciler.InspectForReconciliation(reconcileCtx, action)
		if err != nil {
			return "", fmt.Errorf("integration: observe uncertain action %q: %w", spec.Task.ID, err)
		}
		if observation.ActionID == "" || observation.ActionID != action.ID {
			return "", fmt.Errorf("integration: reconciliation observation identity mismatch: observed_action=%q expected_action=%q", observation.ActionID, action.ID)
		}
		if observation.Source == "" || observation.Operation != action.Action || strings.TrimSpace(observation.Target) != strings.TrimSpace(action.Target) {
			return "", fmt.Errorf("integration: reconciliation observation lacks matching provenance for action %q", action.ID)
		}
		if !observation.IsFresh(time.Now().UTC(), 30*time.Second) {
			return "", fmt.Errorf("integration: reconciliation observation is stale or has an invalid capture time for action %q", action.ID)
		}
		if observation.Status == autonomy.ObservationConfirmed && observation.Confirmed {
			if err := r.workflow.ReconcileConfirmedAttempt(workID, jobID, observation); err != nil {
				return "", fmt.Errorf("integration: confirmed observation could not converge canonical WorkAttempt: %w", err)
			}
			return execution.ReconcileCompleted, nil
		}
		// Contradiction alone does not prove that repeating this operation is safe.
		// Keep the same durable identity pending until an operation-specific retry
		// contract or stronger evidence can justify a next action.
		return "", fmt.Errorf("reconciliation unresolved for task %q: status=%q details=%s", spec.Task.ID, observation.Status, observation.Details)
	}
	return r.workflow.RunWithReconciler(ctx, goal.ID, workID, compiled.Specs, runAction, reconcile)
}
