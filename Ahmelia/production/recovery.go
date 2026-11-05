package production

import (
	"context"
	"fmt"

	"ahmelia/autonomy"
	"ahmelia/planning"
	"ahmelia/tools"
)

// RecoveryService is the Production boundary between a failed verified
// execution and the recovery policy already owned by autonomy/tools.
// It does not execute an Action, create a new Planner, or invent a second
// recovery model.
type RecoveryService struct {
	coordinator *tools.RecoveryCoordinator
	replanner   *autonomy.Replanner
}

// replannerDecider adapts the existing autonomy.Replanner to the tool-layer
// RecoveryDecider contract. The existing Replanner remains the owner of
// retry/skip/regenerate/give-up policy for planning.Task failures.
type replannerDecider struct {
	goal *planning.Goal
	plan *planning.Plan
	task *planning.Task
	r    *autonomy.Replanner
}

func (d replannerDecider) Decide(_ context.Context, failure tools.Failure) tools.RecoveryAction {
	if d.r == nil {
		return tools.RecoveryBlock
	}
	decision := d.r.Decide(d.goal, d.plan, d.task)
	switch decision.Action {
	case autonomy.ReplanRetryTask:
		return tools.RecoveryRetry
	case autonomy.ReplanRegeneratePlan:
		return tools.RecoveryReplan
	case autonomy.ReplanSkipTask:
		return tools.RecoveryBlock
	case autonomy.ReplanGiveUp:
		return tools.RecoveryBlock
	default:
		return tools.RecoveryBlock
	}
}

// NewRecoveryService creates the Production recovery boundary. The supplied
// Replanner is reused; nil is rejected rather than silently creating a second
// recovery policy.
func NewRecoveryService(replanner *autonomy.Replanner) (*RecoveryService, error) {
	if replanner == nil {
		return nil, fmt.Errorf("production: recovery replanner nil")
	}
	return &RecoveryService{replanner: replanner}, nil
}

// Recover diagnoses a failed VerifiedResult and asks the existing Replanner
// for the next policy step. It is intentionally side-effect free: applying
// Retry/Replan/Block remains the responsibility of the caller that owns the
// planning state.
func (s *RecoveryService) Recover(
	ctx context.Context,
	goal *planning.Goal,
	plan *planning.Plan,
	task *planning.Task,
	result tools.VerifiedResult,
) (tools.RecoveryDecision, error) {
	if err := ctx.Err(); err != nil {
		return tools.RecoveryDecision{}, err
	}
	if s == nil || s.replanner == nil {
		return tools.RecoveryDecision{}, fmt.Errorf("production: recovery service unavailable")
	}
	if goal == nil || goal.ID == "" {
		return tools.RecoveryDecision{}, fmt.Errorf("production: recovery goal inválido")
	}
	if plan == nil || plan.GoalID != goal.ID {
		return tools.RecoveryDecision{}, fmt.Errorf("production: recovery plan inválido")
	}
	if task == nil || task.ID == "" || task.GoalID != goal.ID {
		return tools.RecoveryDecision{}, fmt.Errorf("production: recovery task inválida")
	}
	if result.Success {
		return tools.RecoveryDecision{}, fmt.Errorf("production: no se recupera una ejecución verificada como exitosa")
	}

	decider := replannerDecider{goal: goal, plan: plan, task: task, r: s.replanner}
	coordinator := tools.NewRecoveryCoordinator(tools.DefaultFailureDiagnoser{}, decider)
	return coordinator.Recover(ctx, result)
}

// ApplyDecision applies only the mechanical task-side consequences that are
// already defined by planning.Task. Replan and Block are deliberately not
// implemented here because their owners live above this boundary:
// Replanner's regenerate-plan decision must be applied by GoalPlanningAdapter,
// while permanent goal blocking remains an autonomy concern.
func ApplyRecoveryDecision(task *planning.Task, decision tools.RecoveryDecision) error {
	if task == nil || task.ID == "" {
		return fmt.Errorf("production: recovery apply task inválida")
	}
	switch decision.Action {
	case tools.RecoveryRetry:
		task.Retry()
		return nil
	case tools.RecoveryReplan, tools.RecoveryBlock:
		return nil
	case tools.RecoveryAskUser:
		return nil
	default:
		return fmt.Errorf("production: recovery decision inválida %q", decision.Action)
	}
}

// TechnicalRecoveryResult is the B95 recovery decision for a domain failure
// that does not yet have a planning.Task attached. It reuses the canonical
// tools RecoveryCoordinator and does not create a second recovery model.
type TechnicalRecoveryResult struct {
	Decision tools.RecoveryDecision
}

type TechnicalRecoveryService struct {
	decider tools.RecoveryDecider
}

func NewTechnicalRecoveryService() *TechnicalRecoveryService {
	return NewTechnicalRecoveryServiceWithDecider(tools.RetryThenReplan{MaxRetries: 1})
}

// NewTechnicalRecoveryServiceWithDecider injects a RecoveryDecider while still
// reusing the canonical tools.RecoveryCoordinator. It exists for integration
// seams/tests; it does not create a second recovery model.
func NewTechnicalRecoveryServiceWithDecider(decider tools.RecoveryDecider) *TechnicalRecoveryService {
	if decider == nil {
		decider = tools.RetryThenReplan{MaxRetries: 1}
	}
	return &TechnicalRecoveryService{decider: decider}
}

func (s *TechnicalRecoveryService) Recover(ctx context.Context, actionID, reason string, attempts int) (TechnicalRecoveryResult, error) {
	if s == nil {
		return TechnicalRecoveryResult{}, fmt.Errorf("production: technical recovery unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if attempts < 1 {
		attempts = 1
	}
	result := tools.VerifiedResult{
		ActionID: actionID,
		Success:  false,
		Reason:   reason,
		Attempts: []tools.Attempt{{Number: attempts, Verification: tools.Verification{Success: false, Reason: reason}}},
	}
	decider := s.decider
	if decider == nil {
		decider = tools.RetryThenReplan{MaxRetries: 1}
	}
	coordinator := tools.NewRecoveryCoordinator(tools.DefaultFailureDiagnoser{}, decider)
	decision, err := coordinator.Recover(ctx, result)
	if err != nil {
		return TechnicalRecoveryResult{}, err
	}
	return TechnicalRecoveryResult{Decision: decision}, nil
}
