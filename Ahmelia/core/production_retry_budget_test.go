package core

import (
	"context"
	"testing"

	"ahmelia/interaction"
	"ahmelia/production"
	"ahmelia/tools"
)

func TestO5RecoveryCarriesBudgetAndWorkAcrossReplan(t *testing.T) {
	b := production.NewRetryBudget(production.RetryPolicy{MaxAttempts: 2}, "fixed")
	if err := b.BeginAttempt(context.Background(), "fixed"); err != nil {
		t.Fatal(err)
	}
	if err := b.UseModelCall(context.Background()); err != nil {
		t.Fatal(err)
	}
	state := b.Snapshot()
	options := productionRecoveryOptions(production.ExecutionOptions{RecoveryAttempt: 1}, production.ActionResult{WorkID: "same-work", RetryState: &state})
	if options.WorkID != "same-work" || options.RetryState.ModelCalls != 1 || !options.RetryState.Deadline.Equal(state.Deadline) {
		t.Fatalf("lost continuation: %+v", options)
	}
	if err := productionRecoveryStop(options); err != nil {
		t.Fatal(err)
	}
	if err := b.BeginAttempt(context.Background(), "fixed"); err != nil {
		t.Fatal(err)
	}
	state = b.Snapshot()
	options = productionRecoveryOptions(options, production.ActionResult{WorkID: "same-work", RetryState: &state})
	// Resetting the legacy ordinal or requesting more attempts cannot reset
	// a runtime-produced budget after either retry or replan.
	options.RecoveryAttempt = 1
	options.RetryPolicy = production.RetryPolicy{MaxAttempts: 99}
	if err := productionRecoveryStop(options); err == nil {
		t.Fatal("replanning replenished exhausted budget")
	}
}

func TestO5RecoveryPreservesAcceptanceAndCancellation(t *testing.T) {
	req := interaction.SemanticRequest{Objective: interaction.Objective{Text: "Add(2,3)=5", Constraints: []string{"no dependencies"}}}
	req.Frame.Verification = []string{"independent acceptance", "build", "tests"}
	got := productionRecoveryRequest(req, production.ActionResult{FailureReason: "compiler error"}, tools.RecoveryDecision{Action: tools.RecoveryReplan})
	if len(got.Frame.Verification) != 3 || got.Frame.Verification[0] != "independent acceptance" || got.Objective.Constraints[0] != "no dependencies" {
		t.Fatal("recovery weakened acceptance")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := productionRecoveryStop(production.ExecutionOptions{Context: ctx}); err == nil {
		t.Fatal("cancelled retry permitted")
	}
}
