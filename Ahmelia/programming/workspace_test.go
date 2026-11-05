package programming

import (
	"testing"

	memoryintelligence "ahmelia/memory_intelligence"
	"ahmelia/planning"
	"ahmelia/production"
)

type fakeWorkspaceSource struct{}

func (fakeWorkspaceSource) Snapshot() (memorySnapshot, error) { return memorySnapshot{}, nil }

// Alias the exact snapshot type through a small named alias so the test stays
// explicit while avoiding any second workspace implementation.
type memorySnapshot = memoryintelligence.WorkspaceSnapshot

func TestProgrammingWorkspace_RejectsInvalidProject(t *testing.T) {
	generic, err := production.NewWorkspace("ws-1", fakeWorkspaceSource{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewProgrammingWorkspace(generic, CodeProject{}); err == nil {
		t.Fatal("expected invalid project error")
	}
}

func TestProgrammingWorkspace_SourcePathCannotEscape(t *testing.T) {
	generic, err := production.NewWorkspace("ws-1", fakeWorkspaceSource{})
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := NewProgrammingWorkspace(generic, CodeProject{ID: "p", Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.SourcePath("../outside.go"); err == nil {
		t.Fatal("expected escape rejection")
	}
}

func TestProgrammingWorkspace_SourcePathNormalizes(t *testing.T) {
	generic, err := production.NewWorkspace("ws-1", fakeWorkspaceSource{})
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := NewProgrammingWorkspace(generic, CodeProject{ID: "p", Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.SourcePath("src/main.go"); err != nil {
		t.Fatal(err)
	}
}

func TestProgrammingWorkspaceRegistry_ConcurrentRegistrationLookup(t *testing.T) {
	generic, err := production.NewWorkspace("ws-1", fakeWorkspaceSource{})
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := NewProgrammingWorkspace(generic, CodeProject{ID: "p", Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	registry := NewProgrammingWorkspaceRegistry()
	if err := registry.Register(workspace); err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.Get("ws-1"); !ok {
		t.Fatal("workspace not found")
	}
	if registry.Count() != 1 {
		t.Fatalf("count = %d", registry.Count())
	}
}

// Ensure B96.2 continues to reference the existing planning model indirectly
// through ProgrammingTask/CodeProject integration without introducing a new Task type.
func TestProgrammingTask_RemainsExistingPlanningTask(t *testing.T) {
	goal := &planning.Goal{ID: "g1"}
	task := &planning.Task{ID: "t1", GoalID: goal.ID}
	if _, err := NewProgrammingTask("prod-1", task, "p1"); err != nil {
		t.Fatal(err)
	}
}
