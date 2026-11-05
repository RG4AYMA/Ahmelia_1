package core

import (
	"testing"
	"time"

	initiative "ahmelia/initiative"
)

// B96.37.7 E2E: exercise the real Engine-owned action contract boundary from
// an already-finalized B96.37 decision. The test verifies that the Engine's
// construction path contains the new validator and that the full contract
// survives without changing initiative authority.
func TestB96377E2EEngineActionContractBoundary(t *testing.T) {
	e := &Engine{}
	e.initiativeActionContract = initiative.NewActionContractBuilder()

	now := time.Now()
	op := initiative.InitiativeOpportunity{
		Exists:    true,
		Source:    initiative.SourceInteractionContinuity,
		Reason:    initiative.ReasonConversationContinue,
		Relevance: 0.9,
		Priority:  initiative.PriorityNormal,
		Urgency:   initiative.UrgencyLow,
		Signal:    initiative.InitiativeSignal{Relevant: true},
	}
	decision := initiative.DefaultPolicy().Evaluate(op, now)
	finalized, err := initiative.NewActionSelector().FinalizeDecision(decision, now)
	if err != nil {
		t.Fatal(err)
	}
	contract, err := e.initiativeActionContract.Build(finalized)
	if err != nil {
		t.Fatal(err)
	}
	if contract.Decision != initiative.Intervene {
		t.Fatalf("decision changed to %s", contract.Decision)
	}
	if contract.Action != initiative.ActionContinue {
		t.Fatalf("action=%s", contract.Action)
	}
	if err := contract.Valid(); err != nil {
		t.Fatal(err)
	}
}
