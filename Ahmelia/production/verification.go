package production

import (
	"context"
	"fmt"

	"ahmelia/tools"
)

// VerificationService is the Production boundary for the existing
// Execute -> Observe -> Verify contract. It does not create a second
// Verification model and it does not decide what happens after failure.
// The concrete verifier remains the owner of expected-state semantics.
type VerificationService struct {
	verifier tools.Verifier
}

// NewVerificationService creates a Production verification boundary around
// an existing tools.Verifier.
func NewVerificationService(verifier tools.Verifier) (*VerificationService, error) {
	if verifier == nil {
		return nil, fmt.Errorf("production: verification verifier nil")
	}
	return &VerificationService{verifier: verifier}, nil
}

// Verify verifies one already executed and observed Action. Production only
// enforces identity and lifecycle invariants; the verifier decides whether
// the observed state satisfies the Action's expected outcome.
func (s *VerificationService) Verify(
	ctx context.Context,
	binding CapabilityBinding,
	execution tools.Result,
	observation tools.Observation,
) (tools.Verification, error) {
	if err := ctx.Err(); err != nil {
		return tools.Verification{}, err
	}
	if s == nil || s.verifier == nil {
		return tools.Verification{}, fmt.Errorf("production: verification service unavailable")
	}
	if binding.Task == nil || binding.Task.ID == "" {
		return tools.Verification{}, fmt.Errorf("production: verification task inválida")
	}
	if binding.Capability.ID == "" {
		return tools.Verification{}, fmt.Errorf("production: verification capability inválida")
	}
	if err := binding.Action.Validate(); err != nil {
		return tools.Verification{}, err
	}
	if binding.Action.CapabilityID != binding.Capability.ID {
		return tools.Verification{}, fmt.Errorf("production: verification action capability mismatch")
	}
	if execution.ActionID != "" && execution.ActionID != binding.Action.ID {
		return tools.Verification{}, fmt.Errorf("production: verification execution ActionID mismatch: %q != %q", execution.ActionID, binding.Action.ID)
	}

	verification := s.verifier.Verify(ctx, binding.Action, execution, observation)
	// The observation supplied by the Production pipeline is authoritative
	// evidence for this verification. A verifier cannot silently replace it.
	verification.Observation = observation
	return verification, nil
}

// VerificationOutcome keeps Production identity next to the existing tools
// verification result without introducing another verification semantics.
type VerificationOutcome struct {
	TaskID       string
	CapabilityID string
	ActionID     string
	Result       tools.Verification
}

// VerifyOutcome is the identity-preserving convenience form used by higher
// production orchestration layers.
func (s *VerificationService) VerifyOutcome(
	ctx context.Context,
	binding CapabilityBinding,
	execution tools.Result,
	observation tools.Observation,
) (VerificationOutcome, error) {
	verification, err := s.Verify(ctx, binding, execution, observation)
	if err != nil {
		return VerificationOutcome{}, err
	}
	return VerificationOutcome{
		TaskID: binding.Task.ID, CapabilityID: string(binding.Capability.ID),
		ActionID: binding.Action.ID, Result: verification,
	}, nil
}
