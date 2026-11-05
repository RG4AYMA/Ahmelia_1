package production

import (
	"context"
	"fmt"

	"ahmelia/autonomy"
	"ahmelia/tools"
)

// ExecutionGateway is the B95 compatibility boundary for the legacy production
// loop. It is retained for package-level compatibility but is not composed by
// the current Core runtime; B96.42.1 routes live execution through
// Engine.authorityExecute -> ProductionRuntime -> ProviderRegistry instead.
//
// Legacy callers must still provide an AuthorizationResult produced by B95.8.
// Binding and authorization are not repeated here, and a merely valid Action
// can never bypass the authorization gate.
type ExecutionGateway struct {
	runner *tools.Runner
}

// NewExecutionGateway creates a gateway over the existing Tool Runner.
func NewExecutionGateway(runner *tools.Runner) (*ExecutionGateway, error) {
	if runner == nil {
		return nil, fmt.Errorf("production: execution gateway runner nil")
	}
	return &ExecutionGateway{runner: runner}, nil
}

// ExecutionOutcome keeps the authorization decision alongside the existing
// tools.Result. It is an orchestration result, not a second execution model.
type ExecutionOutcome struct {
	Authorization AuthorizationResult
	Result        tools.Result
}

// ExecuteAuthorized is the only operation in B95.9 that crosses into the
// existing executor. Authorization must explicitly allow execution and its
// CapabilityID must match the bound Action.
func (g *ExecutionGateway) ExecuteAuthorized(
	ctx context.Context,
	binding CapabilityBinding,
	authorization AuthorizationResult,
	env tools.Environment,
) ExecutionOutcome {
	out := ExecutionOutcome{Authorization: authorization, Result: tools.Result{ActionID: binding.Action.ID}}

	if g == nil || g.runner == nil {
		out.Result.Err = fmt.Errorf("production: execution gateway unavailable")
		return out
	}
	if binding.Task == nil || binding.Task.ID == "" {
		out.Result.Err = fmt.Errorf("production: execution binding task inválida")
		return out
	}
	if binding.Capability.ID == "" {
		out.Result.Err = fmt.Errorf("production: execution binding capability inválida")
		return out
	}
	if binding.Tool == nil || binding.Tool.ID() == "" || binding.Tool.Executor() == nil {
		out.Result.Err = fmt.Errorf("production: execution binding tool inválida")
		return out
	}
	if err := binding.Action.Validate(); err != nil {
		out.Result.Err = err
		return out
	}
	if binding.Action.CapabilityID != binding.Capability.ID {
		out.Result.Err = fmt.Errorf("production: action capability mismatch")
		return out
	}
	if authorization.CapabilityID != binding.Capability.ID {
		out.Result.Err = fmt.Errorf("production: authorization capability mismatch")
		return out
	}
	if !authorization.Decision.Allowed || !autonomy.CanExecute(authorization.Decision.Authority) {
		out.Result.Err = fmt.Errorf("production: ejecución no autorizada: %s", authorization.Decision.Reason)
		return out
	}
	if env == nil {
		out.Result.Err = fmt.Errorf("production: execution environment nil")
		return out
	}

	// Runner remains the owner of Tool -> Executor resolution and execution.
	out.Result = g.runner.Execute(ctx, binding.Action, env)
	return out
}
