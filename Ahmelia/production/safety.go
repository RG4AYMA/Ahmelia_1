package production

import (
	"fmt"

	"ahmelia/capability"
	"ahmelia/shared"
)

// SafetyAssessment is the production-level safety gate. It combines the
// declarative Capability permission with the existing action safety policy.
type SafetyAssessment struct {
	CapabilityID          capability.ID
	ActionID              string
	Tier                  shared.SafetyTier
	RequiresConfirmation  bool
	RequiresAuthorization bool
	Allowed               bool
	Reason                string
}

// SafetyGate evaluates safety without executing or authorizing an action.
// Authorization remains the responsibility of B94.3/AuthorizationGate.
type SafetyGate struct{}

func NewSafetyGate() *SafetyGate { return &SafetyGate{} }

func (g *SafetyGate) Assess(c capability.Capability, actionID string) (SafetyAssessment, error) {
	if g == nil {
		return SafetyAssessment{}, fmt.Errorf("production: safety gate unavailable")
	}
	if c.ID == "" {
		return SafetyAssessment{}, fmt.Errorf("production: safety capability inválida")
	}
	if !c.Permission.IsValid() {
		return SafetyAssessment{}, fmt.Errorf("production: safety permission inválida: %q", c.Permission)
	}
	if actionID == "" {
		return SafetyAssessment{}, fmt.Errorf("production: safety action ID vacío")
	}

	tier := shared.SafetyTierOf(actionID)
	confirmation := shared.RequiresConfirmation(actionID)
	authorization := shared.RequiresAuthorization(actionID)

	// Unknown/unclassified actions are fail-closed by the shared safety policy.
	// Capability ask_first is an additional authorization requirement, never a bypass.
	if c.Permission == capability.PermissionAskFirst {
		authorization = true
	}

	allowed := !confirmation && !authorization
	reason := "safe to proceed to the existing authorization/execution gates"
	if authorization {
		reason = "explicit authorization required before execution"
	}
	if confirmation {
		reason = "explicit confirmation required before execution"
	}

	return SafetyAssessment{
		CapabilityID:          c.ID,
		ActionID:              actionID,
		Tier:                  tier,
		RequiresConfirmation:  confirmation,
		RequiresAuthorization: authorization,
		Allowed:               allowed,
		Reason:                reason,
	}, nil
}
