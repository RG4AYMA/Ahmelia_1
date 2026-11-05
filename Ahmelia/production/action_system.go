package production

import (
	"fmt"
	"sort"

	"ahmelia/capability"
	"ahmelia/planning"
	"ahmelia/tools"
)

// ActionSystem is the production-facing boundary that turns an existing
// planning.Task + registered Capability into the existing tools.Action.
// It does not execute, authorize, observe, or verify the action.
type ActionSystem struct {
	registry *capability.Registry
}

// NewActionSystem adapts the existing Capability Registry for Production.
func NewActionSystem(registry *capability.Registry) (*ActionSystem, error) {
	if registry == nil {
		return nil, fmt.Errorf("production: action system capability registry nil")
	}
	return &ActionSystem{registry: registry}, nil
}

// CapabilityRegistry returns the exact registry identity supplied by the caller.
func (s *ActionSystem) CapabilityRegistry() *capability.Registry {
	if s == nil {
		return nil
	}
	return s.registry
}

// ActionBinding is the production context around an existing Action. It does
// not replace tools.Action, planning.Task, or capability.Capability.
type ActionBinding struct {
	Task       *planning.Task
	Capability capability.Capability
	Action     tools.Action
}

// Build creates a concrete tools.Action for a Task and a registered Capability.
// The Task remains owned by planning and the Capability by capability.Registry.
func (s *ActionSystem) Build(task *planning.Task, capabilityID capability.ID, actionID string, inputs map[string]string) (ActionBinding, error) {
	if s == nil || s.registry == nil {
		return ActionBinding{}, fmt.Errorf("production: action system unavailable")
	}
	if task == nil || task.ID == "" {
		return ActionBinding{}, fmt.Errorf("production: action task inválida")
	}
	if actionID == "" {
		return ActionBinding{}, fmt.Errorf("production: action ID vacío")
	}
	if capabilityID == "" {
		return ActionBinding{}, fmt.Errorf("production: capability ID vacío")
	}

	c, ok := s.registry.Get(capabilityID)
	if !ok {
		return ActionBinding{}, fmt.Errorf("production: capability %q no está registrada", capabilityID)
	}
	if err := c.Validate(); err != nil {
		return ActionBinding{}, err
	}

	normalized, err := validateInputs(c, inputs)
	if err != nil {
		return ActionBinding{}, err
	}

	action := tools.Action{ID: actionID, CapabilityID: c.ID, Name: c.Action, Inputs: normalized}
	if err := action.Validate(); err != nil {
		return ActionBinding{}, err
	}
	return ActionBinding{Task: task, Capability: c, Action: action}, nil
}

// validateInputs enforces the declarative input contract without executing it.
// Missing required parameters fail closed. Unknown parameters are rejected so
// an action cannot silently carry data that its Capability never declared.
func validateInputs(c capability.Capability, inputs map[string]string) (map[string]string, error) {
	declared := make(map[string]capability.Parameter, len(c.Inputs))
	for _, p := range c.Inputs {
		if p.Name == "" {
			return nil, fmt.Errorf("production: capability %q declara input sin nombre", c.ID)
		}
		if _, exists := declared[p.Name]; exists {
			return nil, fmt.Errorf("production: capability %q declara input duplicado %q", c.ID, p.Name)
		}
		declared[p.Name] = p
	}

	normalized := make(map[string]string, len(inputs))
	keys := make([]string, 0, len(inputs))
	for k := range inputs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if _, ok := declared[k]; !ok {
			return nil, fmt.Errorf("production: input %q no está declarado por capability %q", k, c.ID)
		}
		normalized[k] = inputs[k]
	}
	for name, p := range declared {
		if p.Required {
			if _, ok := normalized[name]; !ok || normalized[name] == "" {
				return nil, fmt.Errorf("production: falta input requerido %q para capability %q", name, c.ID)
			}
		}
	}
	return normalized, nil
}
