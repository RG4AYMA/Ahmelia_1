package tools

import (
	"context"
	"errors"
	"testing"
)

type testDiagnoser struct {
	failure Failure
	err     error
}

func (d testDiagnoser) Diagnose(context.Context, VerifiedResult) (Failure, error) {
	return d.failure, d.err
}

type testDecider struct{ action RecoveryAction }

func (d testDecider) Decide(context.Context, Failure) RecoveryAction { return d.action }

func TestRecoveryCoordinator_UsesVerifiedFailureEvidence(t *testing.T) {
	result := VerifiedResult{ActionID: "a1", Success: false, Reason: "archivo no encontrado", Attempts: []Attempt{{Number: 1}, {Number: 2}}}
	c := NewRecoveryCoordinator(
		testDiagnoser{failure: Failure{ActionID: "a1", Reason: "diagnóstico: falta recurso"}},
		testDecider{action: RecoveryRetry},
	)
	got, err := c.Recover(context.Background(), result)
	if err != nil {
		t.Fatal(err)
	}
	if got.Action != RecoveryRetry {
		t.Fatalf("action=%q", got.Action)
	}
	if got.Reason != "diagnóstico: falta recurso" {
		t.Fatalf("reason=%q", got.Reason)
	}
}

func TestRecoveryCoordinator_FillsMissingEvidenceWithoutChangingIdentity(t *testing.T) {
	result := VerifiedResult{ActionID: "a2", Success: false, Reason: "verify fallo", Attempts: []Attempt{{Number: 1}}}
	c := NewRecoveryCoordinator(testDiagnoser{failure: Failure{}}, testDecider{action: RecoveryReplan})
	got, err := c.Recover(context.Background(), result)
	if err != nil {
		t.Fatal(err)
	}
	if got.Action != RecoveryReplan || got.Reason != "verify fallo" {
		t.Fatalf("got=%+v", got)
	}
}

func TestRecoveryCoordinator_RejectsSuccess(t *testing.T) {
	c := NewRecoveryCoordinator(testDiagnoser{}, testDecider{action: RecoveryRetry})
	_, err := c.Recover(context.Background(), VerifiedResult{ActionID: "ok", Success: true})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRecoveryCoordinator_FailsClosedOnDiagnosisError(t *testing.T) {
	c := NewRecoveryCoordinator(testDiagnoser{err: errors.New("boom")}, testDecider{action: RecoveryRetry})
	_, err := c.Recover(context.Background(), VerifiedResult{ActionID: "a", Success: false})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRecoveryCoordinator_RejectsMismatchedAction(t *testing.T) {
	c := NewRecoveryCoordinator(testDiagnoser{failure: Failure{ActionID: "other"}}, testDecider{action: RecoveryRetry})
	_, err := c.Recover(context.Background(), VerifiedResult{ActionID: "a", Success: false})
	if err == nil {
		t.Fatal("expected mismatch error")
	}
}

func TestRecoveryCoordinator_RejectsInvalidDecision(t *testing.T) {
	c := NewRecoveryCoordinator(testDiagnoser{failure: Failure{ActionID: "a"}}, testDecider{action: "invalid"})
	_, err := c.Recover(context.Background(), VerifiedResult{ActionID: "a", Success: false})
	if err == nil {
		t.Fatal("expected invalid decision error")
	}
}

func TestRetryThenReplan_IsBounded(t *testing.T) {
	p := RetryThenReplan{MaxRetries: 2}
	if got := p.Decide(context.Background(), Failure{Attempts: 1}); got != RecoveryRetry {
		t.Fatalf("got=%q", got)
	}
	if got := p.Decide(context.Background(), Failure{Attempts: 2}); got != RecoveryRetry {
		t.Fatalf("got=%q", got)
	}
	if got := p.Decide(context.Background(), Failure{Attempts: 3}); got != RecoveryReplan {
		t.Fatalf("got=%q", got)
	}
}
