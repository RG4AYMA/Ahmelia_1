package autonomy

import (
	"context"
	"fmt"

	"ahmelia/capability"
	"ahmelia/tools"
)

// CapabilityE2EResult is the auditable result of the B94.8 pipeline.
type CapabilityE2EResult struct {
	Discovery     capability.DiscoveryResult
	Authorization CapabilityAuthorization
	Action        tools.Action
	Verified      tools.VerifiedResult
	Recovery      *tools.RecoveryDecision
}

// CapabilityActionBuilder turns a discovered capability and goal into the
// concrete Action consumed by B94.4. It is deliberately outside discovery and
// execution so neither layer invents parameters.
type CapabilityActionBuilder interface {
	BuildAction(context.Context, string, capability.DiscoveryResult) (tools.Action, error)
}

// CapabilityE2E is the retained B94.8 compatibility integration. It connects
// the historical B94 registry/discovery/authorization/runner/verification
// stack for package-level compatibility, but it is not part of the live Core
// execution graph after B96.42.1. Live runtime execution converges through
// Engine.authorityExecute -> ProductionRuntime -> Capability Provider.
type CapabilityE2E struct {
	registry       *capability.Registry
	discovery      *capability.Discovery
	verifiedRunner *tools.VerifiedRunner
	builder        CapabilityActionBuilder
	environment    tools.Environment
	recovery       *tools.RecoveryCoordinator
	maxRecovery    int
}

func NewCapabilityE2E(
	registry *capability.Registry,
	verifiedRunner *tools.VerifiedRunner,
	builder CapabilityActionBuilder,
	environment tools.Environment,
	recovery *tools.RecoveryCoordinator,
	maxRecovery int,
) *CapabilityE2E {
	if maxRecovery < 0 {
		maxRecovery = 0
	}
	return &CapabilityE2E{
		registry:       registry,
		discovery:      capability.NewDiscovery(registry),
		verifiedRunner: verifiedRunner,
		builder:        builder,
		environment:    environment,
		recovery:       recovery,
		maxRecovery:    maxRecovery,
	}
}

// Execute performs the complete B94.8 path. Recovery is bounded. Retry keeps
// the same Action; Replan asks the ActionBuilder for a new Action. This layer
// never bypasses authorization or verification.
func (e *CapabilityE2E) Execute(ctx context.Context, goal string, approval Approval) (CapabilityE2EResult, error) {
	if err := ctx.Err(); err != nil {
		return CapabilityE2EResult{}, err
	}
	if e == nil || e.registry == nil || e.discovery == nil || e.verifiedRunner == nil || e.builder == nil || e.environment == nil {
		return CapabilityE2EResult{}, fmt.Errorf("capability e2e: dependencias incompletas")
	}

	discovered, ok, err := e.discovery.Best(goal)
	if err != nil {
		return CapabilityE2EResult{}, err
	}
	if !ok {
		return CapabilityE2EResult{}, fmt.Errorf("capability e2e: no se descubrió ninguna capability para %q", goal)
	}

	auth, err := AuthorizeCapability(discovered.Capability, approval)
	if err != nil {
		return CapabilityE2EResult{}, err
	}
	if !auth.Allowed {
		return CapabilityE2EResult{Discovery: discovered, Authorization: auth}, fmt.Errorf("capability e2e: ejecución no autorizada: %s", auth.Reason)
	}

	action, err := e.builder.BuildAction(ctx, goal, discovered)
	if err != nil {
		return CapabilityE2EResult{Discovery: discovered, Authorization: auth}, fmt.Errorf("capability e2e: construir action: %w", err)
	}

	var last tools.VerifiedResult
	for recoveryCount := 0; ; recoveryCount++ {
		if err := ctx.Err(); err != nil {
			return CapabilityE2EResult{Discovery: discovered, Authorization: auth, Action: action, Verified: last}, err
		}

		verified := e.verifiedRunner.Execute(ctx, action, e.environment)
		last = verified
		result := CapabilityE2EResult{
			Discovery:     discovered,
			Authorization: auth,
			Action:        action,
			Verified:      verified,
		}
		if verified.Success {
			return result, nil
		}
		if e.recovery == nil || recoveryCount >= e.maxRecovery {
			return result, fmt.Errorf("capability e2e: ejecución no verificada: %s", verified.Reason)
		}

		decision, err := e.recovery.Recover(ctx, verified)
		if err != nil {
			return result, err
		}
		result.Recovery = &decision

		switch decision.Action {
		case tools.RecoveryRetry:
			continue
		case tools.RecoveryReplan:
			action, err = e.builder.BuildAction(ctx, goal, discovered)
			if err != nil {
				return result, fmt.Errorf("capability e2e: replan construir action: %w", err)
			}
			continue
		case tools.RecoveryBlock, tools.RecoveryAskUser:
			return result, fmt.Errorf("capability e2e: recuperación detenida: %s", decision.Action)
		default:
			return result, fmt.Errorf("capability e2e: decisión de recuperación inválida: %s", decision.Action)
		}
	}
}
