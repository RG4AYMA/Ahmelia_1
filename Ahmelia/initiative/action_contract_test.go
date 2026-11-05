package initiative

import (
	"testing"
	"time"
)

func TestB96377ActionSpecificationsCoverCanonicalActions(t *testing.T) {
	actions := []Action{ActionAsk, ActionSuggest, ActionPropose, ActionResume, ActionComment, ActionWarn, ActionContribute, ActionRemind, ActionShareDiscovery, ActionPresentPossibility, ActionContinue, ActionRedirect}
	for _, action := range actions {
		spec, ok := ActionSpecification(action)
		if !ok || spec.Action != action || spec.Category == "" || spec.ContentMode == "" || !spec.NeedsContent {
			t.Fatalf("incomplete specification for %s: %+v", action, spec)
		}
	}
}

func TestB96377BuildPreservesDecisionBoundary(t *testing.T) {
	now := time.Now()
	builder := NewActionContractBuilder()

	wait := InitiativeDecision{ID: "wait-1", Decision: Wait, Timing: TimingWait, CreatedAt: now, PolicyVersion: "B96.37.2"}
	waitContract, err := builder.Build(wait)
	if err != nil {
		t.Fatal(err)
	}
	if waitContract.Decision != Wait || waitContract.Action != "" {
		t.Fatalf("unexpected WAIT contract: %+v", waitContract)
	}

	op := baseOpportunity()
	decision := DefaultPolicy().Evaluate(op, now)
	finalized, err := NewActionSelector().FinalizeDecision(decision, now)
	if err != nil {
		t.Fatal(err)
	}
	contract, err := builder.Build(finalized)
	if err != nil {
		t.Fatal(err)
	}
	if contract.Decision != Intervene || contract.Action != ActionContinue {
		t.Fatalf("unexpected intervention contract: %+v", contract)
	}
	if err := contract.Valid(); err != nil {
		t.Fatal(err)
	}
}

func TestB96377RejectsActionOnDefer(t *testing.T) {
	now := time.Now()
	d := InitiativeDecision{ID: "defer-1", Decision: Defer, Action: ActionWarn, Timing: TimingDefer, CreatedAt: now, PolicyVersion: "B96.37.2"}
	if _, err := NewActionContractBuilder().Build(d); err == nil {
		t.Fatal("expected DEFER with action to fail")
	}
}

func TestB96377RejectsUnknownAction(t *testing.T) {
	now := time.Now()
	d := InitiativeDecision{ID: "bad-1", Decision: Intervene, Action: Action("DO_MAGIC"), Timing: TimingNow, CreatedAt: now, PolicyVersion: "B96.37.2"}
	if _, err := NewActionContractBuilder().Build(d); err == nil {
		t.Fatal("expected unknown action to fail")
	}
}
