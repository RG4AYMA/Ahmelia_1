package execution

import (
	"context"
	"testing"
	"time"

	"ahmelia/planning"
)

type testArtifactResolver map[string]ArtifactRequirement

func (r testArtifactResolver) ResolveArtifact(id string) (ArtifactRequirement, bool) {
	a, ok := r[id]
	return a, ok
}

func newTestGraphExecutor(t *testing.T) *TaskGraphExecutor {
	t.Helper()
	registry := NewRegistry()
	worker, err := NewLocalWorker("test", WorkerCapabilities{CPU: 1, MemoryMB: 256, ConcurrentSlots: 2})
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(worker); err != nil {
		t.Fatal(err)
	}
	dispatcher, err := NewDispatcher(registry)
	if err != nil {
		t.Fatal(err)
	}
	exec, err := NewTaskGraphExecutor(dispatcher, 0)
	if err != nil {
		t.Fatal(err)
	}
	return exec
}

func TestTaskGraphArtifactGateBlocksUnverifiedInput(t *testing.T) {
	exec := newTestGraphExecutor(t)
	exec.SetArtifactResolver(testArtifactResolver{
		"scene": {ID: "scene", Checksum: "abc", Verified: false},
	})
	task := planning.NewTask("build", "demo", "build from scene", 0, time.Second)
	task.ID = "build"
	spec := TaskExecutionSpec{Task: task, CapabilityID: "demo.build", Operation: "build", Inputs: []ArtifactRequirement{{ID: "scene", Checksum: "abc"}}}
	called := false
	result, err := exec.Run(context.Background(), []TaskExecutionSpec{spec}, func(context.Context, TaskExecutionSpec) error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("task executed with unverified artifact input")
	}
	if _, ok := result.Failed["build"]; !ok {
		t.Fatalf("expected build to fail, got %#v", result.Failed)
	}
}

func TestTaskGraphArtifactGateChecksChecksum(t *testing.T) {
	exec := newTestGraphExecutor(t)
	exec.SetArtifactResolver(testArtifactResolver{
		"scene": {ID: "scene", Checksum: "actual", Verified: true},
	})
	task := planning.NewTask("build", "demo", "build from scene", 0, time.Second)
	task.ID = "build"
	spec := TaskExecutionSpec{Task: task, CapabilityID: "demo.build", Operation: "build", Inputs: []ArtifactRequirement{{ID: "scene", Checksum: "expected"}}}
	result, err := exec.Run(context.Background(), []TaskExecutionSpec{spec}, func(context.Context, TaskExecutionSpec) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := result.Failed["build"]; !ok {
		t.Fatalf("expected checksum mismatch failure, got %#v", result.Failed)
	}
}

func TestTaskGraphArtifactGateAllowsVerifiedMatchingInput(t *testing.T) {
	exec := newTestGraphExecutor(t)
	exec.SetArtifactResolver(testArtifactResolver{
		"scene": {ID: "scene", Checksum: "abc", Verified: true},
	})
	task := planning.NewTask("build", "demo", "build from scene", 0, time.Second)
	task.ID = "build"
	spec := TaskExecutionSpec{Task: task, CapabilityID: "demo.build", Operation: "build", Timeout: time.Second, Inputs: []ArtifactRequirement{{ID: "scene", Checksum: "abc"}}}
	called := false
	result, err := exec.Run(context.Background(), []TaskExecutionSpec{spec}, func(context.Context, TaskExecutionSpec) error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("verified matching artifact did not allow execution")
	}
	if len(result.Completed) != 1 || result.Completed[0] != "build" {
		t.Fatalf("unexpected completed tasks: %#v", result.Completed)
	}
}
