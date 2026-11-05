package production

import (
	"testing"

	"ahmelia/memory_intelligence"
	"ahmelia/planning"
)

type fakeWorkspaceSource struct {
	snapshot memory_intelligence.WorkspaceSnapshot
	calls    int
}

func (f *fakeWorkspaceSource) Snapshot() (memory_intelligence.WorkspaceSnapshot, error) {
	f.calls++
	return f.snapshot, nil
}

func TestWorkspaceReusesExistingSnapshot(t *testing.T) {
	source := &fakeWorkspaceSource{snapshot: memory_intelligence.WorkspaceSnapshot{
		HasActiveApp: true,
		AppName:      "Visual Studio Code",
		Project:      "Ahmelia",
		File:         "production/workspace.go",
	}}

	workspace, err := NewWorkspace("ws-1", source)
	if err != nil {
		t.Fatal(err)
	}

	got, err := workspace.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !got.HasActiveApp || got.Project != "Ahmelia" {
		t.Fatalf("snapshot inesperado: %+v", got)
	}
	if source.calls != 1 {
		t.Fatalf("se esperaba una lectura, got %d", source.calls)
	}
}

func TestProductionBindsWorkspaceByIdentityOnly(t *testing.T) {
	source := &fakeWorkspaceSource{}
	workspace, err := NewWorkspace("ws-1", source)
	if err != nil {
		t.Fatal(err)
	}

	goal := planning.NewGoal("goal", "test", planning.PriorityMedium, "user")
	production, err := NewProduction("p-1", goal)
	if err != nil {
		t.Fatal(err)
	}
	if err := production.SetWorkspaceID("ws-1"); err != nil {
		t.Fatal(err)
	}

	binding, err := production.BindWorkspace(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if binding.Workspace != workspace {
		t.Fatal("la binding debe conservar la misma identidad del workspace")
	}
}

func TestProductionRejectsDifferentWorkspace(t *testing.T) {
	source := &fakeWorkspaceSource{}
	workspace, err := NewWorkspace("ws-2", source)
	if err != nil {
		t.Fatal(err)
	}

	production, err := NewProduction("p-1", planning.NewGoal("goal", "test", planning.PriorityMedium, "user"))
	if err != nil {
		t.Fatal(err)
	}
	if err := production.SetWorkspaceID("ws-1"); err != nil {
		t.Fatal(err)
	}

	if _, err := production.BindWorkspace(workspace); err == nil {
		t.Fatal("se esperaba rechazo por identidad distinta")
	}
}

func TestWorkspaceRegistryRejectsDuplicateIDs(t *testing.T) {
	registry := NewWorkspaceRegistry()
	source := &fakeWorkspaceSource{}

	first, _ := NewWorkspace("ws-1", source)
	second, _ := NewWorkspace("ws-1", source)

	if err := registry.Register(first); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(second); err == nil {
		t.Fatal("se esperaba rechazo del ID duplicado")
	}
	if registry.Count() != 1 {
		t.Fatalf("count inesperado: %d", registry.Count())
	}
}

func TestWorkspaceReadinessDistinguishesReadableFromContext(t *testing.T) {
	empty := &fakeWorkspaceSource{}
	workspace, err := NewWorkspace("ws-empty", empty)
	if err != nil {
		t.Fatal(err)
	}

	readiness := workspace.Readiness()
	if !readiness.Readable {
		t.Fatal("el workspace debería ser legible aunque no tenga contexto activo")
	}
	if readiness.HasContext {
		t.Fatal("no debería inventarse contexto activo")
	}
}
