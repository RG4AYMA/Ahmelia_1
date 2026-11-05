package computeruse

import (
	"context"
	"testing"
)

type fakeAdapter struct {
	states  []Snapshot
	actions []Action
}

func (f *fakeAdapter) Observe(context.Context) (Snapshot, error) {
	s := f.states[0]
	f.states = f.states[1:]
	return s, nil
}
func (f *fakeAdapter) Act(_ context.Context, a Action) (Result, error) {
	f.actions = append(f.actions, a)
	return Result{ActionID: a.ID, Success: true}, nil
}

type fakeVerifier struct{}

func (fakeVerifier) Verify(_ context.Context, s Snapshot, _ Result) (bool, string) {
	return s.UI["ready"] == "yes", "ready state"
}

func TestControllerRequiresPostActionVerification(t *testing.T) {
	adapter := &fakeAdapter{states: []Snapshot{{ID: "before", UI: map[string]string{"ready": "no"}}, {ID: "after", UI: map[string]string{"ready": "yes"}}}}
	c := &Controller{Adapter: adapter, Verifier: fakeVerifier{}}
	step, err := c.Execute(context.Background(), Action{ID: "a1", Kind: Click, Target: "run"})
	if err != nil {
		t.Fatal(err)
	}
	if !step.Verified {
		t.Fatalf("expected verified action, got %+v", step)
	}
	if len(adapter.actions) != 1 {
		t.Fatalf("expected one action, got %d", len(adapter.actions))
	}
}

func TestControllerDoesNotTreatDispatchSuccessAsVerification(t *testing.T) {
	adapter := &fakeAdapter{states: []Snapshot{{ID: "before", UI: map[string]string{"ready": "no"}}, {ID: "after", UI: map[string]string{"ready": "no"}}}}
	c := &Controller{Adapter: adapter, Verifier: fakeVerifier{}}
	step, err := c.Execute(context.Background(), Action{ID: "a1", Kind: Click, Target: "run"})
	if err != nil {
		t.Fatal(err)
	}
	if step.Result.Success != true {
		t.Fatal("expected adapter dispatch to succeed")
	}
	if step.Verified {
		t.Fatal("dispatch success must not imply verified success")
	}
}

func TestExecuteGoalRepeatsUntilVerified(t *testing.T) {
	adapter := &fakeAdapter{states: []Snapshot{
		{ID: "s0", UI: map[string]string{"ready": "no", "step": "0"}},
		{ID: "s1", UI: map[string]string{"ready": "no", "step": "1"}},
		{ID: "s2", UI: map[string]string{"ready": "yes", "step": "2"}},
	}}
	planner := fakePlanner{}
	verifier := fakeProgressVerifier{}
	result, err := ExecuteGoal(context.Background(), adapter, planner, verifier, LoopConfig{MaxSteps: 3})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Completed || len(result.Steps) != 2 {
		t.Fatalf("expected two verified-loop steps, got %+v", result)
	}
}

func TestExecuteGoalStopsOnNoProgress(t *testing.T) {
	adapter := &fakeAdapter{states: []Snapshot{
		{ID: "s0", UI: map[string]string{"ready": "no", "step": "0"}},
		{ID: "s1", UI: map[string]string{"ready": "no", "step": "0"}},
	}}
	result, err := ExecuteGoal(context.Background(), adapter, fakePlanner{}, fakeProgressVerifier{}, LoopConfig{MaxSteps: 5})
	if err != nil {
		t.Fatal(err)
	}
	if result.Completed {
		t.Fatal("unchanged observation must not be considered success")
	}
	if len(result.Steps) != 1 {
		t.Fatalf("expected one step before stop, got %d", len(result.Steps))
	}
}

type fakePlanner struct{}

func (fakePlanner) NextAction(context.Context, Snapshot) (Action, bool, error) {
	return Action{ID: "next", Kind: Click, Target: "continue"}, true, nil
}

type fakeProgressVerifier struct{}

func (fakeProgressVerifier) Verify(_ context.Context, s Snapshot) (bool, string) {
	return s.UI["ready"] == "yes", "ready"
}
func (fakeProgressVerifier) Progress(_ context.Context, before, after Snapshot) (bool, string) {
	return before.UI["step"] != after.UI["step"], "state did not change"
}
