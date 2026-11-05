package tools

import (
	"context"
	"testing"

	"ahmelia/capability"
)

type testEnv struct{ name string }

func (e testEnv) Name() string { return e.name }

type testExec struct {
	called bool
}

func (e *testExec) Execute(ctx context.Context, a Action, env Environment) Result {
	e.called = true
	return Result{ActionID: a.ID, Success: true, Output: env.Name()}
}

type testTool struct {
	id   ID
	caps []capability.ID
	exec *testExec
}

func (t testTool) ID() ID { return t.id }
func (t testTool) Capabilities() []capability.ID {
	return append([]capability.ID(nil), t.caps...)
}
func (t testTool) Executor() Executor { return t.exec }

func TestActionValidate(t *testing.T) {
	if err := (Action{ID: "a", CapabilityID: "cap", Name: "run"}).Validate(); err != nil {
		t.Fatal(err)
	}
	for _, a := range []Action{
		{CapabilityID: "cap", Name: "run"},
		{ID: "a", Name: "run"},
		{ID: "a", CapabilityID: "cap"},
	} {
		if a.Validate() == nil {
			t.Fatalf("expected invalid action: %+v", a)
		}
	}
}

func TestRegistryRegisterGetAndFindCapability(t *testing.T) {
	r := NewRegistry()
	ex := &testExec{}
	capID := capability.ID("local.create_file")
	tool := testTool{id: "filesystem", caps: []capability.ID{capID}, exec: ex}

	if err := r.Register(tool); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Get("filesystem"); !ok {
		t.Fatal("tool not found")
	}
	got := r.FindCapability(capID)
	if len(got) != 1 || got[0].ID() != "filesystem" {
		t.Fatalf("unexpected find: %+v", got)
	}
}

func TestRegistryRejectsDuplicateAndMissingExecutor(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(testTool{id: "x", exec: nil}); err == nil {
		t.Fatal("expected missing executor error")
	}
	ex := &testExec{}
	if err := r.Register(testTool{id: "x", exec: ex}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(testTool{id: "x", exec: ex}); err == nil {
		t.Fatal("expected duplicate error")
	}
}

func TestRunnerExecutesThroughToolAndExecutor(t *testing.T) {
	ex := &testExec{}
	r := NewRegistry()
	capID := capability.ID("filesystem.create_file")
	if err := r.Register(testTool{id: "filesystem", caps: []capability.ID{capID}, exec: ex}); err != nil {
		t.Fatal(err)
	}

	a := Action{ID: "1", CapabilityID: capID, Name: "create_file", Inputs: map[string]string{"path": "/tmp/x"}}
	res := NewRunner(r).Execute(context.Background(), a, testEnv{"filesystem"})
	if !res.Success || res.ActionID != "1" || !ex.called || res.Output != "filesystem" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestRunnerFailsClosedForMissingEnvironment(t *testing.T) {
	ex := &testExec{}
	r := NewRegistry()
	capID := capability.ID("filesystem.create_file")
	if err := r.Register(testTool{id: "filesystem", caps: []capability.ID{capID}, exec: ex}); err != nil {
		t.Fatal(err)
	}

	res := NewRunner(r).Execute(context.Background(), Action{ID: "1", CapabilityID: capID, Name: "create_file"}, nil)
	if res.Err == nil || ex.called {
		t.Fatalf("expected fail-closed result: %+v", res)
	}
}

func TestRunnerRejectsAmbiguousToolResolution(t *testing.T) {
	capID := capability.ID("terminal.run")
	r := NewRegistry()
	if err := r.Register(testTool{id: "terminal_a", caps: []capability.ID{capID}, exec: &testExec{}}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(testTool{id: "terminal_b", caps: []capability.ID{capID}, exec: &testExec{}}); err != nil {
		t.Fatal(err)
	}

	res := NewRunner(r).Execute(context.Background(), Action{ID: "1", CapabilityID: capID, Name: "run"}, testEnv{"os"})
	if res.Err == nil {
		t.Fatal("expected ambiguous tool resolution to fail")
	}
}
