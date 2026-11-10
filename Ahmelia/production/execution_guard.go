package production

import (
	"context"
	"fmt"
	"strings"

	"ahmelia/autonomy"
	"ahmelia/capability"
)

// ExecutionOptions carries execution-time authorization evidence and the
// recovery attempt ordinal. Approval is supplied by the identity/permission
// owner; Production never invents it.
type ExecutionOptions struct {
	RetryPolicy RetryPolicy
	RetryState  *RetryState // continuation snapshot; policy cannot reset it
	// Context propagates cancellation/deadlines from the owning workflow into the provider boundary.
	Context  context.Context
	Approval autonomy.Approval
	// ApprovalResolver is supplied by the identity/permission owner. Production
	// never infers approval from the natural-language request.
	ApprovalResolver func(capability.ID) autonomy.Approval
	RecoveryAttempt  int
	// WorkID is the Core-owned WorkSession to reuse for this execution.
	// Production never creates a second work owner when it is supplied.
	WorkID string
	// TaskID preserves the canonical planning.Task identity on any durable
	// WorkAttempt created for this execution. It is optional for interactive
	// requests that do not originate from a planned task.
	TaskID string
	// AttemptNumber reuses an already-created Work attempt when execution resumes after a suspension.
	AttemptNumber int
	// SelectedProviderID is the concrete instrument selected for this capability.
	SelectedProviderID         string
	ResumePendingAuthorization bool
	// Expected execution identity is supplied only when resuming a persisted
	// authorization. It prevents semantic rediscovery from silently changing
	// the capability, operation, or provider across the authorization boundary.
	ExpectedCapabilityID capability.ID
	ExpectedOperation    string
	ExpectedProviderID   string
	ExpectedRouteKind    capability.IntentKind
	// ExperimentalBypassCognition is ONLY for the direct-LLM diagnostic path.
	ExperimentalBypassCognition bool
	// Async starts a production provider call in the Engine's background job
	// registry and returns before it completes. The default is synchronous.
	Async bool
}

// ExecutionPreflight is the provider-neutral decision produced immediately
// before a Provider can be invoked. It contains no Tool/Environment model.
type ExecutionPreflight struct {
	Capability           capability.Capability
	Authorization        AuthorizationResult
	Safety               SafetyAssessment
	Allowed              bool
	NeedsAuthorization   bool
	AuthorizationReason  string
	RegisteredProviderID string
}

// ExecutionGuard is the single provider-neutral pre-execution invariant gate.
// It reuses the existing capability Registry, SafetyGate and AuthorizationGate
// instead of introducing a parallel permission system.
type ExecutionGuard struct {
	registry      *capability.Registry
	providers     *ProviderRegistry
	safety        *SafetyGate
	authorization *AuthorizationGate
}

func NewExecutionGuard(registry *capability.Registry, providers *ProviderRegistry) (*ExecutionGuard, error) {
	if registry == nil {
		return nil, fmt.Errorf("production: execution guard registry nil")
	}
	if providers == nil {
		return nil, fmt.Errorf("production: execution guard provider registry nil")
	}
	authorization, err := NewAuthorizationGate(registry)
	if err != nil {
		return nil, err
	}
	return &ExecutionGuard{
		registry:      registry,
		providers:     providers,
		safety:        NewSafetyGate(),
		authorization: authorization,
	}, nil
}

// Check enforces identity, Registry membership, Provider availability and
// authorization before execution. Any inconsistency fails closed.
func (g *ExecutionGuard) Check(candidate capability.Capability, capabilityID capability.ID, operation string, approval autonomy.Approval, selectedProvider ...string) (ExecutionPreflight, error) {
	if g == nil || g.registry == nil || g.providers == nil || g.safety == nil || g.authorization == nil {
		return ExecutionPreflight{}, fmt.Errorf("production: execution guard unavailable")
	}
	if strings.TrimSpace(string(capabilityID)) == "" {
		return ExecutionPreflight{}, fmt.Errorf("production: execution capability ID vacío")
	}
	if candidate.ID != capabilityID {
		return ExecutionPreflight{}, fmt.Errorf("production: capability identity mismatch: %q != %q", capabilityID, candidate.ID)
	}
	if err := candidate.Validate(); err != nil {
		return ExecutionPreflight{}, err
	}

	registered, ok := g.registry.Get(candidate.ID)
	if !ok {
		return ExecutionPreflight{}, fmt.Errorf("production: capability %q no está registrada", candidate.ID)
	}
	if err := registered.Validate(); err != nil {
		return ExecutionPreflight{}, fmt.Errorf("production: capability registrada inválida: %w", err)
	}
	if registered.Domain != candidate.Domain || registered.Action != candidate.Action || registered.Permission != candidate.Permission {
		return ExecutionPreflight{}, fmt.Errorf("production: capability %q no coincide con su definición registrada", candidate.ID)
	}

	providers := g.providers.Candidates(registered)
	providerID := ""
	if len(selectedProvider) > 0 {
		providerID = strings.TrimSpace(selectedProvider[0])
	}
	var provider Provider
	var providerOK bool
	if providerID != "" {
		provider, providerOK = g.providers.ResolveFor(registered, providerID)
	} else if len(providers) == 1 {
		provider, providerOK = providers[0], true
	} else if len(providers) > 1 {
		return ExecutionPreflight{}, fmt.Errorf("production: capability %q has %d instruments; explicit provider selection required", registered.ID, len(providers))
	}
	if !providerOK || provider == nil || strings.TrimSpace(provider.ID()) == "" {
		return ExecutionPreflight{}, fmt.Errorf("production: capability %q no tiene proveedor ejecutable", candidate.ID)
	}

	if strings.TrimSpace(operation) == "" {
		operation = registered.Action
	}
	if operation != registered.Action {
		return ExecutionPreflight{}, fmt.Errorf("production: operation mismatch for capability %q: %q != %q", registered.ID, operation, registered.Action)
	}
	safety, err := g.safety.Assess(registered, operation)
	if err != nil {
		return ExecutionPreflight{}, err
	}
	authorization, err := g.authorization.Authorize(registered.ID, approval)
	if err != nil {
		return ExecutionPreflight{}, err
	}

	preflight := ExecutionPreflight{
		Capability:           registered,
		Authorization:        authorization,
		Safety:               safety,
		RegisteredProviderID: provider.ID(),
	}

	if (safety.RequiresConfirmation || safety.RequiresAuthorization) && approval != autonomy.ApprovalGranted {
		preflight.NeedsAuthorization = true
		preflight.AuthorizationReason = safety.Reason
		return preflight, nil
	}
	if !authorization.CanExecute() {
		preflight.NeedsAuthorization = authorization.Decision.NeedsApproval || registered.Permission == capability.PermissionAskFirst
		preflight.AuthorizationReason = authorization.Decision.Reason
		if preflight.AuthorizationReason == "" {
			preflight.AuthorizationReason = safety.Reason
		}
		return preflight, nil
	}
	preflight.Allowed = true
	return preflight, nil
}
