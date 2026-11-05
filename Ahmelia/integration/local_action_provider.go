package integration

import (
	"context"
	"fmt"

	"ahmelia/capability"
	"ahmelia/production"
)

// CapabilityActionExecutor is the provider-side contract used to preserve the
// legacy local action implementation while moving it behind the same Provider
// boundary used by production domains. The execution authority only sees the
// Provider interface; it never invokes the executor directly.
type CapabilityActionExecutor interface {
	ExecuteCapabilityAction(context.Context, production.ProviderRequest) production.ProviderResult
}

// LocalActionProvider adapts the existing local/desktop action surface to the
// canonical capability-provider boundary. It deliberately contains no intent
// classification, authorization policy, recovery policy, or response routing.
type LocalActionProvider struct {
	executor CapabilityActionExecutor
}

func NewLocalActionProvider(executor CapabilityActionExecutor) (*LocalActionProvider, error) {
	if executor == nil {
		return nil, fmt.Errorf("integration: local action executor nil")
	}
	return &LocalActionProvider{executor: executor}, nil
}

func (p *LocalActionProvider) ID() string { return "core.local_action" }

func (p *LocalActionProvider) Execute(ctx context.Context, req production.ProviderRequest) production.ProviderResult {
	if p == nil || p.executor == nil {
		return production.ProviderResult{Success: false, FailureReason: "local_action_provider_unavailable"}
	}
	return p.executor.ExecuteCapabilityAction(ctx, req)
}

// Local action capabilities are projections of the canonical capability
// catalog. This package does not maintain a second capability definition.
func localActionCapabilities() []capability.Capability {
	return capability.BuiltinCapabilities()
}

// RegisterLocalActionProvider is the sole composition point for the legacy
// local action surface. It does not introduce a second registry or authority.
func RegisterLocalActionProvider(caps *capability.Registry, providers *production.ProviderRegistry, executor CapabilityActionExecutor) error {
	if caps == nil || providers == nil {
		return fmt.Errorf("integration: local action registries not configured")
	}
	provider, err := NewLocalActionProvider(executor)
	if err != nil {
		return err
	}
	for _, c := range localActionCapabilities() {
		if !caps.Has(c.ID) {
			if err := caps.Register(c); err != nil {
				return err
			}
		}
	}
	ids := make([]capability.ID, 0, len(localActionCapabilities()))
	for _, c := range localActionCapabilities() {
		ids = append(ids, c.ID)
	}
	return providers.RegisterCapabilities(provider, ids...)
}
