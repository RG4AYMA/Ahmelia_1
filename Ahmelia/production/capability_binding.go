package production

import (
	"fmt"

	"ahmelia/capability"
	"ahmelia/planning"
	"ahmelia/tools"
)

// CapabilityBinding is the immutable relationship selected for one production
// action. The Capability value is the Registry snapshot; the Action retains
// the same Capability ID. Tool is the concrete implementation selected for
// execution, but is not executed by this block.
type CapabilityBinding struct {
	Task       *planning.Task
	Capability capability.Capability
	Tool       tools.Tool
	Action     tools.Action
}

// CapabilityBinder resolves the declarative capability and its concrete tool
// without crossing the execution boundary.
type CapabilityBinder struct {
	capabilities *capability.Registry
	tools        *tools.Registry
}

func NewCapabilityBinder(capabilities *capability.Registry, toolRegistry *tools.Registry) (*CapabilityBinder, error) {
	if capabilities == nil {
		return nil, fmt.Errorf("production: capability binder registry nil")
	}
	if toolRegistry == nil {
		return nil, fmt.Errorf("production: capability binder tool registry nil")
	}
	return &CapabilityBinder{capabilities: capabilities, tools: toolRegistry}, nil
}

// Bind resolves Capability -> Tool and constructs the existing tools.Action.
// It deliberately refuses ambiguous or missing tool mappings.
func (b *CapabilityBinder) Bind(task *planning.Task, capabilityID capability.ID, actionID string, inputs map[string]string) (CapabilityBinding, error) {
	if b == nil || b.capabilities == nil || b.tools == nil {
		return CapabilityBinding{}, fmt.Errorf("production: capability binder unavailable")
	}
	if task == nil || task.ID == "" {
		return CapabilityBinding{}, fmt.Errorf("production: capability binding task inválida")
	}
	if capabilityID == "" {
		return CapabilityBinding{}, fmt.Errorf("production: capability binding ID vacío")
	}
	if actionID == "" {
		return CapabilityBinding{}, fmt.Errorf("production: capability binding action ID vacío")
	}

	c, ok := b.capabilities.Get(capabilityID)
	if !ok {
		return CapabilityBinding{}, fmt.Errorf("production: capability %q no está registrada", capabilityID)
	}
	if err := c.Validate(); err != nil {
		return CapabilityBinding{}, err
	}

	matches := b.tools.FindCapability(c.ID)
	if len(matches) == 0 {
		return CapabilityBinding{}, fmt.Errorf("production: ninguna tool implementa capability %q", c.ID)
	}
	if len(matches) > 1 {
		return CapabilityBinding{}, fmt.Errorf("production: capability %q tiene %d tools compatibles; binding ambiguo", c.ID, len(matches))
	}

	normalized, err := validateInputs(c, inputs)
	if err != nil {
		return CapabilityBinding{}, err
	}
	action := tools.Action{
		ID: actionID, CapabilityID: c.ID, Name: c.Action, Inputs: normalized,
	}
	if err := action.Validate(); err != nil {
		return CapabilityBinding{}, err
	}
	return CapabilityBinding{Task: task, Capability: c, Tool: matches[0], Action: action}, nil
}
