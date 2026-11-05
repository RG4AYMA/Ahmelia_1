package tools

import (
	"context"
	"errors"
	"testing"

	"ahmelia/capability"
)

type b945TestTool struct {
	id   ID
	caps []capability.ID
	exec Executor
}

func (t b945TestTool) ID() ID                        { return t.id }
func (t b945TestTool) Capabilities() []capability.ID { return t.caps }
func (t b945TestTool) Executor() Executor            { return t.exec }

type b945TestExecutor struct {
	calls   int
	success bool
}

func (e *b945TestExecutor) Execute(_ context.Context, a Action, _ Environment) Result {
	e.calls++
	return Result{ActionID: a.ID, Success: e.success, Output: "executed"}
}

type b945TestEnv struct{}

func (b945TestEnv) Name() string { return "test" }

type testObserver struct {
	calls     int
	failFirst bool
}

func (o *testObserver) Observe(_ context.Context, _ Action, _ Result) (Observation, error) {
	o.calls++
	if o.failFirst && o.calls == 1 {
		return Observation{}, errors.New("observer unavailable")
	}
	return Observation{Confirmed: true, Details: "state confirmed"}, nil
}

type testVerifier struct {
	calls     int
	failFirst bool
}

func (v *testVerifier) Verify(_ context.Context, _ Action, _ Result, obs Observation) Verification {
	v.calls++
	if v.failFirst && v.calls == 1 {
		return Verification{Success: false, Reason: "not yet confirmed", Observation: obs}
	}
	return Verification{Success: obs.Confirmed, Reason: "verified", Observation: obs}
}

func newB945VerifiedTestRunner(t *testing.T, maxRetries int, observer Observer, verifier Verifier) (*VerifiedRunner, *b945TestExecutor) {
	t.Helper()
	exec := &b945TestExecutor{success: true}
	registry := NewRegistry()
	if err := registry.Register(b945TestTool{id: "test-tool", caps: []capability.ID{"test.action"}, exec: exec}); err != nil {
		t.Fatal(err)
	}
	runner := NewRunner(registry)
	return NewVerifiedRunner(runner, observer, verifier, maxRetries), exec
}

func b945TestAction() Action { return Action{ID: "attempt-1", CapabilityID: "test.action", Name: "test"} }

func TestVerifiedRunner_ExecuteObserveVerifySuccess(t *testing.T) {
	observer := &testObserver{}
	verifier := &testVerifier{}
	runner, exec := newB945VerifiedTestRunner(t, 0, observer, verifier)

	result := runner.Execute(context.Background(), b945TestAction(), b945TestEnv{})
	if !result.Success {
		t.Fatalf("se esperaba éxito: %s", result.Reason)
	}
	if len(result.Attempts) != 1 {
		t.Fatalf("se esperaba 1 intento, got %d", len(result.Attempts))
	}
	if exec.calls != 1 || observer.calls != 1 || verifier.calls != 1 {
		t.Fatalf("se esperaba una llamada a cada fase: exec=%d observe=%d verify=%d", exec.calls, observer.calls, verifier.calls)
	}
}

func TestVerifiedRunner_RetriesAfterVerificationFailure(t *testing.T) {
	observer := &testObserver{}
	verifier := &testVerifier{failFirst: true}
	runner, exec := newB945VerifiedTestRunner(t, 1, observer, verifier)

	result := runner.Execute(context.Background(), b945TestAction(), b945TestEnv{})
	if !result.Success {
		t.Fatalf("se esperaba éxito en segundo intento: %s", result.Reason)
	}
	if len(result.Attempts) != 2 {
		t.Fatalf("se esperaban 2 intentos, got %d", len(result.Attempts))
	}
	if exec.calls != 2 || observer.calls != 2 || verifier.calls != 2 {
		t.Fatalf("se esperaban dos ciclos completos")
	}
}

func TestVerifiedRunner_ObserverFailureIsNotSuccess(t *testing.T) {
	observer := &testObserver{failFirst: true}
	verifier := &testVerifier{}
	runner, _ := newB945VerifiedTestRunner(t, 0, observer, verifier)

	result := runner.Execute(context.Background(), b945TestAction(), b945TestEnv{})
	if result.Success {
		t.Fatal("un error de observación no puede producir éxito")
	}
	if verifier.calls != 0 {
		t.Fatal("no debe verificarse una observación que no pudo completarse")
	}
}

func TestVerifiedRunner_MissingObserverFailsClosed(t *testing.T) {
	verifier := &testVerifier{}
	runner, _ := newB945VerifiedTestRunner(t, 0, nil, verifier)

	result := runner.Execute(context.Background(), b945TestAction(), b945TestEnv{})
	if result.Success {
		t.Fatal("sin observer no se debe declarar éxito")
	}
}

func TestVerifiedRunner_ContextCancellationStopsBeforeNextAttempt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	observer := &cancelOnFirstObserve{cancel: cancel}
	verifier := &testVerifier{failFirst: true}
	runner, exec := newB945VerifiedTestRunner(t, 3, observer, verifier)

	result := runner.Execute(ctx, b945TestAction(), b945TestEnv{})
	if result.Success {
		t.Fatal("la cancelación no debe producir éxito")
	}
	if exec.calls != 1 {
		t.Fatalf("no se esperaba un segundo intento tras cancelar: calls=%d", exec.calls)
	}
}

type cancelOnFirstObserve struct{ cancel context.CancelFunc }

func (o *cancelOnFirstObserve) Observe(context.Context, Action, Result) (Observation, error) {
	o.cancel()
	return Observation{Confirmed: true, Details: "state confirmed"}, nil
}
