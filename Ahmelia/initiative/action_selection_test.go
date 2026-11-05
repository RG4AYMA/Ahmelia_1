package initiative

import (
	"testing"
	"time"
)

func TestActionSelectorMapsReasonsWithoutChangingDecision(t *testing.T) {
	selector := NewActionSelector()
	cases := []struct {
		reason Reason
		want   Action
	}{
		{ReasonClarificationNeeded, ActionAsk},
		{ReasonPendingTask, ActionRemind},
		{ReasonUnfinishedCommitment, ActionRemind},
		{ReasonUsefulInformation, ActionShareDiscovery},
		{ReasonPotentialProblem, ActionWarn},
		{ReasonNextStepAvailable, ActionSuggest},
		{ReasonConversationContinue, ActionContinue},
		{ReasonPausedThreadAvailable, ActionResume},
		{ReasonAlternativeAvailable, ActionPresentPossibility},
		{ReasonNewPossibility, ActionPropose},
		{ReasonRelevantComment, ActionComment},
		{ReasonTopicRedirection, ActionRedirect},
	}
	for _, tc := range cases {
		if got := selector.Select(InitiativeOpportunity{Reason: tc.reason}, Intervene); got != tc.want {
			t.Fatalf("reason %s: got %s want %s", tc.reason, got, tc.want)
		}
	}
	if got := selector.Select(InitiativeOpportunity{Reason: ReasonWarning}, Wait); got != "" {
		t.Fatalf("non-intervention must not select an action: %s", got)
	}
}

func TestFinalizeDecisionCompletesIntervention(t *testing.T) {
	now := time.Now()
	op := baseOpportunity()
	d := DefaultPolicy().Evaluate(op, now)
	finalized, err := NewActionSelector().FinalizeDecision(d, now)
	if err != nil {
		t.Fatal(err)
	}
	if finalized.Decision != Intervene {
		t.Fatalf("got %s", finalized.Decision)
	}
	if finalized.Action != ActionContinue {
		t.Fatalf("got action %s", finalized.Action)
	}
	if err := ValidateDecision(finalized); err != nil {
		t.Fatal(err)
	}
}

func TestFinalizeDoesNotTurnWaitIntoIntervention(t *testing.T) {
	now := time.Now()
	op := baseOpportunity()
	op.Exists = false
	d := DefaultPolicy().Evaluate(op, now)
	finalized, err := NewActionSelector().FinalizeDecision(d, now)
	if err != nil {
		t.Fatal(err)
	}
	if finalized.Decision != Wait || finalized.Timing != TimingWait {
		t.Fatalf("got %+v", finalized)
	}
	if finalized.Action != "" {
		t.Fatalf("WAIT must not receive an action: %s", finalized.Action)
	}
}

func TestFinalizePreservesCriticalInterrupt(t *testing.T) {
	now := time.Now()
	op := baseOpportunity()
	op.Reason = ReasonWarning
	op.UserAvailability = AvailabilityBusy
	op.Urgency = UrgencyCritical
	op.Interruptibility = InterruptSafe
	op.Signal.Urgency = UrgencyCritical
	d := DefaultPolicy().Evaluate(op, now)
	finalized, err := NewActionSelector().FinalizeDecision(d, now)
	if err != nil {
		t.Fatal(err)
	}
	if finalized.Decision != Intervene || finalized.Timing != TimingInterrupt || finalized.Action != ActionWarn {
		t.Fatalf("got %+v", finalized)
	}
}
