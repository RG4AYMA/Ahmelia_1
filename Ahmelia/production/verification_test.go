package production

import (
	"context"
	"errors"
	"testing"

	"ahmelia/capability"
	"ahmelia/planning"
	"ahmelia/tools"
)

type productionVerifier struct {
	called bool
	result tools.Verification
}

func (v *productionVerifier) Verify(_ context.Context, action tools.Action, _ tools.Result, observation tools.Observation) tools.Verification {
	v.called = true
	result := v.result
	result.Observation = observation
	if result.Reason == "" {
		result.Reason = action.ID + " verified"
	}
	return result
}

func verificationBinding(t *testing.T) CapabilityBinding {
	t.Helper()
	task := planning.NewTask("goal-95", "crear archivo", "", 1, 0)
	c := capability.Capability{ID: "filesystem.create_file", Domain: capability.DomainLocal, Action: "create_file", Permission: capability.PermissionAutomatic}
	action := tools.Action{ID: "action-95.10", CapabilityID: c.ID, Name: c.Action}
	return CapabilityBinding{Task: task, Capability: c, Action: action}
}

func TestVerificationServiceDelegatesToExistingVerifier(t *testing.T) {
	verifier := &productionVerifier{result: tools.Verification{Success: true, Reason: "archivo confirmado"}}
	service, err := NewVerificationService(verifier)
	if err != nil {
		t.Fatal(err)
	}
	binding := verificationBinding(t)
	observation := tools.Observation{Confirmed: true, Details: "enemy.cs existe"}
	result, err := service.Verify(context.Background(), binding, tools.Result{ActionID: binding.Action.ID, Success: true}, observation)
	if err != nil {
		t.Fatal(err)
	}
	if !verifier.called {
		t.Fatal("existing verifier was not called")
	}
	if !result.Success || result.Observation != observation {
		t.Fatalf("unexpected verification: %+v", result)
	}
}

func TestVerificationServicePreservesSuppliedObservation(t *testing.T) {
	verifier := &productionVerifier{result: tools.Verification{Success: true, Observation: tools.Observation{Details: "fake"}}}
	service, _ := NewVerificationService(verifier)
	binding := verificationBinding(t)
	observation := tools.Observation{Confirmed: false, Details: "archivo ausente"}
	result, err := service.Verify(context.Background(), binding, tools.Result{ActionID: binding.Action.ID}, observation)
	if err != nil {
		t.Fatal(err)
	}
	if result.Observation != observation {
		t.Fatalf("observation was replaced: %+v", result.Observation)
	}
}

func TestVerificationServiceRejectsMismatchedExecution(t *testing.T) {
	verifier := &productionVerifier{}
	service, _ := NewVerificationService(verifier)
	binding := verificationBinding(t)
	_, err := service.Verify(context.Background(), binding, tools.Result{ActionID: "other"}, tools.Observation{})
	if err == nil {
		t.Fatal("expected ActionID mismatch")
	}
	if verifier.called {
		t.Fatal("verifier must not be called on identity mismatch")
	}
}

func TestVerificationServiceAllowsTechnicalFailureToBeVerifiedByExistingPolicy(t *testing.T) {
	verifier := &productionVerifier{result: tools.Verification{Success: false, Reason: "technical failure"}}
	service, _ := NewVerificationService(verifier)
	binding := verificationBinding(t)
	result, err := service.Verify(context.Background(), binding, tools.Result{ActionID: binding.Action.ID, Success: false, Err: errors.New("boom")}, tools.Observation{Confirmed: false})
	if err != nil {
		t.Fatal(err)
	}
	if result.Success {
		t.Fatal("expected failed verification")
	}
}

func TestVerificationServiceRejectsCancelledContext(t *testing.T) {
	verifier := &productionVerifier{}
	service, _ := NewVerificationService(verifier)
	binding := verificationBinding(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := service.Verify(ctx, binding, tools.Result{ActionID: binding.Action.ID}, tools.Observation{})
	if err == nil {
		t.Fatal("expected cancellation")
	}
	if verifier.called {
		t.Fatal("verifier must not be called after cancellation")
	}
}

func TestVerificationServiceOutcomePreservesIdentity(t *testing.T) {
	verifier := &productionVerifier{result: tools.Verification{Success: true}}
	service, _ := NewVerificationService(verifier)
	binding := verificationBinding(t)
	outcome, err := service.VerifyOutcome(context.Background(), binding, tools.Result{ActionID: binding.Action.ID}, tools.Observation{Confirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.TaskID != binding.Task.ID || outcome.CapabilityID != string(binding.Capability.ID) || outcome.ActionID != binding.Action.ID {
		t.Fatalf("identity not preserved: %+v", outcome)
	}
}
