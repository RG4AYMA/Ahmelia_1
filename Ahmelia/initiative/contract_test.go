package initiative

import (
	"testing"
	"time"
)

func baseOpportunity() InitiativeOpportunity {
	return InitiativeOpportunity{Exists: true, Source: SourceConversationalContext, Reason: ReasonConversationContinue, Relevance: .9, Priority: PriorityNormal, Urgency: UrgencyLow, UserAvailability: AvailabilityAvailable, Privacy: InformationAllowed, Interruptibility: InterruptSafe, ConversationLoad: LoadNormal, Signal: InitiativeSignal{Source: SourceConversationalContext, Reason: ReasonConversationContinue, Relevant: true, Priority: PriorityNormal, Urgency: UrgencyLow}}
}

func TestContractInterveneWaitDefer(t *testing.T) {
	now := time.Now()
	p := DefaultPolicy()
	got := p.Evaluate(baseOpportunity(), now)
	got.Action = ActionContinue
	if err := ValidateDecision(got); err != nil {
		t.Fatal(err)
	}
	if got.Decision != Intervene || got.Timing != TimingNow {
		t.Fatalf("got %+v", got)
	}

	op := baseOpportunity()
	op.Exists = false
	got = p.Evaluate(op, now)
	if got.Decision != Wait || got.Timing != TimingWait {
		t.Fatalf("expected WAIT, got %+v", got)
	}

	op = baseOpportunity()
	op.UserAvailability = AvailabilityBusy
	got = p.Evaluate(op, now)
	if got.Decision != Defer || got.Timing != TimingDefer {
		t.Fatalf("expected DEFER, got %+v", got)
	}
}

func TestInterruptIsTimingNotDecision(t *testing.T) {
	now := time.Now()
	op := baseOpportunity()
	op.UserAvailability = AvailabilityBusy
	op.Urgency = UrgencyCritical
	op.Interruptibility = InterruptSafe
	op.Signal.Urgency = UrgencyCritical
	got := DefaultPolicy().Evaluate(op, now)
	got.Action = ActionWarn
	if got.Decision != Intervene || got.Timing != TimingInterrupt {
		t.Fatalf("got %+v", got)
	}
	if err := ValidateDecision(got); err != nil {
		t.Fatal(err)
	}
}

func TestPrivacyDoesNotBecomeDisclosureAuthority(t *testing.T) {
	op := baseOpportunity()
	op.Privacy = InformationSensitive
	got := DefaultPolicy().Evaluate(op, time.Now())
	if got.Decision != Intervene {
		t.Fatalf("sensitive content can still have a conversational intervention, got %+v", got)
	}
	if len(got.Constraints) != 1 || got.Constraints[0].Code != "do_not_expose_sensitive_data" {
		t.Fatalf("missing disclosure constraint: %+v", got.Constraints)
	}
}

func TestDeferredInitiativeIsReevaluated(t *testing.T) {
	now := time.Now()
	op := baseOpportunity()
	op.UserAvailability = AvailabilityBusy
	first := DefaultPolicy().Evaluate(op, now)
	if first.Decision != Defer {
		t.Fatalf("got %+v", first)
	}
	op.UserAvailability = AvailabilityAvailable
	next := ReevaluateDeferred(first, op, now.Add(time.Second))
	if next.Decision != Intervene {
		t.Fatalf("got %+v", next)
	}
}

func TestNoContentInDecisionContract(t *testing.T) {
	d := DefaultPolicy().Evaluate(baseOpportunity(), time.Now())
	d.Action = ActionAsk
	if err := ValidateDecision(d); err != nil {
		t.Fatal(err)
	}
}
