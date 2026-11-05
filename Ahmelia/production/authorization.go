package production

import (
	"ahmelia/autonomy"
	"ahmelia/capability"
	"fmt"
)

type AuthorizationGate struct{ registry *capability.Registry }

func NewAuthorizationGate(r *capability.Registry) (*AuthorizationGate, error) {
	if r == nil {
		return nil, fmt.Errorf("production: authorization registry nil")
	}
	return &AuthorizationGate{r}, nil
}

type AuthorizationResult struct {
	CapabilityID capability.ID
	Decision     autonomy.CapabilityAuthorization
}

func (g *AuthorizationGate) Authorize(id capability.ID, a autonomy.Approval) (AuthorizationResult, error) {
	if g == nil || g.registry == nil {
		return AuthorizationResult{}, fmt.Errorf("production: authorization gate unavailable")
	}
	if id == "" {
		return AuthorizationResult{}, fmt.Errorf("production: authorization capability ID vacío")
	}
	d, e := autonomy.AuthorizeCapabilityID(g.registry, id, a)
	if e != nil {
		return AuthorizationResult{}, e
	}
	return AuthorizationResult{id, d}, nil
}
func (g *AuthorizationGate) AuthorizeBinding(b CapabilityBinding, a autonomy.Approval) (AuthorizationResult, error) {
	if b.Task == nil || b.Task.ID == "" {
		return AuthorizationResult{}, fmt.Errorf("production: authorization task inválida")
	}
	if b.Capability.ID == "" {
		return AuthorizationResult{}, fmt.Errorf("production: authorization capability inválida")
	}
	if b.Action.CapabilityID != b.Capability.ID {
		return AuthorizationResult{}, fmt.Errorf("production: action capability mismatch")
	}
	return g.Authorize(b.Capability.ID, a)
}
func (r AuthorizationResult) CanExecute() bool { return r.Decision.Allowed }
