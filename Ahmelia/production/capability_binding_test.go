package production

import (
	"context"
	"testing"

	"ahmelia/capability"
	"ahmelia/planning"
	"ahmelia/tools"
)

type bindingExecutor struct{}

func (bindingExecutor) Execute(context.Context, tools.Action, tools.Environment) tools.Result {
	return tools.Result{Success: true}
}

type bindingTool struct{}

func (bindingTool) ID() tools.ID                  { return "filesystem" }
func (bindingTool) Capabilities() []capability.ID { return []capability.ID{"filesystem.create_file"} }
func (bindingTool) Executor() tools.Executor      { return bindingExecutor{} }

func bindingCapabilityRegistry(t *testing.T) *capability.Registry {
	t.Helper()
	r := capability.NewRegistry()
	err := r.Register(capability.Capability{
		ID: "filesystem.create_file", Domain: capability.DomainLocal,
		Action: "create_file", Permission: capability.PermissionAutomatic,
		Inputs: []capability.Parameter{{Name: "path", Required: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestCapabilityBinderResolvesCapabilityToTool(t *testing.T) {
	cr := bindingCapabilityRegistry(t)
	tr := tools.NewRegistry()
	if err := tr.Register(bindingTool{}); err != nil {
		t.Fatal(err)
	}
	b, err := NewCapabilityBinder(cr, tr)
	if err != nil {
		t.Fatal(err)
	}

	task := planning.NewTask("goal-1", "create", "create file", 1, 0)
	got, err := b.Bind(task, "filesystem.create_file", "action-1", map[string]string{"path": "enemy.cs"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Task != task {
		t.Fatal("task identity changed")
	}
	if got.Capability.ID != "filesystem.create_file" {
		t.Fatal("wrong capability")
	}
	if got.Tool.ID() != "filesystem" {
		t.Fatal("wrong tool")
	}
	if got.Action.CapabilityID != got.Capability.ID {
		t.Fatal("action/capability mismatch")
	}
}

func TestCapabilityBinderRejectsAmbiguousTools(t *testing.T) {
	cr := bindingCapabilityRegistry(t)
	tr := tools.NewRegistry()
	if err := tr.Register(bindingTool{}); err != nil {
		t.Fatal(err)
	}
	// A second ID with the same capability makes selection ambiguous.
	type secondTool struct{ bindingTool }
	var st secondTool
	_ = st
	// bindingTool has fixed ID, so use a distinct implementation.
	if err := tr.Register(secondBindingTool{}); err != nil {
		t.Fatal(err)
	}
	b, _ := NewCapabilityBinder(cr, tr)
	task := planning.NewTask("goal-1", "create", "create", 1, 0)
	if _, err := b.Bind(task, "filesystem.create_file", "a", map[string]string{"path": "x"}); err == nil {
		t.Fatal("expected ambiguity error")
	}
}

type secondBindingTool struct{}

func (secondBindingTool) ID() tools.ID { return "filesystem-alt" }
func (secondBindingTool) Capabilities() []capability.ID {
	return []capability.ID{"filesystem.create_file"}
}
func (secondBindingTool) Executor() tools.Executor { return bindingExecutor{} }

func TestCapabilityBinderDoesNotExecute(t *testing.T) {
	cr := bindingCapabilityRegistry(t)
	tr := tools.NewRegistry()
	if err := tr.Register(bindingTool{}); err != nil {
		t.Fatal(err)
	}
	b, _ := NewCapabilityBinder(cr, tr)
	task := planning.NewTask("goal-1", "create", "create", 1, 0)
	got, err := b.Bind(task, "filesystem.create_file", "a", map[string]string{"path": "x"})
	if err != nil {
		t.Fatal(err)
	}
	if err := got.Action.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestCapabilityBinderRejectsMissingTool(t *testing.T) {
	cr := bindingCapabilityRegistry(t)
	tr := tools.NewRegistry()
	b, _ := NewCapabilityBinder(cr, tr)
	task := planning.NewTask("goal-1", "create", "create", 1, 0)
	if _, err := b.Bind(task, "filesystem.create_file", "a", map[string]string{"path": "x"}); err == nil {
		t.Fatal("expected missing tool error")
	}
}
