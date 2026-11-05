package production

import (
	"ahmelia/capability"
	"ahmelia/planning"
	"testing"
)

func actionTestCapabilityRegistry(t *testing.T) *capability.Registry {
	t.Helper()
	r := capability.NewRegistry()
	err := r.Register(capability.Capability{
		ID: "filesystem.create_file", Domain: capability.DomainLocal, Action: "create_file",
		Permission: capability.PermissionAutomatic,
		Inputs:     []capability.Parameter{{Name: "path", Required: true}, {Name: "content", Required: false}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestActionSystemBuildReusesTaskAndCapability(t *testing.T) {
	r := actionTestCapabilityRegistry(t)
	s, err := NewActionSystem(r)
	if err != nil {
		t.Fatal(err)
	}
	task := planning.NewTask("goal-1", "Crear archivo", "", 1, 0)
	binding, err := s.Build(task, "filesystem.create_file", "action-1", map[string]string{"path": "enemy.cs"})
	if err != nil {
		t.Fatal(err)
	}
	if binding.Task != task {
		t.Fatal("Task identity was not preserved")
	}
	if binding.Capability.ID != "filesystem.create_file" {
		t.Fatal("wrong capability")
	}
	if binding.Action.CapabilityID != binding.Capability.ID || binding.Action.Name != "create_file" {
		t.Fatal("wrong action binding")
	}
	if binding.Action.Inputs["path"] != "enemy.cs" {
		t.Fatal("input not preserved")
	}
}

func TestActionSystemRejectsMissingRequiredInput(t *testing.T) {
	s, _ := NewActionSystem(actionTestCapabilityRegistry(t))
	task := planning.NewTask("goal-1", "Crear", "", 1, 0)
	if _, err := s.Build(task, "filesystem.create_file", "action-1", nil); err == nil {
		t.Fatal("expected missing required input error")
	}
}

func TestActionSystemRejectsUnknownInput(t *testing.T) {
	s, _ := NewActionSystem(actionTestCapabilityRegistry(t))
	task := planning.NewTask("goal-1", "Crear", "", 1, 0)
	if _, err := s.Build(task, "filesystem.create_file", "action-1", map[string]string{"path": "x", "danger": "1"}); err == nil {
		t.Fatal("expected unknown input error")
	}
}

func TestActionSystemRejectsUnknownCapability(t *testing.T) {
	s, _ := NewActionSystem(actionTestCapabilityRegistry(t))
	task := planning.NewTask("goal-1", "Crear", "", 1, 0)
	if _, err := s.Build(task, "missing", "action-1", nil); err == nil {
		t.Fatal("expected missing capability error")
	}
}
