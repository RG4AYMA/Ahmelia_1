package production

import (
	"context"
	"fmt"
	"sync"

	"ahmelia/autonomy"
	"ahmelia/planning"
	"ahmelia/tools"
)

// B95.17 — Production Loop (legacy compatibility orchestration)
//
// This is a retained B95 compatibility checkpoint. The current Core runtime
// does not compose ProductionLoop; B96.42.1 canonicalizes live execution in
// Engine.authorityExecute -> ProductionRuntime -> Capability Provider. It does not invent
// new execution, observation, verification, authorization, or memory models.
// It composes the existing B94/B95 boundaries into one explicit loop:
//
// Task -> CapabilityBinding -> Authorization -> Execution -> Observation
//
//	-> Verification -> Delivery -> Task/Production state.
//
// Recovery remains a separate policy boundary (B95.11); a failed attempt is
// returned to the caller rather than silently retried or replanned here.
type ProductionLoop struct {
	mu sync.Mutex

	production    *Production
	taskSystem    *TaskSystem
	gateway       *ExecutionGateway
	authorization *AuthorizationGate
	observer      tools.Observer
	verifier      *VerificationService
	delivery      *ResultDelivery
}

// ProductionLoopConfig contains already-owned collaborators. The loop only
// orchestrates them and never creates alternate planners, registries, or
// execution models.
type ProductionLoopConfig struct {
	Production    *Production
	TaskSystem    *TaskSystem
	Gateway       *ExecutionGateway
	Authorization *AuthorizationGate
	Observer      tools.Observer
	Verifier      *VerificationService
	Delivery      *ResultDelivery
}

func NewProductionLoop(cfg ProductionLoopConfig) (*ProductionLoop, error) {
	if cfg.Production == nil {
		return nil, fmt.Errorf("production: loop production nil")
	}
	if cfg.TaskSystem == nil {
		return nil, fmt.Errorf("production: loop task system nil")
	}
	if cfg.Gateway == nil {
		return nil, fmt.Errorf("production: loop execution gateway nil")
	}
	if cfg.Authorization == nil {
		return nil, fmt.Errorf("production: loop authorization gate nil")
	}
	if cfg.Observer == nil {
		return nil, fmt.Errorf("production: loop observer nil")
	}
	if cfg.Verifier == nil {
		return nil, fmt.Errorf("production: loop verification service nil")
	}
	if cfg.Delivery == nil {
		return nil, fmt.Errorf("production: loop delivery nil")
	}
	return &ProductionLoop{production: cfg.Production, taskSystem: cfg.TaskSystem, gateway: cfg.Gateway, authorization: cfg.Authorization, observer: cfg.Observer, verifier: cfg.Verifier, delivery: cfg.Delivery}, nil
}

type ProductionRunResult struct {
	ProductionID  string
	GoalID        string
	TaskID        string
	ActionID      string
	Authorization AuthorizationResult
	Execution     tools.Result
	Observation   tools.Observation
	Verification  tools.Verification
	Delivered     ProductionResult
	Success       bool
	Failure       string
}

// Run executes exactly one already-bound Action through the canonical
// Execute -> Observe -> Verify path. Authorization is mandatory and is never
// inferred from Action validity.
func (l *ProductionLoop) Run(ctx context.Context, binding CapabilityBinding, approval autonomy.Approval, env tools.Environment) (ProductionRunResult, error) {
	return l.run(ctx, binding, approval, env, true)
}

func (l *ProductionLoop) run(ctx context.Context, binding CapabilityBinding, approval autonomy.Approval, env tools.Environment, completeProduction bool) (ProductionRunResult, error) {
	if l == nil {
		return ProductionRunResult{}, fmt.Errorf("production: loop unavailable")
	}
	if err := ctx.Err(); err != nil {
		return ProductionRunResult{}, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.production.Goal == nil || l.production.Goal.ID == "" {
		return ProductionRunResult{}, fmt.Errorf("production: loop goal inválido")
	}
	if binding.Task == nil || binding.Task.ID == "" {
		return ProductionRunResult{}, fmt.Errorf("production: loop task inválida")
	}
	if binding.Task.GoalID != l.production.Goal.ID {
		return ProductionRunResult{}, fmt.Errorf("production: loop task pertenece a otro goal")
	}
	if err := binding.Action.Validate(); err != nil {
		return ProductionRunResult{}, err
	}
	if env == nil {
		return ProductionRunResult{}, fmt.Errorf("production: loop environment nil")
	}

	auth, err := l.authorization.AuthorizeBinding(binding, approval)
	if err != nil {
		return ProductionRunResult{}, err
	}
	if !auth.CanExecute() {
		return ProductionRunResult{ProductionID: l.production.ID, GoalID: l.production.Goal.ID, TaskID: binding.Task.ID, ActionID: binding.Action.ID, Authorization: auth, Failure: auth.Decision.Reason}, fmt.Errorf("production: ejecución no autorizada: %s", auth.Decision.Reason)
	}

	if l.production.Status == ProductionPending {
		if err := l.production.Start(); err != nil {
			return ProductionRunResult{}, err
		}
	}
	if l.production.Status != ProductionRunning {
		return ProductionRunResult{}, fmt.Errorf("production: loop no puede ejecutarse desde %q", l.production.Status)
	}
	if err := l.production.SetCurrentTask(binding.Task); err != nil {
		return ProductionRunResult{}, err
	}

	if binding.Task.Status == planning.TaskPending {
		if err := l.taskSystem.Start(binding.Task.ID); err != nil {
			return ProductionRunResult{}, err
		}
	}
	if binding.Task.Status != planning.TaskInProgress {
		return ProductionRunResult{}, fmt.Errorf("production: task no está en ejecución")
	}

	out := ProductionRunResult{ProductionID: l.production.ID, GoalID: l.production.Goal.ID, TaskID: binding.Task.ID, ActionID: binding.Action.ID, Authorization: auth}
	execOut := l.gateway.ExecuteAuthorized(ctx, binding, auth, env)
	out.Execution = execOut.Result
	if execOut.Result.Err != nil {
		_ = l.taskSystem.Fail(binding.Task.ID, execOut.Result.Err.Error())
		out.Failure = execOut.Result.Err.Error()
		return out, nil
	}

	observation, err := l.observer.Observe(ctx, binding.Action, execOut.Result)
	out.Observation = observation
	if err != nil {
		_ = l.taskSystem.Fail(binding.Task.ID, err.Error())
		out.Failure = err.Error()
		return out, nil
	}

	verification, err := l.verifier.Verify(ctx, binding, execOut.Result, observation)
	if err != nil {
		_ = l.taskSystem.Fail(binding.Task.ID, err.Error())
		out.Failure = err.Error()
		return out, nil
	}
	out.Verification = verification
	if !verification.Success {
		_ = l.taskSystem.Fail(binding.Task.ID, verification.Reason)
		out.Failure = verification.Reason
		return out, nil
	}

	verified := tools.VerifiedResult{ActionID: binding.Action.ID, Success: true, Reason: verification.Reason, Attempts: []tools.Attempt{{Number: 1, Execution: execOut.Result, Observation: observation, Verification: verification}}}
	delivered, err := l.delivery.Deliver(l.production.ID, l.production.Goal.ID, binding.Task.ID, verified)
	if err != nil {
		_ = l.taskSystem.Fail(binding.Task.ID, err.Error())
		out.Failure = err.Error()
		return out, nil
	}
	out.Delivered = delivered
	if err := l.taskSystem.Complete(binding.Task.ID); err != nil {
		return ProductionRunResult{}, err
	}
	if completeProduction {
		if err := l.production.Complete(); err != nil {
			return ProductionRunResult{}, err
		}
	}
	out.Success = true
	return out, nil
}
